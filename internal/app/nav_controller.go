package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/justyntemme/razor/internal/fs"
)

// Navigate adds a path to history and requests directory contents.
func (n *NavigationController) Navigate(path string) {
	// Truncate forward history if we're not at the end
	if n.HistoryIndex >= 0 && n.HistoryIndex < len(n.History)-1 {
		n.History = n.History[:n.HistoryIndex+1]
	}
	n.History = append(n.History, path)
	n.HistoryIndex = len(n.History) - 1

	// Limit history size to prevent unbounded memory growth
	if len(n.History) > maxHistorySize {
		excess := len(n.History) - maxHistorySize
		n.History = n.History[excess:]
		n.HistoryIndex -= excess
		if n.HistoryIndex < 0 {
			n.HistoryIndex = 0
		}
	}

	n.RequestDir(path)
}

// GoBack navigates to the parent directory or previous location.
func (n *NavigationController) GoBack(searchGen *int64) {
	// If in recent view, go to last actual directory in history
	if n.deps.UI.IsRecentView() {
		if n.HistoryIndex >= 0 && n.HistoryIndex < len(n.History) {
			n.RequestDir(n.History[n.HistoryIndex])
		} else if len(n.History) > 0 {
			n.HistoryIndex = len(n.History) - 1
			n.RequestDir(n.History[n.HistoryIndex])
		} else {
			n.Navigate(n.deps.HomePath)
		}
		return
	}

	n.state.Mu.RLock()
	currentPath := n.state.State.CurrentPath
	n.state.Mu.RUnlock()

	parent := filepath.Dir(currentPath)
	if parent == currentPath {
		return // Already at root
	}

	if n.HistoryIndex > 0 && n.History[n.HistoryIndex-1] == parent {
		n.HistoryIndex--
	} else {
		// Insert parent into history
		n.History = append(n.History[:n.HistoryIndex], append([]string{parent}, n.History[n.HistoryIndex:]...)...)
	}
	n.RequestDir(parent)
}

// GoForward navigates forward in history.
func (n *NavigationController) GoForward() {
	if n.HistoryIndex < len(n.History)-1 {
		n.HistoryIndex++
		n.RequestDir(n.History[n.HistoryIndex])
	}
}

// GoHome navigates to the user's home directory.
func (n *NavigationController) GoHome() {
	n.Navigate(n.deps.HomePath)
}

// RequestDir sends a request to fetch directory contents.
func (n *NavigationController) RequestDir(path string) {
	n.state.Mu.Lock()
	n.state.State.SelectedIndex = -1
	n.state.Mu.Unlock()

	gen := n.state.SearchGen.Add(1)
	n.deps.FS.RequestChan <- fs.Request{Op: fs.FetchDir, Path: path, Gen: gen}
}

// ExpandPath expands and normalizes a path string, handling:
// - ~ for home directory
// - Relative paths (../, ./)
// - Absolute paths
// - Windows drive letters (C:, D:, etc.)
// - Root path (/)
func (n *NavigationController) ExpandPath(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		n.state.Mu.RLock()
		currentPath := n.state.State.CurrentPath
		n.state.Mu.RUnlock()
		return currentPath
	}

	// Handle home directory expansion
	if strings.HasPrefix(input, "~") {
		if input == "~" {
			return n.deps.HomePath
		}
		if strings.HasPrefix(input, "~/") || strings.HasPrefix(input, "~\\") {
			input = filepath.Join(n.deps.HomePath, input[2:])
			return filepath.Clean(input)
		}
	}

	// Check if it's an absolute path
	if isAbsolutePath(input) {
		return filepath.Clean(input)
	}

	// Handle relative paths - join with current directory
	n.state.Mu.RLock()
	currentPath := n.state.State.CurrentPath
	n.state.Mu.RUnlock()
	return filepath.Clean(filepath.Join(currentPath, input))
}

// ValidatePath checks if a path exists and returns info about it.
func (n *NavigationController) ValidatePath(path string) (exists bool, isDir bool) {
	info, err := os.Stat(path)
	if err != nil {
		return false, false
	}
	return true, info.IsDir()
}

// OpenFileLocation navigates to the directory containing the given file.
func (n *NavigationController) OpenFileLocation(path string) {
	dir := filepath.Dir(path)
	n.Navigate(dir)
}

// GetCurrentPath returns the current directory path.
func (n *NavigationController) GetCurrentPath() string {
	n.state.Mu.RLock()
	defer n.state.Mu.RUnlock()
	return n.state.State.CurrentPath
}

// GetHomePath returns the user's home directory path.
func (n *NavigationController) GetHomePath() string {
	return n.deps.HomePath
}

// isAbsolutePath checks if a path is absolute, handling both Unix and Windows paths.
func isAbsolutePath(path string) bool {
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

// isLetter checks if a byte is an ASCII letter.
func isLetter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}
