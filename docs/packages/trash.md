# Trash Package (`internal/trash`)

The trash package provides cross-platform trash/recycle bin functionality, allowing files to be moved to the system trash instead of being permanently deleted.

## Files

| File | Purpose |
|------|---------|
| `trash.go` | Cross-platform API and shared types |
| `trash_darwin.go` | macOS implementation |
| `trash_linux.go` | Linux (FreeDesktop.org Trash spec) implementation |
| `trash_windows.go` | Windows Recycle Bin implementation |

## Types

### Item

Represents a file or directory in the trash:

```go
type Item struct {
    Name         string    // Original filename
    OriginalPath string    // Full path where the file was deleted from
    TrashPath    string    // Current path in trash
    DeletedAt    time.Time // When the file was deleted
    Size         int64     // Size in bytes
    IsDir        bool      // Whether this is a directory
}
```

## API

### Moving Files to Trash

```go
// Move a file or directory to the system trash
err := trash.MoveToTrash("/path/to/file.txt")
```

This is the primary function used for delete operations. Files can be restored from the system trash.

### Permanent Delete

```go
// Permanently delete a file (bypass trash)
err := trash.PermanentDelete("/path/to/file.txt")
```

This permanently deletes files without using the trash. Use with caution.

### Listing Trash Contents

```go
items, err := trash.List()
for _, item := range items {
    fmt.Printf("%s (deleted %s)\n", item.Name, item.DeletedAt)
}
```

### Restoring Files

```go
// Restore to original location
err := trash.Restore(item)

// Restore to different location
err := trash.RestoreTo(item, "/new/path/file.txt")
```

### Emptying Trash

```go
// Delete all items in trash permanently
err := trash.Empty()

// Delete a specific item from trash
err := trash.Delete(item)
```

### Utility Functions

```go
// Get the trash directory path
path := trash.GetPath()

// Check if trash is available on this platform
if trash.IsAvailable() {
    // Use trash
} else {
    // Fall back to permanent delete
}

// Get platform-appropriate display name
name := trash.DisplayName() // "Trash" or "Recycle Bin"

// Get action phrase
phrase := trash.VerbPhrase() // "Move to Trash" or "Move to Recycle Bin"
```

## Platform Implementations

### macOS (`trash_darwin.go`)

Uses `osascript` to invoke Finder's trash functionality:
- Trash location: `~/.Trash`
- **No restore API available** - macOS does not expose a programmatic way to restore files from Trash
- Items must be manually restored via Finder
- Integrates with Finder's trash

### Linux (`trash_linux.go`)

Follows the [FreeDesktop.org Trash Specification](https://specifications.freedesktop.org/trash-spec/trashspec-latest.html):
- Primary trash: `~/.local/share/Trash`
- Supports `files/` and `info/` directories
- Creates `.trashinfo` metadata files containing original path and deletion date
- **Restore supported** via `.trashinfo` files

### Windows (`trash_windows.go`)

Uses the Windows Shell API:
- Recycle Bin per drive
- **Restore supported** via shell operations
- Integrates with Windows Explorer

## Integration with File Operations

The orchestrator uses trash for delete operations:

```go
// In file_ops.go
func (o *Orchestrator) doDelete(path string) {
    if trash.IsAvailable() {
        err := trash.MoveToTrash(path)
    } else {
        err := trash.PermanentDelete(path)
    }
}

// For permanent delete (bypass trash)
func (o *Orchestrator) doPermanentDelete(path string) {
    err := trash.PermanentDelete(path)
}
```

## Hotkey Integration

Two delete modes are available:

| Action | Default Hotkey | Description |
|--------|----------------|-------------|
| Delete | Delete (Windows/Linux), Backspace (macOS) | Move to trash |
| Permanent Delete | Shift+Delete (Windows/Linux), Shift+Backspace (macOS) | Bypass trash |

## Error Handling

```go
err := trash.MoveToTrash(path)
if err != nil {
    // Common errors:
    // - File doesn't exist
    // - Permission denied
    // - Trash not available (fall back to permanent delete)
    log.Printf("Failed to trash: %v", err)
}
```

## Availability

Trash support depends on the platform:
- **macOS**: Always available (move to trash only, no programmatic restore)
- **Linux**: Available if `~/.local/share/Trash` is writable (restore supported via `.trashinfo`)
- **Windows**: Always available (full restore support)

Use `trash.IsAvailable()` to check before using trash functionality.

## Platform Restore Support

| Platform | Move to Trash | Restore | Empty |
|----------|--------------|---------|-------|
| macOS    | Yes          | No (use Finder) | No |
| Linux    | Yes          | Yes     | Yes   |
| Windows  | Yes          | Yes     | Yes   |
