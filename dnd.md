# Windows Drag-and-Drop Implementation Notes

## Goal
Implement external drag-and-drop from Windows Explorer to Razor using OLE IDropTarget interface, matching the Darwin implementation's feature set (hover highlighting during drag).

## Attempts

### Attempt 1: Basic IDropTarget with Go heap allocation
- Created vtable struct with function pointers
- Allocated COM object on Go heap
- Used `syscall.NewCallback` for callbacks
- **Result**: RegisterDragDrop succeeded, but callbacks never fired. Explorer froze when dragging.

### Attempt 2: Double-pointer indirection (Windigo pattern)
- Based on rodrigocfd/windigo pattern
- Used `ppImpl := &pImpl` for COM double-pointer indirection
- Added pointer cache to prevent GC collection
- **Result**: Same behavior - registration succeeded, callbacks never fired.

### Attempt 3: GlobalAlloc for COM memory
- Based on wailsapp/go-webview2 combridge pattern
- Allocated vtable with Windows `GlobalAlloc` instead of Go heap
- Stored vtable pointer in GlobalAlloc'd memory
- Used `windows.NewCallback` instead of `syscall.NewCallback`
- **Result**: AddRef called during RegisterDragDrop (same thread), but drag callbacks still never fired.

### Attempt 4: Fixed POINTL parameter passing
- Windows x64 passes POINTL struct as single 64-bit value, not two int32 params
- Changed callback signatures: `ptX int32, ptY int32` → `pt uintptr`
- Added `unpackPOINTL()` helper function
- **Result**: No change - callbacks still not firing from drag thread.

### Attempt 5: CGO enablement
- Created `cgo_windows.go` with `import "C"` to enable thread initialization
- Go issue #20823: callbacks from external threads require CGO
- **Result**: CGO file was being skipped (no symbols used). Added `CgoEnabled()` function called from `drop_windows.go` to force CGO linking.

### Attempt 6: CGO build
- With CGO forced, build now requires C compiler
- **Result**: MinGW architecture mismatch - 32-bit libraries being linked for 64-bit build. Produced invalid executable.

## Root Cause Identified

**Go Issue #20823**: `syscall.NewCallback` and `windows.NewCallback` cannot receive calls from external Windows threads without CGO enabled.

When Explorer drags files, it calls IDropTarget methods from its OLE thread. Without CGO, Go runtime isn't initialized on that thread, so callbacks silently fail.

Proof: `C:\razor_callback.txt` (written at start of DragEnter) is never created.

## Current Blocker

MinGW setup produces invalid executables when CGO is enabled:
- `gcc -dumpmachine` shows `x86_64-w64-mingw32` (correct)
- But linking fails or produces "not a valid application for this OS platform"
- Likely multiple MinGW installations conflicting

## What Works (without CGO)
- Win32ViewEvent received correctly
- SetupExternalDrop called with valid HWND
- OleInitialize succeeds
- COM vtable allocated correctly
- RegisterDragDrop succeeds
- AddRef callback works (same thread, during registration)

## Files

- `internal/platform/drop_windows.go` - IDropTarget implementation
- `internal/platform/cgo_windows.go` - CGO enablement
- `internal/app/orchestrator_windows.go` - Win32ViewEvent handling

## Debug Files (on Windows)
- `C:\razor_events.txt` - Gio events
- `C:\razor_dnd_trace.txt` - Setup trace
- `C:\razor_callback.txt` - Callback proof (never created = not firing)

## Resolution

**Deleted `cgo_windows.go`** to fix the invalid executable issue. The CGO approach doesn't work with the current MinGW setup and isn't needed for WM_DROPFILES.

## Resolution - WM_DROPFILES Implementation (Attempt 7)

Successfully implemented WM_DROPFILES approach using window subclassing:

### Implementation Details

1. **DragAcceptFiles** - Called on HWND to enable drop acceptance
2. **SetWindowLongPtr(GWLP_WNDPROC)** - Subclasses Gio's window to intercept messages
3. **WM_DROPFILES handler** - Extracts file paths using DragQueryFileW
4. **DragFinish** - Releases HDROP handle after processing

### Key Code

```go
// Enable drag-and-drop
procDragAcceptFiles.Call(hwnd, 1)

// Subclass window to intercept WM_DROPFILES
newProc := syscall.NewCallback(dropSubclassProc)
oldProc, _, _ := procSetWindowLongPtr.Call(hwnd, gwlpWndProc, newProc)
originalWndProc = oldProc

// In dropSubclassProc:
if msg == WM_DROPFILES {
    handleDropFiles(wParam) // wParam is HDROP
    return 0
}
return procCallWindowProc.Call(originalWndProc, hwnd, msg, wParam, lParam)
```

### Why This Works (Unlike IDropTarget)

- WM_DROPFILES is a regular Windows message delivered to the window's message loop
- Gio's message loop (GetMessage/DispatchMessage) runs on the Go thread
- Our subclass procedure is called from the same thread context
- No cross-thread callback issues like with IDropTarget's OLE callbacks

### Limitations

- No hover highlighting during drag (only notified on drop)
- Cannot update drop target in real-time during drag
- No DragEnter/DragOver/DragLeave events

### Files Modified

- `internal/platform/drop_windows.go` - Replaced IDropTarget with WM_DROPFILES
- `internal/app/orchestrator_windows.go` - Simplified, removed debug file writes
