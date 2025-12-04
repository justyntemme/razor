# Architecture Overview

This document describes the high-level architecture of Razor, including package structure, data flow, and key design decisions.

## Package Dependency Graph

```
cmd/razor/main.go
    └─→ internal/app

internal/app
    ├─→ internal/ui        (UIEvent, State, Renderer)
    ├─→ internal/fs        (Request/Response channels)
    ├─→ internal/search    (Engine detection, query parsing)
    ├─→ internal/store     (Search history, recent files, settings)
    ├─→ internal/config    (Configuration file management)
    ├─→ internal/trash     (Trash/recycle bin operations)
    ├─→ internal/debug     (Debug logging)
    └─→ gioui.org/app      (Window event loop)

internal/ui
    ├─→ gioui.org/*        (Layout, widgets, material)
    ├─→ gioui.org/io/*     (Keyboard, pointer events)
    ├─→ internal/config    (Hotkey matching)
    └─→ internal/debug     (Debug logging)

internal/fs
    ├─→ internal/search    (Query parser, external engines)
    ├─→ github.com/charlievieth/fastwalk  (Parallel directory walking)
    └─→ context            (Cancellation)

internal/search
    ├─→ os/exec            (ripgrep/ugrep subprocess)
    └─→ bufio              (Stream results)

internal/config
    └─→ encoding/json      (Config file parsing)

internal/store
    └─→ modernc.org/sqlite (Pure Go SQLite)

internal/trash
    └─→ Platform APIs      (macOS/Linux/Windows trash)
```

## Goroutine Architecture

Razor uses a multi-goroutine architecture with channel-based communication:

```
┌─────────────────────────────────────────────────────────────────┐
│                     MAIN GOROUTINE                               │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │              Gio Event Loop (app.Main)                   │    │
│  │                                                          │    │
│  │  for { switch window.Event().(type) { ... } }           │    │
│  │                                                          │    │
│  │  FrameEvent → Layout() → UIEvent → handleUIEvent()      │    │
│  └─────────────────────────────────────────────────────────┘    │
└───────────────────────────┬─────────────────────────────────────┘
                            │
        ┌───────────────────┼───────────────────┐
        │                   │                   │
        ▼                   ▼                   ▼
┌───────────────┐   ┌───────────────┐   ┌───────────────┐
│  fs.System    │   │  store.DB     │   │ processEvents │
│  .Start()     │   │  .Start()     │   │ (select loop) │
│               │   │               │   │               │
│  Handles:     │   │  Handles:     │   │  Receives:    │
│  - FetchDir   │   │  - Favorites  │   │  - Responses  │
│  - SearchDir  │   │  - Settings   │   │  - Progress   │
│  - Cancel     │   │               │   │               │
└───────┬───────┘   └───────┬───────┘   └───────────────┘
        │                   │
        ▼                   │
┌───────────────┐           │
│ searchDir()   │           │
│ (spawned per  │           │
│  search req)  │           │
│               │           │
│ Context-      │           │
│ cancellable   │           │
└───────────────┘           │
```

## Channel Communication

All inter-goroutine communication uses buffered channels:

```go
// Filesystem channels
fs.RequestChan  chan Request   // buffered: 10
fs.ResponseChan chan Response  // buffered: 10
fs.ProgressChan chan Progress  // buffered: 100

// Store channels
store.RequestChan  chan Request   // buffered: 10
store.ResponseChan chan Response  // buffered: 10
```

### Why Buffered Channels?

1. **Non-blocking sends**: UI thread never blocks waiting for subsystem
2. **Batch processing**: Multiple requests can queue while subsystem is busy
3. **Progress streaming**: High-frequency progress updates don't block search

## Data Flow Examples

### Navigation Flow

```
User double-clicks folder
    │
    ▼
Renderer.Layout() detects click
    │
    ▼
Returns UIEvent{Action: ActionNavigate, Path: "/new/path"}
    │
    ▼
Orchestrator.handleUIEvent()
    │
    ├─→ Updates history stack
    │
    └─→ Sends fs.Request{Op: FetchDir, Path: "/new/path", Gen: N}
            │
            ▼
        fs.System.Start() receives request
            │
            ▼
        os.ReadDir() + stat each entry
            │
            ▼
        Sends fs.Response{Entries: [...], Gen: N}
            │
            ▼
        Orchestrator.processEvents() receives response
            │
            ├─→ Checks Gen (reject if stale)
            │
            ├─→ Converts Entry → UIEntry
            │
            ├─→ Updates state.Entries
            │
            └─→ window.Invalidate() triggers redraw
```

### Search Flow (with External Engine)

```
User types "contents:TODO ext:go" and presses Enter
    │
    ▼
UIEvent{Action: ActionSearch, Path: "contents:TODO ext:go"}
    │
    ▼
Orchestrator.doSearch()
    │
    ├─→ Increments searchGen (stale result prevention)
    │
    ├─→ Sets state.IsSearchResult = true
    │
    └─→ Sends Request{Op: SearchDir, Query: "...", SearchEngine: 1, Gen: N}
            │
            ▼
        fs.System detects contents: directive + ripgrep available
            │
            ▼
        Spawns goroutine: searchDir() with context.WithCancel()
            │
            ▼
        Runs: rg --files-with-matches "TODO" /path
            │
            ├─→ Streams stdout line-by-line
            │
            ├─→ Sends Progress every 10 files found
            │
            └─→ Applies ext: filter to results
                    │
                    ▼
                Sends Response{Entries: [...], Gen: N}
                    │
                    ▼
                Display in file list
```

### Copy/Paste with Conflict

```
User copies file, pastes to destination with existing file
    │
    ▼
Orchestrator.doPaste() [spawned goroutine]
    │
    ├─→ Detects conflict (os.Stat succeeds on destination)
    │
    ├─→ Sets state.Conflict = ConflictState{Active: true, ...}
    │
    ├─→ window.Invalidate() shows dialog
    │
    └─→ BLOCKS on: <-conflictResponse channel
            │
            │   User clicks "Replace"
            │           │
            │           ▼
            │   UIEvent{Action: ActionConflictReplace}
            │           │
            │           ▼
            │   handleConflictResolution()
            │           │
            │           ▼
            └─── conflictResponse <- ConflictReplaceAll (UNBLOCKS)
                    │
                    ▼
                Deletes existing, copies file
                    │
                    ▼
                requestDir() refreshes list
```

## Generation Counter Pattern

The generation counter prevents stale async results from corrupting the UI:

```go
// In Orchestrator
var searchGen int64
var searchGenMu sync.Mutex

// When starting a new operation
searchGenMu.Lock()
searchGen++
gen := searchGen
searchGenMu.Unlock()

// Send request with generation
fs.RequestChan <- Request{Gen: gen, ...}

// When receiving response
func handleFSResponse(resp Response) {
    searchGenMu.Lock()
    currentGen := searchGen
    searchGenMu.Unlock()

    if resp.Gen < currentGen {
        // STALE - a newer request was made, ignore this response
        return
    }

    // Process response...
}
```

### Why This Matters

```
Time →
    T1: User searches "foo"     Gen=1
    T2: Search starts...
    T3: User clears, searches "bar"  Gen=2 (Gen 1 cancelled)
    T4: Search "bar" starts...
    T5: "foo" results arrive    Gen=1 < 2 → IGNORED
    T6: "bar" results arrive    Gen=2 == 2 → DISPLAYED
```

## State Management

### Global State (Orchestrator)

```go
type Orchestrator struct {
    // Gio window
    window *app.Window

    // Subsystems
    fs    *fs.System
    store *store.DB
    ui    *ui.Renderer

    // Current view state
    state ui.State

    // Navigation history
    history      []string
    historyIndex int

    // Display settings
    sortColumn   ui.SortColumn
    sortAsc      bool
    showDotfiles bool

    // Entries (two-level cache)
    rawEntries []ui.UIEntry  // After search filter
    dirEntries []ui.UIEntry  // Canonical directory listing

    // Search state
    searchGen   int64
    searchGenMu sync.Mutex

    // File conflict state
    conflictResolution ui.ConflictResolution
    conflictResponse   chan ui.ConflictResolution
    conflictAbort      bool

    // Settings
    selectedEngine    search.SearchEngine
    selectedEngineCmd string
    defaultDepth      int
}
```

### UI State (passed to Renderer)

```go
type State struct {
    CurrentPath   string
    Entries       []UIEntry
    SelectedIndex int
    CanBack       bool
    CanForward    bool

    Favorites map[string]bool
    FavList   []FavoriteItem

    Clipboard *Clipboard
    Progress  ProgressState

    DeleteTarget string
    Drives       []DriveItem

    IsSearchResult bool
    SearchQuery    string

    Conflict ConflictState
}
```

### Widget State (Renderer)

Gio widgets require persistent state between frames:

```go
type Renderer struct {
    // Lists need state for scrolling
    listState layout.List
    favState  layout.List

    // Buttons need clickable state
    backBtn widget.Clickable
    fwdBtn  widget.Clickable

    // Editors need text state
    searchEditor widget.Editor
    pathEditor   widget.Editor

    // Dialogs need open/closed state
    settingsOpen      bool
    deleteConfirmOpen bool
    createDialogOpen  bool

    // ... etc
}
```

## Error Handling Strategy

1. **Log and continue**: Most errors are logged but don't crash
2. **User feedback**: Critical errors show in progress bar or dialog
3. **Graceful degradation**: Missing ripgrep falls back to builtin search

```go
// Example: File operation error handling
func (o *Orchestrator) doPaste() {
    // ...
    if err != nil {
        log.Printf("Paste error: %v", err)
        // Don't crash, just log
    }

    // Always refresh directory
    o.requestDir(o.state.CurrentPath)
}
```

## Platform Abstraction

Platform-specific code is isolated in `_darwin.go`, `_linux.go`, `_windows.go` files:

```go
// platform_darwin.go
func platformOpen(path string) error {
    return exec.Command("open", path).Start()
}

// platform_linux.go
func platformOpen(path string) error {
    return exec.Command("xdg-open", path).Start()
}

// platform_windows.go
func platformOpen(path string) error {
    return exec.Command("cmd", "/c", "start", "", path).Start()
}
```

## Debug Logging

Build tags control logging:

```bash
# Debug build - logging enabled
go build -tags debug ./cmd/razor

# Release build - logging compiled out
go build ./cmd/razor
```

```go
// debug_on.go (+build debug)
const debugEnabled = true
func debugLog(format string, args ...interface{}) {
    log.Printf("[DEBUG] "+format, args...)
}

// debug_off.go (+build !debug)
const debugEnabled = false
func debugLog(format string, args ...interface{}) {}
```
