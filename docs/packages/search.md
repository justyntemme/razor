# Search Package (`internal/search`)

The search package handles query parsing, search engine detection, and file matching logic.

## Files

| File | Purpose |
|------|---------|
| `engine.go` | External search engine detection |
| `query.go` | Query parsing and file matching |

## Search Engines

### Engine Types

```go
// File: internal/search/engine.go

type SearchEngine int

const (
    EngineBuiltin SearchEngine = iota  // Built-in Go implementation
    EngineRipgrep                      // ripgrep (rg)
    EngineUgrep                        // ugrep (ug)
)

func (e SearchEngine) String() string {
    switch e {
    case EngineRipgrep:
        return "ripgrep"
    case EngineUgrep:
        return "ugrep"
    default:
        return "builtin"
    }
}
```

### Engine Info

```go
type EngineInfo struct {
    Engine    SearchEngine
    Name      string  // Display name
    Command   string  // Full path to executable
    Available bool    // Is installed?
    Version   string  // Version string
}
```

### Detection

```go
func DetectEngines() []EngineInfo
```

1. Always includes builtin (always available)
2. Checks for `rg` (ripgrep):
   ```go
   path, err := exec.LookPath("rg")
   if err == nil {
       // Get version
       out, _ := exec.Command("rg", "--version").Output()
       // Parse "ripgrep 13.0.0" → "13.0.0"
   }
   ```
3. Checks for `ug` (ugrep):
   ```go
   path, err := exec.LookPath("ug")
   // Similar version detection
   ```

### Helper Functions

```go
// Get engine by string name
func GetEngineByName(name string) SearchEngine

// Get command path for engine
func GetEngineCommand(engine SearchEngine, engines []EngineInfo) string
```

## Query Parsing

### Query Structure

```go
// File: internal/search/query.go

type Query struct {
    Directives []Directive  // Parsed directives
    Raw        string       // Original query string
    Pattern    string       // Remaining pattern (non-directive text)
}
```

### Directive Types

```go
type DirectiveType int

const (
    DirFilename DirectiveType = iota  // filename:*.txt
    DirContents                       // contents:TODO
    DirExt                            // ext:go
    DirSize                           // size:>10MB
    DirModified                       // modified:>2024-01-01
    DirRecursive                      // recursive:3 or depth:3
)
```

### Directive Structure

```go
type Directive struct {
    Type     DirectiveType
    Value    string     // Raw value from query
    Operator Operator   // >, <, >=, <=, =
    NumValue int64      // Parsed numeric value (for size)
    TimeVal  time.Time  // Parsed time (for modified)
}
```

### Operators

```go
type Operator int

const (
    OpEquals Operator = iota
    OpGreater
    OpLess
    OpGreaterEqual
    OpLessEqual
)
```

### Parse Function

```go
func Parse(query string) *Query
```

Example parsing:

```
Input: "ext:go contents:func size:>10KB recursive:2 MyClass"

Output:
  Directives:
    [0] {Type: DirExt, Value: ".go"}
    [1] {Type: DirContents, Value: "func"}
    [2] {Type: DirSize, Value: ">10KB", Operator: OpGreater, NumValue: 10240}
    [3] {Type: DirRecursive, Value: "2", NumValue: 2}
  Pattern: "MyClass"
```

### Size Parsing

```go
func parseSize(s string) (Operator, int64)
```

Supports:
- `10` → 10 bytes
- `10KB` or `10K` → 10 * 1024
- `10MB` or `10M` → 10 * 1024 * 1024
- `10GB` or `10G` → 10 * 1024 * 1024 * 1024
- `>10MB` → greater than
- `<10MB` → less than
- `>=10MB` → greater or equal
- `<=10MB` → less or equal

### Date Parsing

```go
func parseDate(s string) (Operator, time.Time)
```

Supports formats:
- `2024-01-15` (ISO date)
- `01/15/2024` (US format)
- `15/01/2024` (EU format)
- `>2024-01-01` → after date
- `<2024-01-01` → before date

## Matching

### Matcher

```go
type Matcher struct {
    query       *Query
    ctx         context.Context
    contentFunc func(path string) ([]byte, error)  // For reading file contents
}

func NewMatcher(query *Query) *Matcher
func NewMatcherWithContext(ctx context.Context, query *Query) *Matcher
```

### Match Function

```go
func (m *Matcher) Matches(entry fs.Entry, fullPath string) bool
```

Applies all directives in order:

```go
for _, dir := range m.query.Directives {
    switch dir.Type {
    case DirFilename:
        if !matchFilename(entry.Name, dir.Value) {
            return false
        }
    case DirExt:
        if !matchExtension(entry.Name, dir.Value) {
            return false
        }
    case DirSize:
        if !compareInt(entry.Size, dir.NumValue, dir.Operator) {
            return false
        }
    case DirModified:
        if !compareTime(entry.ModTime, dir.TimeVal, dir.Operator) {
            return false
        }
    case DirContents:
        if !m.matchContents(fullPath, dir.Value) {
            return false
        }
    // DirRecursive handled at search level, not matching
    }
}

// Also match pattern against filename
if m.query.Pattern != "" {
    if !strings.Contains(strings.ToLower(entry.Name), strings.ToLower(m.query.Pattern)) {
        return false
    }
}

return true
```

### Filename Matching

```go
func matchFilename(name, pattern string) bool
```

Supports glob patterns:
- `*.txt` → ends with .txt
- `test_*` → starts with test_
- `*config*` → contains config

Uses `filepath.Match()` for glob semantics.

### Extension Matching

```go
func matchExtension(name, ext string) bool
```

Normalizes extension (adds `.` if missing):
```go
if !strings.HasPrefix(ext, ".") {
    ext = "." + ext
}
return strings.EqualFold(filepath.Ext(name), ext)
```

### Content Matching

```go
func (m *Matcher) matchContents(path, pattern string) bool
```

1. Check context for cancellation
2. Read file contents (with size limit)
3. Detect binary files (contains null bytes → skip)
4. Case-insensitive search for pattern

```go
func (m *Matcher) matchContents(path, pattern string) bool {
    // Check cancellation
    if m.ctx != nil && m.ctx.Err() != nil {
        return false
    }

    content, err := m.contentFunc(path)
    if err != nil {
        return false
    }

    // Skip binary files
    if bytes.Contains(content, []byte{0}) {
        return false
    }

    // Case-insensitive search
    return bytes.Contains(
        bytes.ToLower(content),
        bytes.ToLower([]byte(pattern)),
    )
}
```

### Comparison Functions

```go
func compareInt(a, b int64, op Operator) bool {
    switch op {
    case OpEquals:
        return a == b
    case OpGreater:
        return a > b
    case OpLess:
        return a < b
    case OpGreaterEqual:
        return a >= b
    case OpLessEqual:
        return a <= b
    }
    return false
}

func compareTime(a, b time.Time, op Operator) bool {
    switch op {
    case OpEquals:
        return a.Equal(b)
    case OpGreater:
        return a.After(b)
    case OpLess:
        return a.Before(b)
    case OpGreaterEqual:
        return a.After(b) || a.Equal(b)
    case OpLessEqual:
        return a.Before(b) || a.Equal(b)
    }
    return false
}
```

## Query Helpers

```go
// Check if query has content search directive
func (q *Query) HasContentSearch() bool {
    for _, d := range q.Directives {
        if d.Type == DirContents {
            return true
        }
    }
    return false
}

// Get content search pattern
func (q *Query) GetContentPattern() string {
    for _, d := range q.Directives {
        if d.Type == DirContents {
            return d.Value
        }
    }
    return ""
}

// Get recursive depth
func (q *Query) GetDepth(defaultDepth int) int {
    for _, d := range q.Directives {
        if d.Type == DirRecursive {
            if d.NumValue > 0 {
                return int(d.NumValue)
            }
            return defaultDepth
        }
    }
    return 1  // Default: current directory only
}
```

## Usage Example

```go
// Parse query
query := search.Parse("ext:go contents:TODO size:>1KB")

// Create matcher with context
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
matcher := search.NewMatcherWithContext(ctx, query)

// Check each file
for _, entry := range entries {
    if matcher.Matches(entry, entry.Path) {
        results = append(results, entry)
    }
}

// Check if external engine can help
if query.HasContentSearch() {
    engine := search.GetEngineByName("ripgrep")
    if cmd := search.GetEngineCommand(engine, engines); cmd != "" {
        // Use ripgrep for faster content search
        pattern := query.GetContentPattern()
        // Run: rg --files-with-matches "TODO" /path
    }
}
```

## Extension Points

### Adding a New Directive

1. Add to `DirectiveType` enum
2. Add parsing in `Parse()`:
   ```go
   case strings.HasPrefix(lower, "mydir:"):
       value := part[6:]
       directives = append(directives, Directive{
           Type:  DirMyDir,
           Value: value,
       })
   ```
3. Add matching in `Matcher.Matches()`:
   ```go
   case DirMyDir:
       if !matchMyDir(entry, dir.Value) {
           return false
       }
   ```

### Adding a New Search Engine

1. Add to `SearchEngine` enum
2. Add to `DetectEngines()`:
   ```go
   // Check for mytool
   if path, err := exec.LookPath("mytool"); err == nil {
       engines = append(engines, EngineInfo{
           Engine:    EngineMyTool,
           Name:      "MyTool",
           Command:   path,
           Available: true,
       })
   }
   ```
3. Add command building in `fs/system.go`:
   ```go
   case search.EngineMyTool:
       args = []string{"--search", pattern, path}
   ```

### Custom Content Matching

Override the content function in Matcher:
```go
matcher := search.NewMatcher(query)
matcher.contentFunc = func(path string) ([]byte, error) {
    // Custom reading logic (e.g., extract text from PDFs)
    return myCustomRead(path)
}
```
