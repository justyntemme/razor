//go:build windows

package platform

// Windows drag-and-drop implementation using WM_DROPFILES.
// This uses DragAcceptFiles + window subclassing to receive dropped files.
// Unlike IDropTarget, this doesn't require CGO or external thread callbacks.

import (
	"sync"
	"syscall"
	"unsafe"

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

	// Store original window proc for subclassing
	originalWndProc uintptr
	subclassHwnd    uintptr
	subclassCallback uintptr // prevent GC of callback
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

// GWLP_WNDPROC is -4, but needs proper conversion for uintptr
var gwlpWndProc = ^uintptr(4 - 1) // -4 as uintptr

// Windows API
var (
	shell32 = windows.NewLazySystemDLL("shell32.dll")
	user32  = windows.NewLazySystemDLL("user32.dll")

	procDragAcceptFiles  = shell32.NewProc("DragAcceptFiles")
	procDragQueryFileW   = shell32.NewProc("DragQueryFileW")
	procDragQueryPoint   = shell32.NewProc("DragQueryPoint")
	procDragFinish       = shell32.NewProc("DragFinish")
	procSetWindowLongPtr = user32.NewProc("SetWindowLongPtrW")
	procCallWindowProc   = user32.NewProc("CallWindowProcW")
)


// dropSubclassProc handles WM_DROPFILES messages
func dropSubclassProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	if msg == WM_DROPFILES {
		debug.Log(debug.APP, "[Windows DnD] WM_DROPFILES received! wParam=0x%x", wParam)
		handleDropFiles(wParam)
		return 0
	}

	// Call original window procedure for all other messages
	ret, _, _ := procCallWindowProc.Call(originalWndProc, hwnd, uintptr(msg), wParam, lParam)
	return ret
}

// handleDropFiles extracts file paths from HDROP and calls the drop handler
func handleDropFiles(hDrop uintptr) {
	// Get number of files
	count, _, _ := procDragQueryFileW.Call(hDrop, 0xFFFFFFFF, 0, 0)
	debug.Log(debug.APP, "[Windows DnD] Drop contains %d files", count)

	if count == 0 {
		procDragFinish.Call(hDrop)
		return
	}

	var paths []string
	for i := uintptr(0); i < count; i++ {
		// Get required buffer size
		size, _, _ := procDragQueryFileW.Call(hDrop, i, 0, 0)
		if size == 0 {
			continue
		}

		// Allocate buffer and get file path
		buf := make([]uint16, size+1)
		procDragQueryFileW.Call(hDrop, i, uintptr(unsafe.Pointer(&buf[0])), size+1)
		path := windows.UTF16ToString(buf)
		debug.Log(debug.APP, "[Windows DnD] File[%d]: %s", i, path)
		paths = append(paths, path)
	}

	// Release HDROP
	procDragFinish.Call(hDrop)

	// Deliver to handler
	if len(paths) > 0 {
		dropMu.Lock()
		handler := dropHandler
		target := currentDropTarget
		dropMu.Unlock()

		if handler != nil {
			debug.Log(debug.APP, "[Windows DnD] Calling handler with %d files", len(paths))
			handler(paths, target)
		} else {
			debug.Log(debug.APP, "[Windows DnD] No handler, queuing %d files", len(paths))
			dropMu.Lock()
			pendingDrop = append(pendingDrop, paths...)
			dropMu.Unlock()
		}
	}
}

// SetupExternalDrop configures the window to accept external file drops
func SetupExternalDrop(hwnd uintptr) {
	debug.Log(debug.APP, "[Windows DnD] SetupExternalDrop called with hwnd=0x%x", hwnd)

	if hwnd == 0 {
		debug.Log(debug.APP, "[Windows DnD] SetupExternalDrop: hwnd is 0, skipping")
		return
	}

	// Enable drag-and-drop for this window
	procDragAcceptFiles.Call(hwnd, 1)
	debug.Log(debug.APP, "[Windows DnD] DragAcceptFiles called")

	// Subclass the window to intercept WM_DROPFILES
	subclassHwnd = hwnd
	subclassCallback = syscall.NewCallback(dropSubclassProc) // store to prevent GC
	oldProc, _, err := procSetWindowLongPtr.Call(hwnd, gwlpWndProc, subclassCallback)
	if oldProc == 0 {
		debug.Log(debug.APP, "[Windows DnD] SetWindowLongPtr failed: %v", err)
		return
	}
	originalWndProc = oldProc
	debug.Log(debug.APP, "[Windows DnD] Window subclassed: oldProc=0x%x newProc=0x%x", oldProc, subclassCallback)
}

