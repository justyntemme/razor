# Filesystem Package (`internal/fs`)

The fs package handles all filesystem operations asynchronously, including directory listing, searching, and drive enumeration.

## Files

| File | Purpose |
|------|---------|
| `system.go` | Core filesystem operations (~600 lines) |
| `drives_darwin.go` | macOS drive listing |
| `drives_linux.go` | Linux drive listing |
| `drives_windows.go` | Windows drive listing |

## System

The main filesystem handler:

```go
// File: internal/fs/system.go

type System struct {
    RequestChan  chan Request   // Incoming requests (buffered: 10)
    ResponseChan chan Response  // Outgoing responses (buffered: 10)
    ProgressChan chan Progress  // Progress updates (buffered: 100)

    cancelMu     sync.Mutex
    cancelFunc   context.CancelFunc  // Cancel current search
    currentGen   int64               // Current operation generation
    searchActive bool                // Is search running?
}
```

## Request/Response Types

### Request

```go
type Request struct {
    Op           OpType  // Operation type
    Path         string  // Directory path
    Query        string  // Search query (for SearchDir)
    Gen          int64   // Generation counter
    SearchEngine int     // 0=builtin, 1=ripgrep, 2=ugrep
    EngineCmd    string  // External engine command path
    DefaultDepth int     // Default recursive depth
}
```

### OpType

```go
type OpType int

const (
    FetchDir OpType = iota  // List directory contents
    SearchDir               // Search with query
    CancelSearch            // Cancel ongoing search
)
```

### Response

```go
type Response struct {
    Op        OpType
    Path      string
    Entries   []Entry
    Gen       int64    // Echo request's generation
    Cancelled bool     // True if cancelled
    Err       error
}
```

### Entry

```go
type Entry struct {
    Name    string
    Path    string
    IsDir   bool
    Size    int64
    ModTime time.Time
}
```

### Progress

```go
type Progress struct {
    Gen     int64   // Which operation this is for
    Current int64   // Current count/bytes
    Total   int64   // Total (0 = indeterminate)
    Label   string  // Status message
}
```

## Main Loop

```go
// File: internal/fs/system.go

func (s *System) Start()
```

Runs in dedicated goroutine:

```go
for req := range s.RequestChan {
    switch req.Op {
    case FetchDir:
        s.fetchDir(req)
    case SearchDir:
        // Cancel any existing search first
        s.cancelCurrentSearch()
        // Spawn search in new goroutine (for cancellation)
        go s.searchDir(req)
    case CancelSearch:
        s.cancelCurrentSearch()
    }
}
```

## Directory Listing

```go
func (s *System) fetchDir(req Request)
```

1. Cancel any running search
2. Read directory with `os.ReadDir()`
3. Stat each entry for size/modtime
4. Convert to `[]Entry`
5. Send response

## Search Implementation

```go
func (s *System) searchDir(req Request)
```

### Flow

```
┌──────────────────────────────────────────────────────────┐
│                      searchDir()                          │
│                                                           │
│  1. Create cancellable context                           │
│  2. Parse query into directives                          │
│  3. Determine max depth (recursive: directive)           │
│  4. Choose search method:                                │
│                                                           │
│     Has contents: directive?                             │
│         │                                                 │
│         ├─ YES + external engine available               │
│         │       └─→ runExternalSearch()                  │
│         │             └─→ processExternalResults()       │
│         │                                                 │
│         └─ NO or builtin only                            │
│               └─→ builtinSearch()                        │
│                     └─→ walkDirWithProgress()            │
│                                                           │
│  5. Send Response (or skip if cancelled)                 │
└──────────────────────────────────────────────────────────┘
```

### Builtin Search

```go
func (s *System) builtinSearch(ctx context.Context, req Request, query *search.Query, maxDepth int) []Entry
```

1. **Count Phase** (for progress bar):
   ```go
   total := countFiles(ctx, req.Path, maxDepth)
   ```

2. **Search Phase**:
   ```go
   walkDirWithProgress(ctx, req.Path, 0, maxDepth, func(path, name, isDir) bool {
       // Apply filters
       entry := Entry{Name: name, Path: path, ...}
       if matcher.Matches(entry, path) {
           results = append(results, entry)
       }
       // Report progress
       s.ProgressChan <- Progress{Current: current, Total: total}
   })
   ```

### External Search (ripgrep/ugrep)

```go
func (s *System) runExternalSearch(ctx context.Context, req Request, pattern string, maxDepth int) ([]string, error)
```

Builds command:
```go
// ripgrep
rg --files-with-matches --max-depth N --ignore-case -- "pattern" /path

// ugrep
ug -l -r --max-depth=N --ignore-case -- "pattern" /path
```

Streams results:
```go
scanner := bufio.NewScanner(stdout)
for scanner.Scan() {
    results = append(results, scanner.Text())
    // Report progress every 10 files
    if len(results) % 10 == 0 {
        s.ProgressChan <- Progress{Label: fmt.Sprintf("Found %d files...", len(results))}
    }
}
```

### Processing External Results

```go
func (s *System) processExternalResults(ctx context.Context, paths []string, query *search.Query) []Entry
```

1. External search only handles `contents:` directive
2. This function applies remaining filters:
   - `ext:` - file extension
   - `size:` - file size
   - `modified:` - modification date
   - `filename:` - filename pattern
3. Stats each file for metadata

## Cancellation

```go
func (s *System) cancelCurrentSearch()
```

```go
s.cancelMu.Lock()
if s.cancelFunc != nil {
    s.cancelFunc()  // Signal context to cancel
    s.cancelFunc = nil
}
s.cancelMu.Unlock()
```

Context cancellation propagates through:
- `walkDirWithProgress()` checks `ctx.Err()` between files
- `exec.CommandContext()` kills external process
- Builtin content search checks context during file read

## Depth Calculation

```go
// Our depth semantics:
// depth=1: current directory only
// depth=2: current + 1 level
// depth=N: current + (N-1) levels

// Maps to external tools:
// rg --max-depth 1: current only (matches our depth=1)
// ug --max-depth=1: current only (matches our depth=1)
```

Query examples:
- `recursive:` (no value) → uses `DefaultDepth` from settings
- `recursive:3` → maxDepth = 3
- `depth:5` → maxDepth = 5 (alias)
- No directive → maxDepth = 1 (current directory only)

## Drive Enumeration

Platform-specific implementations:

### macOS (`drives_darwin.go`)

```go
func ListDrives() []DriveInfo
```

Lists `/Volumes` directory:
```go
entries, _ := os.ReadDir("/Volumes")
for _, e := range entries {
    drives = append(drives, DriveInfo{
        Name: e.Name(),
        Path: filepath.Join("/Volumes", e.Name()),
    })
}
```

### Linux (`drives_linux.go`)

```go
func ListDrives() []DriveInfo
```

Parses `/proc/mounts` or `/etc/mtab`:
```go
// Look for common mount points
mountPoints := []string{"/", "/home", "/mnt", "/media"}
// Parse mount file for additional mounts
```

### Windows (`drives_windows.go`)

```go
func ListDrives() []DriveInfo
```

Enumerates drive letters A-Z:
```go
for c := 'A'; c <= 'Z'; c++ {
    path := string(c) + ":\\"
    if _, err := os.Stat(path); err == nil {
        drives = append(drives, DriveInfo{
            Name: string(c) + ":",
            Path: path,
        })
    }
}
```

## Skipped Directories

During search, certain directories are skipped:

```go
var skipDirs = map[string]bool{
    "/dev":  true,
    "/proc": true,
    "/sys":  true,
    "/run":  true,
}
```

Also skips:
- Non-regular files (devices, sockets, etc.)
- Files with read errors (permission denied)
- Binary files (for content search - detected by null bytes)

## Progress Reporting

### Determinate Progress

For builtin search with known file count:
```go
s.ProgressChan <- Progress{
    Gen:     req.Gen,
    Current: filesProcessed,
    Total:   totalFiles,
    Label:   "Searching...",
}
```

### Indeterminate Progress

For external search (unknown total):
```go
s.ProgressChan <- Progress{
    Gen:     req.Gen,
    Current: 0,
    Total:   0,  // 0 = indeterminate (animated bar)
    Label:   fmt.Sprintf("Found %d files...", count),
}
```

## Error Handling

- **Permission denied**: Skip file, continue search
- **File not found**: Skip file, continue search
- **Read errors**: Log and skip
- **External process errors**: Fall back to builtin search

## Extension Points

### Adding a New Search Directive

1. Add directive parsing in `internal/search/query.go`
2. Add matching logic in `Matcher.Matches()`
3. If complex, consider external tool integration

### Adding a New External Search Engine

1. Add engine to `internal/search/engine.go`
2. Add detection in `DetectEngines()`
3. Add command building in `runExternalSearch()`
4. Update UI to show in settings

### Adding Custom File Filtering

1. Add filter logic in `walkDirWithProgress()` callback
2. Or add as post-processing in `processExternalResults()`
