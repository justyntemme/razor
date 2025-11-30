//go:build darwin

package fs

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/charlievieth/fastwalk"
)

// Drive represents a mounted drive/volume
type Drive struct {
	Name string
	Path string
}

// ListDrives returns mounted drives on macOS
func ListDrives() []Drive {
	var drives []Drive
	var mu sync.Mutex

	conf := &fastwalk.Config{Follow: true}

	err := fastwalk.Walk(conf, "/Volumes", func(fullPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip the root /Volumes directory itself
		if fullPath == "/Volumes" {
			return nil
		}

		// Only process direct children (skip nested entries)
		if filepath.Dir(fullPath) != "/Volumes" {
			if d.IsDir() {
				return fastwalk.SkipDir
			}
			return nil
		}

		if !d.IsDir() {
			return nil
		}

		name := d.Name()

		// Check if it's a symlink to / (the main drive)
		if target, err := os.Readlink(fullPath); err == nil && target == "/" {
			mu.Lock()
			drives = append([]Drive{{Name: name, Path: "/"}}, drives...)
			mu.Unlock()
			return fastwalk.SkipDir
		}

		// Verify it's accessible
		if _, err := os.Stat(fullPath); err != nil {
			return fastwalk.SkipDir
		}

		mu.Lock()
		drives = append(drives, Drive{Name: name, Path: fullPath})
		mu.Unlock()

		return fastwalk.SkipDir // Don't recurse into volumes
	})

	if err != nil || len(drives) == 0 {
		// Fallback to just root
		return []Drive{{Name: "Macintosh HD", Path: "/"}}
	}

	return drives
}
