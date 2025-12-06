//go:build !darwin && !windows

package platform

// DropHandler is called when files are dropped from an external source
type DropHandler func(paths []string, targetDir string)

// DragUpdateHandler is called when external drag position changes
type DragUpdateHandler func(x, y int)

// DragEndHandler is called when external drag ends
type DragEndHandler func()

// SetDropHandler sets the callback for external file drops (no-op on this platform)
func SetDropHandler(handler DropHandler) {}

// SetDragUpdateHandler sets the callback for external drag position updates (no-op on this platform)
func SetDragUpdateHandler(handler DragUpdateHandler) {}

// SetDragEndHandler sets the callback for when external drag ends (no-op on this platform)
func SetDragEndHandler(handler DragEndHandler) {}

// SetCurrentDropTarget is called by the UI to set the current drop target directory (no-op on this platform)
func SetCurrentDropTarget(path string) {}

// GetCurrentDropTarget returns the current drop target directory (empty on this platform)
func GetCurrentDropTarget() string { return "" }

// SetupExternalDrop configures external file drop handling (no-op on this platform)
func SetupExternalDrop(viewPtr uintptr) {}

// CleanupExternalDrop cleans up external drop resources (no-op on this platform)
func CleanupExternalDrop() {}
