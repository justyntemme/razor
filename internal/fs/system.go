package fs

import (
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/charlievieth/fastwalk"
)

// EventType defines the operation requested.
type EventType int

const (
	FetchDir EventType = iota
	// SearchDir will be implemented in Phase 4
	SearchDir
)

// Request is the input signal from the UI.
type Request struct {
	Op   EventType
	Path string
}

// Entry is a lightweight, decoupled representation of a file.
type Entry struct {
	Name  string
	Path  string
	IsDir bool
	Size  int64
}

// Response is the output signal to the UI.
type Response struct {
	Op      EventType
	Path    string
	Entries []Entry
	Err     error
}

// System manages file I/O on a dedicated goroutine.
type System struct {
	RequestChan  chan Request
	ResponseChan chan Response
}

// NewSystem initializes the buffered channels.
func NewSystem() *System {
	return &System{
		RequestChan:  make(chan Request, 10),
		ResponseChan: make(chan Response, 10),
	}
}

// Start begins the event loop.
func (s *System) Start() {
	for req := range s.RequestChan {
		switch req.Op {
		case FetchDir:
			s.handleFetchDir(req)
		case SearchDir:
			// Placeholder for recursive fastwalk logic
		}
	}
}

// handleFetchDir uses fastwalk to list the immediate directory contents.
func (s *System) handleFetchDir(req Request) {
	absPath, err := filepath.Abs(req.Path)
	if err != nil {
		s.ResponseChan <- Response{Op: req.Op, Err: err}
		return
	}

	// We use a mutex because fastwalk invokes the callback concurrently.
	var mu sync.Mutex
	// Pre-allocate a buffer to minimize slice expansion overhead.
	entries := make([]Entry, 0, 100)

	config := fastwalk.Config{
		Follow: false, // Don't follow symlinks for standard browsing
	}

	// fastwalk.Walk is our high-performance engine.
	err = fastwalk.Walk(&config, absPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// If we can't read a specific file, skip it but don't fail the whole view
			return nil
		}

		// 1. Skip the root directory itself (we only want contents)
		if path == absPath {
			return nil
		}

		// 2. Build the entry
		info, _ := d.Info() // fastwalk often has this cached
		size := int64(0)
		if info != nil {
			size = info.Size()
		}

		entry := Entry{
			Name:  d.Name(),
			Path:  path,
			IsDir: d.IsDir(),
			Size:  size,
		}

		// 3. Thread-safe append
		mu.Lock()
		entries = append(entries, entry)
		mu.Unlock()

		// 4. Depth Control:
		// If it's a directory, we list it (above), but we return SkipDir
		// so fastwalk doesn't recursively enter it.
		if d.IsDir() {
			return filepath.SkipDir
		}

		return nil
	})

	if err != nil && err != filepath.SkipDir {
		s.ResponseChan <- Response{Op: req.Op, Err: err}
		return
	}

	// Post-processing: Sort purely for UI presentation.
	// Logic: Directories first, then alphabetical.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})

	// Send result back to the Orchestrator
	s.ResponseChan <- Response{
		Op:      req.Op,
		Path:    absPath,
		Entries: entries,
	}
}