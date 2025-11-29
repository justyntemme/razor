package fs

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charlievieth/fastwalk"
	"github.com/justyntemme/fast-text-search/fts"
)

type EventType int

const (
	FetchDir EventType = iota
	SearchDir
)

type Request struct {
	Op    EventType
	Path  string
	Query string // Search term
}

type Entry struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

type Response struct {
	Op      EventType
	Path    string
	Entries []Entry
	Err     error
}

type System struct {
	RequestChan  chan Request
	ResponseChan chan Response
}

func NewSystem() *System {
	return &System{
		RequestChan:  make(chan Request, 10),
		ResponseChan: make(chan Response, 10),
	}
}

func (s *System) Start() {
	for req := range s.RequestChan {
		switch req.Op {
		case FetchDir:
			s.handleFetchDir(req)
		case SearchDir:
			s.handleSearchDir(req)
		}
	}
}

func (s *System) handleFetchDir(req Request) {
	absPath, err := filepath.Abs(req.Path)
	if err != nil {
		s.ResponseChan <- Response{Op: req.Op, Err: err}
		return
	}

	var mu sync.Mutex
	entries := make([]Entry, 0, 100)

	config := fastwalk.Config{
		Follow: false,
	}

	err = fastwalk.Walk(&config, absPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == absPath {
			return nil
		}

		info, _ := d.Info()
		size := int64(0)
		var modTime time.Time

		if info != nil {
			size = info.Size()
			modTime = info.ModTime()
		}

		entry := Entry{
			Name:    d.Name(),
			Path:    path,
			IsDir:   d.IsDir(),
			Size:    size,
			ModTime: modTime,
		}

		mu.Lock()
		entries = append(entries, entry)
		mu.Unlock()

		if d.IsDir() {
			return filepath.SkipDir
		}

		return nil
	})

	if err != nil && err != filepath.SkipDir {
		s.ResponseChan <- Response{Op: req.Op, Err: err}
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})

	s.ResponseChan <- Response{
		Op:      req.Op,
		Path:    absPath,
		Entries: entries,
	}
}

func (s *System) handleSearchDir(req Request) {
	absPath, err := filepath.Abs(req.Path)
	if err != nil {
		s.ResponseChan <- Response{Op: req.Op, Err: err}
		return
	}

	// Use a map to track unique entries by path (prevents duplicates)
	uniqueEntries := make(map[string]Entry)
	var mu sync.Mutex

	// 1. Content Search (fast-text-search)
	// FTS(searchString, directory, ignoreExt, ignoreFolders, fileName, extType)
	// We pass empty slices for ignores to search everything.
	contentResults := fts.FTS(req.Query, absPath, []string{}, []string{}, "", "")

	for _, res := range contentResults {
		// res is the path string itself
		info, err := os.Stat(res)
		if err != nil {
			continue
		}

		uniqueEntries[res] = Entry{
			Name:    info.Name(),
			Path:    res,
			IsDir:   info.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}
	}

	// 2. Filename Search (fastwalk)
	config := fastwalk.Config{
		Follow: false,
	}

	queryLower := strings.ToLower(req.Query)

	_ = fastwalk.Walk(&config, absPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == absPath {
			return nil
		}

		// Check if filename matches query (case-insensitive substring)
		if strings.Contains(strings.ToLower(d.Name()), queryLower) {
			mu.Lock()
			defer mu.Unlock()

			// Only add if not already found by content search
			if _, exists := uniqueEntries[path]; !exists {
				info, _ := d.Info()
				size := int64(0)
				var modTime time.Time
				if info != nil {
					size = info.Size()
					modTime = info.ModTime()
				}

				uniqueEntries[path] = Entry{
					Name:    d.Name(),
					Path:    path,
					IsDir:   d.IsDir(),
					Size:    size,
					ModTime: modTime,
				}
			}
		}
		return nil
	})

	// Convert map back to slice
	entries := make([]Entry, 0, len(uniqueEntries))
	for _, entry := range uniqueEntries {
		entries = append(entries, entry)
	}

	// Sort results
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})

	// Send back results
	s.ResponseChan <- Response{
		Op:      req.Op,
		Path:    req.Path, // Keep context of where we searched
		Entries: entries,
	}
}