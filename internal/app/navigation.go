package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/justyntemme/razor/internal/fs"
)

// expandPath expands and normalizes a path string, handling:
// - ~ for home directory
// - Relative paths (../, ./)
// - Absolute paths
// - Windows drive letters (C:, D:, etc.)
// - Root path (/)
func (o *Orchestrator) expandPath(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return o.state.CurrentPath
	}

	// Handle home directory expansion
	if strings.HasPrefix(input, "~") {
		if input == "~" {
			return o.homePath
		}
		if strings.HasPrefix(input, "~/") || strings.HasPrefix(input, "~\\") {
			input = filepath.Join(o.homePath, input[2:])
			return filepath.Clean(input)
		}
	}

	// Check if it's an absolute path
	if o.isAbsolutePath(input) {
		return filepath.Clean(input)
	}

	// Handle relative paths - join with current directory
	return filepath.Clean(filepath.Join(o.state.CurrentPath, input))
}

// isAbsolutePath checks if a path is absolute, handling both Unix and Windows paths
func (o *Orchestrator) isAbsolutePath(path string) bool {
	if len(path) == 0 {
		return false
	}

	// Unix absolute path
	if path[0] == '/' {
		return true
	}

	// Windows absolute path checks
	if runtime.GOOS == "windows" {
		// Drive letter paths: C:\, D:\, C:/, etc.
		if len(path) >= 2 && isLetter(path[0]) && path[1] == ':' {
			return true
		}
		// UNC paths: \\server\share
		if len(path) >= 2 && path[0] == '\\' && path[1] == '\\' {
			return true
		}
	}

	return false
}

// isLetter checks if a byte is an ASCII letter
func isLetter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

// validatePath checks if a path exists and returns info about it
func (o *Orchestrator) validatePath(path string) (exists bool, isDir bool) {
	info, err := os.Stat(path)
	if err != nil {
		return false, false
	}
	return true, info.IsDir()
}

// navigate adds a path to history and requests directory contents
func (o *Orchestrator) navigate(path string) {
	if o.historyIndex >= 0 && o.historyIndex < len(o.history)-1 {
		o.history = o.history[:o.historyIndex+1]
	}
	o.history = append(o.history, path)
	o.historyIndex = len(o.history) - 1

	// Limit history size to prevent unbounded memory growth
	if len(o.history) > maxHistorySize {
		// Remove oldest entries
		excess := len(o.history) - maxHistorySize
		o.history = o.history[excess:]
		o.historyIndex -= excess
		if o.historyIndex < 0 {
			o.historyIndex = 0
		}
	}

	o.requestDir(path)
}

// goBack navigates to the parent directory
func (o *Orchestrator) goBack() {
	parent := filepath.Dir(o.state.CurrentPath)
	if parent == o.state.CurrentPath {
		return
	}
	if o.historyIndex > 0 && o.history[o.historyIndex-1] == parent {
		o.historyIndex--
	} else {
		o.history = append(o.history[:o.historyIndex], append([]string{parent}, o.history[o.historyIndex:]...)...)
	}
	o.requestDir(parent)
}

// goForward navigates forward in history
func (o *Orchestrator) goForward() {
	if o.historyIndex < len(o.history)-1 {
		o.historyIndex++
		o.requestDir(o.history[o.historyIndex])
	}
}

// requestDir sends a request to fetch directory contents
func (o *Orchestrator) requestDir(path string) {
	o.state.SelectedIndex = -1
	// Increment generation to invalidate any pending search results (atomic)
	gen := o.searchGen.Add(1)
	o.fs.RequestChan <- fs.Request{Op: fs.FetchDir, Path: path, Gen: gen}
}
