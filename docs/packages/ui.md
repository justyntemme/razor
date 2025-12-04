# UI Package (`internal/ui`)

The UI package contains all Gio-based rendering code. It defines the visual structure of the application and handles user interaction events.

## Files

| File | Purpose |
|------|---------|
| `types.go` | UI actions, events, state types |
| `renderer.go` | Main Renderer struct, widget state |
| `renderer_rows.go` | File row rendering |
| `renderer_input.go` | Keyboard input handling |
| `renderer_preview.go` | Preview pane rendering |
| `renderer_setters.go` | State setter methods |
| `renderer_tabs.go` | Tab-related rendering |
| `layout.go` | Top-level layout composition |
| `layout_navbar.go` | Navigation bar layout |
| `layout_sidebar.go` | Sidebar (favorites/drives) layout |
| `layout_filelist.go` | File list view layout |
| `layout_grid.go` | Grid/icon view layout |
| `layout_search.go` | Search bar and directive pills |
| `layout_preview.go` | Preview pane layout |
| `layout_menus.go` | Context menus and file menu |
| `layout_dialogs.go` | Delete confirm, create file/folder dialogs |
| `layout_modals.go` | Settings modal |
| `layout_browser_tabs.go` | Browser tab bar layout |
| `colors.go` | Theme color definitions (light/dark) |
| `tabs.go` | Reusable tab bar component |
| `markdown.go` | Markdown rendering (goldmark) |
| `orgmode.go` | Org-mode rendering (organelle) |
| `toast.go` | Toast notification UI |
| `thumbnail_cache.go` | Image thumbnail caching |
| `clickdrag.go` | Unified click and drag gesture handling |
| `heic_*.go` | Platform-specific HEIC image support |
| `debug_*.go` | UI debug flags |

## Key Types

### UIAction

Enumeration of all possible user actions:

```go
// File: internal/ui/renderer.go:27-61

type UIAction int

const (
    ActionNone UIAction = iota
    ActionNavigate        // Navigate to directory
    ActionBack            // Go back in history
    ActionForward         // Go forward in history
    ActionHome            // Go to home directory
    ActionNewWindow       // Open new window
    ActionSelect          // Select file/folder
    ActionSearch          // Execute search
    ActionOpen            // Open file with default app
    ActionOpenWith        // Open with specific app
    ActionOpenWithApp     // Open with chosen app
    ActionAddFavorite     // Add to favorites
    ActionRemoveFavorite  // Remove from favorites
    ActionSort            // Change sort order
    ActionToggleDotfiles  // Show/hide dotfiles
    ActionCopy            // Copy to clipboard
    ActionCut             // Cut to clipboard
    ActionPaste           // Paste from clipboard
    ActionDelete          // Delete file/folder
    ActionConfirmDelete   // Confirm deletion
    ActionCancelDelete    // Cancel deletion
    ActionCreateFile      // Create new file
    ActionCreateFolder    // Create new folder
    ActionClearSearch     // Clear search results
    ActionRename          // Rename file/folder
    // Conflict resolution
    ActionConflictReplace
    ActionConflictKeepBoth
    ActionConflictSkip
    ActionConflictStop
    // Settings
    ActionChangeSearchEngine
    ActionChangeDefaultDepth
    ActionChangeTheme
)
```

### UIEvent

Returned from Layout() to communicate user actions to Orchestrator:

```go
// File: internal/ui/renderer.go:117-131

type UIEvent struct {
    Action        UIAction
    Path          string      // File path or search query
    OldPath       string      // Original path (for rename)
    NewIndex      int         // Selected index
    SortColumn    SortColumn
    SortAscending bool
    ShowDotfiles  bool
    ClipOp        ClipOp
    FileName      string      // New file/folder name
    AppPath       string      // Application path (open with)
    SearchEngine  string      // Selected search engine
    DefaultDepth  int         // Recursive search depth
    DarkMode      bool        // Theme setting
}
```

### State

Current application state passed to Renderer:

```go
// File: internal/ui/renderer.go:203-218

type State struct {
    CurrentPath    string
    Entries        []UIEntry       // Displayed files/folders
    SelectedIndex  int             // -1 = none selected
    CanBack        bool
    CanForward     bool
    Favorites      map[string]bool // Quick lookup
    FavList        []FavoriteItem  // For display
    Clipboard      *Clipboard
    Progress       ProgressState
    DeleteTarget   string
    Drives         []DriveItem
    IsSearchResult bool            // True when showing search results
    SearchQuery    string
    Conflict       ConflictState   // Conflict dialog state
}
```

### UIEntry

Represents a file or folder in the list:

```go
// File: internal/ui/renderer.go:133-140

type UIEntry struct {
    Name, Path    string
    IsDir         bool
    Size          int64
    ModTime       time.Time
    Clickable     widget.Clickable  // Gio widget state
    RightClickTag int               // For right-click detection
    LastClick     time.Time         // For double-click detection
}
```

### Renderer

Holds all widget state and rendering logic:

```go
// File: internal/ui/renderer.go:220-310

type Renderer struct {
    Theme *material.Theme

    // Scrollable lists
    listState  layout.List  // Main file list
    favState   layout.List  // Favorites list
    driveState layout.List  // Drives list

    // Navigation buttons
    backBtn, fwdBtn widget.Clickable
    homeBtn         widget.Clickable

    // Path editing
    pathEditor widget.Editor
    pathClick  widget.Clickable
    isEditing  bool

    // Search
    searchEditor       widget.Editor
    searchClearBtn     widget.Clickable
    searchActive       bool
    lastSearchQuery    string
    detectedDirectives []DetectedDirective

    // Context menu
    menuVisible      bool
    menuPos          image.Point
    menuPath         string
    menuIsDir        bool
    menuIsFav        bool
    menuIsBackground bool

    // Menu item buttons
    openBtn, copyBtn, cutBtn, pasteBtn widget.Clickable
    deleteBtn, renameBtn, favBtn       widget.Clickable
    openWithBtn                        widget.Clickable

    // File menu
    fileMenuBtn  widget.Clickable
    fileMenuOpen bool
    newWindowBtn widget.Clickable
    newFileBtn   widget.Clickable
    newFolderBtn widget.Clickable
    settingsBtn  widget.Clickable

    // Settings dialog
    settingsOpen     bool
    settingsCloseBtn widget.Clickable
    searchEngine     widget.Enum
    showDotfilesCheck widget.Bool
    darkModeCheck    widget.Bool

    // Delete confirmation
    deleteConfirmOpen bool
    deleteConfirmYes  widget.Clickable
    deleteConfirmNo   widget.Clickable

    // Inline rename
    renameIndex  int
    renameEditor widget.Editor
    renamePath   string

    // Create dialog
    createDialogOpen   bool
    createDialogIsDir  bool
    createDialogEditor widget.Editor
    createDialogOK     widget.Clickable
    createDialogCancel widget.Clickable

    // Conflict dialog
    conflictReplaceBtn  widget.Clickable
    conflictKeepBothBtn widget.Clickable
    conflictSkipBtn     widget.Clickable
    conflictStopBtn     widget.Clickable
    conflictApplyToAll  widget.Bool

    // Column sorting
    headerBtns    [4]widget.Clickable
    SortColumn    SortColumn
    SortAscending bool

    // Settings values
    ShowDotfiles   bool
    SearchEngines  []SearchEngineInfo
    SelectedEngine string
    DefaultDepth   int
    DarkMode       bool

    // Animation
    progressAnimStart time.Time
}
```

## Main Entry Point: Layout()

```go
// File: internal/ui/layout.go:24-112

func (r *Renderer) Layout(gtx layout.Context, state *State) UIEvent
```

This function:
1. Sets up keyboard focus
2. Processes global keyboard input
3. Composes the main layout (menu bar, nav bar, sidebar, file list, progress)
4. Renders overlay dialogs (context menu, settings, delete confirm, etc.)
5. Returns any UIEvent generated by user interaction

### Layout Structure

```
┌─────────────────────────────────────────────────────────┐
│  File Menu Button                                        │
├─────────────────────────────────────────────────────────┤
│  ◀ ▶ ⌂  │  /path/to/directory  │  🔍 Search...         │
├─────────┬───────────────────────────────────────────────┤
│         │  Name ▲        Date Modified    Type    Size  │
│ FAVS    ├───────────────────────────────────────────────┤
│  ...    │  Documents/    2024-01-15       Folder        │
│         │  file.txt      2024-01-14       TXT     1.2KB │
│ DRIVES  │  image.png     2024-01-13       PNG     45KB  │
│  ...    │  ...                                          │
├─────────┴───────────────────────────────────────────────┤
│  Progress Bar (when active)                              │
└─────────────────────────────────────────────────────────┘
```

## Key Functions

### Navigation Bar

```go
// File: internal/ui/layout.go:114-296

func (r *Renderer) layoutNavBar(gtx layout.Context, state *State, keyTag *layout.List, eventOut *UIEvent) layout.Dimensions
```

Contains:
- Back/Forward/Home buttons
- Path display (click to edit)
- Search box with directive pills

### Sidebar

```go
// File: internal/ui/layout.go:397-476

func (r *Renderer) layoutSidebar(gtx layout.Context, state *State, eventOut *UIEvent) layout.Dimensions
```

Contains:
- Favorites list (with right-click remove)
- Drives list

### File List

```go
// File: internal/ui/layout.go:478-620

func (r *Renderer) layoutFileList(gtx layout.Context, state *State, keyTag *layout.List, eventOut *UIEvent) layout.Dimensions
```

Features:
- Column headers (sortable)
- Scrollable list of files/folders
- Selection highlight
- Double-click to open
- Right-click context menu
- Background right-click (new file/folder)

### Row Rendering

```go
// File: internal/ui/renderer.go:515-598

func (r *Renderer) renderRow(gtx layout.Context, item *UIEntry, index int, selected bool, isRenaming bool) (layout.Dimensions, bool, bool, image.Point, *UIEvent)
```

Returns:
- Dimensions of rendered row
- Whether left-clicked
- Whether right-clicked
- Click position (for context menu)
- Rename event (if inline rename completed)

### Context Menu

```go
// File: internal/ui/layout.go:622-732

func (r *Renderer) layoutContextMenu(gtx layout.Context, state *State, eventOut *UIEvent) layout.Dimensions
```

Shows different options based on:
- File vs folder
- Background click vs item click
- Is favorite or not
- Clipboard has content or not

### Dialogs

| Function | File:Line | Purpose |
|----------|-----------|---------|
| `layoutSettingsModal` | layout.go:1228-1351 | Settings dialog |
| `layoutDeleteConfirm` | layout.go:912-969 | Delete confirmation |
| `layoutCreateDialog` | layout.go:971-1073 | New file/folder dialog |
| `layoutConflictDialog` | layout.go:1075-1213 | File conflict resolution |

### Progress Bar

```go
// File: internal/ui/layout.go:734-832

func (r *Renderer) layoutProgressBar(gtx layout.Context, state *State) layout.Dimensions
```

Two modes:
- **Determinate**: Shows percentage (copy/delete operations)
- **Indeterminate**: Animated sliding bar (content search)

## Color Definitions

```go
// File: internal/ui/renderer.go:312-323

var (
    colWhite      = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
    colBlack      = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
    colGray       = color.NRGBA{R: 100, G: 100, B: 100, A: 255}
    colLightGray  = color.NRGBA{R: 200, G: 200, B: 200, A: 255}
    colDirBlue    = color.NRGBA{R: 0, G: 0, B: 128, A: 255}
    colSelected   = color.NRGBA{R: 200, G: 220, B: 255, A: 255}
    colSidebar    = color.NRGBA{R: 245, G: 245, B: 245, A: 255}
    colDisabled   = color.NRGBA{R: 150, G: 150, B: 150, A: 255}
    colProgress   = color.NRGBA{R: 66, G: 133, B: 244, A: 255}
    colDanger     = color.NRGBA{R: 220, G: 53, B: 69, A: 255}
    colHomeBtnBg  = color.NRGBA{R: 76, G: 175, B: 80, A: 255}
    colDriveIcon  = color.NRGBA{R: 96, G: 125, B: 139, A: 255}
    colSuccess    = color.NRGBA{R: 40, G: 167, B: 69, A: 255}
    colAccent     = color.NRGBA{R: 66, G: 133, B: 244, A: 255}
    colDirective  = color.NRGBA{R: 103, G: 58, B: 183, A: 255}
    colDirectiveBg = color.NRGBA{R: 237, G: 231, B: 246, A: 255}
)
```

## Search Directive Visual Feedback

The search box shows colored pills for recognized directives:

```go
// File: internal/ui/renderer.go:166-199

func parseDirectivesForDisplay(text string) ([]DetectedDirective, string)
```

Directive colors (in `layoutSearchBox`):
- `contents:` - Orange
- `ext:` - Green
- `size:` - Blue
- `modified:` - Pink
- `recursive:`/`depth:` - Yellow
- `filename:` - Purple

## Keyboard Shortcuts (processGlobalInput)

```go
// File: internal/ui/renderer.go:384-460
```

| Key | Action |
|-----|--------|
| Up/Down | Navigate list |
| Enter | Open selected item |
| Alt+Left | Go back |
| Alt+Right | Go forward |
| Ctrl+C | Copy |
| Ctrl+X | Cut |
| Ctrl+V | Paste |
| Ctrl+N | New file |
| Ctrl+Shift+N | New folder |
| Delete | Delete selected |

## Helper Functions

### Renderer Initialization

```go
// File: internal/ui/renderer.go:325-343

func NewRenderer() *Renderer
```

### Setters for External State

```go
func (r *Renderer) SetShowDotfilesCheck(v bool)
func (r *Renderer) SetSearchEngine(engineID string)
func (r *Renderer) SetDefaultDepth(depth int)
func (r *Renderer) SetDarkMode(dark bool)
```

### Dialog Controls

```go
func (r *Renderer) ShowCreateDialog(isDir bool)
func (r *Renderer) StartRename(index int, path, name string)
func (r *Renderer) CancelRename()
```

### Utilities

```go
// File: internal/ui/renderer.go:738-749

func formatSize(bytes int64) string  // "1.2 KB", "45 MB", etc.
```

## Extension Points

### Adding a New Dialog

1. Add state fields to `Renderer`:
   ```go
   myDialogOpen bool
   myDialogBtn  widget.Clickable
   ```

2. Add layout function:
   ```go
   func (r *Renderer) layoutMyDialog(gtx layout.Context, eventOut *UIEvent) layout.Dimensions
   ```

3. Add to main Layout() stack:
   ```go
   layout.Stacked(func(gtx layout.Context) layout.Dimensions {
       return r.layoutMyDialog(gtx, &eventOut)
   }),
   ```

### Adding a New Context Menu Item

1. Add button field to `Renderer`:
   ```go
   myActionBtn widget.Clickable
   ```

2. Add UIAction constant:
   ```go
   ActionMyAction
   ```

3. Add to `layoutContextMenu`:
   ```go
   if r.myActionBtn.Clicked(gtx) {
       r.menuVisible = false
       *eventOut = UIEvent{Action: ActionMyAction, Path: r.menuPath}
   }
   return r.menuItem(gtx, &r.myActionBtn, "My Action")
   ```

4. Handle in Orchestrator's `handleUIEvent`

### Adding a New Setting

1. Add widget to `Renderer`:
   ```go
   mySettingCheck widget.Bool
   MySetting      bool
   ```

2. Add setter:
   ```go
   func (r *Renderer) SetMySetting(v bool) {
       r.MySetting = v
       r.mySettingCheck.Value = v
   }
   ```

3. Add UIAction and UIEvent field

4. Add to settings dialog layout

5. Handle in Orchestrator (save to database)
