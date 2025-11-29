package fs

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/justyntemme/razor/internal/search"
)

type OpType int

const (
	FetchDir OpType = iota
	SearchDir
	CancelSearch
)

type Request struct {
	Op           OpType
	Path         string
	Query        string
	Gen          int64  // Generation counter to track stale requests
	SearchEngine int    // Search engine type (0=builtin, 1=ripgrep, 2=ugrep)
	EngineCmd    string // Command for external engine
}

type Entry struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

type Response struct {
	Op        OpType
	Path      string
	Entries   []Entry
	Err       error
	Gen       int64 // Generation counter from request
	Cancelled bool  // True if search was cancelled
}

// Progress represents a progress update during long operations
type Progress struct {
	Gen     int64
	Current int64
	Total   int64
	Label   string
}

type System struct {
	RequestChan  chan Request
	ResponseChan chan Response
	ProgressChan chan Progress // Channel for progress updates
	
	// Cancellation support
	cancelMu     sync.Mutex
	cancelFunc   context.CancelFunc
	currentGen   int64
	searchActive bool
}

func NewSystem() *System {
	return &System{
		RequestChan:  make(chan Request, 10),
		ResponseChan: make(chan Response, 10),
		ProgressChan: make(chan Progress, 100), // Buffered to avoid blocking
	}
}

func (s *System) Start() {
	for req := range s.RequestChan {
		log.Printf("[FS] Received request: op=%d path=%q query=%q gen=%d", req.Op, req.Path, req.Query, req.Gen)
		
		switch req.Op {
		case CancelSearch:
			s.cancelMu.Lock()
			if s.cancelFunc != nil {
				log.Printf("[FS] Cancelling current search (gen %d)", s.currentGen)
				s.cancelFunc()
				s.cancelFunc = nil
			}
			s.cancelMu.Unlock()
			// Don't send a response for cancel - the search goroutine will handle it
			
		case FetchDir:
			// Cancel any running search before fetching directory
			s.cancelMu.Lock()
			if s.cancelFunc != nil {
				s.cancelFunc()
				s.cancelFunc = nil
			}
			s.cancelMu.Unlock()
			
			resp := s.fetchDir(req.Path)
			resp.Gen = req.Gen
			log.Printf("[FS] Sending response: op=%d path=%q entries=%d gen=%d", resp.Op, resp.Path, len(resp.Entries), resp.Gen)
			s.ResponseChan <- resp
			
		case SearchDir:
			// Cancel any existing search
			s.cancelMu.Lock()
			if s.cancelFunc != nil {
				log.Printf("[FS] Cancelling previous search before starting new one")
				s.cancelFunc()
			}
			ctx, cancel := context.WithCancel(context.Background())
			s.cancelFunc = cancel
			s.currentGen = req.Gen
			s.searchActive = true
			s.cancelMu.Unlock()
			
			// Run search in goroutine so we can process cancel requests
			go func(ctx context.Context, req Request) {
				resp := s.searchDir(ctx, req.Path, req.Query, req.Gen, search.SearchEngine(req.SearchEngine), req.EngineCmd)
				resp.Gen = req.Gen
				
				// Check if cancelled
				if ctx.Err() != nil {
					resp.Cancelled = true
					log.Printf("[FS] Search cancelled (gen %d)", req.Gen)
				}
				
				s.cancelMu.Lock()
				s.searchActive = false
				s.cancelMu.Unlock()
				
				log.Printf("[FS] Sending response: op=%d path=%q entries=%d gen=%d cancelled=%v", 
					resp.Op, resp.Path, len(resp.Entries), resp.Gen, resp.Cancelled)
				s.ResponseChan <- resp
			}(ctx, req)
		}
	}
}

// Directories to skip during search (system/special directories)
var skipDirs = map[string]bool{
	"/dev":     true,
	"/proc":    true,
	"/sys":     true,
	"/run":     true,
	"/snap":    true,
	"/boot":    true,
	"/lost+found": true,
}

// shouldSkipPath returns true if the path should be skipped during search
func shouldSkipPath(path string) bool {
	// Check exact matches
	if skipDirs[path] {
		return true
	}
	// Check if path is under a skip directory
	for skipDir := range skipDirs {
		if strings.HasPrefix(path, skipDir+"/") {
			return true
		}
	}
	return false
}

func (s *System) fetchDir(path string) Response {
	entries, err := os.ReadDir(path)
	if err != nil {
		return Response{Op: FetchDir, Path: path, Err: err}
	}

	result := make([]Entry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		result = append(result, Entry{
			Name:    e.Name(),
			Path:    filepath.Join(path, e.Name()),
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}

	return Response{Op: FetchDir, Path: path, Entries: result}
}

func (s *System) searchDir(ctx context.Context, basePath, queryStr string, gen int64, engine search.SearchEngine, engineCmd string) Response {
	log.Printf("[FS_SEARCH] searchDir called: basePath=%q queryStr=%q engine=%d cmd=%q", basePath, queryStr, engine, engineCmd)
	query := search.Parse(queryStr)
	if query.IsEmpty() {
		log.Printf("[FS_SEARCH] Query is empty, falling back to fetchDir")
		return s.fetchDir(basePath)
	}

	log.Printf("[FS_SEARCH] Query parsed: HasContentSearch=%v, HasRecursive=%v, directives=%d", 
		query.HasContentSearch(), query.HasRecursive(), len(query.Directives))
	
	var results []Entry

	// Default depth is 1 (current directory only)
	// Use recursive: directive to enable deeper search
	maxDepth := 1
	if query.HasRecursive() {
		maxDepth = query.GetRecursiveDepth()
		if maxDepth <= 0 {
			maxDepth = 10 // Default recursive depth
		}
	}
	
	log.Printf("[FS_SEARCH] maxDepth=%d", maxDepth)

	// Check if we should use an external search engine for content searches
	if query.HasContentSearch() && engine != search.EngineBuiltin && engineCmd != "" {
		// Use external search engine (ripgrep or ugrep)
		contentPattern := query.GetContentPattern()
		if contentPattern != "" {
			log.Printf("[FS_SEARCH] Using external engine %d for content search: %q", engine, contentPattern)
			
			// Show initial indeterminate progress
			s.ProgressChan <- Progress{
				Gen:   gen,
				Label: "Searching with " + engine.String() + "...",
				Total: 0, // Indeterminate while external tool runs
			}
			
			// Progress callback for streaming updates during search
			progressFn := func(found int) {
				select {
				case s.ProgressChan <- Progress{
					Gen:     gen,
					Current: int64(found),
					Total:   0, // Still indeterminate - we don't know total yet
					Label:   fmt.Sprintf("Found %d files...", found),
				}:
				default:
				}
			}
			
			// Run external search with progress callback
			matchingPaths, err := search.SearchWithEngine(ctx, engine, engineCmd, contentPattern, basePath, maxDepth, progressFn)
			if err != nil {
				if ctx.Err() != nil {
					return Response{Op: SearchDir, Path: basePath, Cancelled: true}
				}
				log.Printf("[FS_SEARCH] External search error: %v", err)
				// Fall back to builtin on error
			} else {
				log.Printf("[FS_SEARCH] External search found %d matching files", len(matchingPaths))
				
				// Process results directly instead of walking entire directory
				results, err := s.processExternalResults(ctx, gen, matchingPaths, query)
				if err != nil {
					if ctx.Err() != nil {
						return Response{Op: SearchDir, Path: basePath, Cancelled: true}
					}
					log.Printf("[FS_SEARCH] processExternalResults error: %v", err)
					return Response{Op: SearchDir, Path: basePath, Err: err}
				}
				
				log.Printf("[FS_SEARCH] External search complete: %d results found", len(results))
				return Response{Op: SearchDir, Path: basePath, Entries: results}
			}
		}
	}
	
	// Use builtin search
	// Use context-aware matcher for cancellable content searches
	matcher := search.NewMatcherWithContext(ctx, query)

	// First pass: count total files/bytes for progress (skip if cancelled)
	var totalFiles int
	var totalBytes int64
	if query.HasContentSearch() {
		s.countFiles(ctx, basePath, 0, maxDepth, &totalFiles, &totalBytes)
		if ctx.Err() != nil {
			return Response{Op: SearchDir, Path: basePath, Cancelled: true}
		}
		log.Printf("[FS_SEARCH] Total files to search: %d, total bytes: %d", totalFiles, totalBytes)
	}

	// Create progress tracker
	progress := &searchProgress{
		gen:        gen,
		totalFiles: totalFiles,
		totalBytes: totalBytes,
		progressCh: s.ProgressChan,
	}

	err := s.walkDirWithProgress(ctx, basePath, 0, maxDepth, matcher, &results, progress)
	if err != nil {
		log.Printf("[FS_SEARCH] walkDir error: %v", err)
		return Response{Op: SearchDir, Path: basePath, Err: err}
	}

	log.Printf("[FS_SEARCH] Search complete: %d results found", len(results))
	return Response{Op: SearchDir, Path: basePath, Entries: results}
}

// processExternalResults directly processes paths from external search engines
// This is more efficient than walking the entire directory tree
func (s *System) processExternalResults(ctx context.Context, gen int64, paths []string, query *search.Query) ([]Entry, error) {
	total := len(paths)
	if total == 0 {
		return nil, nil
	}
	
	var results []Entry
	lastPct := 0
	
	for i, path := range paths {
		// Check for cancellation
		if ctx.Err() != nil {
			return results, ctx.Err()
		}
		
		// Report progress every 5%
		pct := (i * 100) / total
		if pct >= lastPct+5 || i == total-1 {
			lastPct = pct
			select {
			case s.ProgressChan <- Progress{
				Gen:     gen,
				Current: int64(i + 1),
				Total:   int64(total),
				Label:   "Processing results...",
			}:
			default:
			}
		}
		
		// Get file info
		info, err := os.Stat(path)
		if err != nil {
			// File might have been deleted since search
			continue
		}
		
		// Apply additional filters (ext:, size:, modified:, filename:)
		// Skip content matching since external tool already did that
		if matchesNonContentDirectives(query, path, info) {
			results = append(results, Entry{
				Name:    filepath.Base(path),
				Path:    path,
				IsDir:   info.IsDir(),
				Size:    info.Size(),
				ModTime: info.ModTime(),
			})
		}
	}
	
	return results, nil
}

// matchesNonContentDirectives checks if a file matches all non-content directives
func matchesNonContentDirectives(query *search.Query, path string, info os.FileInfo) bool {
	for _, d := range query.Directives {
		switch d.Type {
		case search.DirContents:
			// Skip - external tool already matched this
			continue
		case search.DirRecursive:
			// Skip - this is a control directive
			continue
		case search.DirFilename:
			if !search.MatchGlob(strings.ToLower(info.Name()), strings.ToLower(d.Value)) {
				return false
			}
		case search.DirExt:
			ext := strings.ToLower(filepath.Ext(info.Name()))
			if ext != d.Value {
				return false
			}
		case search.DirSize:
			if !search.CompareInt(info.Size(), d.NumValue, d.Operator) {
				return false
			}
		case search.DirModified:
			if !d.TimeVal.IsZero() && !search.CompareTime(info.ModTime(), d.TimeVal, d.Operator) {
				return false
			}
		}
	}
	return true
}

type searchProgress struct {
	gen           int64
	totalFiles    int
	totalBytes    int64
	currentFiles  int
	currentBytes  int64
	progressCh    chan Progress
	lastReportPct int  // Last reported percentage to avoid spamming
	useFileCount  bool // Use file count instead of bytes for progress
}

func (p *searchProgress) report(label string) {
	if p.progressCh == nil {
		return
	}
	
	var current, total int64
	
	if p.useFileCount {
		// Use file count for progress (external search engines)
		current = int64(p.currentFiles)
		total = int64(p.totalFiles)
	} else {
		// Use byte count for progress (builtin search)
		current = p.currentBytes
		total = p.totalBytes
	}
	
	// If no total, send indeterminate progress
	if total == 0 {
		select {
		case p.progressCh <- Progress{
			Gen:     p.gen,
			Current: current,
			Total:   0, // Indeterminate
			Label:   label,
		}:
		default:
		}
		return
	}
	
	// Only report every 5% to avoid flooding
	pct := int(float64(current) / float64(total) * 100)
	if pct == p.lastReportPct && pct != 100 {
		return
	}
	p.lastReportPct = pct
	
	select {
	case p.progressCh <- Progress{
		Gen:     p.gen,
		Current: current,
		Total:   total,
		Label:   label,
	}:
	default:
		// Channel full, skip this update
	}
}

func (s *System) countFiles(ctx context.Context, path string, depth, maxDepth int, fileCount *int, byteCount *int64) {
	// Check for cancellation
	if ctx.Err() != nil {
		return
	}
	
	if depth > maxDepth {
		return
	}
	
	// Skip system directories
	if shouldSkipPath(path) {
		return
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return
	}

	for _, e := range entries {
		// Check for cancellation periodically
		if ctx.Err() != nil {
			return
		}
		
		fullPath := filepath.Join(path, e.Name())
		
		if e.IsDir() {
			if depth < maxDepth {
				s.countFiles(ctx, fullPath, depth+1, maxDepth, fileCount, byteCount)
			}
		} else {
			info, err := e.Info()
			if err != nil {
				continue
			}
			// Only count regular files that would be searched (skip large files and special files)
			if info.Mode().IsRegular() && info.Size() <= 10*1024*1024 {
				*fileCount++
				*byteCount += info.Size()
			}
		}
	}
}

func (s *System) walkDirWithProgress(ctx context.Context, path string, depth, maxDepth int, matcher *search.Matcher, results *[]Entry, progress *searchProgress) error {
	// Check for cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}
	
	if depth > maxDepth {
		return nil
	}
	
	// Skip system directories
	if shouldSkipPath(path) {
		log.Printf("[FS_WALK] Skipping system directory: %s", path)
		return nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		// Don't fail on permission errors, just skip
		log.Printf("[FS_WALK] Cannot read %q: %v", path, err)
		return nil
	}

	log.Printf("[FS_WALK] Walking %q (depth %d): %d entries", path, depth, len(entries))
	
	for _, e := range entries {
		// Check for cancellation periodically
		if ctx.Err() != nil {
			return ctx.Err()
		}
		
		fullPath := filepath.Join(path, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		
		// Skip non-regular files (devices, sockets, etc.)
		if !e.IsDir() && !info.Mode().IsRegular() {
			continue
		}

		// Update progress before checking file
		if !e.IsDir() && info.Mode().IsRegular() && info.Size() <= 10*1024*1024 {
			progress.currentFiles++
			progress.currentBytes += info.Size()
			progress.report("Searching file contents...")
		}

		// Check if this entry matches
		if matcher.Match(fullPath, info) {
			log.Printf("[FS_WALK] MATCH: %s", fullPath)
			*results = append(*results, Entry{
				Name:    e.Name(),
				Path:    fullPath,
				IsDir:   e.IsDir(),
				Size:    info.Size(),
				ModTime: info.ModTime(),
			})
		}

		// Recurse into directories
		if e.IsDir() && depth < maxDepth {
			s.walkDirWithProgress(ctx, fullPath, depth+1, maxDepth, matcher, results, progress)
		}
	}

	return nil
}
