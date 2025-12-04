//go:build windows

package platform

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
	dropTarget        *razorDropTarget
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

// COM/OLE constants
const (
	S_OK           = 0
	S_FALSE        = 1
	E_NOINTERFACE  = 0x80004002
	E_UNEXPECTED   = 0x8000FFFF
	DROPEFFECT_NONE = 0
	DROPEFFECT_COPY = 1
	DROPEFFECT_MOVE = 2
	DROPEFFECT_LINK = 4
	CF_HDROP       = 15
	DVASPECT_CONTENT = 1
	TYMED_HGLOBAL  = 1
)

// GUIDs
var (
	IID_IUnknown    = windows.GUID{0x00000000, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	IID_IDropTarget = windows.GUID{0x00000122, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

// POINTL structure
type POINTL struct {
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

// IDropTarget vtable - must match COM interface exactly
type iDropTargetVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	DragEnter      uintptr
	DragOver       uintptr
	DragLeave      uintptr
	Drop           uintptr
}

// IDataObject vtable (partial - only what we need)
type iDataObjectVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	GetData        uintptr
	// ... other methods we don't need
}

// razorDropTarget implements IDropTarget
// COM object layout: first field is pointer to vtable
// Windows calls: obj->lpVtbl->Method(obj, ...)
// Which means: (*(*vtbl)(obj))[methodIndex](obj, ...)
type razorDropTarget struct {
	lpVtbl    *iDropTargetVtbl // Pointer to vtable - MUST be first field and be a real pointer
	refs      int32
	_padding  uint32           // Ensure 8-byte alignment on 64-bit
	hwnd      windows.HWND
	oleInited bool
}

var (
	ole32    = windows.NewLazySystemDLL("ole32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	user32   = windows.NewLazySystemDLL("user32.dll")

	procOleInitialize      = ole32.NewProc("OleInitialize")
	procOleUninitialize    = ole32.NewProc("OleUninitialize")
	procRegisterDragDrop   = ole32.NewProc("RegisterDragDrop")
	procRevokeDragDrop     = ole32.NewProc("RevokeDragDrop")
	procReleaseStgMedium   = ole32.NewProc("ReleaseStgMedium")
	procDragQueryFileW     = shell32.NewProc("DragQueryFileW")
	procScreenToClient     = user32.NewProc("ScreenToClient")
	procGetDpiForWindow    = user32.NewProc("GetDpiForWindow")
)

// Global vtable instance (must stay alive for duration of program)
var globalVtbl *iDropTargetVtbl

func init() {
	debug.Log(debug.APP, "[Windows DnD] init() called - creating vtable callbacks")
	globalVtbl = &iDropTargetVtbl{
		QueryInterface: syscall.NewCallback(dropTargetQueryInterface),
		AddRef:         syscall.NewCallback(dropTargetAddRef),
		Release:        syscall.NewCallback(dropTargetRelease),
		DragEnter:      syscall.NewCallback(dropTargetDragEnter),
		DragOver:       syscall.NewCallback(dropTargetDragOver),
		DragLeave:      syscall.NewCallback(dropTargetDragLeave),
		Drop:           syscall.NewCallback(dropTargetDrop),
	}
	debug.Log(debug.APP, "[Windows DnD] vtable created: QueryInterface=%x, DragEnter=%x, Drop=%x",
		globalVtbl.QueryInterface, globalVtbl.DragEnter, globalVtbl.Drop)
}

// SetupExternalDrop configures the window to accept external file drops via OLE IDropTarget
func SetupExternalDrop(hwnd uintptr) {
	debug.Log(debug.APP, "[Windows DnD] SetupExternalDrop called with hwnd=%d (0x%x)", hwnd, hwnd)

	if hwnd == 0 {
		debug.Log(debug.APP, "[Windows DnD] SetupExternalDrop: hwnd is 0, skipping")
		return
	}

	// Initialize OLE
	debug.Log(debug.APP, "[Windows DnD] Calling OleInitialize...")
	hr, _, err := procOleInitialize.Call(0)
	debug.Log(debug.APP, "[Windows DnD] OleInitialize returned: hr=0x%x, err=%v", hr, err)
	if hr != S_OK && hr != S_FALSE {
		debug.Log(debug.APP, "[Windows DnD] OleInitialize FAILED: 0x%x", hr)
		return
	}
	debug.Log(debug.APP, "[Windows DnD] OleInitialize succeeded (hr=0x%x)", hr)

	// Create our drop target
	debug.Log(debug.APP, "[Windows DnD] Creating razorDropTarget struct...")
	debug.Log(debug.APP, "[Windows DnD] globalVtbl at %p, DragEnter callback=0x%x", globalVtbl, globalVtbl.DragEnter)
	dropTarget = &razorDropTarget{
		lpVtbl:    globalVtbl,
		refs:      1,
		hwnd:      windows.HWND(hwnd),
		oleInited: true,
	}
	debug.Log(debug.APP, "[Windows DnD] razorDropTarget created at %p, lpVtbl=%p", dropTarget, dropTarget.lpVtbl)

	// Register for drag-drop
	debug.Log(debug.APP, "[Windows DnD] Calling RegisterDragDrop(hwnd=0x%x, dropTarget=%p)...", hwnd, dropTarget)
	hr, _, err = procRegisterDragDrop.Call(
		hwnd,
		uintptr(unsafe.Pointer(dropTarget)),
	)
	debug.Log(debug.APP, "[Windows DnD] RegisterDragDrop returned: hr=0x%x, err=%v", hr, err)
	if hr != S_OK {
		debug.Log(debug.APP, "[Windows DnD] RegisterDragDrop FAILED: 0x%x", hr)
		// Common errors:
		// 0x80040100 = DRAGDROP_E_NOTREGISTERED
		// 0x80040101 = DRAGDROP_E_ALREADYREGISTERED
		// 0x800401F0 = CO_E_NOTINITIALIZED
		return
	}

	debug.Log(debug.APP, "[Windows DnD] SUCCESS - IDropTarget registered for hwnd=0x%x", hwnd)
}

// IDropTarget::QueryInterface
func dropTargetQueryInterface(this uintptr, riid *windows.GUID, ppvObject *uintptr) (ret uintptr) {
	defer func() {
		if r := recover(); r != nil {
			debug.Log(debug.APP, "[Windows DnD] PANIC in QueryInterface: %v", r)
			ret = E_UNEXPECTED
		}
	}()

	debug.Log(debug.APP, "[Windows DnD] QueryInterface called, this=%x", this)
	if riid == nil || ppvObject == nil {
		debug.Log(debug.APP, "[Windows DnD] QueryInterface: nil riid or ppvObject")
		return E_UNEXPECTED
	}

	*ppvObject = 0

	if guidEqual(riid, &IID_IUnknown) || guidEqual(riid, &IID_IDropTarget) {
		debug.Log(debug.APP, "[Windows DnD] QueryInterface: matched IUnknown or IDropTarget")
		*ppvObject = this
		dropTargetAddRef(this)
		return S_OK
	}

	debug.Log(debug.APP, "[Windows DnD] QueryInterface: no interface match")
	return E_NOINTERFACE
}

// IDropTarget::AddRef
func dropTargetAddRef(this uintptr) (ret uintptr) {
	defer func() {
		if r := recover(); r != nil {
			debug.Log(debug.APP, "[Windows DnD] PANIC in AddRef: %v", r)
			ret = 1
		}
	}()

	dt := (*razorDropTarget)(unsafe.Pointer(this))
	dt.refs++
	debug.Log(debug.APP, "[Windows DnD] AddRef: refs now %d", dt.refs)
	return uintptr(dt.refs)
}

// IDropTarget::Release
func dropTargetRelease(this uintptr) (ret uintptr) {
	defer func() {
		if r := recover(); r != nil {
			debug.Log(debug.APP, "[Windows DnD] PANIC in Release: %v", r)
			ret = 0
		}
	}()

	dt := (*razorDropTarget)(unsafe.Pointer(this))
	dt.refs--
	debug.Log(debug.APP, "[Windows DnD] Release: refs now %d", dt.refs)
	if dt.refs == 0 {
		debug.Log(debug.APP, "[Windows DnD] Release: refs=0, calling OleUninitialize")
		if dt.oleInited {
			procOleUninitialize.Call()
		}
		return 0
	}
	return uintptr(dt.refs)
}

// IDropTarget::DragEnter
// Signature: HRESULT DragEnter(IDataObject *pDataObject, DWORD grfKeyState, POINTL pt, DWORD *pdwEffect)
// All parameters as uintptr for syscall.NewCallback compatibility
// CRITICAL: This callback runs on Windows' OLE thread - must return IMMEDIATELY or it freezes drag-drop system-wide
func dropTargetDragEnter(this, pDataObject, grfKeyState, ptLo, ptHi, pdwEffect uintptr) uintptr {
	// Minimal implementation - just accept the drop and return immediately
	if pdwEffect != 0 {
		*(*uint32)(unsafe.Pointer(pdwEffect)) = DROPEFFECT_COPY
	}

	// Notify Go code asynchronously
	go func() {
		debug.Log(debug.APP, "[Windows DnD] DragEnter (async): this=%x", this)
		dropMu.Lock()
		currentDropTarget = ""
		handler := dragUpdateHandler
		dropMu.Unlock()

		if handler != nil {
			dt := (*razorDropTarget)(unsafe.Pointer(this))
			x, y := screenToClient(dt.hwnd, int32(ptLo), int32(ptHi))
			handler(x, y)
		}
	}()

	return S_OK
}

// IDropTarget::DragOver
// Signature: HRESULT DragOver(DWORD grfKeyState, POINTL pt, DWORD *pdwEffect)
// All parameters as uintptr for syscall.NewCallback compatibility
// CRITICAL: Must return immediately - runs on OLE thread
func dropTargetDragOver(this, grfKeyState, ptLo, ptHi, pdwEffect uintptr) uintptr {
	// Minimal implementation - just accept and return
	if pdwEffect != 0 {
		*(*uint32)(unsafe.Pointer(pdwEffect)) = DROPEFFECT_COPY
	}

	// Notify Go code asynchronously (but not too often - DragOver is called frequently)
	go func() {
		dropMu.Lock()
		handler := dragUpdateHandler
		dropMu.Unlock()

		if handler != nil {
			dt := (*razorDropTarget)(unsafe.Pointer(this))
			x, y := screenToClient(dt.hwnd, int32(ptLo), int32(ptHi))
			handler(x, y)
		}
	}()

	return S_OK
}

// IDropTarget::DragLeave
// Signature: HRESULT DragLeave(void)
// CRITICAL: Must return immediately - runs on OLE thread
func dropTargetDragLeave(this uintptr) uintptr {
	go func() {
		debug.Log(debug.APP, "[Windows DnD] DragLeave (async)")
		dropMu.Lock()
		handler := dragEndHandler
		currentDropTarget = ""
		dropMu.Unlock()

		if handler != nil {
			handler()
		}
	}()

	return S_OK
}

// IDropTarget::Drop
// Signature: HRESULT Drop(IDataObject *pDataObject, DWORD grfKeyState, POINTL pt, DWORD *pdwEffect)
// All parameters as uintptr for syscall.NewCallback compatibility
func dropTargetDrop(this, pDataObject, grfKeyState, ptLo, ptHi, pdwEffect uintptr) uintptr {
	debug.Log(debug.APP, "[Windows DnD] Drop: this=%x, pDataObject=%x", this, pDataObject)

	// IMPORTANT: Extract file paths SYNCHRONOUSLY before returning
	// The pDataObject pointer becomes invalid after we return!
	paths := getDroppedFiles(pDataObject)
	debug.Log(debug.APP, "[Windows DnD] Drop: extracted %d files", len(paths))

	// Set effect
	if pdwEffect != 0 {
		*(*uint32)(unsafe.Pointer(pdwEffect)) = DROPEFFECT_COPY
	}

	// Process the drop asynchronously
	go func() {
		debug.Log(debug.APP, "[Windows DnD] Drop (async): processing %d files: %v", len(paths), paths)

		// End the drag state first
		dropMu.Lock()
		endHandler := dragEndHandler
		dropMu.Unlock()

		if endHandler != nil {
			endHandler()
		}

		// Then handle the drop
		dropMu.Lock()
		handler := dropHandler
		target := currentDropTarget
		dropMu.Unlock()

		if handler != nil && len(paths) > 0 {
			debug.Log(debug.APP, "[Windows DnD] Drop: calling dropHandler with %d paths to target=%s", len(paths), target)
			handler(paths, target)
		} else if len(paths) > 0 {
			debug.Log(debug.APP, "[Windows DnD] Drop: no handler, storing %d paths as pending", len(paths))
			dropMu.Lock()
			pendingDrop = append(pendingDrop, paths...)
			dropMu.Unlock()
		}
	}()

	debug.Log(debug.APP, "[Windows DnD] Drop: returning S_OK")
	return S_OK
}

// screenToClient converts screen coordinates to client coordinates with DPI scaling
func screenToClient(hwnd windows.HWND, screenX, screenY int32) (int, int) {
	pt := struct{ X, Y int32 }{screenX, screenY}
	procScreenToClient.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&pt)))

	// Get DPI for proper scaling
	dpi, _, _ := procGetDpiForWindow.Call(uintptr(hwnd))
	if dpi == 0 {
		dpi = 96 // Default DPI
	}

	// Gio uses physical pixels, so we don't need to scale
	// The coordinates from ScreenToClient are already in physical pixels
	return int(pt.X), int(pt.Y)
}

// getDroppedFiles extracts file paths from an IDataObject
func getDroppedFiles(pDataObject uintptr) []string {
	debug.Log(debug.APP, "[Windows DnD] getDroppedFiles called with pDataObject=%x", pDataObject)
	if pDataObject == 0 {
		debug.Log(debug.APP, "[Windows DnD] getDroppedFiles: pDataObject is nil!")
		return nil
	}

	// Get IDataObject vtable
	vtblPtr := *(*uintptr)(unsafe.Pointer(pDataObject))
	vtbl := (*iDataObjectVtbl)(unsafe.Pointer(vtblPtr))
	debug.Log(debug.APP, "[Windows DnD] getDroppedFiles: vtblPtr=%x, GetData=%x", vtblPtr, vtbl.GetData)

	// Prepare FORMATETC for CF_HDROP
	formatetc := FORMATETC{
		cfFormat: CF_HDROP,
		ptd:      0,
		dwAspect: DVASPECT_CONTENT,
		lindex:   -1,
		tymed:    TYMED_HGLOBAL,
	}

	var stgmedium STGMEDIUM

	// Call GetData
	debug.Log(debug.APP, "[Windows DnD] getDroppedFiles: calling IDataObject::GetData...")
	hr, _, _ := syscall.SyscallN(
		vtbl.GetData,
		pDataObject,
		uintptr(unsafe.Pointer(&formatetc)),
		uintptr(unsafe.Pointer(&stgmedium)),
	)

	if hr != S_OK {
		debug.Log(debug.APP, "[Windows DnD] getDroppedFiles: GetData FAILED: hr=0x%x", hr)
		return nil
	}
	debug.Log(debug.APP, "[Windows DnD] getDroppedFiles: GetData succeeded, stgmedium.hGlobal=%x", stgmedium.hGlobal)

	defer procReleaseStgMedium.Call(uintptr(unsafe.Pointer(&stgmedium)))

	hdrop := stgmedium.hGlobal

	// Get file count
	count, _, _ := procDragQueryFileW.Call(hdrop, 0xFFFFFFFF, 0, 0)
	debug.Log(debug.APP, "[Windows DnD] getDroppedFiles: DragQueryFileW count=%d", count)
	if count == 0 {
		debug.Log(debug.APP, "[Windows DnD] getDroppedFiles: no files!")
		return nil
	}

	var paths []string
	for i := uintptr(0); i < count; i++ {
		// Get required buffer size
		size, _, _ := procDragQueryFileW.Call(hdrop, i, 0, 0)
		debug.Log(debug.APP, "[Windows DnD] getDroppedFiles: file[%d] size=%d", i, size)
		if size == 0 {
			continue
		}

		// Allocate buffer and get the path
		buf := make([]uint16, size+1)
		procDragQueryFileW.Call(hdrop, i, uintptr(unsafe.Pointer(&buf[0])), size+1)

		path := windows.UTF16ToString(buf)
		debug.Log(debug.APP, "[Windows DnD] getDroppedFiles: path[%d]=%s", i, path)
		paths = append(paths, path)
	}

	debug.Log(debug.APP, "[Windows DnD] getDroppedFiles: returning %d paths", len(paths))
	return paths
}

// guidEqual compares two GUIDs
func guidEqual(a, b *windows.GUID) bool {
	return a.Data1 == b.Data1 &&
		a.Data2 == b.Data2 &&
		a.Data3 == b.Data3 &&
		a.Data4 == b.Data4
}
