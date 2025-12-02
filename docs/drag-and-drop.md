# Drag and Drop Implementation Guide for Gio

This document provides a guide for implementing drag-and-drop file moving in Gio using the proper `widget.Draggable` and `io/transfer` APIs.

## Table of Contents

1. [Overview](#overview)
2. [Key Gio Concepts](#key-gio-concepts)
3. [Architecture](#architecture)
4. [Implementation](#implementation)
5. [Visual Feedback](#visual-feedback)
6. [Implementation Checklist](#implementation-checklist)

---

## Overview

Gio provides built-in drag-and-drop support through `widget.Draggable` for drag sources and `io/transfer` package for the transfer protocol. This is the proper way to implement drag-and-drop rather than manually handling pointer events.

### Why Use widget.Draggable

1. **Framework integration**: Works with Gio's event system without fighting for pointer events
2. **Proper transfer protocol**: Uses MIME types for data transfer between drag sources and drop targets
3. **Built-in visual feedback**: Automatically shows a drag widget during the operation
4. **Multi-touch support**: Handles pointer tracking correctly

---

## Key Gio Concepts

### Pointer Positions and Coordinate Spaces

For smooth drag operations, understanding coordinate spaces is essential:

1. **Pointer positions are LOCAL**: `e.Position.X/Y` is relative to the clip area where `event.Op` was registered
2. **Clip areas define coordinate spaces**: Each `clip.Rect{}.Push()` creates a new coordinate space
3. **Events are consumed in reverse order**: Layout happens top-down, events process bottom-up

### The Jitter Problem (for manual implementations)

If implementing drag manually (not recommended), avoid incremental deltas:

```go
// PROBLEMATIC - causes jitter
delta := int(e.Position.X - lastX)
lastX = e.Position.X  // lastX is in relative coords that shift during drag
```

Instead, calculate absolute positions using the element's known screen position.

---

## Architecture

### Components

1. **Drag Source**: Each draggable row has a `widget.Draggable` field
2. **Drop Target**: Folder rows register with `transfer.TargetFilter` to receive drops
3. **MIME Type**: Custom MIME type identifies file path data
4. **Data Format**: Paths are encoded as newline-separated strings

### Data Flow

```
Drag Start → Draggable.Update() requests data
          → Draggable.Offer() provides paths
          → User drags over folders
          → Drop on folder triggers transfer.DataEvent
          → Handler reads paths, emits ActionMove event
```

---

## Implementation

### 1. Define MIME Type

```go
// In types.go
const FileDragMIME = "application/x-razor-file-paths"
```

### 2. Add Draggable to Entry

```go
type UIEntry struct {
    Name, Path string
    IsDir      bool
    // ... other fields ...
    Draggable  widget.Draggable // For drag-and-drop
}
```

### 3. Set Up Drag Source

In the row rendering function:

```go
func (r *Renderer) renderRow(gtx layout.Context, item *UIEntry, state *State) layout.Dimensions {
    // Set MIME type for the draggable
    item.Draggable.Type = FileDragMIME

    // Check if drag data is being requested
    if mime, ok := item.Draggable.Update(gtx); ok {
        // Collect paths to drag (single item or multi-select)
        var paths []string
        if isMultiSelect {
            for idx := range state.SelectedIndices {
                paths = append(paths, state.Entries[idx].Path)
            }
        } else {
            paths = []string{item.Path}
        }
        // Provide the data as newline-separated paths
        pathData := strings.Join(paths, "\n")
        item.Draggable.Offer(gtx, FileDragMIME, strings.NewReader(pathData))
    }

    // Wrap row with Draggable.Layout
    return item.Draggable.Layout(gtx,
        // Normal widget (when not dragging)
        func(gtx layout.Context) layout.Dimensions {
            return r.renderRowContent(gtx, item)
        },
        // Drag widget (shown during drag)
        func(gtx layout.Context) layout.Dimensions {
            return r.renderDragPreview(gtx, item, state)
        },
    )
}
```

### 4. Set Up Drop Targets

Register folder rows as drop targets and handle drop events:

```go
func (r *Renderer) renderRow(gtx layout.Context, item *UIEntry) {
    // Register as drop target if this is a directory
    if item.IsDir {
        // Check for drop events
        for {
            ev, ok := gtx.Event(transfer.TargetFilter{
                Target: item,  // Use item pointer as stable tag
                Type:   FileDragMIME,
            })
            if !ok {
                break
            }
            switch e := ev.(type) {
            case transfer.DataEvent:
                // Read the dropped data
                data := e.Open()
                content, err := io.ReadAll(data)
                data.Close()
                if err == nil {
                    paths := strings.Split(string(content), "\n")
                    // Emit move event
                    eventOut = UIEvent{
                        Action: ActionMove,
                        Paths:  paths,
                        Path:   item.Path, // Destination folder
                    }
                }
            }
        }

        // Register drop target in layout
        event.Op(gtx.Ops, item)
    }
}
```

### 5. Handle the Move Event

In the orchestrator:

```go
case ui.ActionMove:
    if len(evt.Paths) > 0 && evt.Path != "" {
        go o.doMove(evt.Paths, evt.Path)
    }
```

---

## Visual Feedback

### Drag Preview Widget

The second function passed to `Draggable.Layout()` renders the drag preview:

```go
func (r *Renderer) renderDragPreview(gtx layout.Context, item *UIEntry, state *State) layout.Dimensions {
    // Count items being dragged
    dragCount := 1
    if len(state.SelectedIndices) > 1 {
        dragCount = len(state.SelectedIndices)
    }

    // Draw compact preview box
    bgColor := color.NRGBA{R: 255, G: 255, B: 255, A: 230}
    borderColor := color.NRGBA{R: 66, G: 133, B: 244, A: 255}

    width, height := gtx.Dp(200), gtx.Dp(32)

    // Draw background with border
    paint.FillShape(gtx.Ops, borderColor, clip.Rect{Max: image.Pt(width, height)}.Op())
    inner := image.Rect(2, 2, width-2, height-2)
    paint.FillShape(gtx.Ops, bgColor, clip.Rect{Min: inner.Min, Max: inner.Max}.Op())

    // Draw label
    var label string
    if dragCount > 1 {
        label = fmt.Sprintf("Moving %d items", dragCount)
    } else {
        label = item.Name
    }

    // ... render label ...

    return layout.Dimensions{Size: image.Pt(width, height)}
}
```

### Drop Target Highlighting

Track which folder is being hovered and highlight it:

```go
// In row rendering, after checking drop events
isDropTarget := item.IsDir && r.dropTargetPath == item.Path

if isDropTarget {
    // Draw blue border highlight
    borderColor := color.NRGBA{R: 66, G: 133, B: 244, A: 255}
    borderWidth := gtx.Dp(2)
    bounds := gtx.Constraints.Max

    paint.FillShape(gtx.Ops, borderColor, clip.Rect{Max: bounds}.Op())
    inner := image.Rect(borderWidth, borderWidth, bounds.X-borderWidth, bounds.Y-borderWidth)
    paint.FillShape(gtx.Ops, bgColor, clip.Rect{Min: inner.Min, Max: inner.Max}.Op())
}
```

---

## Implementation Checklist

### Setup

- [ ] Define custom MIME type constant
- [ ] Add `widget.Draggable` field to `UIEntry`
- [ ] Add `ActionMove` to `UIAction` enum

### Drag Source

- [ ] Set `item.Draggable.Type = FileDragMIME`
- [ ] Handle `item.Draggable.Update(gtx)` to detect data requests
- [ ] Call `item.Draggable.Offer()` with paths data
- [ ] Use `item.Draggable.Layout()` to wrap row rendering
- [ ] Implement drag preview widget function

### Drop Target

- [ ] Register `transfer.TargetFilter` on folder rows
- [ ] Handle `transfer.DataEvent` to read dropped paths
- [ ] Call `event.Op(gtx.Ops, item)` for directories
- [ ] Emit `ActionMove` event with source paths and destination

### Visual Feedback

- [ ] Implement drag preview widget showing item count/name
- [ ] Track drop target path for hover highlighting
- [ ] Draw border highlight on valid drop targets

### Backend

- [ ] Handle `ActionMove` in orchestrator
- [ ] Implement `doMove()` function for file operations
- [ ] Handle name conflicts appropriately
- [ ] Show progress during move operations

### Testing

- [ ] Test single file drag to folder
- [ ] Test multi-select drag to folder
- [ ] Test drag to invalid targets (files, same folder)
- [ ] Test drag cancel (press Escape, release outside window)
- [ ] Test visual feedback during drag

---

## References

- Gio widget.Draggable: https://pkg.go.dev/gioui.org/widget#Draggable
- Gio io/transfer: https://pkg.go.dev/gioui.org/io/transfer
- Reference implementation: https://github.com/oligo/gioview
- Column resize (manual drag): `internal/ui/renderer_rows.go:renderColumns()`
