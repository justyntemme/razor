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
type DropHandler func(paths []string)

var (
	dropHandler DropHandler
	dropMu      sync.Mutex
	pendingDrop []string
)

// SetDropHandler sets the callback for external file drops
func SetDropHandler(handler DropHandler) {
	dropMu.Lock()
	defer dropMu.Unlock()
	dropHandler = handler

	// If there were pending drops before the handler was set, deliver them now
	if len(pendingDrop) > 0 && handler != nil {
		handler(pendingDrop)
		pendingDrop = nil
	}
}

// SetupExternalDrop configures the NSView to accept external file drops.
// This should be called when AppKitViewEvent is received with a valid View pointer.
func SetupExternalDrop(viewPtr uintptr) {
	if viewPtr == 0 {
		return
	}
	C.razor_setupExternalDrop(C.uintptr_t(viewPtr))
}

//export razor_onExternalDrop
func razor_onExternalDrop(pathCStr *C.char) {
	path := C.GoString(pathCStr)

	dropMu.Lock()
	defer dropMu.Unlock()

	if dropHandler != nil {
		// Deliver immediately
		dropHandler([]string{path})
	} else {
		// Queue for later delivery
		pendingDrop = append(pendingDrop, path)
	}
}
