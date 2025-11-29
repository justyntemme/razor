package fs

import (
	"os"
	"path/filepath"
	"time"

	"github.com/justyntemme/razor/internal/search"
)

type OpType int

const (
	FetchDir OpType = iota
	SearchDir
)

type Request struct {
	Op    OpType
	Path  string
	Query string
}

type Entry struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

type Response struct {
	Op      OpType
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
		var resp Response
		switch req.Op {
		case FetchDir:
			resp = s.fetchDir(req.Path)
		case SearchDir:
			resp = s.searchDir(req.Path, req.Query)
		}
		s.ResponseChan <- resp
	}
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

func (s *System) searchDir(basePath, queryStr string) Response {
	query := search.Parse(queryStr)
	if query.IsEmpty() {
		return s.fetchDir(basePath)
	}

	matcher := search.NewMatcher(query)
	var results []Entry

	// Determine search depth - content searches go recursive
	maxDepth := 1
	if query.HasContentSearch() {
		maxDepth = 10 // Limit recursion depth for safety
	}

	err := s.walkDir(basePath, 0, maxDepth, matcher, &results)
	if err != nil {
		return Response{Op: SearchDir, Path: basePath, Err: err}
	}

	return Response{Op: SearchDir, Path: basePath, Entries: results}
}

func (s *System) walkDir(path string, depth, maxDepth int, matcher *search.Matcher, results *[]Entry) error {
	if depth > maxDepth {
		return nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	for _, e := range entries {
		fullPath := filepath.Join(path, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}

		// Check if this entry matches
		if matcher.Match(fullPath, info) {
			*results = append(*results, Entry{
				Name:    e.Name(),
				Path:    fullPath,
				IsDir:   e.IsDir(),
				Size:    info.Size(),
				ModTime: info.ModTime(),
			})
		}

		// Recurse into directories for content search
		if e.IsDir() && depth < maxDepth {
			s.walkDir(fullPath, depth+1, maxDepth, matcher, results)
		}
	}

	return nil
}