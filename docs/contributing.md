# Contributing Guide

Welcome! This guide will help you set up your development environment and understand how to contribute to Razor.

## Prerequisites

- **Go 1.21+** - [Download](https://go.dev/dl/)
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

### Enable Debug Logging

```bash
go build -tags debug -o razor_bin ./cmd/razor
./razor_bin 2>&1 | grep DEBUG
```

### Add Debug Logs

```go
// In internal/app/orchestrator.go or other app package files
debugLog("My message: %v", value)
```

### Common Debug Tags

```
[DEBUG]           - General debug
[FS]              - Filesystem operations
[FS_SEARCH]       - Search operations
[FS_RESP]         - Response handling
[SEARCH]          - Search engine selection
[SETTINGS]        - Settings changes
[CLEAR]           - Search clearing
```

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
