# Razor

A fast, lightweight file manager built with Go and [Gio UI](https://gioui.org/).

![Razor Screenshot](docs/screenshot.png)

## Features

- **Fast Navigation** - Keyboard-driven with mouse support
- **Sortable Columns** - Click column headers to sort by name, date, type, or size
- **Favorites Sidebar** - Quick access to frequently used directories
- **Advanced Search** - Filename, content, extension, size, and date filtering
- **File Operations** - Copy, cut, paste, delete with progress tracking
- **Dotfiles Toggle** - Show/hide hidden files
- **Cross-Platform** - Works on Linux, macOS, and Windows

## Installation

```bash
go install github.com/justyntemme/razor/cmd/razor@latest
```

Or build from source:

```bash
git clone https://github.com/justyntemme/razor.git
cd razor
go build -o razor ./cmd/razor
```

### Debug Build

To enable debug logging:

```bash
go build -tags debug -o razor ./cmd/razor
```

## Usage

```bash
# Start in home directory
razor

# Start in specific directory
razor -path /some/directory

# Generate fresh config.json (backs up existing with timestamp)
razor --generate-config
```

## Search

The search bar supports powerful directive-based filtering. By default, searches match filenames only.

### Basic Search

Just type to search filenames:

```
report          # Files containing "report" in name
*.go            # Files matching glob pattern
test*           # Files starting with "test"
```

### Search Directives

Use directives for advanced filtering:

| Directive | Description | Example |
|-----------|-------------|---------|
| `filename:` | Search by filename (default) | `filename:readme` |
| `contents:` | Search file contents | `contents:TODO` |
| `ext:` | Filter by extension | `ext:go` or `ext:.go` |
| `size:` | Filter by file size | `size:>1MB` |
| `modified:` | Filter by modification date | `modified:>2024-01-01` |

### Size Operators

```
size:>1MB       # Larger than 1 megabyte
size:<100KB     # Smaller than 100 kilobytes
size:>=500B     # At least 500 bytes
size:1GB        # Exactly 1 gigabyte
```

Supported units: `B`, `KB`, `MB`, `GB`

### Date Operators

```
modified:>2024-01-01    # Modified after January 1, 2024
modified:<2024-06-15    # Modified before June 15, 2024
modified:>=2024-01      # Modified in or after January 2024
modified:today          # Modified today
modified:yesterday      # Modified yesterday
modified:week           # Modified in the last week
modified:month          # Modified in the last month
modified:year           # Modified in the last year
```

### Combining Directives

Multiple directives are combined with AND logic:

```
ext:go contents:func        # Go files containing "func"
ext:md modified:>week       # Markdown files modified in the last week
size:>1MB ext:log           # Log files larger than 1MB
*.go size:<100KB            # Small Go files
```

### Examples

```
# Find all Go test files
*_test.go

# Find large video files
ext:mp4 size:>100MB

# Find recently modified configuration files
ext:yaml modified:>week

# Find TODO comments in Python files
ext:py contents:TODO

# Find old log files
ext:log modified:<2024-01-01 size:>10MB
```

## Configuration

Configuration is stored in `~/.config/razor/config.json` on all platforms. The file is created with defaults on first run.

### Example config.json

```json
{
  "ui": {
    "theme": "light",
    "sidebar": {
      "layout": "stacked",
      "tabStyle": "manila",
      "width": 200,
      "position": "left",
      "showFavorites": true,
      "showDrives": true,
      "collapsible": true
    },
    "fileList": {
      "showDotfiles": false,
      "defaultSort": "name",
      "sortAscending": true,
      "rowHeight": "normal",
      "showIcons": true
    }
  },
  "search": {
    "engine": "builtin",
    "defaultDepth": 2,
    "rememberLastQuery": false
  },
  "behavior": {
    "confirmDelete": true,
    "doubleClickToOpen": true,
    "restoreLastPath": true,
    "singleClickToSelect": true
  },
  "favorites": [
    {"name": "Home", "path": "/Users/you", "icon": "home"},
    {"name": "Documents", "path": "/Users/you/Documents", "icon": "folder"}
  ]
}
```

### Sidebar Options

**Layout** (`ui.sidebar.layout`):
- `"stacked"` - Show both Favorites and Drives stacked vertically (default)
- `"tabbed"` - Tabs to switch between Favorites and Drives
- `"favorites_only"` - Only show Favorites
- `"drives_only"` - Only show Drives

**Tab Style** (`ui.sidebar.tabStyle`) - Only applies when layout is `"tabbed"`:
- `"manila"` - Vertical folder-style tabs on left side with rotated text (default)
- `"underline"` - Horizontal tabs at top with underline indicator (legacy style)
- `"pill"` - Horizontal tabs with pill/rounded background

To get the legacy horizontal tab layout, set:
```json
{
  "ui": {
    "sidebar": {
      "layout": "tabbed",
      "tabStyle": "underline"
    }
  }
}
```

### Preview Pane

**Preview** (`preview`):
- `enabled` - Whether preview is enabled (default: `true`)
- `position` - Position of preview pane: `"right"` | `"bottom"` (default: `"right"`)
- `widthPercent` - Width as percentage of screen (default: `33` for 1/3)
- `textExtensions` - File extensions to preview (default: `[".txt", ".json", ".csv", ".md", ".log", ".xml", ".yaml", ".yml", ".toml", ".ini", ".conf", ".cfg"]`)
- `maxFileSize` - Maximum file size to preview in bytes (default: `1048576` = 1MB)

When you click a file with a supported extension, the preview pane opens on the right. JSON files are automatically formatted with indentation. Press Escape or navigate away to close the preview.

### Keyboard Shortcuts

All keyboard shortcuts are configurable via the `hotkeys` section in config.json. Default shortcuts vary by platform (macOS uses Cmd for navigation, Windows/Linux use Alt).

#### Default Shortcuts

| Action | macOS | Windows/Linux |
|--------|-------|---------------|
| **File Operations** | | |
| Copy | Ctrl+C | Ctrl+C |
| Cut | Ctrl+X | Ctrl+X |
| Paste | Ctrl+V | Ctrl+V |
| Delete | Backspace | Delete |
| Rename | F2 | F2 |
| New File | Ctrl+N | Ctrl+N |
| New Folder | Ctrl+Shift+N | Ctrl+Shift+N |
| Select All | Ctrl+A | Ctrl+A |
| **Navigation** | | |
| Back | Cmd+Left | Alt+Left |
| Forward | Cmd+Right | Alt+Right |
| Up | Cmd+Up | Alt+Up |
| Home | Cmd+Shift+H | Alt+Home |
| Refresh | F5 | F5 |
| **UI** | | |
| Focus Search | Ctrl+F | Ctrl+F |
| Toggle Preview | Ctrl+P | Ctrl+P |
| Toggle Hidden | Cmd+Shift+> | Alt+Shift+> |
| Escape | Escape | Escape |
| **Tabs (Vim-style)** | | |
| New Tab | Ctrl+T | Ctrl+T |
| Close Tab | Ctrl+W | Ctrl+W |
| Next Tab | Ctrl+L | Ctrl+L |
| Previous Tab | Ctrl+H | Ctrl+H |
| Switch to Tab 1-6 | Ctrl+Shift+1-6 | Ctrl+Shift+1-6 |

#### Custom Hotkeys

Override any shortcut in your config.json:

```json
{
  "hotkeys": {
    "copy": "Ctrl+C",
    "cut": "Ctrl+X",
    "paste": "Ctrl+V",
    "delete": "Delete",
    "rename": "F2",
    "newFile": "Ctrl+N",
    "newFolder": "Ctrl+Shift+N",
    "selectAll": "Ctrl+A",
    "back": "Alt+Left",
    "forward": "Alt+Right",
    "up": "Alt+Up",
    "home": "Alt+Home",
    "refresh": "F5",
    "focusSearch": "Ctrl+F",
    "togglePreview": "Ctrl+P",
    "toggleHidden": "Alt+Shift+>",
    "escape": "Escape",
    "newTab": "Ctrl+T",
    "closeTab": "Ctrl+W",
    "nextTab": "Ctrl+L",
    "prevTab": "Ctrl+H",
    "tab1": "Ctrl+Shift+1",
    "tab2": "Ctrl+Shift+2",
    "tab3": "Ctrl+Shift+3",
    "tab4": "Ctrl+Shift+4",
    "tab5": "Ctrl+Shift+5",
    "tab6": "Ctrl+Shift+6"
  }
}
```

Supported modifiers: `Ctrl`, `Shift`, `Alt`, `Cmd` (or `Super`, `Meta`, `Command`)

Supported keys: `A-Z`, `0-9`, `F1-F12`, `Delete`, `Backspace`, `Escape`, `Enter`, `Tab`, `Space`, `Up`, `Down`, `Left`, `Right`, `Home`, `End`, `PageUp`, `PageDown`

### Search Engines

**Engine** (`search.engine`):
- `"builtin"` - Built-in Go-based search (always available)
- `"ripgrep"` - Use ripgrep for content search (must be installed)
- `"ugrep"` - Use ugrep for content search (must be installed)

### Data Storage

Additional data (search history, etc.) is stored in:

- All platforms: `~/.config/razor/razor.db` (SQLite)

## Architecture

```
razor/
├── cmd/razor/           # Application entry point
│   ├── main.go
│   ├── console_*.go     # Platform-specific console handling
├── internal/
│   ├── app/             # Application orchestrator
│   │   ├── orchestrator.go
│   │   ├── debug_on.go  # Debug build logging
│   │   ├── debug_off.go # Release build (no-op)
│   │   └── open_*.go    # Platform-specific file opening
│   ├── fs/              # Filesystem operations
│   │   └── system.go
│   ├── search/          # Search query parser
│   │   └── query.go
│   ├── store/           # SQLite persistence
│   │   └── db.go
│   └── ui/              # Gio UI components
│       ├── renderer.go
│       └── layout.go
```

## Dependencies

- [Gio](https://gioui.org/) - Immediate mode GUI
- [go-sqlite3](https://github.com/mattn/go-sqlite3) - SQLite driver

## License

MIT License - see [LICENSE](LICENSE) for details.

