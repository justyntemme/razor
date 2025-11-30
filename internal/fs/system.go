package fs

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charlievieth/fastwalk"
	"github.com/justyntemme/razor/internal/debug"
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
	DefaultDepth int    // Default recursive depth when not specified
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
		debug.Log(debug.FS, "Request: op=%d path=%q query=%q gen=%d", req.Op, req.Path, req.Query, req.Gen)

		switch req.Op {
		case CancelSearch:
			s.cancelMu.Lock()
			if s.cancelFunc != nil {
				debug.Log(debug.FS, "Cancelling current search (gen %d)", s.currentGen)
				s.cancelFunc()
				s.cancelFunc = nil
			}
			s.cancelMu.Unlock()
			// Don't send a response for cancel - the search goroutine will handle it

		case FetchDir:
			// Cancel any running search before fetching directory
			s.cancelMu.Lock()
			if s.cancelFunc != nil {
				debug.Log(debug.FS, "Cancelling search before FetchDir")
				s.cancelFunc()
				s.cancelFunc = nil
			}
			s.cancelMu.Unlock()

			resp := s.fetchDir(req.Path)
			resp.Gen = req.Gen
			debug.Log(debug.FS, "FetchDir response: path=%q entries=%d gen=%d err=%v",
				resp.Path, len(resp.Entries), resp.Gen, resp.Err)
			s.ResponseChan <- resp

		case SearchDir:
			// Cancel any existing search
			s.cancelMu.Lock()
			if s.cancelFunc != nil {
				debug.Log(debug.FS, "Cancelling previous search before new one")
				s.cancelFunc()
			}
			ctx, cancel := context.WithCancel(context.Background())
			s.cancelFunc = cancel
			s.currentGen = req.Gen
			s.searchActive = true
			s.cancelMu.Unlock()

			// Run search in goroutine so we can process cancel requests
			go func(ctx context.Context, req Request) {
				defaultDepth := req.DefaultDepth
				if defaultDepth <= 0 {
					defaultDepth = 2 // Fallback default
				}
				resp := s.searchDir(ctx, req.Path, req.Query, req.Gen, search.SearchEngine(req.SearchEngine), req.EngineCmd, defaultDepth)
				resp.Gen = req.Gen

				// Check if cancelled
				if ctx.Err() != nil {
					resp.Cancelled = true
					debug.Log(debug.FS, "Search cancelled (gen %d)", req.Gen)
				}

				s.cancelMu.Lock()
				s.searchActive = false
				s.cancelMu.Unlock()

				debug.Log(debug.FS, "SearchDir response: path=%q entries=%d gen=%d cancelled=%v",
					resp.Path, len(resp.Entries), resp.Gen, resp.Cancelled)
				s.ResponseChan <- resp
			}(ctx, req)
		}
	}
}

// skipDirRoots contains top-level directories to skip (without trailing slash)
// Using a map for O(1) lookup of exact matches and prefix checks
var skipDirRoots = map[string]bool{
	"dev":        true,
	"proc":       true,
	"sys":        true,
	"run":        true,
	"snap":       true,
	"boot":       true,
	"lost+found": true,
}

// shouldSkipPath returns true if the path should be skipped during search.
// Optimized: extracts first path component after "/" and does single map lookup.
func shouldSkipPath(path string) bool {
	// Must start with "/" (Unix absolute path)
	if len(path) < 2 || path[0] != '/' {
		return false
	}
	// Find end of first component (e.g., "/dev/foo" -> "dev")
	rest := path[1:]
	slashIdx := strings.IndexByte(rest, '/')
	var firstComponent string
	if slashIdx == -1 {
		firstComponent = rest // No more slashes, e.g., "/dev"
	} else {
		firstComponent = rest[:slashIdx] // e.g., "/dev/foo" -> "dev"
	}
	return skipDirRoots[firstComponent]
}

func (s *System) fetchDir(path string) Response {
	debug.Log(debug.FS, "fetchDir: reading %q", path)

	var result []Entry
	var mu sync.Mutex

	// Configure fastwalk for single directory (depth 1)
	conf := &fastwalk.Config{
		Follow: true, // Follow symlinks to get target info
	}

	pathLen := len(path)

	err := fastwalk.Walk(conf, path, func(fullPath string, d fs.DirEntry, err error) error {
		if err != nil {
			debug.Log(debug.FS_ENTRY, "fetchDir: walk error at %q: %v", fullPath, err)
			return nil // Skip errors, continue walking
		}

		// Skip the root directory itself
		if fullPath == path {
			return nil
		}

		// Only process direct children (depth 1)
		// Optimized: use string slicing instead of filepath.Rel
		// fullPath starts with path, so check if remainder has any separators
		relStart := pathLen
		if relStart < len(fullPath) && (fullPath[relStart] == '/' || fullPath[relStart] == '\\') {
			relStart++
		}
		rel := fullPath[relStart:]
		if strings.ContainsAny(rel, "/\\") {
			// This is a nested entry, skip it but don't recurse into directories
			if d.IsDir() {
				return fastwalk.SkipDir
			}
			return nil
		}

		// Get full file info (fastwalk.StatDirEntry follows symlinks when Follow=true)
		info, err := fastwalk.StatDirEntry(fullPath, d)
		if err != nil {
			// Try lstat as fallback for broken symlinks
			info, err = os.Lstat(fullPath)
			if err != nil {
				debug.Log(debug.FS_ENTRY, "fetchDir: skipping %q: stat error: %v", d.Name(), err)
				return nil
			}
			debug.Log(debug.FS_ENTRY, "fetchDir: %q: using lstat (symlink target inaccessible)", d.Name())
		}

		isDir := info.IsDir()

		debug.Log(debug.FS_ENTRY, "fetchDir: %q isDir=%v size=%d mode=%s",
			d.Name(), isDir, info.Size(), info.Mode())

		mu.Lock()
		result = append(result, Entry{
			Name:    d.Name(),
			Path:    fullPath,
			IsDir:   isDir,
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
		mu.Unlock()

		// Don't recurse into subdirectories for fetchDir (single level only)
		if d.IsDir() {
			return fastwalk.SkipDir
		}
		return nil
	})

	if err != nil {
		debug.Log(debug.FS, "fetchDir: walk error: %v", err)
		return Response{Op: FetchDir, Path: path, Err: err}
	}

	debug.Log(debug.FS, "fetchDir: returning %d entries", len(result))
	return Response{Op: FetchDir, Path: path, Entries: result}
}

func (s *System) searchDir(ctx context.Context, basePath, queryStr string, gen int64, engine search.SearchEngine, engineCmd string, defaultDepth int) Response {
	debug.Log(debug.SEARCH, "searchDir: basePath=%q query=%q engine=%d defaultDepth=%d",
		basePath, queryStr, engine, defaultDepth)

	query := search.Parse(queryStr)
	if query.IsEmpty() {
		debug.Log(debug.SEARCH, "searchDir: empty query, falling back to fetchDir")
		return s.fetchDir(basePath)
	}

	debug.Log(debug.SEARCH, "searchDir: parsed query: contentSearch=%v recursive=%v directives=%d",
		query.HasContentSearch(), query.HasRecursive(), len(query.Directives))

	var results []Entry

	// Default depth is 1 (current directory only)
	// Use recursive: directive to enable deeper search
	maxDepth := 1
	if query.HasRecursive() {
		maxDepth = query.GetRecursiveDepth(defaultDepth)
		if maxDepth <= 0 {
			maxDepth = defaultDepth // Fallback to configured default
		}
	}

	debug.Log(debug.SEARCH, "searchDir: maxDepth=%d", maxDepth)

	// Check if we should use an external search engine for content searches
	if query.HasContentSearch() && engine != search.EngineBuiltin && engineCmd != "" {
		// Use external search engine (ripgrep or ugrep)
		contentPattern := query.GetContentPattern()
		if contentPattern != "" {
			debug.Log(debug.SEARCH, "searchDir: using external engine %s for pattern=%q", engine.String(), contentPattern)
			
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
					debug.Log(debug.SEARCH, "searchDir: external search cancelled")
					return Response{Op: SearchDir, Path: basePath, Cancelled: true}
				}
				debug.Log(debug.SEARCH, "searchDir: external search error: %v, falling back to builtin", err)
				// Fall back to builtin on error
			} else {
				debug.Log(debug.SEARCH, "searchDir: external search found %d paths", len(matchingPaths))

				// Process results directly instead of walking entire directory
				results, err := s.processExternalResults(ctx, gen, matchingPaths, query)
				if err != nil {
					if ctx.Err() != nil {
						debug.Log(debug.SEARCH, "searchDir: result processing cancelled")
						return Response{Op: SearchDir, Path: basePath, Cancelled: true}
					}
					debug.Log(debug.SEARCH, "searchDir: processExternalResults error: %v", err)
					return Response{Op: SearchDir, Path: basePath, Err: err}
				}

				debug.Log(debug.SEARCH, "searchDir: external search complete, %d results", len(results))
				return Response{Op: SearchDir, Path: basePath, Entries: results}
			}
		}
	}
	
	// Use builtin search (single-pass with streaming progress)
	debug.Log(debug.SEARCH, "searchDir: using builtin search")
	matcher := search.NewMatcherWithContext(ctx, query)

	// Create progress tracker with streaming/indeterminate progress
	// (no longer doing a separate count pass - use single walk)
	progress := &searchProgress{
		gen:        gen,
		totalFiles: 0, // Will use indeterminate progress
		totalBytes: 0,
		progressCh: s.ProgressChan,
	}

	results, err := s.walkDirWithProgress(ctx, basePath, maxDepth, matcher, progress)
	if err != nil {
		debug.Log(debug.SEARCH, "searchDir: walkDir error: %v", err)
		return Response{Op: SearchDir, Path: basePath, Err: err}
	}

	debug.Log(debug.SEARCH, "searchDir: complete, %d results", len(results))
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

func (s *System) walkDirWithProgress(ctx context.Context, basePath string, maxDepth int, matcher *search.Matcher, progress *searchProgress) ([]Entry, error) {
	debug.Log(debug.FS_WALK, "walkDir: starting path=%q maxDepth=%d", basePath, maxDepth)

	var results []Entry
	var mu sync.Mutex

	// Don't follow symlinks in recursive searches to avoid infinite loops
	// (e.g., symlinks pointing to parent directories)
	conf := &fastwalk.Config{
		Follow: false,
	}

	err := fastwalk.Walk(conf, basePath, func(fullPath string, d fs.DirEntry, err error) error {
		// Check for cancellation
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err != nil {
			debug.Log(debug.FS_WALK, "walkDir: error at %q: %v", fullPath, err)
			return nil // Skip errors, continue walking
		}

		// Skip the root itself
		if fullPath == basePath {
			return nil
		}

		// Skip system directories
		if shouldSkipPath(fullPath) {
			debug.Log(debug.FS_WALK, "walkDir: skipping system directory: %s", fullPath)
			if d.IsDir() {
				return fastwalk.SkipDir
			}
			return nil
		}

		// Check depth limit using fastwalk's depth tracking
		depth := fastwalk.DirEntryDepth(d)
		if depth > maxDepth {
			if d.IsDir() {
				return fastwalk.SkipDir
			}
			return nil
		}

		// Get full file info (follows symlinks)
		info, err := fastwalk.StatDirEntry(fullPath, d)
		if err != nil {
			// Try lstat as fallback
			info, err = os.Lstat(fullPath)
			if err != nil {
				debug.Log(debug.FS_WALK, "walkDir: skipping %q: stat error: %v", d.Name(), err)
				return nil
			}
		}

		isDir := info.IsDir()

		// Skip non-regular files (devices, sockets, etc.)
		if !isDir && !info.Mode().IsRegular() {
			return nil
		}

		// Update progress for files (streaming progress - indeterminate with count)
		if !isDir && info.Mode().IsRegular() {
			progress.currentFiles++
			progress.currentBytes += info.Size()
			progress.report(fmt.Sprintf("Searched %d files...", progress.currentFiles))
		}

		// Check if this entry matches
		if matcher.Match(fullPath, info) {
			debug.Log(debug.FS_WALK, "walkDir: MATCH %s", fullPath)
			mu.Lock()
			results = append(results, Entry{
				Name:    d.Name(),
				Path:    fullPath,
				IsDir:   isDir,
				Size:    info.Size(),
				ModTime: info.ModTime(),
			})
			mu.Unlock()
		}

		return nil
	})

	if err != nil && ctx.Err() == nil {
		debug.Log(debug.FS_WALK, "walkDir: walk error: %v", err)
		return results, err
	}

	debug.Log(debug.FS_WALK, "walkDir: complete, %d results", len(results))
	return results, nil
}
