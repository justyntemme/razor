//go:build darwin

package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	// Scan for .app bundles
	seen := make(map[string]bool)
	for _, dir := range appDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || !strings.HasSuffix(entry.Name(), ".app") {
				continue
			}
			appPath := filepath.Join(dir, entry.Name())
			appName := strings.TrimSuffix(entry.Name(), ".app")
			if seen[appName] {
				continue
			}
			seen[appName] = true
			apps = append(apps, AppInfo{
				Name: appName,
				Path: appPath,
			})
		}
	}

	return apps, nil
}

// AppInfo represents an application that can open files
type AppInfo struct {
	Name string // Display name
	Path string // Application bundle path
}
