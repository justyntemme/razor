//go:build darwin

package fs

import (
	"os"
	"path/filepath"
)

// Drive represents a mounted drive/volume
type Drive struct {
	Name string
	Path string
}

// ListDrives returns mounted drives on macOS
func ListDrives() []Drive {
	var drives []Drive

	// List /Volumes - all mounted volumes appear here on macOS
	entries, err := os.ReadDir("/Volumes")
	if err != nil {
		// Fallback to just root
		return []Drive{{Name: "Macintosh HD", Path: "/"}}
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		path := filepath.Join("/Volumes", name)

		// Check if it's a symlink to / (the main drive)
		if target, err := os.Readlink(path); err == nil && target == "/" {
			drives = append([]Drive{{Name: name, Path: "/"}}, drives...)
			continue
		}

		// Verify it's accessible
		if _, err := os.Stat(path); err != nil {
			continue
		}

		drives = append(drives, Drive{Name: name, Path: path})
	}

	// Ensure we always have at least the root
	if len(drives) == 0 {
		drives = append(drives, Drive{Name: "Macintosh HD", Path: "/"})
	}

	return drives
}
