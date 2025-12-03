# Contributing Guide

Welcome! This guide will help you set up your development environment and understand how to contribute to Razor.

## Prerequisites

- **Go 1.25+** - [Download](https://go.dev/dl/)
- **Git** - For version control
- **Optional**: ripgrep (`rg`) or ugrep (`ug`) for faster content search

### Platform-Specific Requirements

**macOS**: Xcode Command Line Tools
```bash
xcode-select --install
```

**Linux**: Development packages
```bash
# Debian/Ubuntu
sudo apt-get install libgl1-mesa-dev xorg-dev

# Fedora
sudo dnf install mesa-libGL-devel libX11-devel libXcursor-devel libXrandr-devel libXinerama-devel libXi-devel libXxf86vm-devel
```

**Windows**: No additional requirements

## Getting Started

### Clone the Repository

```bash
git clone https://github.com/justyntemme/razor.git
cd razor
```

### Build

```bash
# Standard build
go build -o razor_bin ./cmd/razor

# Debug build (with logging)
go build -tags debug -o razor_bin ./cmd/razor

# Run
./razor_bin

# Run with specific start path
./razor_bin -path /path/to/directory
```

### Run Tests

```bash
go test ./...
```

## Project Structure

```
razor/
├── cmd/razor/              # Entry point
│   ├── main.go             # CLI flags, starts app
│   └── console_*.go        # Platform-specific console handling
│
├── internal/
│   ├── app/                # Application orchestration
│   │   ├── orchestrator.go # Main controller
│   │   ├── platform_*.go   # Platform-specific operations
│   │   └── debug_*.go      # Debug logging
│   │
│   ├── ui/                 # Gio UI components
│   │   ├── renderer.go     # Types and widget state
│   │   └── layout.go       # Layout composition
│   │
│   ├── fs/                 # Filesystem operations
│   │   ├── system.go       # Async file operations
│   │   └── drives_*.go     # Drive enumeration
│   │
│   ├── search/             # Search engine
│   │   ├── engine.go       # External engine detection
│   │   └── query.go        # Query parsing
│   │
│   ├── config/             # Configuration management
│   │   ├── config.go       # Config file loading/saving
│   │   └── hotkeys.go      # Hotkey parsing
│   │
│   ├── trash/              # Trash/recycle bin
│   │   ├── trash.go        # Cross-platform API
│   │   └── trash_*.go      # Platform implementations
│   │
│   └── store/              # Persistence
│       └── db.go           # SQLite operations
│
├── docs/                   # Documentation
└── TODO.org                # Project roadmap
```

## Development Workflow

### 1. Create a Branch

```bash
git checkout -b feature/my-feature
# or
git checkout -b fix/bug-description
```

### 2. Make Changes

Follow the existing code style:
- Use `gofmt` for formatting
- Keep functions focused and small
- Add comments for complex logic
- Update documentation if needed

### 3. Test Your Changes

```bash
# Run the app
go build -tags debug -o razor_bin ./cmd/razor && ./razor_bin

# Watch logs for debug output
# Logs are printed to stderr
```

### 4. Commit

```bash
git add .
git commit -m "feat: add my feature"
```

Commit message prefixes:
- `feat:` - New feature
- `fix:` - Bug fix
- `docs:` - Documentation
- `refactor:` - Code refactoring
- `test:` - Tests
- `chore:` - Maintenance

### 5. Push and Create PR

```bash
git push origin feature/my-feature
```

Then create a Pull Request on GitHub.

## Common Tasks

### Adding a New UIAction

1. **Add the action constant** in `internal/ui/renderer.go`:
   ```go
   const (
       // ...existing...
       ActionMyAction
   )
   ```

2. **Add UIEvent fields** if needed:
   ```go
   type UIEvent struct {
       // ...existing...
       MyField string
   }
   ```

3. **Emit the event** from UI code in `internal/ui/layout.go`:
   ```go
   if r.myButton.Clicked(gtx) {
       *eventOut = UIEvent{Action: ActionMyAction, MyField: "value"}
   }
   ```

4. **Handle the event** in `internal/app/orchestrator.go`:
   ```go
   case ui.ActionMyAction:
       o.doMyAction(evt.MyField)
   ```

### Adding a New Setting

1. **Add UI widget** in `internal/ui/renderer.go`:
   ```go
   type Renderer struct {
       // ...
       mySettingCheck widget.Bool
       MySetting      bool
   }
   ```

2. **Add setter**:
   ```go
   func (r *Renderer) SetMySetting(v bool) {
       r.MySetting = v
       r.mySettingCheck.Value = v
   }
   ```

3. **Add to settings dialog** in `internal/ui/layout.go`:
   ```go
   layout.Rigid(func(gtx layout.Context) layout.Dimensions {
       if r.mySettingCheck.Update(gtx) {
           r.MySetting = r.mySettingCheck.Value
           *eventOut = UIEvent{Action: ActionMySettingChanged, ...}
       }
       cb := material.CheckBox(r.Theme, &r.mySettingCheck, "My Setting")
       return cb.Layout(gtx)
   }),
   ```

4. **Handle in orchestrator** - save to database:
   ```go
   case ui.ActionMySettingChanged:
       o.store.RequestChan <- store.Request{
           Op: store.SaveSetting,
           Key: "my_setting",
           Value: fmt.Sprint(evt.MySetting),
       }
   ```

5. **Load on startup** in `handleStoreResponse`:
   ```go
   if val, ok := resp.Settings["my_setting"]; ok {
       o.ui.SetMySetting(val == "true")
   }
   ```

### Adding a New Search Directive

1. **Add directive type** in `internal/search/query.go`:
   ```go
   const (
       // ...existing...
       DirMyDirective
   )
   ```

2. **Parse in Parse()**:
   ```go
   case strings.HasPrefix(lower, "mydir:"):
       value := part[7:]
       directives = append(directives, Directive{
           Type:  DirMyDirective,
           Value: value,
       })
   ```

3. **Match in Matcher.Matches()**:
   ```go
   case DirMyDirective:
       if !matchMyDirective(entry, dir.Value) {
           return false
       }
   ```

4. **Add visual pill** in `internal/ui/renderer.go`:
   ```go
   // In parseDirectivesForDisplay
   knownDirectives := []string{
       // ...existing...
       "mydir:",
   }
   ```

### Adding Platform-Specific Code

1. Create files with build tags:
   ```
   myfeature_darwin.go   // macOS
   myfeature_linux.go    // Linux
   myfeature_windows.go  // Windows
   ```

2. Each file has a build constraint comment at top (Go 1.17+):
   ```go
   //go:build darwin

   package app
   ```

3. Implement the same function in each file:
   ```go
   func myFeature() error {
       // Platform-specific implementation
   }
   ```

## Code Style

### REQUIRED: Use fastwalk for Directory Operations

**All filesystem walking/reading code MUST use [fastwalk](https://github.com/charlievieth/fastwalk)** instead of `os.ReadDir()`, `os.ReadDirEntry()`, or `filepath.Walk()`.

```go
import "github.com/charlievieth/fastwalk"

// Directory listing
conf := &fastwalk.Config{Follow: true}
fastwalk.Walk(conf, path, func(fullPath string, d fs.DirEntry, err error) error {
    if err != nil {
        return nil // Skip errors
    }
    // Use fastwalk.StatDirEntry for file info (follows symlinks)
    info, err := fastwalk.StatDirEntry(fullPath, d)
    // Use fastwalk.DirEntryDepth(d) for depth checking
    return nil
})
```

Fastwalk provides:
- Parallel directory walking with worker pools
- Efficient symlink handling via `StatDirEntry()`
- Built-in depth tracking via `DirEntryDepth()`
- Better performance for large directories

### Naming Conventions

- **Exported**: `PascalCase` (e.g., `UIEvent`, `FetchDir`)
- **Unexported**: `camelCase` (e.g., `handleUIEvent`, `doSearch`)
- **Constants**: `PascalCase` or `camelCase` depending on export
- **Acronyms**: Keep consistent (`UIEvent`, not `UiEvent`)

### Error Handling

```go
// Log and continue (most common in Razor)
if err != nil {
    log.Printf("Operation failed: %v", err)
    return
}

// Return error to caller
if err != nil {
    return fmt.Errorf("operation failed: %w", err)
}
```

### Channel Patterns

```go
// Non-blocking send with select
select {
case ch <- value:
default:
    // Channel full, handle accordingly
}

// Receive with timeout
select {
case value := <-ch:
    // Handle value
case <-time.After(5 * time.Second):
    // Timeout
}
```

### Gio Patterns

See [Gio Patterns Guide](./gio-patterns.md) for detailed Gio usage.

Key rules:
- Check `widget.Clicked(gtx)` **before** layout
- Use `material.Clickable(gtx, &btn, content)` for click handling
- Return `layout.Dimensions` from all layout functions
- Use `defer clip.Rect{...}.Push(gtx.Ops).Pop()` for clipping

## Debugging

Razor uses a centralized debug logging system in `internal/debug/` with categorized output.

### Enable Debug Logging

```bash
# Build with debug enabled
go build -tags debug -o razor_bin ./cmd/razor

# Run and view all debug output
./razor_bin 2>&1

# Filter by category
./razor_bin 2>&1 | grep '\[FS\]'
./razor_bin 2>&1 | grep '\[SEARCH\]'
```

### Debug Categories

| Category | Description |
|----------|-------------|
| `APP` | Application orchestration, navigation, state |
| `FS` | Filesystem operations (fetch, walk) |
| `SEARCH` | Search engine, query parsing, matching |
| `STORE` | Database operations, settings, favorites |
| `UI` | UI events, layout, rendering |
| `FS_ENTRY` | Individual entry processing (verbose) |
| `FS_WALK` | Directory walking during search (verbose) |
| `UI_EVENT` | UI event handling (verbose) |
| `UI_LAYOUT` | Layout calculations (extremely verbose) |

### Environment Variable Control

Use `RAZOR_DEBUG` to control which categories are enabled:

```bash
# Enable only specific categories
RAZOR_DEBUG=FS,SEARCH ./razor_bin

# Enable all categories (including verbose)
RAZOR_DEBUG=all ./razor_bin

# Disable all categories
RAZOR_DEBUG=none ./razor_bin
```

### Add Debug Logs

```go
import "github.com/justyntemme/razor/internal/debug"

// Log with a category
debug.Log(debug.FS, "fetchDir: reading %q", path)
debug.Log(debug.SEARCH, "query=%q engine=%d", query, engine)
debug.Log(debug.APP, "Settings: dark_mode=%v", darkMode)

// Use verbose categories for detailed tracing
debug.Log(debug.FS_ENTRY, "entry %q isDir=%v size=%d", name, isDir, size)
```

### Debug Package API

```go
// Check if debug is enabled (compile-time constant)
if debug.Enabled {
    // expensive debug-only operations
}

// Enable/disable categories at runtime
debug.Enable(debug.FS_ENTRY)   // Enable verbose entry logging
debug.Disable(debug.UI_LAYOUT) // Disable layout spam

// Check category state
if debug.IsEnabled(debug.SEARCH) {
    // conditional logging
}

// List enabled categories
cats := debug.ListEnabled()
```

### Release Builds

In release builds (without `-tags debug`), all debug logging is compiled out:
- `debug.Log()` becomes a no-op
- `debug.Enabled` is `false`
- No runtime overhead

## Documentation

When adding features:

1. Update `TODO.org` if implementing a planned feature
2. Add/update package documentation in `docs/packages/`
3. Update `docs/README.md` if adding major features
4. Add code comments for complex logic

## Getting Help

- Check existing [documentation](./README.md)
- Review `TODO.org` for project roadmap
- Open an issue for questions or bugs
