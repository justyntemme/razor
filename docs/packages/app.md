# App Package (`internal/app`)

The app package contains the Orchestrator and its controllers, which form the central coordination layer managing all subsystems.

## Files

| File | Purpose |
|------|---------|
| `orchestrator.go` | Central event loop controller |
| `state_owner.go` | Canonical file entry cache, state management |
| `controllers.go` | Shared dependencies and controller types |
| `nav_controller.go` | Navigation history, path expansion |
| `search_controller.go` | Search execution, engine management |
| `tabs.go` | Tab state management |
| `file_ops.go` | File operations (copy, paste, delete, rename) |
| `conflict.go` | File conflict resolution dialog handling |
| `watcher.go` | Directory change detection (fsnotify) |
| `platform_darwin.go` | macOS file operations |
| `platform_linux.go` | Linux file operations |
| `platform_windows.go` | Windows file operations |
| `debug_on.go` | Debug logging (build tag: debug) |
| `debug_off.go` | No-op logging (release build) |

## Orchestrator

The Orchestrator is the heart of the application:

```go
// File: internal/app/orchestrator.go:24-52

type Orchestrator struct {
    // Gio window handle
    window *app.Window

    // Subsystems
    fs    *fs.System    // Filesystem operations
    store *store.DB     // SQLite persistence
    ui    *ui.Renderer  // UI rendering

    // Current state
    state ui.State

    // Navigation history
    history      []string
    historyIndex int

    // Display settings
    sortColumn   ui.SortColumn
    sortAsc      bool
    showDotfiles bool

    // Entry caching (two-level)
    rawEntries []ui.UIEntry  // Filtered/searched results
    dirEntries []ui.UIEntry  // Canonical directory listing

    // Progress tracking
    progressMu sync.Mutex

    // Home directory
    homePath string

    // Stale result prevention
    searchGen   int64
    searchGenMu sync.Mutex

    // Conflict resolution
    conflictResolution ui.ConflictResolution
    conflictResponse   chan ui.ConflictResolution
    conflictAbort      bool

    // Search engine
    searchEngines     []search.EngineInfo
    selectedEngine    search.SearchEngine
    selectedEngineCmd string
    defaultDepth      int
}
```

## Initialization

```go
// File: internal/app/orchestrator.go:54-93

func NewOrchestrator() *Orchestrator
```

1. Gets home directory
2. Detects available search engines (ripgrep, ugrep)
3. Converts engine info to UI format
4. Creates all subsystems
5. Sets up conflict resolution channel
6. Configures default settings

## Main Event Loop

```go
// File: internal/app/orchestrator.go:167-213

func (o *Orchestrator) Run(startPath string) error
```

```
┌─────────────────────────────────────────────────────────────┐
│                        Run()                                 │
│                                                              │
│  1. Open SQLite database                                    │
│  2. Start goroutines:                                       │
│     - fs.Start()        (filesystem operations)             │
│     - store.Start()     (database operations)               │
│     - processEvents()   (response handler)                  │
│  3. Request initial data:                                   │
│     - FetchFavorites                                        │
│     - FetchSettings                                         │
│     - refreshDrives()                                       │
│  4. Navigate to start path                                  │
│  5. Enter Gio event loop:                                   │
│                                                              │
│     for {                                                   │
│         switch e := window.Event().(type) {                 │
│         case app.DestroyEvent:                              │
│             return e.Err                                    │
│         case app.FrameEvent:                                │
│             gtx := app.NewContext(&ops, e)                  │
│             evt := ui.Layout(gtx, &state)                   │
│             handleUIEvent(evt)                              │
│             e.Frame(gtx.Ops)                                │
│         }                                                   │
│     }                                                       │
└─────────────────────────────────────────────────────────────┘
```

## Event Handling

```go
// File: internal/app/orchestrator.go:223-357

func (o *Orchestrator) handleUIEvent(evt ui.UIEvent)
```

Handles all UIActions:

| Action | Handler |
|--------|---------|
| `ActionNavigate` | `navigate(path)` - push to history, fetch directory |
| `ActionBack` | `goBack()` - navigate to parent |
| `ActionForward` | `goForward()` - navigate forward in history |
| `ActionHome` | Navigate to home directory |
| `ActionNewWindow` | `openNewWindow()` - spawn new process |
| `ActionSelect` | Update `state.SelectedIndex` |
| `ActionSearch` | `doSearch(query)` - execute search |
| `ActionOpen` | `platformOpen(path)` - open with default app |
| `ActionOpenWith` | `platformOpenWith(path, app)` |
| `ActionAddFavorite` | Send to store |
| `ActionRemoveFavorite` | Send to store |
| `ActionSort` | Update sort settings, reapply |
| `ActionToggleDotfiles` | Toggle, save setting, reapply |
| `ActionCopy` | Set clipboard with ClipCopy |
| `ActionCut` | Set clipboard with ClipCut |
| `ActionPaste` | `doPaste()` in goroutine |
| `ActionConfirmDelete` | `doDelete(path)` in goroutine |
| `ActionCreateFile` | `doCreateFile(name)` in goroutine |
| `ActionCreateFolder` | `doCreateFolder(name)` in goroutine |
| `ActionRename` | `doRename(old, new)` in goroutine |
| `ActionClearSearch` | Cancel search, restore directory |
| `ActionConflict*` | `handleConflictResolution()` |
| `ActionChangeSearchEngine` | Update engine, save setting |
| `ActionChangeDefaultDepth` | Update depth, save setting |
| `ActionChangeTheme` | Save setting |

## Navigation

### History Management

```go
// File: internal/app/orchestrator.go:380-407

func (o *Orchestrator) navigate(path string)
func (o *Orchestrator) goBack()
func (o *Orchestrator) goForward()
```

History is a simple stack with an index:

```
history = ["/home", "/home/docs", "/home/docs/project"]
                                           ↑
                                    historyIndex = 2

After goBack():
history = ["/home", "/home/docs", "/home/docs/project"]
                         ↑
                  historyIndex = 1

After navigate("/tmp"):
history = ["/home", "/home/docs", "/tmp"]  // truncated
                                    ↑
                             historyIndex = 2
```

### Path Expansion

```go
// File: internal/app/orchestrator.go:95-125

func (o *Orchestrator) expandPath(input string) string
```

Handles:
- `~` → home directory
- `~/path` → home + path
- Relative paths (`../`, `./`)
- Absolute paths
- Windows drive letters (`C:\`)
- UNC paths (`\\server\share`)

## Search

```go
// File: internal/app/orchestrator.go:419-496

func (o *Orchestrator) doSearch(query string)
```

1. Empty query → restore directory listing
2. Check for incomplete directives (don't search yet)
3. Set `state.IsSearchResult = true`
4. Show progress bar for directive searches
5. Increment generation counter
6. Send search request to filesystem

### Generation Counter

Prevents stale results from corrupting the display:

```go
o.searchGenMu.Lock()
o.searchGen++
gen := o.searchGen
o.searchGenMu.Unlock()

// Include gen in request
fs.RequestChan <- fs.Request{Gen: gen, ...}

// In response handler:
if resp.Gen < currentGen {
    // Stale, ignore
    return
}
```

## Async Response Processing

```go
// File: internal/app/orchestrator.go:548-559

func (o *Orchestrator) processEvents()
```

Runs in dedicated goroutine, handles:
- `fs.ResponseChan` → `handleFSResponse()`
- `fs.ProgressChan` → `handleProgress()`
- `store.ResponseChan` → `handleStoreResponse()`

### Filesystem Response

```go
// File: internal/app/orchestrator.go:576-640

func (o *Orchestrator) handleFSResponse(resp fs.Response)
```

1. Clear progress indicator
2. Check if cancelled → ignore
3. Check generation → ignore if stale
4. Convert `fs.Entry` → `ui.UIEntry`
5. Update `dirEntries` or `rawEntries` based on operation type
6. Apply filter and sort
7. Invalidate window

### Store Response

```go
// File: internal/app/orchestrator.go:642-689

func (o *Orchestrator) handleStoreResponse(resp store.Response)
```

Handles:
- `FetchFavorites` → Update favorites map and list
- `FetchSettings` → Apply settings (dotfiles, engine, depth, theme)

## File Operations

### Copy/Paste

```go
// File: internal/app/orchestrator.go:822-900

func (o *Orchestrator) doPaste()
```

1. Check for conflict (destination exists)
2. If conflict:
   - Check remembered resolution ("Apply to All")
   - Or show conflict dialog and wait on channel
3. Perform operation based on resolution:
   - Replace: delete destination, then copy
   - Keep Both: rename with `_copy1`, `_copy2`, etc.
   - Skip: do nothing
   - Stop: abort operation
4. Track progress with `progressWriter`
5. If cut operation: delete source after successful copy
6. Refresh directory

### Conflict Resolution

```go
// File: internal/app/orchestrator.go:902-931

func (o *Orchestrator) resolveConflict(src, dst string, srcInfo, dstInfo os.FileInfo) ui.ConflictResolution
```

1. Check if resolution already remembered
2. Check if abort requested
3. Show dialog by setting `state.Conflict.Active = true`
4. Block on `<-conflictResponse` channel
5. Return user's choice

The UI sends response via:
```go
// File: internal/app/orchestrator.go:809-820

func (o *Orchestrator) handleConflictResolution(resolution ui.ConflictResolution)
```

### Delete

```go
// File: internal/app/orchestrator.go:1056-1078

func (o *Orchestrator) doDelete(path string)
```

1. Show progress
2. `os.Remove()` for files, `os.RemoveAll()` for directories
3. Clear progress
4. Refresh directory

### Create File/Folder

```go
// File: internal/app/orchestrator.go:738-784

func (o *Orchestrator) doCreateFile(name string)
func (o *Orchestrator) doCreateFolder(name string)
```

1. Build full path
2. Check if already exists
3. Create file (`os.Create`) or folder (`os.Mkdir`)
4. Refresh directory

### Rename

```go
// File: internal/app/orchestrator.go:786-807

func (o *Orchestrator) doRename(oldPath, newPath string)
```

1. Validate paths
2. Check destination doesn't exist
3. `os.Rename()`
4. Refresh directory

## Sorting and Filtering

```go
// File: internal/app/orchestrator.go:694-727

func (o *Orchestrator) applyFilterAndSort()
func (o *Orchestrator) getComparator() func(a, b ui.UIEntry) bool
```

Filter:
- Remove dotfiles if `showDotfiles == false`

Sort:
- Directories always before files
- Then by selected column (Name, Date, Type, Size)
- Ascending or descending

## Progress Tracking

```go
// File: internal/app/orchestrator.go:731-736

func (o *Orchestrator) setProgress(active bool, label string, current, total int64)
```

Thread-safe progress update:
```go
o.progressMu.Lock()
o.state.Progress = ui.ProgressState{...}
o.progressMu.Unlock()
o.window.Invalidate()
```

Used by:
- File copy (bytes copied / total bytes)
- Directory copy (bytes copied / total bytes)
- Delete (indeterminate)
- Search (files searched / total, or indeterminate for external)

## Platform Functions

Defined per-platform in separate files:

```go
// platform_darwin.go / platform_linux.go / platform_windows.go

func platformOpen(path string) error
func platformOpenWith(path, app string) error
```

### macOS

```go
// Open with default app
exec.Command("open", path).Start()

// Open with specific app
exec.Command("open", "-a", app, path).Start()

// Reveal in Finder (uses AppleScript)
script := fmt.Sprintf(`tell application "Finder" to reveal POSIX file "%s"`, path)
exec.Command("osascript", "-e", script).Start()
```

### Linux

```go
// Open with default app
exec.Command("xdg-open", path).Start()

// Try various openers
for _, opener := range []string{"xdg-open", "kde-open5", "gnome-open", "gio"} {
    if _, err := exec.LookPath(opener); err == nil {
        return exec.Command(opener, path).Start()
    }
}
```

### Windows

```go
// Open with default app
exec.Command("cmd", "/c", "start", "", path).Start()

// Open With dialog
exec.Command("rundll32", "shell32.dll,OpenAs_RunDLL", path).Start()
```

## Debug Logging

```go
// debug_on.go (build tag: debug)
const debugEnabled = true
func debugLog(format string, args ...interface{}) {
    log.Printf("[DEBUG] "+format, args...)
}

// debug_off.go (build tag: !debug)
const debugEnabled = false
func debugLog(format string, args ...interface{}) {}
```

Build with logging:
```bash
go build -tags debug ./cmd/razor
```

## Entry Point

```go
// File: internal/app/orchestrator.go:1106-1115

func Main(startPath string)
```

Called from `cmd/razor/main.go`:
1. Spawns Orchestrator in goroutine
2. Calls `app.Main()` to run Gio event loop

## Extension Points

### Adding a New File Operation

1. Add UIAction in `internal/ui/renderer.go`
2. Add case in `handleUIEvent()`
3. Implement operation function (spawn in goroutine if slow)
4. Call `requestDir()` to refresh after completion

### Adding a New Setting

1. Add field to Orchestrator
2. Handle in `handleStoreResponse()` for loading
3. Handle UIAction for saving
4. Save via `store.RequestChan <- Request{Op: SaveSetting, Key: "...", Value: "..."}`

### Adding Platform-Specific Behavior

1. Create new `func myFeature() error` in each platform file
2. Use build tags to ensure correct implementation per OS
3. Call from Orchestrator as needed
