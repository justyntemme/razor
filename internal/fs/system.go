package fs

import (
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/charlievieth/fastwalk"
)

type EventType int

const (
	FetchDir EventType = iota
	SearchDir
)

type Request struct {
	Op   EventType
	Path string
}

type Entry struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	ModTime time.Time // Captured for UI columns
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
			// Placeholder
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