# Razor File Manager - Developer Documentation

Welcome to the Razor developer documentation. This guide provides comprehensive information for contributors who want to understand, modify, or extend the codebase.

## Documentation Index

### Architecture & Design
- [Architecture Overview](./architecture.md) - High-level system design, package structure, and data flow
- [Gio UI Patterns](./gio-patterns.md) - Comprehensive guide to Gio library usage in Razor

### Package Documentation
- [App Package](./packages/app.md) - Orchestrator, controllers, event handling, and application lifecycle
- [UI Package](./packages/ui.md) - Renderer, layout, widgets, and user interaction
- [Filesystem Package](./packages/fs.md) - Directory operations, searching, and async I/O
- [Search Package](./packages/search.md) - Query parsing, directives, and external engine integration
- [Store Package](./packages/store.md) - SQLite persistence for search history, recent files, and settings
- [Debug Package](./packages/debug.md) - Categorized debug logging system
- [Config Package](./packages/config.md) - Configuration file management and hotkeys
- [Trash Package](./packages/trash.md) - Cross-platform trash/recycle bin support

### Guides
- [Contributing Guide](./contributing.md) - How to set up, build, and contribute
- [Drag and Drop](./drag-and-drop.md) - Implementing drag-and-drop in Gio
- [Touchable Widget](./touchable.md) - Unified click, right-click, and drag handling

## Quick Start for Contributors

### Building
```bash
# Standard build
go build -o razor ./cmd/razor

# Debug build (with logging)
go build -tags debug -o razor ./cmd/razor
```

### Project Structure
```
razor/
├── cmd/razor/              # Entry point
│   ├── main.go             # CLI flags and app bootstrap
│   └── console_*.go        # Platform-specific console handling
│
├── internal/
│   ├── app/                # Application orchestration
│   │   ├── orchestrator.go # Central event loop controller
│   │   ├── state_owner.go  # Canonical file entry cache
│   │   ├── controllers.go  # Shared dependencies and types
│   │   ├── nav_controller.go    # Navigation history, path expansion
│   │   ├── search_controller.go # Search execution, engine management
│   │   ├── tabs.go         # Tab state management
│   │   ├── file_ops.go     # File operations (copy, paste, delete, rename)
│   │   ├── conflict.go     # File conflict resolution
│   │   ├── watcher.go      # Directory change watcher (fsnotify)
│   │   └── platform_*.go   # Platform-specific file opening
│   │
│   ├── ui/                 # Gio UI components (~30 files)
│   │   ├── renderer.go     # Main UI renderer
│   │   ├── types.go        # UI actions, events, state types
│   │   ├── layout.go       # Top-level layout composition
│   │   ├── layout_*.go     # Specific UI sections (navbar, sidebar, filelist, etc.)
│   │   ├── renderer_*.go   # Specific rendering logic (rows, preview, tabs, input)
│   │   ├── colors.go       # Theme color definitions
│   │   ├── tabs.go         # Reusable tab bar component
│   │   ├── markdown.go     # Markdown rendering
│   │   ├── orgmode.go      # Org-mode rendering
│   │   ├── toast.go        # Toast notification UI
│   │   └── thumbnail_cache.go # Image thumbnail caching
│   │
│   ├── fs/                 # Filesystem operations
│   │   ├── system.go       # Async file operations, search coordination
│   │   └── drives_*.go     # Platform-specific drive enumeration
│   │
│   ├── search/             # Search engine abstraction
│   │   ├── engine.go       # Engine detection (builtin, ripgrep, ugrep)
│   │   └── query.go        # Query parsing with directives
│   │
│   ├── config/             # Configuration management
│   │   ├── config.go       # Config loading, defaults, persistence
│   │   ├── hotkeys.go      # Hotkey parsing and matching
│   │   ├── hotkeys_*.go    # Platform-specific default hotkeys
│   │   └── terminals_*.go  # Platform-specific terminal detection
│   │
│   ├── store/              # SQLite persistence
│   │   └── db.go           # Search history, recent files, settings
│   │
│   ├── trash/              # Trash/recycle bin support
│   │   ├── trash.go        # Cross-platform API
│   │   └── trash_*.go      # Platform-specific implementations
│   │
│   ├── platform/           # Platform-specific features
│   │   └── drop_*.go       # External drag-drop handling
│   │
│   └── debug/              # Conditional debug logging
│       ├── debug_on.go     # Debug build: enables logging
│       └── debug_off.go    # Release build: no-op logging
│
├── docs/                   # This documentation
└── TODO.org                # Project roadmap
```

### Key Concepts

1. **Immediate-Mode UI**: Gio redraws every frame from state - no retained UI tree
2. **Channel Architecture**: Subsystems communicate via buffered channels
3. **Generation Counters**: Prevent stale async results from displaying
4. **Context Cancellation**: Long operations can be cancelled gracefully

### Code Navigation Tips

| To understand... | Start with... |
|-----------------|---------------|
| Main event loop | `internal/app/orchestrator.go:Run()` |
| UI rendering | `internal/ui/layout.go:Layout()` |
| File list rendering | `internal/ui/layout_filelist.go` |
| Grid view | `internal/ui/layout_grid.go` |
| File operations | `internal/app/file_ops.go` |
| Navigation | `internal/app/nav_controller.go` |
| Search logic | `internal/app/search_controller.go` |
| Query parsing | `internal/search/query.go` |
| Settings storage | `internal/store/db.go` |
| Tab management | `internal/app/tabs.go` |

## Technology Stack

- **UI Framework**: [Gio](https://gioui.org) - Immediate-mode GUI in Go
- **Database**: SQLite via [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) (pure Go)
- **External Search**: Optional ripgrep/ugrep integration
- **Platforms**: Linux, macOS, Windows
