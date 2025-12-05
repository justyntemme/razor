//go:build windows

package platform

// Windows IDropTarget implementation using GlobalAlloc for COM-compatible memory.
// Based on wailsapp/go-webview2 combridge pattern.
// Key insight: COM vtables must be in Windows-allocated memory, not Go heap.

import (
	"fmt"
	"os"
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

	// Global drop target - prevent GC
	globalDropTarget *dropTargetImpl
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
	S_OK             = 0
	S_FALSE          = 1
	E_NOINTERFACE    = 0x80004002
	E_UNEXPECTED     = 0x8000FFFF
	DROPEFFECT_NONE  = 0
	DROPEFFECT_COPY  = 1
	DROPEFFECT_MOVE  = 2
	DROPEFFECT_LINK  = 4
	CF_HDROP         = 15
	DVASPECT_CONTENT = 1
	TYMED_HGLOBAL    = 1
	GMEM_FIXED       = 0x0000
	GMEM_ZEROINIT    = 0x0040
)

// GUIDs
var (
	IID_IUnknown    = windows.GUID{0x00000000, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	IID_IDropTarget = windows.GUID{0x00000122, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

// POINTL structure for drag coordinates (must match Windows POINTL exactly)
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

// Windows API
var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	ole32    = windows.NewLazySystemDLL("ole32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	user32   = windows.NewLazySystemDLL("user32.dll")

	procGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	procGlobalFree       = kernel32.NewProc("GlobalFree")
	procOleInitialize    = ole32.NewProc("OleInitialize")
	procRegisterDragDrop = ole32.NewProc("RegisterDragDrop")
	procRevokeDragDrop   = ole32.NewProc("RevokeDragDrop")
	procReleaseStgMedium = ole32.NewProc("ReleaseStgMedium")
	procDragQueryFileW   = shell32.NewProc("DragQueryFileW")
	procScreenToClient   = user32.NewProc("ScreenToClient")
)

// dropTargetImpl holds our IDropTarget state
// The COM object pointer points to vtablePtr, which points to the vtable
type dropTargetImpl struct {
	vtablePtr uintptr      // Points to vtable (allocated with GlobalAlloc)
	vtable    uintptr      // The vtable itself (allocated with GlobalAlloc)
	refCount  int32        // Reference count
	hwnd      windows.HWND // Window handle
}

// Global map from COM pointer to Go object
var (
	comObjects   = make(map[uintptr]*dropTargetImpl)
	comObjectsMu sync.RWMutex
)

// globalAlloc allocates Windows memory that won't be moved by Go GC
func globalAlloc(size int) uintptr {
	ptr, _, _ := procGlobalAlloc.Call(GMEM_FIXED|GMEM_ZEROINIT, uintptr(size))
	return ptr
}

// globalFree frees Windows memory
func globalFree(ptr uintptr) {
	procGlobalFree.Call(ptr)
}

// Callback functions - these must have the exact Windows x64 calling convention signatures

func dtQueryInterface(this uintptr, riid uintptr, ppvObject uintptr) uintptr {
	writeTraceFile(fmt.Sprintf(">>> QueryInterface called: this=0x%x riid=0x%x ppvObject=0x%x", this, riid, ppvObject))
	debug.Log(debug.APP, "[Windows DnD] QueryInterface called: this=%x", this)

	if riid == 0 || ppvObject == 0 {
		writeTraceFile("QueryInterface: riid or ppvObject is 0, returning E_UNEXPECTED")
		return E_UNEXPECTED
	}

	guid := (*windows.GUID)(unsafe.Pointer(riid))
	ppv := (*uintptr)(unsafe.Pointer(ppvObject))
	*ppv = 0

	if guidEqual(guid, &IID_IUnknown) || guidEqual(guid, &IID_IDropTarget) {
		writeTraceFile("QueryInterface: matched IDropTarget/IUnknown")
		debug.Log(debug.APP, "[Windows DnD] QueryInterface: matched IDropTarget/IUnknown")
		*ppv = this
		dtAddRef(this)
		return S_OK
	}

	writeTraceFile("QueryInterface: E_NOINTERFACE")
	debug.Log(debug.APP, "[Windows DnD] QueryInterface: E_NOINTERFACE")
	return E_NOINTERFACE
}

func dtAddRef(this uintptr) uintptr {
	writeTraceFile(fmt.Sprintf(">>> AddRef called: this=0x%x", this))
	debug.Log(debug.APP, "[Windows DnD] AddRef called: this=%x", this)

	comObjectsMu.RLock()
	impl := comObjects[this]
	comObjectsMu.RUnlock()

	if impl == nil {
		writeTraceFile("AddRef: impl not found!")
		debug.Log(debug.APP, "[Windows DnD] AddRef: impl not found!")
		return 1
	}

	newCount := atomic.AddInt32(&impl.refCount, 1)
	writeTraceFile(fmt.Sprintf("AddRef: count=%d", newCount))
	debug.Log(debug.APP, "[Windows DnD] AddRef: count=%d", newCount)
	return uintptr(newCount)
}

func dtRelease(this uintptr) uintptr {
	debug.Log(debug.APP, "[Windows DnD] Release called: this=%x", this)

	comObjectsMu.RLock()
	impl := comObjects[this]
	comObjectsMu.RUnlock()

	if impl == nil {
		debug.Log(debug.APP, "[Windows DnD] Release: impl not found!")
		return 0
	}

	newCount := atomic.AddInt32(&impl.refCount, -1)
	debug.Log(debug.APP, "[Windows DnD] Release: count=%d", newCount)

	if newCount == 0 {
		comObjectsMu.Lock()
		delete(comObjects, this)
		comObjectsMu.Unlock()
		// Free Windows memory
		globalFree(impl.vtable)
		globalFree(impl.vtablePtr)
	}

	return uintptr(newCount)
}

// POINTL packed as a single 64-bit value on x64: low 32 bits = X, high 32 bits = Y
func unpackPOINTL(pt uintptr) (x, y int32) {
	return int32(pt & 0xFFFFFFFF), int32(pt >> 32)
}

func dtDragEnter(this uintptr, pDataObject uintptr, grfKeyState uint32, pt uintptr, pdwEffect uintptr) uintptr {
	ptX, ptY := unpackPOINTL(pt)
	writeTraceFile(fmt.Sprintf(">>> DragEnter called: this=0x%x pDataObject=0x%x pt=(%d,%d) pdwEffect=0x%x", this, pDataObject, ptX, ptY, pdwEffect))
	debug.Log(debug.APP, "[Windows DnD] DragEnter: this=%x pt=(%d,%d)", this, ptX, ptY)

	if pdwEffect != 0 {
		*(*uint32)(unsafe.Pointer(pdwEffect)) = DROPEFFECT_COPY
		writeTraceFile("DragEnter: set pdwEffect to DROPEFFECT_COPY")
	}

	comObjectsMu.RLock()
	impl := comObjects[this]
	comObjectsMu.RUnlock()

	if impl != nil {
		writeTraceFile("DragEnter: found impl, calling handler")
		go func() {
			dropMu.Lock()
			handler := dragUpdateHandler
			currentDropTarget = ""
			dropMu.Unlock()

			if handler != nil {
				x, y := screenToClient(impl.hwnd, ptX, ptY)
				handler(x, y)
			}
		}()
	} else {
		writeTraceFile("DragEnter: impl not found!")
	}

	writeTraceFile("DragEnter: returning S_OK")
	return S_OK
}

func dtDragOver(this uintptr, grfKeyState uint32, pt uintptr, pdwEffect uintptr) uintptr {
	ptX, ptY := unpackPOINTL(pt)

	if pdwEffect != 0 {
		*(*uint32)(unsafe.Pointer(pdwEffect)) = DROPEFFECT_COPY
	}

	comObjectsMu.RLock()
	impl := comObjects[this]
	comObjectsMu.RUnlock()

	if impl != nil {
		go func() {
			dropMu.Lock()
			handler := dragUpdateHandler
			dropMu.Unlock()

			if handler != nil {
				x, y := screenToClient(impl.hwnd, ptX, ptY)
				handler(x, y)
			}
		}()
	}

	return S_OK
}

func dtDragLeave(this uintptr) uintptr {
	debug.Log(debug.APP, "[Windows DnD] DragLeave: this=%x", this)

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
}

func dtDrop(this uintptr, pDataObject uintptr, grfKeyState uint32, pt uintptr, pdwEffect uintptr) uintptr {
	ptX, ptY := unpackPOINTL(pt)
	debug.Log(debug.APP, "[Windows DnD] Drop: this=%x pDataObject=%x pt=(%d,%d)", this, pDataObject, ptX, ptY)
	writeTraceFile(fmt.Sprintf(">>> Drop called: this=0x%x pDataObject=0x%x pt=(%d,%d)", this, pDataObject, ptX, ptY))

	// Extract files synchronously before returning
	paths := getDroppedFiles(pDataObject)
	debug.Log(debug.APP, "[Windows DnD] Drop: extracted %d files", len(paths))
	writeTraceFile(fmt.Sprintf("Drop: extracted %d files", len(paths)))

	if pdwEffect != 0 {
		*(*uint32)(unsafe.Pointer(pdwEffect)) = DROPEFFECT_COPY
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
}

// writeTraceFile writes debug info to a file for tracing
func writeTraceFile(msg string) {
	f, err := os.OpenFile(`C:\razor_dnd_trace.txt`, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(msg + "\n")
}


// SetupExternalDrop configures the window to accept external file drops
func SetupExternalDrop(hwnd uintptr) {
	debug.Log(debug.APP, "[Windows DnD] SetupExternalDrop called with hwnd=%d (0x%x)", hwnd, hwnd)
	writeTraceFile(fmt.Sprintf("SetupExternalDrop called with hwnd=%d (0x%x)", hwnd, hwnd))

	if hwnd == 0 {
		debug.Log(debug.APP, "[Windows DnD] SetupExternalDrop: hwnd is 0, skipping")
		writeTraceFile("hwnd is 0, skipping")
		return
	}

	// Initialize OLE
	debug.Log(debug.APP, "[Windows DnD] Calling OleInitialize...")
	writeTraceFile("Calling OleInitialize...")
	hr, _, err := procOleInitialize.Call(0)
	debug.Log(debug.APP, "[Windows DnD] OleInitialize returned: hr=0x%x, err=%v", hr, err)
	writeTraceFile(fmt.Sprintf("OleInitialize returned: hr=0x%x, err=%v", hr, err))
	if hr != S_OK && hr != S_FALSE {
		debug.Log(debug.APP, "[Windows DnD] OleInitialize FAILED: 0x%x", hr)
		writeTraceFile(fmt.Sprintf("OleInitialize FAILED: 0x%x", hr))
		return
	}

	// Allocate vtable in Windows memory (7 function pointers)
	vtableSize := 7 * 8 // 7 pointers, 8 bytes each on x64
	vtable := globalAlloc(vtableSize)
	if vtable == 0 {
		debug.Log(debug.APP, "[Windows DnD] Failed to allocate vtable")
		writeTraceFile("Failed to allocate vtable")
		return
	}
	writeTraceFile(fmt.Sprintf("Allocated vtable at 0x%x", vtable))

	// Create callbacks and trace their addresses
	cbQueryInterface := windows.NewCallback(dtQueryInterface)
	cbAddRef := windows.NewCallback(dtAddRef)
	cbRelease := windows.NewCallback(dtRelease)
	cbDragEnter := windows.NewCallback(dtDragEnter)
	cbDragOver := windows.NewCallback(dtDragOver)
	cbDragLeave := windows.NewCallback(dtDragLeave)
	cbDrop := windows.NewCallback(dtDrop)

	writeTraceFile(fmt.Sprintf("Callbacks created:"))
	writeTraceFile(fmt.Sprintf("  QueryInterface: 0x%x", cbQueryInterface))
	writeTraceFile(fmt.Sprintf("  AddRef: 0x%x", cbAddRef))
	writeTraceFile(fmt.Sprintf("  Release: 0x%x", cbRelease))
	writeTraceFile(fmt.Sprintf("  DragEnter: 0x%x", cbDragEnter))
	writeTraceFile(fmt.Sprintf("  DragOver: 0x%x", cbDragOver))
	writeTraceFile(fmt.Sprintf("  DragLeave: 0x%x", cbDragLeave))
	writeTraceFile(fmt.Sprintf("  Drop: 0x%x", cbDrop))

	// Fill vtable with callback pointers
	vtableSlice := (*[7]uintptr)(unsafe.Pointer(vtable))
	vtableSlice[0] = cbQueryInterface
	vtableSlice[1] = cbAddRef
	vtableSlice[2] = cbRelease
	vtableSlice[3] = cbDragEnter
	vtableSlice[4] = cbDragOver
	vtableSlice[5] = cbDragLeave
	vtableSlice[6] = cbDrop

	// Verify vtable contents
	writeTraceFile(fmt.Sprintf("VTable contents at 0x%x:", vtable))
	for i := 0; i < 7; i++ {
		writeTraceFile(fmt.Sprintf("  [%d] = 0x%x", i, vtableSlice[i]))
	}

	debug.Log(debug.APP, "[Windows DnD] vtable=%x callbacks: QI=%x AddRef=%x Release=%x DragEnter=%x DragOver=%x DragLeave=%x Drop=%x",
		vtable, vtableSlice[0], vtableSlice[1], vtableSlice[2], vtableSlice[3], vtableSlice[4], vtableSlice[5], vtableSlice[6])

	// Allocate COM object (just a pointer to vtable)
	vtablePtr := globalAlloc(8)
	if vtablePtr == 0 {
		debug.Log(debug.APP, "[Windows DnD] Failed to allocate vtablePtr")
		writeTraceFile("Failed to allocate vtablePtr")
		globalFree(vtable)
		return
	}
	writeTraceFile(fmt.Sprintf("Allocated vtablePtr at 0x%x", vtablePtr))

	// Store pointer to vtable
	*(*uintptr)(unsafe.Pointer(vtablePtr)) = vtable
	writeTraceFile(fmt.Sprintf("Stored vtable pointer: *0x%x = 0x%x", vtablePtr, vtable))

	// Verify the indirection
	storedVtable := *(*uintptr)(unsafe.Pointer(vtablePtr))
	writeTraceFile(fmt.Sprintf("Verification: reading *0x%x = 0x%x", vtablePtr, storedVtable))

	// Create Go-side impl struct
	impl := &dropTargetImpl{
		vtablePtr: vtablePtr,
		vtable:    vtable,
		refCount:  1,
		hwnd:      windows.HWND(hwnd),
	}
	globalDropTarget = impl // Prevent GC

	// Register in map
	comObjectsMu.Lock()
	comObjects[vtablePtr] = impl
	comObjectsMu.Unlock()

	debug.Log(debug.APP, "[Windows DnD] Created COM object: vtablePtr=%x vtable=%x", vtablePtr, vtable)
	writeTraceFile(fmt.Sprintf("Created COM object: vtablePtr=0x%x vtable=0x%x", vtablePtr, vtable))

	// Register with Windows
	debug.Log(debug.APP, "[Windows DnD] Calling RegisterDragDrop...")
	writeTraceFile(fmt.Sprintf("Calling RegisterDragDrop(hwnd=0x%x, pDropTarget=0x%x)", hwnd, vtablePtr))
	hr, _, err = procRegisterDragDrop.Call(hwnd, vtablePtr)
	debug.Log(debug.APP, "[Windows DnD] RegisterDragDrop returned: hr=0x%x, err=%v", hr, err)
	writeTraceFile(fmt.Sprintf("RegisterDragDrop returned: hr=0x%x, err=%v", hr, err))
	if hr != S_OK {
		debug.Log(debug.APP, "[Windows DnD] RegisterDragDrop FAILED: 0x%x", hr)
		writeTraceFile(fmt.Sprintf("RegisterDragDrop FAILED: 0x%x", hr))
		comObjectsMu.Lock()
		delete(comObjects, vtablePtr)
		comObjectsMu.Unlock()
		globalFree(vtable)
		globalFree(vtablePtr)
		globalDropTarget = nil
		return
	}

	debug.Log(debug.APP, "[Windows DnD] SUCCESS - IDropTarget registered")
	writeTraceFile("SUCCESS - IDropTarget registered")
	writeTraceFile("---")
}

// screenToClient converts screen coordinates to client coordinates
func screenToClient(hwnd windows.HWND, screenX, screenY int32) (int, int) {
	type point struct {
		X, Y int32
	}
	pt := point{screenX, screenY}
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
