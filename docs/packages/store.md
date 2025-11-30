# Store Package (`internal/store`)

The store package handles SQLite persistence for favorites and application settings.

## Files

| File | Purpose |
|------|---------|
| `db.go` | Database operations (~150 lines) |

## Database Schema

```sql
-- Favorites table
CREATE TABLE IF NOT EXISTS favorites (
    path TEXT PRIMARY KEY,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Settings table (key-value store)
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
```

## DB Structure

```go
// File: internal/store/db.go

type DB struct {
    conn         *sql.DB
    RequestChan  chan Request   // Incoming requests (buffered: 10)
    ResponseChan chan Response  // Outgoing responses (buffered: 10)
}
```

## Request/Response Types

### EventType

```go
type EventType int

const (
    FetchFavorites EventType = iota  // Get all favorites
    AddFavorite                      // Add path to favorites
    RemoveFavorite                   // Remove path from favorites
    FetchSettings                    // Get all settings
    SaveSetting                      // Save a setting
)
```

### Request

```go
type Request struct {
    Op         EventType
    Path       string     // For favorites operations
    Key, Value string     // For settings operations
}
```

### Response

```go
type Response struct {
    Op        EventType
    Favorites []string           // For FetchFavorites
    Settings  map[string]string  // For FetchSettings
    Err       error
}
```

## Initialization

```go
func NewDB() *DB {
    return &DB{
        RequestChan:  make(chan Request, 10),
        ResponseChan: make(chan Response, 10),
    }
}
```

## Opening the Database

```go
func (d *DB) Open(dbPath string) error
```

1. Creates parent directories if needed
2. Opens SQLite connection
3. Sets pragmas for performance:
   ```sql
   PRAGMA journal_mode=WAL;
   PRAGMA synchronous=NORMAL;
   ```
4. Creates tables if they don't exist

Database location:
```go
// On macOS: ~/Library/Application Support/razor/razor.db
// On Linux: ~/.config/razor/razor.db
// On Windows: %APPDATA%/razor/razor.db

configDir, _ := os.UserConfigDir()
dbPath := filepath.Join(configDir, "razor", "razor.db")
```

## Main Loop

```go
func (d *DB) Start()
```

Runs in dedicated goroutine:

```go
for req := range d.RequestChan {
    switch req.Op {
    case FetchFavorites:
        d.fetchFavorites()
    case AddFavorite:
        d.execAndFetch("INSERT OR IGNORE INTO favorites (path) VALUES (?)", req.Path)
    case RemoveFavorite:
        d.execAndFetch("DELETE FROM favorites WHERE path = ?", req.Path)
    case FetchSettings:
        d.fetchSettings()
    case SaveSetting:
        d.saveSetting(req.Key, req.Value)
    }
}
```

## Operations

### Fetch Favorites

```go
func (d *DB) fetchFavorites()
```

```sql
SELECT path FROM favorites ORDER BY created_at ASC
```

Returns paths in order they were added.

### Add/Remove Favorite

```go
func (d *DB) execAndFetch(query, path string)
```

Executes query then re-fetches all favorites:
```go
d.conn.Exec(query, path)
d.fetchFavorites()  // Send updated list
```

Using `INSERT OR IGNORE` prevents duplicates.

### Fetch Settings

```go
func (d *DB) fetchSettings()
```

```sql
SELECT key, value FROM settings
```

Returns all settings as `map[string]string`.

### Save Setting

```go
func (d *DB) saveSetting(key, value string)
```

```sql
INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)
```

`INSERT OR REPLACE` (UPSERT) handles both insert and update.

## Stored Settings

| Key | Values | Purpose |
|-----|--------|---------|
| `show_dotfiles` | `"true"` / `"false"` | Show hidden files |
| `dark_mode` | `"true"` / `"false"` | Dark theme |
| `search_engine` | `"builtin"` / `"ripgrep"` / `"ugrep"` | Preferred search engine |
| `default_depth` | `"1"` to `"20"` | Default recursive search depth |

## Usage from Orchestrator

### Fetching on Startup

```go
// In Orchestrator.Run():
o.store.RequestChan <- store.Request{Op: store.FetchFavorites}
o.store.RequestChan <- store.Request{Op: store.FetchSettings}
```

### Handling Responses

```go
// In Orchestrator.handleStoreResponse():
func (o *Orchestrator) handleStoreResponse(resp store.Response) {
    switch resp.Op {
    case store.FetchFavorites:
        o.state.Favorites = make(map[string]bool)
        o.state.FavList = make([]ui.FavoriteItem, len(resp.Favorites))
        for i, path := range resp.Favorites {
            o.state.Favorites[path] = true
            o.state.FavList[i] = ui.FavoriteItem{
                Path: path,
                Name: filepath.Base(path),
            }
        }

    case store.FetchSettings:
        if val, ok := resp.Settings["show_dotfiles"]; ok {
            o.showDotfiles = val == "true"
            o.ui.SetShowDotfilesCheck(o.showDotfiles)
        }
        if val, ok := resp.Settings["dark_mode"]; ok {
            o.ui.SetDarkMode(val == "true")
        }
        if val, ok := resp.Settings["search_engine"]; ok {
            o.handleSearchEngineChange(val)
        }
        if val, ok := resp.Settings["default_depth"]; ok {
            if depth, err := strconv.Atoi(val); err == nil {
                o.defaultDepth = depth
                o.ui.SetDefaultDepth(depth)
            }
        }
    }
}
```

### Saving Settings

```go
// Toggle dotfiles
o.store.RequestChan <- store.Request{
    Op:    store.SaveSetting,
    Key:   "show_dotfiles",
    Value: "true",
}

// Change search engine
o.store.RequestChan <- store.Request{
    Op:    store.SaveSetting,
    Key:   "search_engine",
    Value: "ripgrep",
}
```

### Managing Favorites

```go
// Add favorite
o.store.RequestChan <- store.Request{
    Op:   store.AddFavorite,
    Path: "/path/to/folder",
}

// Remove favorite
o.store.RequestChan <- store.Request{
    Op:   store.RemoveFavorite,
    Path: "/path/to/folder",
}
```

## Thread Safety

All database operations happen in a single goroutine (`Start()`), so no mutex is needed. The channel-based design serializes all requests automatically.

## Cleanup

```go
func (d *DB) Close() {
    if d.conn != nil {
        d.conn.Close()
    }
}
```

Called in `Orchestrator.Run()` via `defer`:
```go
defer o.store.Close()
```

## SQLite Implementation

Uses [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite):
- Pure Go implementation (no CGO)
- Cross-platform without external dependencies
- Compatible with standard `database/sql` interface

## Extension Points

### Adding a New Setting

1. Define key constant (optional but recommended):
   ```go
   const SettingMyOption = "my_option"
   ```

2. Save from Orchestrator:
   ```go
   o.store.RequestChan <- store.Request{
       Op:    store.SaveSetting,
       Key:   "my_option",
       Value: "my_value",
   }
   ```

3. Load in `handleStoreResponse`:
   ```go
   if val, ok := resp.Settings["my_option"]; ok {
       o.myOption = val
   }
   ```

### Adding a New Table

1. Add to schema in `Open()`:
   ```go
   schema := `
       CREATE TABLE IF NOT EXISTS favorites (...);
       CREATE TABLE IF NOT EXISTS settings (...);
       CREATE TABLE IF NOT EXISTS my_table (
           id INTEGER PRIMARY KEY,
           data TEXT NOT NULL
       );
   `
   ```

2. Add new EventType:
   ```go
   const (
       // ...existing...
       FetchMyData
       SaveMyData
   )
   ```

3. Add handler in `Start()`:
   ```go
   case FetchMyData:
       d.fetchMyData()
   case SaveMyData:
       d.saveMyData(req.Key, req.Value)
   ```

4. Add Response fields if needed:
   ```go
   type Response struct {
       // ...existing...
       MyData []MyDataItem
   }
   ```

### Adding Complex Queries

For operations beyond simple CRUD:

```go
func (d *DB) customOperation(req Request) {
    tx, err := d.conn.Begin()
    if err != nil {
        d.ResponseChan <- Response{Err: err}
        return
    }
    defer tx.Rollback()

    // Multiple operations...
    tx.Exec(...)
    tx.Exec(...)

    if err := tx.Commit(); err != nil {
        d.ResponseChan <- Response{Err: err}
        return
    }

    d.ResponseChan <- Response{Op: req.Op}
}
```
