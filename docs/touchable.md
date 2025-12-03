# Touchable Widget: Unified Click, Right-Click, and Drag Handling

## The Problem

Gio UI framework provides separate gesture handlers for clicks and drags:
- `gesture.Click` - Handles left-clicks, double-clicks, and hover detection
- `gesture.Drag` - Handles drag operations with a 3dp movement threshold
- `widget.Draggable` - Higher-level widget that wraps `gesture.Drag` with transfer support

The challenge: **Using both `Clickable` and `Draggable` on the same UI element causes conflicts**. When `Draggable` is active, it captures all pointer events, preventing `Clickable` from receiving clicks. This is because drag gestures need to intercept the initial press to track potential drags.

## Failed Approaches

### 1. Using `widget.Draggable` Directly
The official `widget.Draggable` worked for drag-and-drop but completely blocked:
- Single clicks for selection
- Double-clicks for opening files/folders
- Right-clicks for context menus

### 2. Layering Click and Drag Handlers
Attempted to layer `gesture.Click` on top of `gesture.Drag`:
- Drag would still capture events before Click could process them
- The 3dp movement threshold in Drag doesn't help because the initial press is captured

## The Solution: Touchable Widget

We created a custom `Touchable` widget that combines all interaction types into a single unified handler.

### Key Insights

1. **`gesture.Drag` has built-in touch slop**: It won't report drag events until the pointer moves 3dp from the initial press point. This allows clicks to "fall through" to `gesture.Click` if the user doesn't drag.

2. **Track drag state explicitly**: By tracking `dragStarted` ourselves, we can suppress click events when a drag actually occurred.

3. **Right-click is separate**: Right-clicks don't conflict with drag because they use `pointer.ButtonSecondary`, while drags use `pointer.ButtonPrimary`.

### Implementation

```go
type Touchable struct {
    Type string           // MIME type for drag-and-drop transfers

    click gesture.Click   // For click, double-click, and hover
    drag  gesture.Drag    // For drag operations

    clickPos    f32.Point // Position where drag started
    dragPos     f32.Point // Current drag position (relative to start)
    pid         pointer.ID
    dragStarted bool      // True if drag threshold was exceeded
}
```

**Event Processing Order**:
1. Process right-click events via `pointer.Filter` with `pointer.ButtonSecondary`
2. Process left-click events via `gesture.Click.Update()` - but only report clicks if `!dragStarted`
3. Process drag events via `gesture.Drag.Update()` - set `dragStarted = true` on `pointer.Drag`

**Layout**:
1. Render the content widget to get dimensions
2. Set up clip area with content dimensions
3. Register `gesture.Click` and `gesture.Drag` handlers
4. Register `event.Op` for right-click and transfer events
5. Render drag shadow if dragging (using `op.Defer` for z-order)

### Drop Target Registration

Drop targets (directories) are registered separately from the Touchable:
```go
if item.IsDir {
    stack := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
    event.Op(gtx.Ops, &item.DropTag)
    stack.Pop()
}
```

The drop target registration happens AFTER `Touchable.Layout()` to avoid z-order conflicts that would block click/hover detection.

## Transfer Protocol

Gio's drag-and-drop transfer system works as follows:

1. **Source Registration**: The drag source registers via `event.Op(gtx.Ops, source)` inside a clip area
2. **Target Registration**: Drop targets register via `event.Op(gtx.Ops, target)` inside their clip areas
3. **MIME Type Matching**: Source and target must share a common MIME type
4. **Event Flow**:
   - `InitiateEvent` sent to source and all potential targets when drag starts
   - `RequestEvent` sent to source when drop occurs on a target
   - Source responds with `OfferCmd` containing the data
   - Target receives `DataEvent` with the transferred data
   - `CancelEvent` sent to all when drag is cancelled or completed

### Our Implementation

```go
const FileDragMIME = "application/x-razor-file-paths"

// Source side (in Touchable.Update):
func (t *Touchable) Update(gtx layout.Context) (mime string, requested bool) {
    for {
        ev, ok := gtx.Event(transfer.SourceFilter{Target: t, Type: t.Type})
        if !ok { break }
        if e, ok := ev.(transfer.RequestEvent); ok {
            return e.Type, true
        }
    }
    return "", false
}

// Target side (in renderRow):
for {
    ev, ok := gtx.Event(transfer.TargetFilter{Target: &item.DropTag, Type: FileDragMIME})
    if !ok { break }
    if e, ok := ev.(transfer.DataEvent); ok && e.Type == FileDragMIME {
        reader := e.Open()
        data, _ := io.ReadAll(reader)
        reader.Close()
        sourcePath := string(data)
        // Create move event...
    }
}
```

## Lessons Learned

1. **Unique Tags**: Each drop target needs a unique tag. Using `int` defaulting to `0` caused all directories to share the same tag. Use `struct{}` so each instance has a unique address.

2. **Event Processing Order**: Process events BEFORE registering handlers (Gio delivers events based on previous frame's registrations).

3. **Clip Area Ordering**: The z-order of clip areas affects which elements receive events. Drop target registration after Touchable.Layout() prevents blocking click events.

4. **Hover During Drag**: `gesture.Click.Hovered()` doesn't work during drag because the drag gesture captures the pointer. For drag hover detection, rely on the transfer system's hit-testing at drop time.

5. **Debug Logging Performance**: Extensive debug logging in hot paths (like row rendering) significantly impacts performance. Remove or guard with flags.

## Files

- `internal/ui/clickdrag.go` - Touchable widget implementation
- `internal/ui/types.go` - UIEntry with Touch field and FileDragMIME constant
- `internal/ui/renderer_rows.go` - Row rendering with drag/drop support
- `internal/app/file_ops.go` - doMove() for handling drop operations
