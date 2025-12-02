# Known Bugs

## Search History Dropdown Explosion

**Status**: Open
**Severity**: Medium
**Component**: UI / Search

### Description
When typing into the search box while viewing a directory with a long path string, the search history suggestions dropdown causes the window layout to explode vertically. This creates large amounts of whitespace above the current working directory (home button row) and below it, pushing content around.

### Steps to Reproduce
1. Navigate to a directory with a long path (deep nesting)
2. Click on the search box
3. Start typing - the search history suggestions appear
4. Observe the layout breaking with excessive vertical whitespace

### Expected Behavior
The search history dropdown should overlay on top of existing content without affecting the main layout dimensions.

### Likely Cause
The search history suggestions overlay is not properly constrained and is affecting the parent layout's size calculations instead of floating above it.

### Files to Investigate
- `internal/ui/layout.go` - `layoutSearchHistoryOverlay()` function
- Search history dropdown positioning and constraints

---

## Template for New Bugs

```markdown
## Bug Title

**Status**: Open | In Progress | Fixed
**Severity**: Low | Medium | High | Critical
**Component**: UI | FS | Config | etc.

### Description
Brief description of the bug.

### Steps to Reproduce
1. Step one
2. Step two
3. etc.

### Expected Behavior
What should happen.

### Actual Behavior
What actually happens.

### Likely Cause
Initial investigation notes.

### Files to Investigate
- `path/to/file.go`
```
