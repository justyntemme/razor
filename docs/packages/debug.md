# Debug Package (`internal/debug`)

The debug package provides a centralized, categorized debug logging system that compiles to no-ops in release builds.

## Files

| File | Purpose |
|------|---------|
| `debug_on.go` | Active implementation (build tag: debug) |
| `debug_off.go` | No-op implementation (release build) |

## Build Tags

```bash
# Debug build - logging enabled
go build -tags debug -o razor_bin ./cmd/razor

# Release build - logging disabled (no-op)
go build -o razor_bin ./cmd/razor
```

## Categories

Categories allow granular control over which subsystems produce debug output.

### Core Categories (enabled by default in debug builds)

| Category | Description | Example Output |
|----------|-------------|----------------|
| `APP` | Application orchestration | Navigation, state changes, settings |
| `FS` | Filesystem operations | Directory fetch, search requests |
| `SEARCH` | Search functionality | Query parsing, engine selection |
| `STORE` | Database operations | Settings load/save, favorites |
| `UI` | User interface | Events, theme changes |

### Verbose Categories (disabled by default)

| Category | Description | When to Use |
|----------|-------------|-------------|
| `FS_ENTRY` | Per-entry processing | Debugging specific file handling |
| `FS_WALK` | Directory traversal | Debugging recursive searches |
| `UI_EVENT` | UI event handling | Debugging click/input handling |
| `UI_LAYOUT` | Layout calculations | Debugging rendering issues |

## API

### Logging

```go
import "github.com/justyntemme/razor/internal/debug"

// Basic logging with category
debug.Log(debug.FS, "fetchDir: reading %q", path)

// Format string supports all fmt.Printf verbs
debug.Log(debug.SEARCH, "query=%q engine=%d depth=%d", query, engine, depth)
```

### Compile-Time Check

```go
// Enabled is a compile-time constant
// In release builds, this entire block is eliminated
if debug.Enabled {
    expensiveDebugOperation()
}
```

### Runtime Category Control

```go
// Enable a category
debug.Enable(debug.FS_ENTRY)

// Disable a category
debug.Disable(debug.UI_LAYOUT)

// Check if enabled
if debug.IsEnabled(debug.SEARCH) {
    // ...
}

// Bulk enable/disable
debug.EnableAll()
debug.DisableAll()

// Set multiple categories
debug.SetCategories(map[debug.Category]bool{
    debug.FS:       true,
    debug.FS_ENTRY: true,
    debug.SEARCH:   false,
})

// List currently enabled
enabled := debug.ListEnabled()
```

## Environment Variable

The `RAZOR_DEBUG` environment variable controls categories at startup:

```bash
# Enable specific categories only
RAZOR_DEBUG=FS,SEARCH ./razor_bin

# Enable all categories (including verbose)
RAZOR_DEBUG=all ./razor_bin

# Disable all categories
RAZOR_DEBUG=none ./razor_bin
```

## Output Format

```
HH:MM:SS.μμμμμμ [CATEGORY] message
```

Example:
```
14:32:45.123456 [FS] Request: op=0 path="/" query="" gen=1
14:32:45.124789 [FS] fetchDir: reading "/"
14:32:45.125012 [FS] fetchDir: found 23 raw entries
14:32:45.126234 [FS] fetchDir: returning 23 entries
14:32:45.126567 [FS] FetchDir response: path="/" entries=23 gen=1 err=<nil>
```

## Usage Guidelines

### When to Use Each Category

**APP** - Orchestrator-level events:
```go
debug.Log(debug.APP, "Navigate: path=%q", path)
debug.Log(debug.APP, "Settings: dark_mode=%v", darkMode)
debug.Log(debug.APP, "FSResponse: op=%d entries=%d", op, len(entries))
```

**FS** - Filesystem operations:
```go
debug.Log(debug.FS, "Request: op=%d path=%q gen=%d", op, path, gen)
debug.Log(debug.FS, "fetchDir: reading %q", path)
debug.Log(debug.FS, "FetchDir response: entries=%d", len(entries))
```

**SEARCH** - Search functionality:
```go
debug.Log(debug.SEARCH, "doSearch: query=%q", query)
debug.Log(debug.SEARCH, "searchDir: using external engine %s", engine)
debug.Log(debug.SEARCH, "searchDir: complete, %d results", len(results))
```

**STORE** - Database operations:
```go
debug.Log(debug.STORE, "Opening database: %s", path)
debug.Log(debug.STORE, "saveSetting: %q = %q", key, value)
debug.Log(debug.STORE, "fetchFavorites: returning %d", len(favs))
```

### Verbose Categories

Use verbose categories sparingly - they can produce massive output:

**FS_ENTRY** - Individual file/directory entries:
```go
// Only enable when debugging specific entry handling
debug.Log(debug.FS_ENTRY, "fetchDir: %q isDir=%v size=%d", name, isDir, size)
```

**FS_WALK** - Directory traversal during search:
```go
// Only enable when debugging recursive search issues
debug.Log(debug.FS_WALK, "walkDir: %q depth=%d entries=%d", path, depth, n)
```

### Avoiding Log Spam

1. **Use appropriate categories**: Don't log per-entry data with core categories
2. **Log state changes**: Log when things change, not on every frame
3. **Include context**: Log enough info to understand the issue
4. **Keep messages concise**: Use key=value format

Good:
```go
debug.Log(debug.FS, "fetchDir: path=%q entries=%d", path, len(entries))
```

Bad:
```go
debug.Log(debug.FS, "Now I am going to fetch the directory at path %s and we expect to find some files there", path)
```

## Adding a New Category

1. Add constant in both `debug_on.go` and `debug_off.go`:
```go
const (
    // ...existing...
    MY_CAT Category = "MY_CAT"
)
```

2. Add to `enabledCategories` map in `debug_on.go`:
```go
var enabledCategories = map[Category]bool{
    // ...existing...
    MY_CAT: true,  // or false for verbose categories
}
```

3. Use in code:
```go
debug.Log(debug.MY_CAT, "something happened: %v", value)
```

## Implementation Notes

### Release Build Optimization

In release builds (`!debug`), all functions are no-ops:

```go
func Log(cat Category, format string, args ...interface{}) {}
func Enable(cat Category) {}
func IsEnabled(cat Category) bool { return false }
```

The Go compiler optimizes these away entirely, resulting in zero runtime overhead.

### Thread Safety

The `enabledCategories` map is protected by a `sync.RWMutex`:
- Multiple goroutines can log simultaneously (read lock)
- Category enable/disable acquires write lock
- Minimal contention in normal operation
