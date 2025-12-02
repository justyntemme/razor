//go:build darwin

package trash

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// macOS uses ~/.Trash for the user's trash.
// Files are moved there directly without metadata files (unlike Linux freedesktop spec).
// To handle name conflicts, we append timestamps to filenames.

func getPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".Trash")
}

func isAvailable() bool {
	trashPath := getPath()
	if trashPath == "" {
		return false
	}
	info, err := os.Stat(trashPath)
	return err == nil && info.IsDir()
}

func moveToTrash(path string) error {
	trashPath := getPath()
	if trashPath == "" {
		return fmt.Errorf("trash directory not found")
	}

	// Get the base name
	baseName := filepath.Base(path)

	// Generate destination path, handling conflicts
	destPath := filepath.Join(trashPath, baseName)
	if _, err := os.Stat(destPath); err == nil {
		// File exists, add timestamp to make unique
		ext := filepath.Ext(baseName)
		name := strings.TrimSuffix(baseName, ext)
		timestamp := time.Now().Format("2006-01-02-150405")
		destPath = filepath.Join(trashPath, fmt.Sprintf("%s %s%s", name, timestamp, ext))
	}

	// Try to rename (move) the file - fastest if on same filesystem
	if err := os.Rename(path, destPath); err != nil {
		// If rename fails (cross-device), fall back to copy+delete
		return moveFileCrossDevice(path, destPath)
	}

	return nil
}

// moveFileCrossDevice handles moving files across different filesystems
func moveFileCrossDevice(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if srcInfo.IsDir() {
		// For directories, use recursive copy
		if err := copyDirRecursive(src, dst); err != nil {
			return err
		}
		return os.RemoveAll(src)
	}

	// For files, simple copy
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, data, srcInfo.Mode())
}

func copyDirRecursive(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDirRecursive(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func restore(item Item) error {
	// macOS doesn't track original paths - restore is not supported
	// Users should use Copy/Cut to move files out of trash
	return fmt.Errorf("restore not supported on macOS - use Copy or Cut instead")
}

func restoreTo(item Item, destPath string) error {
	// Restore not supported - use Copy or Cut instead for consistency across platforms
	return fmt.Errorf("restore not supported - use Copy or Cut instead")
}

func list() ([]Item, error) {
	trashPath := getPath()
	if trashPath == "" {
		return nil, fmt.Errorf("trash directory not found")
	}

	entries, err := os.ReadDir(trashPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Empty trash
		}
		return nil, err
	}

	var items []Item
	for _, entry := range entries {
		// Skip .DS_Store and other hidden system files
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		fullPath := filepath.Join(trashPath, entry.Name())

		// On macOS, we don't have metadata about original location
		// We'll use the trash path as both for now
		// Users can restore to a chosen location
		items = append(items, Item{
			Name:         entry.Name(),
			OriginalPath: "", // macOS doesn't store original path without .DS_Store parsing
			TrashPath:    fullPath,
			DeletedAt:    info.ModTime(),
			Size:         info.Size(),
			IsDir:        entry.IsDir(),
		})
	}

	return items, nil
}

func empty() error {
	trashPath := getPath()
	if trashPath == "" {
		return fmt.Errorf("trash directory not found")
	}

	entries, err := os.ReadDir(trashPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Already empty
		}
		return err
	}

	var lastErr error
	for _, entry := range entries {
		// Skip .DS_Store - let the system manage it
		if entry.Name() == ".DS_Store" {
			continue
		}

		fullPath := filepath.Join(trashPath, entry.Name())
		if err := os.RemoveAll(fullPath); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

func deleteItem(item Item) error {
	return os.RemoveAll(item.TrashPath)
}

func displayName() string {
	return "Trash"
}
