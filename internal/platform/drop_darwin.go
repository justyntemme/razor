//go:build darwin && !ios

package platform

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit -framework Foundation

#include <stdint.h>

void razor_setupExternalDrop(uintptr_t viewPtr);
*/
import "C"

import (
	"sync"
)

// DropHandler is called when files are dropped from an external source (e.g., Finder)
// The targetDir parameter is the directory that was being hovered when dropped (or empty for current dir)
type DropHandler func(paths []string, targetDir string)

// DragUpdateHandler is called when external drag position changes (for hover highlighting)
type DragUpdateHandler func(x, y int)

// DragEndHandler is called when external drag ends (either dropped or cancelled)
type DragEndHandler func()

var (
	dropHandler       DropHandler
	dragUpdateHandler DragUpdateHandler
	dragEndHandler    DragEndHandler
	dropMu            sync.Mutex
	pendingDrop       []string
	currentDropTarget string // Set by UI based on drag position
)

// SetDropHandler sets the callback for external file drops
func SetDropHandler(handler DropHandler) {
	dropMu.Lock()
	defer dropMu.Unlock()
	dropHandler = handler

	// If there were pending drops before the handler was set, deliver them now
	if len(pendingDrop) > 0 && handler != nil {
		handler(pendingDrop, "")
		pendingDrop = nil
	}
}

// SetDragUpdateHandler sets the callback for external drag position updates
func SetDragUpdateHandler(handler DragUpdateHandler) {
	dropMu.Lock()
	defer dropMu.Unlock()
	dragUpdateHandler = handler
}

// SetDragEndHandler sets the callback for when external drag ends
func SetDragEndHandler(handler DragEndHandler) {
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

// SetupExternalDrop configures the NSView to accept external file drops.
// This should be called when AppKitViewEvent is received with a valid View pointer.
func SetupExternalDrop(viewPtr uintptr) {
	if viewPtr == 0 {
		return
	}
	C.razor_setupExternalDrop(C.uintptr_t(viewPtr))
}

//export razor_onExternalDragStart
func razor_onExternalDragStart() {
	dropMu.Lock()
	currentDropTarget = "" // Reset drop target when drag starts
	dropMu.Unlock()
}

//export razor_onExternalDragUpdate
func razor_onExternalDragUpdate(x, y C.int) {
	dropMu.Lock()
	handler := dragUpdateHandler
	dropMu.Unlock()

	if handler != nil {
		handler(int(x), int(y))
	}
}

//export razor_onExternalDragEnd
func razor_onExternalDragEnd() {
	dropMu.Lock()
	handler := dragEndHandler
	currentDropTarget = ""
	dropMu.Unlock()

	if handler != nil {
		// Run in goroutine to avoid blocking the main thread
		go handler()
	}
}

//export razor_onExternalDrop
func razor_onExternalDrop(pathCStr *C.char) {
	path := C.GoString(pathCStr)

	dropMu.Lock()
	handler := dropHandler
	target := currentDropTarget
	dropMu.Unlock()

	if handler != nil {
		// Run in goroutine to avoid blocking the main thread
		go handler([]string{path}, target)
	} else {
		dropMu.Lock()
		pendingDrop = append(pendingDrop, path)
		dropMu.Unlock()
	}
}
