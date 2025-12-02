//go:build linux

package trash

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Linux uses the freedesktop.org trash specification.
// Trash location: ~/.local/share/Trash/
// Structure:
//   - files/     - actual trashed files
//   - info/      - .trashinfo metadata files
//
// .trashinfo format:
// [Trash Info]
// Path=/original/path/to/file
// DeletionDate=2024-01-15T10:30:45

func getPath() string {
	// Check XDG_DATA_HOME first
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "Trash")
}

func getFilesPath() string {
	return filepath.Join(getPath(), "files")
}

func getInfoPath() string {
	return filepath.Join(getPath(), "info")
}

func isAvailable() bool {
	trashPath := getPath()
	if trashPath == "" {
		return false
	}
	// Try to create trash directories if they don't exist
	filesPath := getFilesPath()
	infoPath := getInfoPath()

	if err := os.MkdirAll(filesPath, 0700); err != nil {
		return false
	}
	if err := os.MkdirAll(infoPath, 0700); err != nil {
		return false
	}
	return true
}

func moveToTrash(path string) error {
	// Ensure trash directories exist
	filesPath := getFilesPath()
	infoPath := getInfoPath()

	if err := os.MkdirAll(filesPath, 0700); err != nil {
		return fmt.Errorf("cannot create trash files directory: %w", err)
	}
	if err := os.MkdirAll(infoPath, 0700); err != nil {
		return fmt.Errorf("cannot create trash info directory: %w", err)
	}

	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	// Generate unique name in trash
	baseName := filepath.Base(absPath)
	destName := baseName
	destPath := filepath.Join(filesPath, destName)

	// Handle conflicts by appending numbers
	counter := 1
	for {
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			break
		}
		ext := filepath.Ext(baseName)
		name := strings.TrimSuffix(baseName, ext)
		destName = fmt.Sprintf("%s.%d%s", name, counter, ext)
		destPath = filepath.Join(filesPath, destName)
		counter++
	}

	// Create .trashinfo file
	infoContent := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n",
		url.PathEscape(absPath),
		time.Now().Format("2006-01-02T15:04:05"))

	infoFilePath := filepath.Join(infoPath, destName+".trashinfo")
	if err := os.WriteFile(infoFilePath, []byte(infoContent), 0600); err != nil {
		return fmt.Errorf("cannot create trashinfo file: %w", err)
	}

	// Move the file to trash
	if err := os.Rename(absPath, destPath); err != nil {
		// Clean up info file on failure
		os.Remove(infoFilePath)
		return fmt.Errorf("cannot move file to trash: %w", err)
	}

	return nil
}

func restore(item Item) error {
	// Restore not supported - use Copy or Cut instead for consistency across platforms
	return fmt.Errorf("restore not supported - use Copy or Cut instead")
}

func restoreTo(item Item, destPath string) error {
	// Restore not supported - use Copy or Cut instead for consistency across platforms
	return fmt.Errorf("restore not supported - use Copy or Cut instead")
}

func list() ([]Item, error) {
	filesPath := getFilesPath()
	infoPath := getInfoPath()

	entries, err := os.ReadDir(filesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Empty trash
		}
		return nil, err
	}

	var items []Item
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		fullPath := filepath.Join(filesPath, entry.Name())

		// Try to read .trashinfo for original path and deletion date
		item := Item{
			Name:      entry.Name(),
			TrashPath: fullPath,
			DeletedAt: info.ModTime(),
			Size:      info.Size(),
			IsDir:     entry.IsDir(),
		}

		// Parse .trashinfo file
		infoFilePath := filepath.Join(infoPath, entry.Name()+".trashinfo")
		if origPath, delTime, err := parseTrashInfo(infoFilePath); err == nil {
			item.OriginalPath = origPath
			if !delTime.IsZero() {
				item.DeletedAt = delTime
			}
		}

		items = append(items, item)
	}

	return items, nil
}

func parseTrashInfo(path string) (originalPath string, deletionDate time.Time, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", time.Time{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Path=") {
			// URL decode the path
			encodedPath := strings.TrimPrefix(line, "Path=")
			decoded, err := url.PathUnescape(encodedPath)
			if err == nil {
				originalPath = decoded
			} else {
				originalPath = encodedPath
			}
		} else if strings.HasPrefix(line, "DeletionDate=") {
			dateStr := strings.TrimPrefix(line, "DeletionDate=")
			// Parse ISO 8601 format
			if t, err := time.Parse("2006-01-02T15:04:05", dateStr); err == nil {
				deletionDate = t
			}
		}
	}

	return originalPath, deletionDate, scanner.Err()
}

func empty() error {
	filesPath := getFilesPath()
	infoPath := getInfoPath()

	// Remove all files in files directory
	entries, err := os.ReadDir(filesPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var lastErr error
	for _, entry := range entries {
		fullPath := filepath.Join(filesPath, entry.Name())
		if err := os.RemoveAll(fullPath); err != nil {
			lastErr = err
		}
	}

	// Remove all .trashinfo files
	infoEntries, err := os.ReadDir(infoPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	for _, entry := range infoEntries {
		fullPath := filepath.Join(infoPath, entry.Name())
		if err := os.Remove(fullPath); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

func deleteItem(item Item) error {
	// Remove the file
	if err := os.RemoveAll(item.TrashPath); err != nil {
		return err
	}

	// Remove the .trashinfo file
	infoFile := filepath.Join(getInfoPath(), filepath.Base(item.TrashPath)+".trashinfo")
	os.Remove(infoFile) // Ignore error

	return nil
}

func displayName() string {
	return "Trash"
}
