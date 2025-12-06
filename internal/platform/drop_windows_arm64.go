//go:build windows && arm64

package platform

// Windows ARM64 stub - external drag-and-drop not supported on ARM64
// The windigo library doesn't support ARM64 architecture

// DropHandler is called when files are dropped from an external source
type DropHandler func(paths []string, targetDir string)

// DragUpdateHandler is called when external drag position changes
type DragUpdateHandler func(x, y int)

// DragEndHandler is called when external drag ends
type DragEndHandler func()

// SetDropHandler sets the callback for external file drops (no-op on ARM64)
func SetDropHandler(handler DropHandler) {}

// SetDragUpdateHandler sets the callback for external drag position updates (no-op on ARM64)
func SetDragUpdateHandler(handler DragUpdateHandler) {}

// SetDragEndHandler sets the callback for when external drag ends (no-op on ARM64)
func SetDragEndHandler(handler DragEndHandler) {}

// SetCurrentDropTarget is called by the UI to set the current drop target directory (no-op on ARM64)
func SetCurrentDropTarget(path string) {}

// GetCurrentDropTarget returns the current drop target directory (empty on ARM64)
func GetCurrentDropTarget() string { return "" }

// SetupExternalDrop configures external file drop handling (no-op on ARM64)
func SetupExternalDrop(hwnd uintptr) {}

// CleanupExternalDrop cleans up external drop resources (no-op on ARM64)
func CleanupExternalDrop() {}
