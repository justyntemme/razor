package app

import (
	"fmt"
	"io"
	iofs "io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/charlievieth/fastwalk"
	"github.com/justyntemme/razor/internal/debug"
	"github.com/justyntemme/razor/internal/trash"
	"github.com/justyntemme/razor/internal/ui"
)

// ============================================================================
// File Operation Utilities - shared helper functions to reduce duplication
// ============================================================================

// Common file permission modes
const (
	DirPermission  = 0o755 // Standard directory permissions
	FilePermission = 0o644 // Standard file permissions
)

// pathExists checks if a path exists on the filesystem.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// deleteItem removes a file or directory (recursively for directories).
func deleteItem(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}

// newProgressWriter creates a progress-tracking writer that updates state atomically.
func (o *Orchestrator) newProgressWriter(w io.Writer) io.Writer {
	return &progressWriter{
		w: w,
		onWrite: func(n int64) {
			atomic.AddInt64(&o.state.Progress.Current, n)
			o.window.Invalidate()
		},
	}
}

// refreshCurrentDir refreshes the current directory view while preserving tree expansion state.
// This is used for fsnotify updates where we want to update contents without resetting the view.
func (o *Orchestrator) refreshCurrentDir() {
	// Use StateOwner to refresh while preserving expansions
	o.stateOwner.RefreshCurrentDir()

	// Sync to state for UI
	snapshot := o.stateOwner.GetSnapshot()
	o.stateMu.Lock()
	o.state.Entries = snapshot.Entries
	o.stateMu.Unlock()

	o.window.Invalidate()
}

// refreshExpandedDir refreshes an expanded directory's entries in the current view
func (o *Orchestrator) refreshExpandedDir(path string) {
	// Use StateOwner to refresh the expanded directory
	o.stateOwner.RefreshExpandedDir(path)

	// Sync to state for UI
	snapshot := o.stateOwner.GetSnapshot()
	o.stateMu.Lock()
	o.state.Entries = snapshot.Entries
	o.stateMu.Unlock()

	o.window.Invalidate()
}

// refreshTabEntries marks a tab for refresh when switched to
// In the new architecture, tabs don't cache entries - they re-fetch from filesystem
func (o *Orchestrator) refreshTabEntries(tabIndex int) {
	// Tabs no longer cache entries - they will be refreshed when switched to
	// via loadTabState which calls navCtrl.RequestDir
	debug.Log(debug.APP, "Tab %d marked for refresh on switch", tabIndex)
}

// refreshTabExpandedDir marks an expanded directory in a tab for refresh
// In the new architecture, tabs don't cache entries - they re-fetch from filesystem
func (o *Orchestrator) refreshTabExpandedDir(tabIndex int, path string) {
	// Tabs no longer cache entries - expanded dirs will be refreshed when tab is switched to
	debug.Log(debug.APP, "Tab %d expanded dir %s marked for refresh on switch", tabIndex, path)
}

// ============================================================================
// File Operations
// ============================================================================

// doCreateFile creates a new empty file
func (o *Orchestrator) doCreateFile(name string) {
	if name == "" {
		return
	}

	path := filepath.Join(o.state.CurrentPath, name)
	if pathExists(path) {
		o.ui.ShowError("File already exists: " + name)
		return
	}

	file, err := os.Create(path)
	if err != nil {
		o.ui.ShowError("Error creating file: " + err.Error())
		return
	}
	file.Close()

	o.refreshCurrentDir()
}

// doCreateFolder creates a new folder
func (o *Orchestrator) doCreateFolder(name string) {
	if name == "" {
		return
	}

	path := filepath.Join(o.state.CurrentPath, name)
	if pathExists(path) {
		o.ui.ShowError("Folder already exists: " + name)
		return
	}

	if err := os.Mkdir(path, DirPermission); err != nil {
		o.ui.ShowError("Error creating folder: " + err.Error())
		return
	}

	o.refreshCurrentDir()
}

// doRename renames a file or folder
func (o *Orchestrator) doRename(oldPath, newPath string) {
	if oldPath == "" || newPath == "" || oldPath == newPath {
		return
	}

	if pathExists(newPath) {
		o.ui.ShowError("Cannot rename: a file with that name already exists")
		return
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		o.ui.ShowError("Error renaming: " + err.Error())
		return
	}

	log.Printf("Renamed %s to %s", oldPath, newPath)
	o.refreshCurrentDir()
}

// doDelete moves a file or folder to trash (or permanently deletes if trash unavailable)
func (o *Orchestrator) doDelete(path string) {
	if !pathExists(path) {
		o.ui.ShowError("Delete error: file does not exist")
		return
	}

	var err error
	if trash.IsAvailable() {
		o.setProgress(true, "Moving to "+trash.DisplayName()+": "+filepath.Base(path), 0, 0)
		err = trash.MoveToTrash(path)
	} else {
		o.setProgress(true, "Deleting "+filepath.Base(path), 0, 0)
		err = deleteItem(path)
	}
	o.setProgress(false, "", 0, 0)

	if err != nil {
		o.ui.ShowError("Delete error: " + err.Error())
		return
	}

	// Remove from StateOwner (preserves expansion state)
	o.stateOwner.RemoveEntry(path)

	// Sync to state for UI
	snapshot := o.stateOwner.GetSnapshot()
	o.stateMu.Lock()
	o.state.Entries = snapshot.Entries
	o.state.SelectedIndex = -1
	o.state.SelectedIndices = nil
	o.stateMu.Unlock()
	o.ui.ResetMultiSelect()

	o.window.Invalidate()
}

// doDeleteMultiple moves multiple files or folders to trash
func (o *Orchestrator) doDeleteMultiple(paths []string) {
	total := len(paths)
	deletedPaths := make([]string, 0, total)
	useTrash := trash.IsAvailable()

	var errorCount int
	for i, path := range paths {
		if !pathExists(path) {
			errorCount++
			continue
		}

		var label string
		var err error
		if useTrash {
			label = fmt.Sprintf("Moving to %s (%d/%d) %s", trash.DisplayName(), i+1, total, filepath.Base(path))
			o.setProgress(true, label, 0, 0)
			err = trash.MoveToTrash(path)
		} else {
			label = fmt.Sprintf("Deleting (%d/%d) %s", i+1, total, filepath.Base(path))
			o.setProgress(true, label, 0, 0)
			err = deleteItem(path)
		}

		if err != nil {
			log.Printf("Delete error for %s: %v", path, err)
			errorCount++
		} else {
			deletedPaths = append(deletedPaths, path)
		}
	}
	if errorCount > 0 {
		o.ui.ShowError(fmt.Sprintf("Failed to delete %d of %d items", errorCount, total))
	}
	o.setProgress(false, "", 0, 0)

	// Remove deleted entries from StateOwner (preserves expansion state)
	o.stateOwner.RemoveEntries(deletedPaths)

	// Sync to state for UI
	snapshot := o.stateOwner.GetSnapshot()
	o.stateMu.Lock()
	o.state.Entries = snapshot.Entries
	o.state.SelectedIndex = -1
	o.state.SelectedIndices = nil
	o.stateMu.Unlock()
	o.ui.ResetMultiSelect()

	o.window.Invalidate()
}

// doPermanentDelete permanently deletes a file or folder (bypassing trash)
func (o *Orchestrator) doPermanentDelete(path string) {
	if !pathExists(path) {
		o.ui.ShowError("Delete error: file does not exist")
		return
	}

	o.setProgress(true, "Permanently deleting "+filepath.Base(path), 0, 0)
	err := deleteItem(path)
	o.setProgress(false, "", 0, 0)

	if err != nil {
		o.ui.ShowError("Delete error: " + err.Error())
		return
	}

	// Remove from StateOwner (preserves expansion state)
	o.stateOwner.RemoveEntry(path)

	// Sync to state for UI
	snapshot := o.stateOwner.GetSnapshot()
	o.stateMu.Lock()
	o.state.Entries = snapshot.Entries
	o.state.SelectedIndex = -1
	o.state.SelectedIndices = nil
	o.stateMu.Unlock()
	o.ui.ResetMultiSelect()

	o.window.Invalidate()
}

// doPermanentDeleteMultiple permanently deletes multiple files (bypassing trash)
func (o *Orchestrator) doPermanentDeleteMultiple(paths []string) {
	total := len(paths)
	deletedPaths := make([]string, 0, total)

	var errorCount int
	for i, path := range paths {
		if !pathExists(path) {
			errorCount++
			continue
		}

		label := fmt.Sprintf("Permanently deleting (%d/%d) %s", i+1, total, filepath.Base(path))
		o.setProgress(true, label, 0, 0)
		err := deleteItem(path)

		if err != nil {
			log.Printf("Permanent delete error for %s: %v", path, err)
			errorCount++
		} else {
			deletedPaths = append(deletedPaths, path)
		}
	}
	o.setProgress(false, "", 0, 0)
	if errorCount > 0 {
		o.ui.ShowError(fmt.Sprintf("Failed to delete %d of %d items", errorCount, total))
	}

	// Remove deleted entries from StateOwner (preserves expansion state)
	o.stateOwner.RemoveEntries(deletedPaths)

	// Sync to state for UI
	snapshot := o.stateOwner.GetSnapshot()
	o.stateMu.Lock()
	o.state.Entries = snapshot.Entries
	o.state.SelectedIndex = -1
	o.state.SelectedIndices = nil
	o.stateMu.Unlock()
	o.ui.ResetMultiSelect()

	o.window.Invalidate()
}

// doPaste pastes the clipboard contents to the current directory
func (o *Orchestrator) doPaste() {
	clip := o.state.Clipboard
	if clip == nil || len(clip.Paths) == 0 {
		return
	}

	// Reset conflict resolution state
	o.conflictResolution = ui.ConflictAsk
	o.conflictAbort = false

	dstDir := o.state.CurrentPath
	isCut := clip.Op == ui.ClipCut
	totalFiles := len(clip.Paths)
	var lastErr error

	for i, src := range clip.Paths {
		if o.conflictAbort {
			break
		}

		dstName := filepath.Base(src)
		dst := filepath.Join(dstDir, dstName)

		srcInfo, err := os.Stat(src)
		if err != nil {
			o.ui.ShowError("Cannot access: " + filepath.Base(src))
			lastErr = err
			continue
		}

		// Check for conflict (including pasting to same directory)
		sameFile := src == dst
		dstInfo, err := os.Stat(dst)
		if err == nil || sameFile {
			// Use srcInfo as dstInfo when pasting to same location
			if sameFile {
				dstInfo = srcInfo
			}
			// Destination exists - need to resolve conflict
			remainingFiles := totalFiles - i
			resolution := o.resolveConflict(src, dst, srcInfo, dstInfo, remainingFiles)

			switch resolution {
			case ui.ConflictReplaceAll:
				// Replace - delete destination first (skip if same file)
				if sameFile {
					o.ui.ShowError("Cannot replace a file with itself")
					continue
				}
				deleteItem(dst)
			case ui.ConflictKeepBothAll:
				// Keep both - rename destination
				ext := filepath.Ext(dstName)
				base := strings.TrimSuffix(dstName, ext)
				for j := 1; ; j++ {
					dst = filepath.Join(dstDir, base+"_copy"+strconv.Itoa(j)+ext)
					if !pathExists(dst) {
						break
					}
				}
			case ui.ConflictSkipAll:
				// Skip this file
				continue
			case ui.ConflictAsk:
				// User clicked Stop or dialog was aborted
				o.conflictAbort = true
				continue
			}
		}

		// Check if abort was triggered (by Stop button)
		if o.conflictAbort {
			break
		}

		label := "Copying"
		if isCut {
			label = "Moving"
		}

		// Show progress with file count for multiple files
		progressLabel := label + " " + filepath.Base(src)
		if totalFiles > 1 {
			progressLabel = fmt.Sprintf("%s (%d/%d) %s", label, i+1, totalFiles, filepath.Base(src))
		}

		if srcInfo.IsDir() {
			o.setProgress(true, progressLabel, 0, 0)
			err = o.copyDir(src, dst, isCut)
		} else {
			o.setProgress(true, progressLabel, 0, srcInfo.Size())
			err = o.copyFile(src, dst, isCut)
		}

		if err != nil {
			o.ui.ShowError("Error copying " + filepath.Base(src) + ": " + err.Error())
			lastErr = err
		}
	}

	o.setProgress(false, "", 0, 0)

	// Note: We don't show a summary toast here since individual errors are already shown
	_ = lastErr

	// Clear clipboard after cut operation completes
	if isCut && !o.conflictAbort {
		o.state.Clipboard = nil
	}

	o.refreshCurrentDir()
}

// copyFile copies a single file with progress tracking
func (o *Orchestrator) copyFile(src, dst string, move bool) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(o.newProgressWriter(dstFile), srcFile); err != nil {
		return err
	}

	if err := os.Chmod(dst, info.Mode()); err != nil {
		return err
	}

	if move {
		return os.Remove(src)
	}
	return nil
}

// copyDir copies a directory recursively with progress tracking
func (o *Orchestrator) copyDir(src, dst string, move bool) error {
	// Single-pass copy using fastwalk: count total size while building file list
	var totalSize atomic.Int64
	type copyItem struct {
		srcPath string
		dstPath string
		isDir   bool
		mode    iofs.FileMode
	}
	var items []copyItem
	var itemsMu sync.Mutex

	conf := &fastwalk.Config{Follow: true}
	srcLen := len(src)

	err := fastwalk.Walk(conf, src, func(fullPath string, d iofs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // Skip errors, continue walking
		}

		// Get relative path from source root
		relPath := fullPath[srcLen:]
		if len(relPath) > 0 && (relPath[0] == '/' || relPath[0] == '\\') {
			relPath = relPath[1:]
		}
		if relPath == "" {
			return nil // Skip source root itself
		}

		dstPath := filepath.Join(dst, relPath)
		info, err := fastwalk.StatDirEntry(fullPath, d)
		if err != nil {
			return nil // Skip files we can't stat
		}

		if info.IsDir() {
			itemsMu.Lock()
			items = append(items, copyItem{srcPath: fullPath, dstPath: dstPath, isDir: true, mode: info.Mode()})
			itemsMu.Unlock()
		} else {
			totalSize.Add(info.Size())
			itemsMu.Lock()
			items = append(items, copyItem{srcPath: fullPath, dstPath: dstPath, isDir: false, mode: info.Mode()})
			itemsMu.Unlock()
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Set up progress with counted total
	o.progressMu.Lock()
	o.state.Progress.Total = totalSize.Load()
	o.state.Progress.Current = 0
	o.progressMu.Unlock()

	// Create destination root
	if err := os.MkdirAll(dst, DirPermission); err != nil {
		return err
	}

	// Process items: directories first (sorted by path length to ensure parents exist)
	// then files
	sort.Slice(items, func(i, j int) bool {
		// Directories before files
		if items[i].isDir != items[j].isDir {
			return items[i].isDir
		}
		// Shorter paths first (parents before children)
		return len(items[i].dstPath) < len(items[j].dstPath)
	})

	for _, item := range items {
		if item.isDir {
			if err := os.MkdirAll(item.dstPath, item.mode); err != nil {
				return err
			}
		} else {
			if err := o.copyFileWithProgress(item.srcPath, item.dstPath); err != nil {
				return err
			}
		}
	}

	if move {
		return os.RemoveAll(src)
	}
	return nil
}

// copyFileWithProgress copies a single file with atomic progress updates
func (o *Orchestrator) copyFileWithProgress(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(o.newProgressWriter(dstFile), srcFile); err != nil {
		return err
	}

	return os.Chmod(dst, info.Mode())
}

// progressWriter wraps an io.Writer and calls onWrite after each write
type progressWriter struct {
	w       io.Writer
	onWrite func(int64)
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.w.Write(p)
	if n > 0 && pw.onWrite != nil {
		pw.onWrite(int64(n))
	}
	return n, err
}

// doMove moves files/folders to a destination directory (drag-and-drop)
func (o *Orchestrator) doMove(sources []string, dstDir string) {
	if len(sources) == 0 {
		return
	}

	// Reset conflict resolution state
	o.conflictResolution = ui.ConflictAsk
	o.conflictAbort = false

	totalFiles := len(sources)
	var lastErr error

	for i, src := range sources {
		if o.conflictAbort {
			break
		}

		dstName := filepath.Base(src)
		dst := filepath.Join(dstDir, dstName)

		// Skip if source and destination are the same
		if src == dst {
			debug.Log(debug.APP, "Move: skipping %s, same location", src)
			continue
		}

		// Skip if trying to move into itself (for directories)
		if strings.HasPrefix(dst, src+string(filepath.Separator)) {
			debug.Log(debug.APP, "Move: skipping %s, cannot move into itself", src)
			continue
		}

		srcInfo, err := os.Stat(src)
		if err != nil {
			o.ui.ShowError("Cannot access: " + filepath.Base(src))
			lastErr = err
			continue
		}

		// Check for conflict
		dstInfo, err := os.Stat(dst)
		if err == nil {
			// Destination exists - need to resolve conflict
			remainingFiles := totalFiles - i
			resolution := o.resolveConflict(src, dst, srcInfo, dstInfo, remainingFiles)

			switch resolution {
			case ui.ConflictReplaceAll:
				// Replace - delete destination first
				deleteItem(dst)
			case ui.ConflictKeepBothAll:
				// Keep both - rename destination
				ext := filepath.Ext(dstName)
				base := strings.TrimSuffix(dstName, ext)
				for j := 1; ; j++ {
					dst = filepath.Join(dstDir, base+"_copy"+strconv.Itoa(j)+ext)
					if !pathExists(dst) {
						break
					}
				}
			case ui.ConflictSkipAll:
				// Skip this file
				continue
			case ui.ConflictAsk:
				// User clicked Stop or dialog was aborted
				o.conflictAbort = true
				continue
			}
		}

		// Check if abort was triggered (by Stop button)
		if o.conflictAbort {
			break
		}

		// Show progress
		progressLabel := fmt.Sprintf("Moving (%d/%d) %s", i+1, totalFiles, filepath.Base(src))
		if totalFiles == 1 {
			progressLabel = "Moving " + filepath.Base(src)
		}

		if srcInfo.IsDir() {
			o.setProgress(true, progressLabel, 0, 0)
			if err := o.copyDir(src, dst, true); err != nil {
				o.ui.ShowError("Error moving " + filepath.Base(src) + ": " + err.Error())
				lastErr = err
			}
		} else {
			size := srcInfo.Size()
			o.setProgress(true, progressLabel, 0, size)
			if err := o.copyFile(src, dst, true); err != nil {
				o.ui.ShowError("Error moving " + filepath.Base(src) + ": " + err.Error())
				lastErr = err
			}
		}
	}

	o.setProgress(false, "", 0, 0)

	// Note: We don't show a summary toast here since individual errors are already shown
	_ = lastErr

	// Refresh directory to show changes
	o.navCtrl.RequestDir(o.state.CurrentPath)
}

// doCopyExternal copies files from external sources (e.g., Finder) to the destination directory
func (o *Orchestrator) doCopyExternal(sources []string, dstDir string) {
	if len(sources) == 0 || dstDir == "" {
		return
	}

	debug.Log(debug.APP, "doCopyExternal: %d files to %s", len(sources), dstDir)

	// Reset conflict resolution state
	o.conflictResolution = ui.ConflictAsk
	o.conflictAbort = false

	totalFiles := len(sources)
	var lastErr error

	for i, src := range sources {
		if o.conflictAbort {
			break
		}

		dstName := filepath.Base(src)
		dst := filepath.Join(dstDir, dstName)

		srcInfo, err := os.Stat(src)
		if err != nil {
			o.ui.ShowError("Cannot access: " + filepath.Base(src))
			lastErr = err
			continue
		}

		// Check for conflict
		dstInfo, err := os.Stat(dst)
		if err == nil {
			// Destination exists - need to resolve conflict
			remainingFiles := totalFiles - i
			resolution := o.resolveConflict(src, dst, srcInfo, dstInfo, remainingFiles)

			switch resolution {
			case ui.ConflictReplaceAll:
				// Replace - delete destination first
				deleteItem(dst)
			case ui.ConflictKeepBothAll:
				// Keep both - rename destination
				ext := filepath.Ext(dstName)
				base := strings.TrimSuffix(dstName, ext)
				for j := 1; ; j++ {
					dst = filepath.Join(dstDir, base+"_copy"+strconv.Itoa(j)+ext)
					if !pathExists(dst) {
						break
					}
				}
			case ui.ConflictSkipAll:
				// Skip this file
				continue
			case ui.ConflictAsk:
				// User clicked Stop or dialog was aborted
				o.conflictAbort = true
				continue
			}
		}

		// Check if abort was triggered (by Stop button)
		if o.conflictAbort {
			break
		}

		// Show progress with file count for multiple files
		progressLabel := "Copying " + filepath.Base(src)
		if totalFiles > 1 {
			progressLabel = fmt.Sprintf("Copying (%d/%d) %s", i+1, totalFiles, filepath.Base(src))
		}

		if srcInfo.IsDir() {
			o.setProgress(true, progressLabel, 0, 0)
			err = o.copyDir(src, dst, false)
		} else {
			o.setProgress(true, progressLabel, 0, srcInfo.Size())
			err = o.copyFile(src, dst, false)
		}

		if err != nil {
			o.ui.ShowError("Error copying " + filepath.Base(src) + ": " + err.Error())
			lastErr = err
		}
	}

	o.setProgress(false, "", 0, 0)

	// Note: We don't show a summary toast here since individual errors are already shown
	_ = lastErr

	o.refreshCurrentDir()
}
