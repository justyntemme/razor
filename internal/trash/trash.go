// Package trash provides cross-platform trash/recycle bin functionality.
// It moves files to the system trash instead of permanently deleting them,
// and provides functionality to view, restore, and empty the trash.
package trash

import (
	"os"
	"time"
)

// Item represents a file or directory in the trash
type Item struct {
	Name         string    // Original filename
	OriginalPath string    // Full path where the file was deleted from
	TrashPath    string    // Current path in trash
	DeletedAt    time.Time // When the file was deleted
	Size         int64     // Size in bytes
	IsDir        bool      // Whether this is a directory
}

// MoveToTrash moves a file or directory to the system trash.
// Returns an error if the operation fails.
func MoveToTrash(path string) error {
	return moveToTrash(path)
}

// Restore restores a trashed item to its original location.
// Returns an error if the original location is occupied or restoration fails.
func Restore(item Item) error {
	return restore(item)
}

// RestoreTo restores a trashed item to a specified location.
// Use this when the original location is unavailable.
func RestoreTo(item Item, destPath string) error {
	return restoreTo(item, destPath)
}

// List returns all items currently in the trash.
func List() ([]Item, error) {
	return list()
}

// Empty permanently deletes all items in the trash.
func Empty() error {
	return empty()
}

// Delete permanently deletes a specific item from the trash.
func Delete(item Item) error {
	return deleteItem(item)
}

// GetPath returns the path to the trash directory.
func GetPath() string {
	return getPath()
}

// IsAvailable returns true if trash functionality is available on this platform.
func IsAvailable() bool {
	return isAvailable()
}

// PermanentDelete permanently deletes a file without using the trash.
// This is the same as the old deleteItem behavior.
func PermanentDelete(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}

// DisplayName returns the platform-appropriate name for the trash.
// "Trash" on macOS/Linux, "Recycle Bin" on Windows.
func DisplayName() string {
	return displayName()
}

// VerbPhrase returns the action phrase for moving to trash.
// "Move to Trash" on macOS/Linux, "Move to Recycle Bin" on Windows.
func VerbPhrase() string {
	return "Move to " + DisplayName()
}
