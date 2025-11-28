package fs

import (
	"log"
	"path/filepath"
)

// EventType defines what kind of file operation occurred.
type EventType int

const (
	FetchDir EventType = iota
)

// Request represents a command sent from the UI to the FS worker.
type Request struct {
	Op   EventType
	Path string
}

// Response represents data returning from the FS worker to the UI.
type Response struct {
	Op    EventType
	Path  string
	Items []string // Simplified for now (just names)
	Err   error
}

// System handles all IO operations asynchronously.
type System struct {
	// RequestChan is where the UI sends commands.
	RequestChan chan Request
	// ResponseChan is where the UI listens for results.
	ResponseChan chan Response
}

// NewSystem initializes the channels.
func NewSystem() *System {
	return &System{
		// Buffered channels to prevent blocking the UI thread on send
		RequestChan:  make(chan Request, 10),
		ResponseChan: make(chan Response, 10),
	}
}

// Start begins the FS worker loop.
// This should be run in a separate goroutine.
func (s *System) Start() {
	for req := range s.RequestChan {
		switch req.Op {
		case FetchDir:
			s.handleFetchDir(req)
		}
	}
}

// handleFetchDir is a placeholder for the future fastwalk implementation.
func (s *System) handleFetchDir(req Request) {
	// Simulate work
	absPath, err := filepath.Abs(req.Path)
	if err != nil {
		s.ResponseChan <- Response{Op: req.Op, Err: err}
		return
	}

	// Just echoing the path back for the "Hello World" phase,
	// later this will be the file list.
	log.Printf("FS: Listing directory %s", absPath)

	s.ResponseChan <- Response{
		Op:    req.Op,
		Path:  absPath,
		Items: []string{"file1.txt", "file2.go", "folder_a"},
		Err:   nil,
	}
}
