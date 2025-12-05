//go:build windows

package platform

// Windows IDropTarget implementation based on rodrigocfd/windigo pattern.
// Key insight: COM uses double-pointer indirection (ppvt **vtbl pattern).
// No CGO required - pure Go syscalls.

import (
	"sync"
	"sync/atomic"
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

	// Pointer cache to prevent GC from collecting COM objects
	ptrCache     = make(map[uintptr]struct{})
	ptrCacheMu   sync.Mutex
)

func ptrCacheAdd(p unsafe.Pointer) {
	ptrCacheMu.Lock()
	ptrCache[uintptr(p)] = struct{}{}
	ptrCacheMu.Unlock()
}

func ptrCacheDelete(p unsafe.Pointer) {
	ptrCacheMu.Lock()
	delete(ptrCache, uintptr(p))
	ptrCacheMu.Unlock()
}

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

// COM/OLE constants
const (
	S_OK            = 0
	S_FALSE         = 1
	E_NOINTERFACE   = 0x80004002
	E_UNEXPECTED    = 0x8000FFFF
	DROPEFFECT_NONE = 0
	DROPEFFECT_COPY = 1
	DROPEFFECT_MOVE = 2
	DROPEFFECT_LINK = 4
	CF_HDROP        = 15
	DVASPECT_CONTENT = 1
	TYMED_HGLOBAL   = 1
)

// GUIDs
var (
	IID_IUnknown    = windows.GUID{0x00000000, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	IID_IDropTarget = windows.GUID{0x00000122, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

// POINT structure (matches Windows POINT)
type POINT struct {
	X int32
	Y int32
}

// FORMATETC structure
type FORMATETC struct {
	cfFormat uint16
	ptd      uintptr
	dwAspect uint32
	lindex   int32
	tymed    uint32
}

// STGMEDIUM structure
type STGMEDIUM struct {
	tymed          uint32
	hGlobal        uintptr
	pUnkForRelease uintptr
}

// _IUnknownVt is the base COM vtable
type _IUnknownVt struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
}

// _IDropTargetVt extends IUnknown vtable with IDropTarget methods
type _IDropTargetVt struct {
	_IUnknownVt
	DragEnter uintptr
	DragOver  uintptr
	DragLeave uintptr
	Drop      uintptr
}

// _IDropTargetImpl is our implementation struct
// Critical: vt must be first field (vtable)
type _IDropTargetImpl struct {
	vt      _IDropTargetVt
	counter uint32
	hwnd    windows.HWND
}

// Global vtable pointers - initialized once
var dropTargetVtPtrs _IDropTargetVt
var dropTargetVtInit bool

// Windows API
var (
	ole32   = windows.NewLazySystemDLL("ole32.dll")
	shell32 = windows.NewLazySystemDLL("shell32.dll")
	user32  = windows.NewLazySystemDLL("user32.dll")

	procOleInitialize    = ole32.NewProc("OleInitialize")
	procOleUninitialize  = ole32.NewProc("OleUninitialize")
	procRegisterDragDrop = ole32.NewProc("RegisterDragDrop")
	procRevokeDragDrop   = ole32.NewProc("RevokeDragDrop")
	procReleaseStgMedium = ole32.NewProc("ReleaseStgMedium")
	procDragQueryFileW   = shell32.NewProc("DragQueryFileW")
	procScreenToClient   = user32.NewProc("ScreenToClient")
)

func initDropTargetVt() {
	if dropTargetVtInit {
		return
	}
	dropTargetVtInit = true

	debug.Log(debug.APP, "[Windows DnD] Initializing vtable callbacks")

	dropTargetVtPtrs = _IDropTargetVt{
		_IUnknownVt: _IUnknownVt{
			QueryInterface: syscall.NewCallback(dropTargetQueryInterface),
			AddRef: syscall.NewCallback(func(p uintptr) uintptr {
				debug.Log(debug.APP, "[Windows DnD] AddRef called, p=%x", p)
				ppImpl := (**_IDropTargetImpl)(unsafe.Pointer(p))
				newCount := atomic.AddUint32(&(*ppImpl).counter, 1)
				debug.Log(debug.APP, "[Windows DnD] AddRef: count now %d", newCount)
				return uintptr(newCount)
			}),
			Release: syscall.NewCallback(func(p uintptr) uintptr {
				debug.Log(debug.APP, "[Windows DnD] Release called, p=%x", p)
				ppImpl := (**_IDropTargetImpl)(unsafe.Pointer(p))
				newCount := atomic.AddUint32(&(*ppImpl).counter, ^uint32(0)) // decrement
				debug.Log(debug.APP, "[Windows DnD] Release: count now %d", newCount)
				if newCount == 0 {
					ptrCacheDelete(unsafe.Pointer(*ppImpl))
					ptrCacheDelete(unsafe.Pointer(ppImpl))
				}
				return uintptr(newCount)
			}),
		},
		DragEnter: syscall.NewCallback(func(p uintptr, vtDataObj uintptr, grfKeyState uint32, pt POINT, pdwEffect *uint32) uintptr {
			debug.Log(debug.APP, "[Windows DnD] DragEnter: p=%x pt=(%d,%d)", p, pt.X, pt.Y)
			if pdwEffect != nil {
				*pdwEffect = DROPEFFECT_COPY
			}

			// Notify handler asynchronously
			go func() {
				dropMu.Lock()
				handler := dragUpdateHandler
				currentDropTarget = ""
				dropMu.Unlock()

				if handler != nil {
					ppImpl := (**_IDropTargetImpl)(unsafe.Pointer(p))
					x, y := screenToClient((*ppImpl).hwnd, pt.X, pt.Y)
					handler(x, y)
				}
			}()

			return S_OK
		}),
		DragOver: syscall.NewCallback(func(p uintptr, grfKeyState uint32, pt POINT, pdwEffect *uint32) uintptr {
			if pdwEffect != nil {
				*pdwEffect = DROPEFFECT_COPY
			}

			go func() {
				dropMu.Lock()
				handler := dragUpdateHandler
				dropMu.Unlock()

				if handler != nil {
					ppImpl := (**_IDropTargetImpl)(unsafe.Pointer(p))
					x, y := screenToClient((*ppImpl).hwnd, pt.X, pt.Y)
					handler(x, y)
				}
			}()

			return S_OK
		}),
		DragLeave: syscall.NewCallback(func(p uintptr) uintptr {
			debug.Log(debug.APP, "[Windows DnD] DragLeave: p=%x", p)
			go func() {
				dropMu.Lock()
				handler := dragEndHandler
				currentDropTarget = ""
				dropMu.Unlock()

				if handler != nil {
					handler()
				}
			}()
			return S_OK
		}),
		Drop: syscall.NewCallback(func(p uintptr, vtDataObj uintptr, grfKeyState uint32, pt POINT, pdwEffect *uint32) uintptr {
			debug.Log(debug.APP, "[Windows DnD] Drop: p=%x vtDataObj=%x pt=(%d,%d)", p, vtDataObj, pt.X, pt.Y)

			// Extract files synchronously before returning
			paths := getDroppedFiles(vtDataObj)
			debug.Log(debug.APP, "[Windows DnD] Drop: extracted %d files", len(paths))

			if pdwEffect != nil {
				*pdwEffect = DROPEFFECT_COPY
			}

			go func() {
				dropMu.Lock()
				endHandler := dragEndHandler
				dropMu.Unlock()
				if endHandler != nil {
					endHandler()
				}

				dropMu.Lock()
				handler := dropHandler
				target := currentDropTarget
				dropMu.Unlock()

				if handler != nil && len(paths) > 0 {
					debug.Log(debug.APP, "[Windows DnD] Drop: calling handler with %d paths", len(paths))
					handler(paths, target)
				} else if len(paths) > 0 {
					dropMu.Lock()
					pendingDrop = append(pendingDrop, paths...)
					dropMu.Unlock()
				}
			}()

			return S_OK
		}),
	}

	debug.Log(debug.APP, "[Windows DnD] vtable initialized: DragEnter=%x Drop=%x",
		dropTargetVtPtrs.DragEnter, dropTargetVtPtrs.Drop)
}

// QueryInterface callback
func dropTargetQueryInterface(p uintptr, riid *windows.GUID, ppvObject *uintptr) uintptr {
	debug.Log(debug.APP, "[Windows DnD] QueryInterface: p=%x", p)
	if riid == nil || ppvObject == nil {
		return E_UNEXPECTED
	}
	*ppvObject = 0

	if guidEqual(riid, &IID_IUnknown) || guidEqual(riid, &IID_IDropTarget) {
		debug.Log(debug.APP, "[Windows DnD] QueryInterface: matched")
		*ppvObject = p
		// Call AddRef
		ppImpl := (**_IDropTargetImpl)(unsafe.Pointer(p))
		atomic.AddUint32(&(*ppImpl).counter, 1)
		return S_OK
	}
	return E_NOINTERFACE
}

// SetupExternalDrop configures the window to accept external file drops
func SetupExternalDrop(hwnd uintptr) {
	debug.Log(debug.APP, "[Windows DnD] SetupExternalDrop called with hwnd=%d (0x%x)", hwnd, hwnd)

	if hwnd == 0 {
		debug.Log(debug.APP, "[Windows DnD] SetupExternalDrop: hwnd is 0, skipping")
		return
	}

	// Initialize vtable
	initDropTargetVt()

	// Initialize OLE
	debug.Log(debug.APP, "[Windows DnD] Calling OleInitialize...")
	hr, _, err := procOleInitialize.Call(0)
	debug.Log(debug.APP, "[Windows DnD] OleInitialize returned: hr=0x%x, err=%v", hr, err)
	if hr != S_OK && hr != S_FALSE {
		debug.Log(debug.APP, "[Windows DnD] OleInitialize FAILED: 0x%x", hr)
		return
	}

	// Create implementation struct (on Go heap)
	pImpl := &_IDropTargetImpl{
		vt:      dropTargetVtPtrs, // copy vtable pointers
		counter: 1,
		hwnd:    windows.HWND(hwnd),
	}
	ptrCacheAdd(unsafe.Pointer(pImpl)) // prevent GC

	// Create pointer-to-pointer (COM expects this indirection)
	ppImpl := &pImpl
	ptrCacheAdd(unsafe.Pointer(ppImpl)) // prevent GC of this too

	debug.Log(debug.APP, "[Windows DnD] Created: pImpl=%p ppImpl=%p", pImpl, ppImpl)

	// Register with Windows - pass ppImpl (pointer to pointer)
	debug.Log(debug.APP, "[Windows DnD] Calling RegisterDragDrop...")
	hr, _, err = procRegisterDragDrop.Call(
		hwnd,
		uintptr(unsafe.Pointer(ppImpl)),
	)
	debug.Log(debug.APP, "[Windows DnD] RegisterDragDrop returned: hr=0x%x, err=%v", hr, err)
	if hr != S_OK {
		debug.Log(debug.APP, "[Windows DnD] RegisterDragDrop FAILED: 0x%x", hr)
		ptrCacheDelete(unsafe.Pointer(pImpl))
		ptrCacheDelete(unsafe.Pointer(ppImpl))
		return
	}

	debug.Log(debug.APP, "[Windows DnD] SUCCESS - IDropTarget registered")
}

// screenToClient converts screen coordinates to client coordinates
func screenToClient(hwnd windows.HWND, screenX, screenY int32) (int, int) {
	pt := POINT{screenX, screenY}
	procScreenToClient.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&pt)))
	return int(pt.X), int(pt.Y)
}

// getDroppedFiles extracts file paths from an IDataObject
func getDroppedFiles(pDataObject uintptr) []string {
	debug.Log(debug.APP, "[Windows DnD] getDroppedFiles: pDataObject=%x", pDataObject)
	if pDataObject == 0 {
		return nil
	}

	// IDataObject vtable
	type iDataObjectVt struct {
		QueryInterface uintptr
		AddRef         uintptr
		Release        uintptr
		GetData        uintptr
	}

	// Get vtable pointer (first field of COM object)
	ppvt := *(**iDataObjectVt)(unsafe.Pointer(pDataObject))
	debug.Log(debug.APP, "[Windows DnD] getDroppedFiles: vtable=%p GetData=%x", ppvt, ppvt.GetData)

	// Request CF_HDROP format
	formatetc := FORMATETC{
		cfFormat: CF_HDROP,
		ptd:      0,
		dwAspect: DVASPECT_CONTENT,
		lindex:   -1,
		tymed:    TYMED_HGLOBAL,
	}
	var stgmedium STGMEDIUM

	hr, _, _ := syscall.SyscallN(
		ppvt.GetData,
		pDataObject,
		uintptr(unsafe.Pointer(&formatetc)),
		uintptr(unsafe.Pointer(&stgmedium)),
	)
	if hr != S_OK {
		debug.Log(debug.APP, "[Windows DnD] getDroppedFiles: GetData failed hr=0x%x", hr)
		return nil
	}
	defer procReleaseStgMedium.Call(uintptr(unsafe.Pointer(&stgmedium)))

	hdrop := stgmedium.hGlobal
	count, _, _ := procDragQueryFileW.Call(hdrop, 0xFFFFFFFF, 0, 0)
	debug.Log(debug.APP, "[Windows DnD] getDroppedFiles: %d files", count)
	if count == 0 {
		return nil
	}

	var paths []string
	for i := uintptr(0); i < count; i++ {
		size, _, _ := procDragQueryFileW.Call(hdrop, i, 0, 0)
		if size == 0 {
			continue
		}
		buf := make([]uint16, size+1)
		procDragQueryFileW.Call(hdrop, i, uintptr(unsafe.Pointer(&buf[0])), size+1)
		path := windows.UTF16ToString(buf)
		debug.Log(debug.APP, "[Windows DnD] getDroppedFiles: path[%d]=%s", i, path)
		paths = append(paths, path)
	}
	return paths
}

// guidEqual compares two GUIDs
func guidEqual(a, b *windows.GUID) bool {
	return a.Data1 == b.Data1 &&
		a.Data2 == b.Data2 &&
		a.Data3 == b.Data3 &&
		a.Data4 == b.Data4
}
