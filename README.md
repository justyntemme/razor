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

