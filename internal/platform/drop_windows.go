//go:build windows && amd64

package platform

// Windows drag-and-drop implementation using IDropTarget via windigo library.
// This provides full OLE drag-and-drop with hover highlighting support.

import (
	"runtime"
	"sync"

	"github.com/justyntemme/razor/internal/debug"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
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

	// OLE state
	oleInitialized bool
	dropTargetHwnd win.HWND
	releaser       *win.OleReleaser
	dropTarget     *win.IDropTarget

	// Track if we can accept the current drag
	canAcceptCurrentDrag bool
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

// SetupExternalDrop configures the window to accept external file drops using IDropTarget
func SetupExternalDrop(hwnd uintptr) {
	debug.Log(debug.APP, "[Windows DnD] SetupExternalDrop called with hwnd=0x%x", hwnd)

	// Must be called from the main thread
	runtime.LockOSThread()

	// Initialize OLE if not already done
	if !oleInitialized {
		if err := win.OleInitialize(); err != nil {
			debug.Log(debug.APP, "[Windows DnD] OleInitialize failed: %v", err)
			return
		}
		oleInitialized = true
		debug.Log(debug.APP, "[Windows DnD] OleInitialize succeeded")
	}

	dropTargetHwnd = win.HWND(hwnd)

	// Create releaser for COM objects
	releaser = win.NewOleReleaser()

	// Create IDropTarget implementation
	dropTarget = win.NewIDropTargetImpl(releaser)

	// Set up callbacks
	dropTarget.DragEnter(onDragEnter)
	dropTarget.DragOver(onDragOver)
	dropTarget.DragLeave(onDragLeave)
	dropTarget.Drop(onDrop)

	// Register the drop target with Windows
	if err := dropTargetHwnd.RegisterDragDrop(dropTarget); err != nil {
		debug.Log(debug.APP, "[Windows DnD] RegisterDragDrop failed: %v", err)
		releaser.Release()
		releaser = nil
		dropTarget = nil
		return
	}

	debug.Log(debug.APP, "[Windows DnD] RegisterDragDrop succeeded for hwnd=0x%x", hwnd)
}

// CleanupExternalDrop should be called before window destruction
func CleanupExternalDrop() {
	if dropTargetHwnd != 0 {
		debug.Log(debug.APP, "[Windows DnD] RevokeDragDrop for hwnd=0x%x", dropTargetHwnd)
		dropTargetHwnd.RevokeDragDrop()
		dropTargetHwnd = 0
	}

	if releaser != nil {
		releaser.Release()
		releaser = nil
	}

	if oleInitialized {
		win.OleUninitialize()
		oleInitialized = false
	}
}

// onDragEnter is called when a drag operation enters the window
func onDragEnter(dataObj *win.IDataObject, keyState co.MK, pt win.POINT, effect *co.DROPEFFECT) co.HRESULT {
	debug.Log(debug.APP, "[Windows DnD] DragEnter at (%d, %d), keyState=0x%x", pt.X, pt.Y, keyState)

	// Check if we can accept CF_HDROP
	canAcceptCurrentDrag = canAcceptDrop(dataObj)

	if canAcceptCurrentDrag {
		*effect = co.DROPEFFECT_COPY
		debug.Log(debug.APP, "[Windows DnD] DragEnter: accepting drop")
	} else {
		*effect = co.DROPEFFECT_NONE
		debug.Log(debug.APP, "[Windows DnD] DragEnter: rejecting drop (no CF_HDROP)")
	}

	// Notify update handler
	dropMu.Lock()
	handler := dragUpdateHandler
	dropMu.Unlock()
	if handler != nil {
		handler(int(pt.X), int(pt.Y))
	}

	return co.HRESULT_S_OK
}

// onDragOver is called continuously as the drag moves over the window
func onDragOver(keyState co.MK, pt win.POINT, effect *co.DROPEFFECT) co.HRESULT {
	// Use the cached acceptance state from DragEnter
	if canAcceptCurrentDrag {
		*effect = co.DROPEFFECT_COPY
	} else {
		*effect = co.DROPEFFECT_NONE
	}

	// Notify update handler
	dropMu.Lock()
	handler := dragUpdateHandler
	dropMu.Unlock()
	if handler != nil {
		handler(int(pt.X), int(pt.Y))
	}

	return co.HRESULT_S_OK
}

// onDragLeave is called when the drag leaves the window
func onDragLeave() co.HRESULT {
	debug.Log(debug.APP, "[Windows DnD] DragLeave")

	canAcceptCurrentDrag = false

	// Notify end handler
	dropMu.Lock()
	handler := dragEndHandler
	dropMu.Unlock()
	if handler != nil {
		handler()
	}

	return co.HRESULT_S_OK
}

// onDrop is called when files are dropped
func onDrop(dataObj *win.IDataObject, keyState co.MK, pt win.POINT, effect *co.DROPEFFECT) co.HRESULT {
	debug.Log(debug.APP, "[Windows DnD] Drop at (%d, %d)", pt.X, pt.Y)

	paths := extractDroppedFiles(dataObj)
	debug.Log(debug.APP, "[Windows DnD] Drop: extracted %d files", len(paths))

	if len(paths) > 0 {
		*effect = co.DROPEFFECT_COPY

		dropMu.Lock()
		handler := dropHandler
		target := currentDropTarget
		dropMu.Unlock()

		if handler != nil {
			handler(paths, target)
		} else {
			// Queue for later delivery
			dropMu.Lock()
			pendingDrop = append(pendingDrop, paths...)
			dropMu.Unlock()
		}
	} else {
		*effect = co.DROPEFFECT_NONE
	}

	canAcceptCurrentDrag = false

	// Notify end handler
	dropMu.Lock()
	endHandler := dragEndHandler
	dropMu.Unlock()
	if endHandler != nil {
		endHandler()
	}

	return co.HRESULT_S_OK
}

// canAcceptDrop checks if the data object contains CF_HDROP format
func canAcceptDrop(dataObj *win.IDataObject) bool {
	formatetc := win.FORMATETC{
		CfFormat: co.CF_HDROP,
		Aspect:   co.DVASPECT_CONTENT,
		Lindex:   -1,
		Tymed:    co.TYMED_HGLOBAL,
	}

	err := dataObj.QueryGetData(&formatetc)
	return err == nil
}

// extractDroppedFiles gets the file paths from the data object
func extractDroppedFiles(dataObj *win.IDataObject) []string {
	formatetc := win.FORMATETC{
		CfFormat: co.CF_HDROP,
		Aspect:   co.DVASPECT_CONTENT,
		Lindex:   -1,
		Tymed:    co.TYMED_HGLOBAL,
	}

	stgmedium, err := dataObj.GetData(&formatetc)
	if err != nil {
		debug.Log(debug.APP, "[Windows DnD] GetData failed: %v", err)
		return nil
	}
	defer win.ReleaseStgMedium(&stgmedium)

	hGlobal, ok := stgmedium.HGlobal()
	if !ok {
		debug.Log(debug.APP, "[Windows DnD] Failed to get HGlobal from STGMEDIUM")
		return nil
	}

	// Lock the global memory to get HDROP
	hMem, err := hGlobal.GlobalLock()
	if err != nil {
		debug.Log(debug.APP, "[Windows DnD] GlobalLock failed: %v", err)
		return nil
	}
	defer hGlobal.GlobalUnlock()

	// Convert to HDROP and query files
	// Note: Do NOT call DragFinish - ReleaseStgMedium handles cleanup
	hdrop := win.HDROP(hMem)
	paths, err := hdrop.DragQueryFile()
	if err != nil {
		debug.Log(debug.APP, "[Windows DnD] DragQueryFile failed: %v", err)
		return nil
	}

	debug.Log(debug.APP, "[Windows DnD] DragQueryFile returned %d paths", len(paths))
	for i, p := range paths {
		debug.Log(debug.APP, "[Windows DnD]   [%d] %s", i, p)
	}

	return paths
}
