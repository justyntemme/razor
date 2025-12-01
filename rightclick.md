# Right-Click in Empty Space Fix

## Problem
Right-clicking in empty space below the file list (or in an empty directory) does not show the context menu.

## Root Cause
The current code in `layoutFileList` does not properly handle background click events. The working implementation from commit `2be439f` uses a specific pattern with `PassOp` that allows events to pass through layers correctly.

## Working Solution (from commit 2be439f)

The fix requires these specific changes to the `layout.Flexed(1, ...)` section in `layoutFileList`:

### 1. Register background handler FIRST with PassOp

```go
layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
    // Register background right-click handler FIRST, covering entire area
    // Use PassOp so events pass through to list items on top
    bgArea := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
    passOp := pointer.PassOp{}.Push(gtx.Ops)
    event.Op(gtx.Ops, &r.bgRightClickTag)
    passOp.Pop()
    bgArea.Pop()

    // Track if any file row was right-clicked
    fileRightClicked := false

    // Use PassOp on the list so events pass through to background handler
    defer pointer.PassOp{}.Push(gtx.Ops).Pop()

    // Layout file list (row handlers register on top of background)
    dims := r.listState.Layout(gtx, len(state.Entries), func(gtx layout.Context, i int) layout.Dimensions {
        // ... row handling code ...
    })

    // Check for background right-click AFTER processing file rows
    // Only show background menu if no file was right-clicked
    if !fileRightClicked {
        for {
            ev, ok := gtx.Event(pointer.Filter{Target: &r.bgRightClickTag, Kinds: pointer.Press})
            if !ok {
                break
            }
            if e, ok := ev.(pointer.Event); ok && e.Buttons.Contain(pointer.ButtonSecondary) {
                r.menuVisible = true
                r.menuPos = r.mousePos
                r.menuPath = state.CurrentPath
                r.menuIsDir = true
                r.menuIsFav = false
                r.menuIsBackground = true
            }
        }
    }

    return dims
}),
```

### 2. Key Points

1. **PassOp on background handler**: The `pointer.PassOp{}` wrapping the background `event.Op` allows click events to pass through to elements rendered on top (the file list rows).

2. **PassOp on list**: The `defer pointer.PassOp{}.Push(gtx.Ops).Pop()` before the list layout allows events from the list to also reach the background handler.

3. **Process events AFTER list**: Background events are checked AFTER the list is rendered and row clicks are processed. This ensures row clicks take priority.

4. **No layout.Stack wrapper**: The working version does NOT wrap the list in `layout.Stack{}` - it's a direct `r.listState.Layout()` call.

5. **Track row clicks**: Use a `fileRightClicked` bool to prevent showing background menu when a file row was right-clicked.

## Critical: Do NOT consume pointer.Press globally

The global mouse tracking at the top of `Layout()` must NOT include `pointer.Press` in its filter:

```go
// WRONG - this consumes all click events:
gtx.Event(pointer.Filter{Target: &r.mouseTag, Kinds: pointer.Move | pointer.Press})

// CORRECT - only track movement:
gtx.Event(pointer.Filter{Target: &r.mouseTag, Kinds: pointer.Move})
```

If `pointer.Press` is included in global tracking, it will consume click events before they reach specific handlers.

## Testing

1. Navigate to a directory with few files (so there's empty space below)
2. Right-click in the empty space - should show context menu with "New File", "New Folder", "Paste" options
3. Navigate to an empty directory
4. Right-click anywhere - should show context menu
5. Verify left-click on files still works (selection)
6. Verify double-click on folders still navigates into them
7. Verify shift+click still toggles multi-select
