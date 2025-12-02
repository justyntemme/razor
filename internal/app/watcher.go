package app

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/justyntemme/razor/internal/debug"
)

// DirectoryWatcher watches directories for changes and notifies when refreshes are needed
type DirectoryWatcher struct {
	watcher    *fsnotify.Watcher
	mu         sync.Mutex
	watching   map[string]bool // Currently watched paths
	notify     chan string     // Channel to send changed directory paths
	done       chan struct{}   // Shutdown signal
	debounceMs int             // Debounce interval in milliseconds
}

// NewDirectoryWatcher creates a new directory watcher
func NewDirectoryWatcher(debounceMs int) (*DirectoryWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if debounceMs <= 0 {
		debounceMs = 200 // Default 200ms debounce
	}

	dw := &DirectoryWatcher{
		watcher:    w,
		watching:   make(map[string]bool),
		notify:     make(chan string, 10),
		done:       make(chan struct{}),
		debounceMs: debounceMs,
	}

	go dw.run()
	return dw, nil
}

// run processes filesystem events with debouncing
func (dw *DirectoryWatcher) run() {
	// Debounce: track last event time per directory
	lastEvent := make(map[string]time.Time)
	pending := make(map[string]bool)
	ticker := time.NewTicker(time.Duration(dw.debounceMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-dw.done:
			return

		case event, ok := <-dw.watcher.Events:
			if !ok {
				return
			}

			// We care about creates, deletes, renames, and writes
			if event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) ||
				event.Has(fsnotify.Rename) || event.Has(fsnotify.Write) {
				// fsnotify reports the full path of the changed file
				// We need to find which watched directory contains this file
				changedPath := event.Name
				parentDir := filepath.Dir(changedPath)

				dw.mu.Lock()
				// Check if the parent directory is one we're watching
				if dw.watching[parentDir] {
					lastEvent[parentDir] = time.Now()
					pending[parentDir] = true
					debug.Log(debug.APP, "FSNotify event: %s on %s (parent: %s)", event.Op, changedPath, parentDir)
				} else if dw.watching[changedPath] {
					// The watched directory itself was modified (e.g., permissions changed)
					lastEvent[changedPath] = time.Now()
					pending[changedPath] = true
					debug.Log(debug.APP, "FSNotify event: %s on watched dir %s", event.Op, changedPath)
				}
				dw.mu.Unlock()
			}

		case err, ok := <-dw.watcher.Errors:
			if !ok {
				return
			}
			debug.Log(debug.APP, "FSNotify error: %v", err)

		case <-ticker.C:
			// Check for debounced events ready to fire
			now := time.Now()
			debounce := time.Duration(dw.debounceMs) * time.Millisecond

			for dir, isPending := range pending {
				if isPending {
					if now.Sub(lastEvent[dir]) >= debounce {
						// Debounce period passed, send notification
						select {
						case dw.notify <- dir:
							debug.Log(debug.APP, "Directory change notification: %s", dir)
						default:
							// Channel full, skip
						}
						delete(pending, dir)
						delete(lastEvent, dir)
					}
				}
			}
		}
	}
}

// Watch adds a directory to the watch list
func (dw *DirectoryWatcher) Watch(path string) error {
	dw.mu.Lock()
	defer dw.mu.Unlock()

	if dw.watching[path] {
		return nil // Already watching
	}

	err := dw.watcher.Add(path)
	if err != nil {
		return err
	}

	dw.watching[path] = true
	debug.Log(debug.APP, "Now watching directory: %s", path)
	return nil
}

// Unwatch removes a directory from the watch list
func (dw *DirectoryWatcher) Unwatch(path string) error {
	dw.mu.Lock()
	defer dw.mu.Unlock()

	if !dw.watching[path] {
		return nil // Not watching
	}

	err := dw.watcher.Remove(path)
	if err != nil {
		// Ignore errors when removing - path may already be gone
		debug.Log(debug.APP, "Error unwatching %s: %v", path, err)
	}

	delete(dw.watching, path)
	debug.Log(debug.APP, "Stopped watching directory: %s", path)
	return nil
}

// UnwatchAll removes all directories from the watch list
func (dw *DirectoryWatcher) UnwatchAll() {
	dw.mu.Lock()
	defer dw.mu.Unlock()

	for path := range dw.watching {
		dw.watcher.Remove(path)
	}
	dw.watching = make(map[string]bool)
}

// Notify returns the channel that receives directory change notifications
func (dw *DirectoryWatcher) Notify() <-chan string {
	return dw.notify
}

// Close shuts down the watcher
func (dw *DirectoryWatcher) Close() error {
	close(dw.done)
	return dw.watcher.Close()
}
