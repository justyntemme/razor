# Config Package (`internal/config`)

The config package handles loading, saving, and managing application configuration from `~/.config/razor/config.json`.

## Files

| File | Purpose |
|------|---------|
| `config.go` | Configuration structs, loading, saving, and defaults |
| `hotkeys.go` | Hotkey parsing and matching logic |
| `hotkeys_darwin.go` | macOS default hotkeys |
| `hotkeys_linux.go` | Linux default hotkeys |
| `hotkeys_windows.go` | Windows default hotkeys |
| `terminals_darwin.go` | macOS terminal configuration |
| `terminals_linux.go` | Linux terminal configuration |
| `terminals_windows.go` | Windows terminal configuration |

## Configuration File Location

All platforms use the same path:
```
~/.config/razor/config.json
```

## Configuration Structure

### Top-Level Config

```go
type Config struct {
    UI        UIConfig        `json:"ui"`
    Search    SearchConfig    `json:"search"`
    Behavior  BehaviorConfig  `json:"behavior"`
    Tabs      TabsConfig      `json:"tabs"`
    Panels    PanelsConfig    `json:"panels"`
    Preview   PreviewConfig   `json:"preview"`
    Terminal  TerminalConfig  `json:"terminal"`
    Hotkeys   HotkeysConfig   `json:"hotkeys"`
    Favorites []FavoriteEntry `json:"favorites"`
}
```

### UI Configuration

```go
type UIConfig struct {
    Theme     string          `json:"theme"`     // "light" or "dark"
    Sidebar   SidebarConfig   `json:"sidebar"`
    Toolbar   ToolbarConfig   `json:"toolbar"`
    FileList  FileListConfig  `json:"fileList"`
    StatusBar StatusBarConfig `json:"statusBar"`
}

type SidebarConfig struct {
    Layout        string `json:"layout"`        // "tabbed" | "stacked" | "favorites_only" | "drives_only"
    TabStyle      string `json:"tabStyle"`      // "manila" | "underline" | "pill"
    Width         int    `json:"width"`
    Position      string `json:"position"`      // "left" | "right"
    ShowFavorites bool   `json:"showFavorites"`
    ShowDrives    bool   `json:"showDrives"`
    Collapsible   bool   `json:"collapsible"`
}

type FileListConfig struct {
    ShowDotfiles  bool   `json:"showDotfiles"`
    DefaultSort   string `json:"defaultSort"`   // "name" | "date" | "type" | "size"
    SortAscending bool   `json:"sortAscending"`
    RowHeight     string `json:"rowHeight"`     // "compact" | "normal" | "comfortable"
    ShowIcons     bool   `json:"showIcons"`
}
```

### Preview Configuration

```go
type PreviewConfig struct {
    Enabled          bool     `json:"enabled"`
    Position         string   `json:"position"`         // "right" | "bottom"
    WidthPercent     int      `json:"widthPercent"`     // Percentage of screen width
    TextExtensions   []string `json:"textExtensions"`   // Extensions for text preview
    ImageExtensions  []string `json:"imageExtensions"`  // Extensions for image preview
    MaxFileSize      int64    `json:"maxFileSize"`      // Max preview size in bytes
    MarkdownRendered bool     `json:"markdownRendered"` // Default markdown view mode
}
```

### Hotkeys Configuration

```go
type HotkeysConfig struct {
    // File operations
    Copy            string `json:"copy"`
    Cut             string `json:"cut"`
    Paste           string `json:"paste"`
    Delete          string `json:"delete"`
    PermanentDelete string `json:"permanentDelete"` // Bypass trash
    Rename          string `json:"rename"`
    NewFile         string `json:"newFile"`
    NewFolder       string `json:"newFolder"`
    SelectAll       string `json:"selectAll"`

    // Navigation
    Back    string `json:"back"`
    Forward string `json:"forward"`
    Up      string `json:"up"`
    Home    string `json:"home"`
    Refresh string `json:"refresh"`

    // UI
    FocusSearch   string `json:"focusSearch"`
    TogglePreview string `json:"togglePreview"`
    ToggleHidden  string `json:"toggleHidden"`
    Escape        string `json:"escape"`

    // Tabs
    NewTab     string `json:"newTab"`
    NewTabHome string `json:"newTabHome"`
    CloseTab   string `json:"closeTab"`
    NextTab    string `json:"nextTab"`
    PrevTab    string `json:"prevTab"`
    Tab1-Tab6  string `json:"tab1"` // ... through tab6
}
```

## Manager API

### Creating a Manager

```go
manager := config.NewManager()
```

### Loading Configuration

```go
err := manager.Load()
// If config doesn't exist, creates default
// If JSON parse fails, stores error and uses defaults
```

### Getting Configuration

```go
cfg := manager.Get() // Returns copy of current config

// Or use specific getters
isDark := manager.IsDarkMode()
layout := manager.GetSidebarLayout()
preview := manager.GetPreviewConfig()
hotkeys := manager.GetHotkeys()
```

### Saving Settings

```go
manager.SetTheme("dark")
manager.SetShowDotfiles(true)
manager.SetSearchEngine("ripgrep")
manager.SetDefaultDepth(5)
manager.SetSidebarLayout("tabbed")
```

### Managing Favorites

```go
manager.AddFavorite("Projects", "/home/user/projects")
manager.RemoveFavorite("/home/user/old")
favorites := manager.GetFavorites()
```

### Generating Fresh Config

```go
// Backs up existing config and creates fresh default
backupPath, err := config.GenerateConfig()
```

## Hotkey Parsing

The hotkeys module parses key combinations:

```go
// Supported modifiers
Ctrl, Shift, Alt, Cmd (or Super, Meta, Command)

// Supported keys
A-Z, 0-9, F1-F12
Delete, Backspace, Escape, Enter, Tab, Space
Up, Down, Left, Right, Home, End, PageUp, PageDown

// Examples
"Ctrl+C"
"Ctrl+Shift+N"
"Alt+Left"
"Cmd+Shift+." (macOS)
"F2"
```

### Platform Defaults

Different platforms have different default hotkeys:

**macOS:**
- Navigation uses Cmd+Arrow
- Toggle hidden uses Cmd+Shift+.
- Delete uses Backspace

**Windows/Linux:**
- Navigation uses Alt+Arrow
- Toggle hidden uses Ctrl+.
- Delete uses Delete key

## Error Handling

```go
// Check for parse errors after Load()
if err := manager.ParseError(); err != nil {
    // Config file had JSON syntax errors
    // Application is running with defaults
    log.Printf("Config parse error: %v", err)
}
```

## Extension Points

### Adding a New Setting

1. Add field to appropriate config struct
2. Add to `DefaultConfig()` function
3. Add getter/setter methods to Manager
4. Update JSON parsing (automatic for struct fields)

### Adding Platform-Specific Defaults

1. Create `setting_darwin.go`, `setting_linux.go`, `setting_windows.go`
2. Each file provides platform-specific default value
3. Use in `DefaultConfig()` or `DefaultHotkeys()`
