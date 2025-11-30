# Razor File Manager - Developer Documentation

Welcome to the Razor developer documentation. This guide provides comprehensive information for contributors who want to understand, modify, or extend the codebase.

## Documentation Index

### Architecture & Design
- [Architecture Overview](./architecture.md) - High-level system design, package structure, and data flow
- [Gio UI Patterns](./gio-patterns.md) - Comprehensive guide to Gio library usage in Razor

### Package Documentation
- [App Package](./packages/app.md) - Orchestrator, event handling, and application lifecycle
- [UI Package](./packages/ui.md) - Renderer, layout, widgets, and user interaction
- [Filesystem Package](./packages/fs.md) - Directory operations, searching, and async I/O
- [Search Package](./packages/search.md) - Query parsing, directives, and external engine integration
- [Store Package](./packages/store.md) - SQLite persistence for favorites and settings

### Guides
- [Contributing Guide](./contributing.md) - How to set up, build, and contribute
- [Adding Features](./guides/adding-features.md) - Step-by-step guide for common modifications

## Quick Start for Contributors

### Building
```bash
# Standard build
go build -o razor_bin ./cmd/razor

# Debug build (with logging)
go build -tags debug -o razor_bin ./cmd/razor
```

### Project Structure
```
razor/
├── cmd/razor/           # Entry point
├── internal/
│   ├── app/             # Application orchestration
│   ├── ui/              # Gio UI components
│   ├── fs/              # Filesystem operations
│   ├── search/          # Search engine
│   └── store/           # SQLite persistence
├── docs/                # This documentation
└── TODO.org             # Project roadmap
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
| File operations | `internal/fs/system.go:Start()` |
| Search logic | `internal/search/query.go` |
| Settings storage | `internal/store/db.go` |

## Technology Stack

- **UI Framework**: [Gio](https://gioui.org) - Immediate-mode GUI in Go
- **Database**: SQLite via [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) (pure Go)
- **External Search**: Optional ripgrep/ugrep integration
- **Platforms**: Linux, macOS, Windows
