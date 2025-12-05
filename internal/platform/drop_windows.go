//go:build windows

package platform

// Windows drag-and-drop implementation using WM_DROPFILES.
// This uses DragAcceptFiles + window subclassing to receive dropped files.
// Unlike IDropTarget, this doesn't require CGO or external thread callbacks.

import (
	"sync"

	"github.com/justyntemme/razor/internal/debug"
	"golang.org/x/sys/windows"
)

// DropHandler is called when files are dropped from an external source
type DropHandler func(paths []string, targetDir string)

// DragUpdateHandler is called when external drag position changes
type DragUpdateHandler func(x, y int)

// DragEndHandler is called when external drag ends
type DragEndHandler func()

var (
	dropHandler       DropHandler
	dragUpdateHandler DragUpdateHandler
	dragEndHandler    DragEndHandler
	dropMu            sync.Mutex
	pendingDrop       []string
	currentDropTarget string

)

// SetDropHandler sets the callback for external file drops
func SetDropHandler(handler DropHandler) {
	debug.Log(debug.APP, "[Windows DnD] SetDropHandler called, handler=%v", handler != nil)
	dropMu.Lock()
	defer dropMu.Unlock()
	dropHandler = handler

	if len(pendingDrop) > 0 && handler != nil {
		debug.Log(debug.APP, "[Windows DnD] SetDropHandler: delivering %d pending drops", len(pendingDrop))
		handler(pendingDrop, "")
		pendingDrop = nil
	}
}

// SetDragUpdateHandler sets the callback for external drag position updates
func SetDragUpdateHandler(handler DragUpdateHandler) {
	debug.Log(debug.APP, "[Windows DnD] SetDragUpdateHandler called, handler=%v", handler != nil)
	dropMu.Lock()
	defer dropMu.Unlock()
	dragUpdateHandler = handler
}

// SetDragEndHandler sets the callback for when external drag ends
func SetDragEndHandler(handler DragEndHandler) {
	debug.Log(debug.APP, "[Windows DnD] SetDragEndHandler called, handler=%v", handler != nil)
	dropMu.Lock()
	defer dropMu.Unlock()
	dragEndHandler = handler
}

// SetCurrentDropTarget is called by the UI to set the current drop target directory
func SetCurrentDropTarget(path string) {
	dropMu.Lock()
	defer dropMu.Unlock()
	currentDropTarget = path
}

// GetCurrentDropTarget returns the current drop target directory
func GetCurrentDropTarget() string {
	dropMu.Lock()
	defer dropMu.Unlock()
	return currentDropTarget
}

// Windows constants for WM_DROPFILES
const (
	WM_DROPFILES = 0x0233
)

// Windows API
var (
	shell32 = windows.NewLazySystemDLL("shell32.dll")

	procDragAcceptFiles = shell32.NewProc("DragAcceptFiles")
)



// SetupExternalDrop configures the window to accept external file drops
// Note: On Windows, WM_DROPFILES handling requires either CGO or a custom message loop.
// For now, this is a no-op - actual drop handling is not implemented.
func SetupExternalDrop(hwnd uintptr) {
	debug.Log(debug.APP, "[Windows DnD] SetupExternalDrop called with hwnd=0x%x (disabled)", hwnd)
	// Disabled - causes crash. Need to investigate why.
}

