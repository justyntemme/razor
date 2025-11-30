package app

import (
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
	"github.com/justyntemme/razor/internal/ui"
)

// doCreateFile creates a new empty file
func (o *Orchestrator) doCreateFile(name string) {
	if name == "" {
		return
	}

	path := filepath.Join(o.state.CurrentPath, name)

	// Check if file already exists
	if _, err := os.Stat(path); err == nil {
		log.Printf("File already exists: %s", path)
		return
	}

	// Create the file
	file, err := os.Create(path)
	if err != nil {
		log.Printf("Error creating file: %v", err)
		return
	}
	file.Close()

	// Refresh the directory
	o.requestDir(o.state.CurrentPath)
}

// doCreateFolder creates a new folder
func (o *Orchestrator) doCreateFolder(name string) {
	if name == "" {
		return
	}

	path := filepath.Join(o.state.CurrentPath, name)

	// Check if folder already exists
	if _, err := os.Stat(path); err == nil {
		log.Printf("Folder already exists: %s", path)
		return
	}

	// Create the folder
	if err := os.Mkdir(path, 0755); err != nil {
		log.Printf("Error creating folder: %v", err)
		return
	}

	// Refresh the directory
	o.requestDir(o.state.CurrentPath)
}

// doRename renames a file or folder
func (o *Orchestrator) doRename(oldPath, newPath string) {
	if oldPath == "" || newPath == "" || oldPath == newPath {
		return
	}

	// Check if new path already exists
	if _, err := os.Stat(newPath); err == nil {
		log.Printf("Cannot rename: destination already exists: %s", newPath)
		return
	}

	// Perform the rename
	if err := os.Rename(oldPath, newPath); err != nil {
		log.Printf("Error renaming %s to %s: %v", oldPath, newPath, err)
		return
	}

	log.Printf("Renamed %s to %s", oldPath, newPath)

	// Refresh the directory
	o.requestDir(o.state.CurrentPath)
}

// doDelete deletes a file or folder
func (o *Orchestrator) doDelete(path string) {
	info, err := os.Stat(path)
	if err != nil {
		log.Printf("Delete error: %v", err)
		return
	}

	o.setProgress(true, "Deleting "+filepath.Base(path), 0, 0)

	if info.IsDir() {
		err = os.RemoveAll(path)
	} else {
		err = os.Remove(path)
	}

	o.setProgress(false, "", 0, 0)

	if err != nil {
		log.Printf("Delete error: %v", err)
	}

	o.requestDir(o.state.CurrentPath)
}

// doPaste pastes the clipboard contents to the current directory
func (o *Orchestrator) doPaste() {
	clip := o.state.Clipboard
	if clip == nil {
		return
	}

	// Reset conflict resolution state
	o.conflictResolution = ui.ConflictAsk
	o.conflictAbort = false

	src := clip.Path
	dstDir := o.state.CurrentPath
	dstName := filepath.Base(src)
	dst := filepath.Join(dstDir, dstName)

	srcInfo, err := os.Stat(src)
	if err != nil {
		log.Printf("Paste error: %v", err)
		return
	}

	// Check for conflict
	dstInfo, err := os.Stat(dst)
	if err == nil {
		// Destination exists - need to resolve conflict
		resolution := o.resolveConflict(src, dst, srcInfo, dstInfo)

		switch resolution {
		case ui.ConflictReplaceAll:
			// Replace - delete destination first
			if dstInfo.IsDir() {
				os.RemoveAll(dst)
			} else {
				os.Remove(dst)
			}
		case ui.ConflictKeepBothAll:
			// Keep both - rename destination
			ext := filepath.Ext(dstName)
			base := strings.TrimSuffix(dstName, ext)
			for i := 1; ; i++ {
				dst = filepath.Join(dstDir, base+"_copy"+strconv.Itoa(i)+ext)
				if _, err := os.Stat(dst); os.IsNotExist(err) {
					break
				}
			}
		case ui.ConflictSkipAll:
			// Skip this file
			o.requestDir(o.state.CurrentPath)
			return
		case ui.ConflictAsk:
			// User clicked Stop or dialog was aborted
			o.requestDir(o.state.CurrentPath)
			return
		}
	}

	label := "Copying"
	if clip.Op == ui.ClipCut {
		label = "Moving"
	}

	if srcInfo.IsDir() {
		o.setProgress(true, label+" folder...", 0, 0)
		err = o.copyDir(src, dst, clip.Op == ui.ClipCut)
	} else {
		o.setProgress(true, label+" "+filepath.Base(src), 0, srcInfo.Size())
		err = o.copyFile(src, dst, clip.Op == ui.ClipCut)
	}

	o.setProgress(false, "", 0, 0)

	if err != nil {
		log.Printf("Paste error: %v", err)
	} else if clip.Op == ui.ClipCut {
		o.state.Clipboard = nil
	}

	o.requestDir(o.state.CurrentPath)
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

	// Progress-tracking writer (uses atomic add to avoid races with UI thread)
	pw := &progressWriter{
		w: dstFile,
		onWrite: func(n int64) {
			atomic.AddInt64(&o.state.Progress.Current, n)
			o.window.Invalidate()
		},
	}

	if _, err := io.Copy(pw, srcFile); err != nil {
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
	if err := os.MkdirAll(dst, 0755); err != nil {
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

	// Progress-tracking writer (uses atomic add to avoid races with UI thread)
	pw := &progressWriter{
		w: dstFile,
		onWrite: func(n int64) {
			atomic.AddInt64(&o.state.Progress.Current, n)
			o.window.Invalidate()
		},
	}

	if _, err := io.Copy(pw, srcFile); err != nil {
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
