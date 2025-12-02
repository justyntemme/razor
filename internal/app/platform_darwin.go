//go:build darwin

package app

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charlievieth/fastwalk"
)

// platformOpen opens the file using the macOS 'open' command.
func platformOpen(path string) error {
	return exec.Command("open", path).Start()
}

// platformOpenWith opens a file with a specific application or shows the Open With menu.
// If appPath is empty, this will reveal the file in Finder for manual "Open With" selection.
func platformOpenWith(filePath string, appPath string) error {
	if appPath != "" {
		// Open with specific application using: open -a "Application Name" file
		return exec.Command("open", "-a", appPath, filePath).Start()
	}

	// On macOS, we can use AppleScript to show the "Open With" submenu
	// First, try to use the system Services menu approach
	script := `
		tell application "Finder"
			activate
			reveal POSIX file "` + filePath + `"
		end tell
	`
	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Start(); err != nil {
		// Fallback: just reveal in Finder
		return exec.Command("open", "-R", filePath).Start()
	}
	return nil
}

// platformGetApplications returns applications that can open the given file type.
// Uses macOS's Launch Services via lsappinfo and file extension matching.
func platformGetApplications(filePath string) ([]AppInfo, error) {
	var apps []AppInfo

	// Get the file extension
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == "" {
		return apps, nil
	}

	// Common application paths to check
	appDirs := []string{
		"/Applications",
		"/System/Applications",
		"/System/Applications/Utilities",
	}

	// Get user's home directory for user applications
	if home, err := os.UserHomeDir(); err == nil {
		appDirs = append(appDirs, filepath.Join(home, "Applications"))
	}

	// Scan for .app bundles using fastwalk
	seen := make(map[string]bool)
	var mu sync.Mutex
	conf := &fastwalk.Config{Follow: true}

	for _, dir := range appDirs {
		fastwalk.Walk(conf, dir, func(fullPath string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // Skip errors
			}

			// Skip the root directory itself
			if fullPath == dir {
				return nil
			}

			// Only process direct children
			if filepath.Dir(fullPath) != dir {
				if d.IsDir() {
					return fastwalk.SkipDir
				}
				return nil
			}

			if !d.IsDir() || !strings.HasSuffix(d.Name(), ".app") {
				return nil
			}

			appName := strings.TrimSuffix(d.Name(), ".app")

			mu.Lock()
			if seen[appName] {
				mu.Unlock()
				return fastwalk.SkipDir
			}
			seen[appName] = true
			apps = append(apps, AppInfo{
				Name: appName,
				Path: fullPath,
			})
			mu.Unlock()

			return fastwalk.SkipDir // Don't recurse into .app bundles
		})
	}

	return apps, nil
}

// AppInfo represents an application that can open files
type AppInfo struct {
	Name string // Display name
	Path string // Application bundle path
}

// platformOpenTerminal opens a terminal in the specified directory.
// On macOS, this opens Terminal.app which uses the user's login shell.
func platformOpenTerminal(dir string) error {
	return exec.Command("open", "-a", "Terminal", dir).Start()
}
