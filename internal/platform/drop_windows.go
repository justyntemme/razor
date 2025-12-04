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
	dropMu.Lock()
	defer dropMu.Unlock()
	dropHandler = handler

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

// IDropTarget vtable
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
type razorDropTarget struct {
	vtbl     *iDropTargetVtbl
	refs     int32
	hwnd     windows.HWND
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
	globalVtbl = &iDropTargetVtbl{
		QueryInterface: syscall.NewCallback(dropTargetQueryInterface),
		AddRef:         syscall.NewCallback(dropTargetAddRef),
		Release:        syscall.NewCallback(dropTargetRelease),
		DragEnter:      syscall.NewCallback(dropTargetDragEnter),
		DragOver:       syscall.NewCallback(dropTargetDragOver),
		DragLeave:      syscall.NewCallback(dropTargetDragLeave),
		Drop:           syscall.NewCallback(dropTargetDrop),
	}
}

// SetupExternalDrop configures the window to accept external file drops via OLE IDropTarget
func SetupExternalDrop(hwnd uintptr) {
	if hwnd == 0 {
		debug.Log(debug.APP, "SetupExternalDrop: hwnd is 0, skipping")
		return
	}

	debug.Log(debug.APP, "SetupExternalDrop called with hwnd=%d", hwnd)

	// Initialize OLE
	hr, _, _ := procOleInitialize.Call(0)
	if hr != S_OK && hr != S_FALSE {
		debug.Log(debug.APP, "OleInitialize failed: 0x%x", hr)
		return
	}

	// Create our drop target
	dropTarget = &razorDropTarget{
		vtbl:      globalVtbl,
		refs:      1,
		hwnd:      windows.HWND(hwnd),
		oleInited: true,
	}

	// Register for drag-drop
	hr, _, _ = procRegisterDragDrop.Call(
		hwnd,
		uintptr(unsafe.Pointer(dropTarget)),
	)
	if hr != S_OK {
		debug.Log(debug.APP, "RegisterDragDrop failed: 0x%x", hr)
		return
	}

	debug.Log(debug.APP, "SetupExternalDrop: successfully registered IDropTarget")
}

// IDropTarget::QueryInterface
func dropTargetQueryInterface(this uintptr, riid *windows.GUID, ppvObject *uintptr) uintptr {
	if riid == nil || ppvObject == nil {
		return E_UNEXPECTED
	}

	*ppvObject = 0

	if guidEqual(riid, &IID_IUnknown) || guidEqual(riid, &IID_IDropTarget) {
		*ppvObject = this
		dropTargetAddRef(this)
		return S_OK
	}

	return E_NOINTERFACE
}

// IDropTarget::AddRef
func dropTargetAddRef(this uintptr) uintptr {
	dt := (*razorDropTarget)(unsafe.Pointer(this))
	dt.refs++
	return uintptr(dt.refs)
}

// IDropTarget::Release
func dropTargetRelease(this uintptr) uintptr {
	dt := (*razorDropTarget)(unsafe.Pointer(this))
	dt.refs--
	if dt.refs == 0 {
		if dt.oleInited {
			procOleUninitialize.Call()
		}
		return 0
	}
	return uintptr(dt.refs)
}

// IDropTarget::DragEnter
func dropTargetDragEnter(this uintptr, pDataObject uintptr, grfKeyState uint32, ptX, ptY int32, pdwEffect *uint32) uintptr {
	debug.Log(debug.APP, "DragEnter: x=%d, y=%d", ptX, ptY)

	dt := (*razorDropTarget)(unsafe.Pointer(this))

	// Reset drop target when drag starts
	dropMu.Lock()
	currentDropTarget = ""
	dropMu.Unlock()

	// Convert screen to client coordinates and notify handler
	x, y := screenToClient(dt.hwnd, ptX, ptY)

	dropMu.Lock()
	handler := dragUpdateHandler
	dropMu.Unlock()

	if handler != nil {
		handler(x, y)
	}

	// Accept copy operations
	if pdwEffect != nil {
		*pdwEffect = DROPEFFECT_COPY
	}

	return S_OK
}

// IDropTarget::DragOver
func dropTargetDragOver(this uintptr, grfKeyState uint32, ptX, ptY int32, pdwEffect *uint32) uintptr {
	dt := (*razorDropTarget)(unsafe.Pointer(this))

	// Convert screen to client coordinates
	x, y := screenToClient(dt.hwnd, ptX, ptY)

	dropMu.Lock()
	handler := dragUpdateHandler
	dropMu.Unlock()

	if handler != nil {
		handler(x, y)
	}

	if pdwEffect != nil {
		*pdwEffect = DROPEFFECT_COPY
	}

	return S_OK
}

// IDropTarget::DragLeave
func dropTargetDragLeave(this uintptr) uintptr {
	debug.Log(debug.APP, "DragLeave")

	dropMu.Lock()
	handler := dragEndHandler
	currentDropTarget = ""
	dropMu.Unlock()

	if handler != nil {
		go handler()
	}

	return S_OK
}

// IDropTarget::Drop
func dropTargetDrop(this uintptr, pDataObject uintptr, grfKeyState uint32, ptX, ptY int32, pdwEffect *uint32) uintptr {
	debug.Log(debug.APP, "Drop: x=%d, y=%d", ptX, ptY)

	// Get dropped files
	paths := getDroppedFiles(pDataObject)

	debug.Log(debug.APP, "Drop: got %d files", len(paths))

	// End the drag state first
	dropMu.Lock()
	endHandler := dragEndHandler
	dropMu.Unlock()

	if endHandler != nil {
		go endHandler()
	}

	// Then handle the drop
	dropMu.Lock()
	handler := dropHandler
	target := currentDropTarget
	dropMu.Unlock()

	if handler != nil && len(paths) > 0 {
		go handler(paths, target)
	} else if len(paths) > 0 {
		dropMu.Lock()
		pendingDrop = append(pendingDrop, paths...)
		dropMu.Unlock()
	}

	if pdwEffect != nil {
		*pdwEffect = DROPEFFECT_COPY
	}

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
	if pDataObject == 0 {
		return nil
	}

	// Get IDataObject vtable
	vtblPtr := *(*uintptr)(unsafe.Pointer(pDataObject))
	vtbl := (*iDataObjectVtbl)(unsafe.Pointer(vtblPtr))

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
	hr, _, _ := syscall.SyscallN(
		vtbl.GetData,
		pDataObject,
		uintptr(unsafe.Pointer(&formatetc)),
		uintptr(unsafe.Pointer(&stgmedium)),
	)

	if hr != S_OK {
		debug.Log(debug.APP, "GetData failed: 0x%x", hr)
		return nil
	}

	defer procReleaseStgMedium.Call(uintptr(unsafe.Pointer(&stgmedium)))

	hdrop := stgmedium.hGlobal

	// Get file count
	count, _, _ := procDragQueryFileW.Call(hdrop, 0xFFFFFFFF, 0, 0)
	if count == 0 {
		return nil
	}

	debug.Log(debug.APP, "getDroppedFiles: count=%d", count)

	var paths []string
	for i := uintptr(0); i < count; i++ {
		// Get required buffer size
		size, _, _ := procDragQueryFileW.Call(hdrop, i, 0, 0)
		if size == 0 {
			continue
		}

		// Allocate buffer and get the path
		buf := make([]uint16, size+1)
		procDragQueryFileW.Call(hdrop, i, uintptr(unsafe.Pointer(&buf[0])), size+1)

		path := windows.UTF16ToString(buf)
		debug.Log(debug.APP, "getDroppedFiles: path[%d]=%s", i, path)
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
