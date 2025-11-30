# Gio UI Patterns Guide

This document explains how Razor uses the [Gio](https://gioui.org) immediate-mode GUI framework. Understanding these patterns is essential for modifying the UI.

## Table of Contents

1. [Immediate-Mode Basics](#immediate-mode-basics)
2. [Layout System](#layout-system)
3. [Widget Patterns](#widget-patterns)
4. [Event Handling](#event-handling)
5. [Styling and Drawing](#styling-and-drawing)
6. [Animation](#animation)
7. [Common Patterns in Razor](#common-patterns-in-razor)

---

## Immediate-Mode Basics

Unlike retained-mode GUIs (React, Qt), Gio redraws everything every frame:

```go
// Main event loop
for {
    switch e := window.Event().(type) {
    case app.FrameEvent:
        gtx := app.NewContext(&ops, e)

        // EVERY frame: rebuild entire UI from state
        event := renderer.Layout(gtx, &state)

        // Submit drawing operations
        e.Frame(gtx.Ops)
    }
}
```

### Key Implications

1. **No UI tree to update** - just redraw from current state
2. **State lives in your structs** - not in UI components
3. **Simpler mental model** - state → render → events → update state → render
4. **Performance** - Gio optimizes drawing, you just describe what to show

### The Frame Cycle

```
┌─────────────────────────────────────────────────────┐
│                    FrameEvent                        │
│                         │                            │
│                         ▼                            │
│              Create layout.Context (gtx)            │
│                         │                            │
│                         ▼                            │
│              Call Layout functions                   │
│              ├─ Read widget state                    │
│              ├─ Check for events (clicks, keys)     │
│              ├─ Draw UI elements                     │
│              └─ Return dimensions                    │
│                         │                            │
│                         ▼                            │
│              Submit ops to GPU (e.Frame)            │
│                         │                            │
│                         ▼                            │
│              Wait for next FrameEvent               │
└─────────────────────────────────────────────────────┘
```

---

## Layout System

Gio uses a constraint-based layout system similar to CSS Flexbox.

### layout.Context (gtx)

Every layout function receives a `layout.Context`:

```go
type Context struct {
    Ops         *op.Ops       // Drawing operations
    Constraints Constraints   // Min/max size
    Metric      unit.Metric   // Dp to pixels conversion
    Now         time.Time     // Current frame time
    // ...
}
```

### layout.Dimensions

Every layout function returns dimensions:

```go
type Dimensions struct {
    Size     image.Point  // Actual size used
    Baseline int          // Text baseline (optional)
}
```

### Flex Layout

The most common layout pattern:

```go
// Vertical layout (column)
layout.Flex{Axis: layout.Vertical}.Layout(gtx,
    layout.Rigid(header),    // Fixed size
    layout.Flexed(1, body),  // Takes remaining space
    layout.Rigid(footer),    // Fixed size
)

// Horizontal layout (row)
layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
    layout.Rigid(sidebar),   // Fixed width
    layout.Flexed(1, main),  // Takes remaining space
)
```

**File Reference**: [`internal/ui/layout.go:55-101`](/Users/justyntemme/Documents/code/razor/internal/ui/layout.go)

```go
// Main application layout
return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
    // File menu bar
    layout.Rigid(func(gtx layout.Context) layout.Dimensions {
        return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, ...)
    }),

    // Navigation bar
    layout.Rigid(func(gtx layout.Context) layout.Dimensions {
        return r.layoutNavBar(gtx, state, keyTag, &eventOut)
    }),

    // Main content (sidebar + file list)
    layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
        return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
            layout.Rigid(sidebar),      // Fixed 180dp
            layout.Rigid(divider),      // 1dp line
            layout.Flexed(1, fileList), // Remaining space
        )
    }),

    // Progress bar
    layout.Rigid(func(gtx layout.Context) layout.Dimensions {
        return r.layoutProgressBar(gtx, state)
    }),
)
```

### Stack Layout

For overlays (modals, menus):

```go
layout.Stack{}.Layout(gtx,
    // Background layer
    layout.Expanded(func(gtx layout.Context) layout.Dimensions {
        // Fills entire area
    }),

    // Content layer (on top)
    layout.Stacked(func(gtx layout.Context) layout.Dimensions {
        // Positioned content
    }),
)
```

**File Reference**: [`internal/ui/layout.go:38-109`](/Users/justyntemme/Documents/code/razor/internal/ui/layout.go)

### Inset (Padding/Margin)

```go
layout.Inset{
    Top:    unit.Dp(8),
    Bottom: unit.Dp(8),
    Left:   unit.Dp(12),
    Right:  unit.Dp(12),
}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
    // Content with padding
})

// Shorthand for uniform padding
layout.UniformInset(unit.Dp(20)).Layout(gtx, content)
```

### Spacer

```go
layout.Spacer{Width: unit.Dp(8)}.Layout   // Horizontal space
layout.Spacer{Height: unit.Dp(16)}.Layout // Vertical space
```

### List (Scrollable)

For scrollable content:

```go
// Define list state (persistent)
var listState layout.List
listState.Axis = layout.Vertical

// Render
listState.Layout(gtx, len(items), func(gtx layout.Context, index int) layout.Dimensions {
    return renderItem(gtx, items[index])
})
```

**File Reference**: [`internal/ui/layout.go:521-568`](/Users/justyntemme/Documents/code/razor/internal/ui/layout.go)

```go
// File list rendering
dims := r.listState.Layout(gtx, len(state.Entries), func(gtx layout.Context, i int) layout.Dimensions {
    item := &state.Entries[i]
    rowDims, leftClicked, rightClicked, clickPos, renameEvt := r.renderRow(gtx, item, i, i == state.SelectedIndex, isRenaming)

    if leftClicked && !isRenaming {
        *eventOut = UIEvent{Action: ActionSelect, NewIndex: i}
        // Double-click detection
        if now := time.Now(); !item.LastClick.IsZero() && now.Sub(item.LastClick) < 500*time.Millisecond {
            if item.IsDir {
                *eventOut = UIEvent{Action: ActionNavigate, Path: item.Path}
            }
        }
        item.LastClick = time.Now()
    }

    return rowDims
})
```

---

## Widget Patterns

### Clickable (Buttons, List Items)

```go
// State (persistent across frames)
var btn widget.Clickable

// Check for click BEFORE layout
if btn.Clicked(gtx) {
    // Handle click
}

// Render
material.Clickable(gtx, &btn, func(gtx layout.Context) layout.Dimensions {
    // Button content
})

// Or use material button
material.Button(theme, &btn, "Label").Layout(gtx)
```

**File Reference**: [`internal/ui/renderer.go:515-598`](/Users/justyntemme/Documents/code/razor/internal/ui/renderer.go)

```go
func (r *Renderer) renderRow(gtx layout.Context, item *UIEntry, ...) (...) {
    // Check for left-click BEFORE layout (Gio pattern)
    leftClicked := item.Clickable.Clicked(gtx)

    // ... handle click ...

    dims := material.Clickable(gtx, &item.Clickable, func(gtx layout.Context) layout.Dimensions {
        // Row content
        return r.renderRowContent(gtx, item, false)
    })

    return dims, leftClicked, rightClicked, clickPos, renameEvent
}
```

### Editor (Text Input)

```go
// State
var editor widget.Editor
editor.SingleLine = true  // No line breaks
editor.Submit = true      // Enter triggers submit event

// Check for events
for {
    evt, ok := editor.Update(gtx)
    if !ok {
        break
    }
    switch evt.(type) {
    case widget.ChangeEvent:
        // Text changed
        text := editor.Text()
    case widget.SubmitEvent:
        // Enter pressed
        text := editor.Text()
    }
}

// Render
material.Editor(theme, &editor, "Placeholder").Layout(gtx)
```

**File Reference**: [`internal/ui/layout.go:261-292`](/Users/justyntemme/Documents/code/razor/internal/ui/layout.go)

```go
// Search box with directive detection
for {
    evt, ok := r.searchEditor.Update(gtx)
    if !ok {
        break
    }
    switch e := evt.(type) {
    case widget.ChangeEvent:
        text := r.searchEditor.Text()
        r.detectedDirectives, _ = parseDirectivesForDisplay(text)
        // Search-as-you-type (if no directives)
        if !hasDirective(text) && text != "" {
            *eventOut = UIEvent{Action: ActionSearch, Path: text}
        }
    case widget.SubmitEvent:
        // Enter pressed - run full search
        *eventOut = UIEvent{Action: ActionSearch, Path: r.searchEditor.Text()}
    }
}
```

### Enum (Radio Buttons)

```go
// State
var choice widget.Enum

// Check for selection change
if choice.Update(gtx) {
    selected := choice.Value
}

// Render options
material.RadioButton(theme, &choice, "option1", "Option 1").Layout(gtx)
material.RadioButton(theme, &choice, "option2", "Option 2").Layout(gtx)
```

**File Reference**: [`internal/ui/layout.go:1366-1388`](/Users/justyntemme/Documents/code/razor/internal/ui/layout.go)

### Bool (Checkbox)

```go
// State
var checked widget.Bool

// Check for change
if checked.Update(gtx) {
    isChecked := checked.Value
}

// Render
material.CheckBox(theme, &checked, "Label").Layout(gtx)
```

---

## Event Handling

### Keyboard Events

```go
// Request focus
gtx.Execute(key.FocusCmd{Tag: &someWidget})

// Handle key events
for {
    ev, ok := gtx.Event(key.Filter{Focus: true, Name: ""})
    if !ok {
        break
    }
    if k, ok := ev.(key.Event); ok && k.State == key.Press {
        switch k.Name {
        case "Up":
            // Handle up arrow
        case "Down":
            // Handle down arrow
        case "C":
            if k.Modifiers.Contain(key.ModCtrl) {
                // Ctrl+C
            }
        case key.NameEscape:
            // Escape key
        }
    }
}
```

**File Reference**: [`internal/ui/renderer.go:384-460`](/Users/justyntemme/Documents/code/razor/internal/ui/renderer.go)

```go
func (r *Renderer) processGlobalInput(gtx layout.Context, state *State) UIEvent {
    for {
        e, ok := gtx.Event(key.Filter{Focus: true, Name: ""})
        if !ok {
            break
        }
        if r.isEditing || r.settingsOpen || r.deleteConfirmOpen || r.createDialogOpen {
            continue  // Skip global shortcuts when modal is open
        }
        k, ok := e.(key.Event)
        if !ok || k.State != key.Press {
            continue
        }
        switch k.Name {
        case "Up":
            if idx := state.SelectedIndex - 1; idx >= 0 {
                r.listState.ScrollTo(idx)
                return UIEvent{Action: ActionSelect, NewIndex: idx}
            }
        case "Down":
            // ...
        case "C":
            if k.Modifiers.Contain(key.ModCtrl) && state.SelectedIndex >= 0 {
                return UIEvent{Action: ActionCopy, Path: state.Entries[state.SelectedIndex].Path}
            }
        }
    }
    return UIEvent{}
}
```

### Pointer (Mouse) Events

```go
// Register event handler for a region
defer clip.Rect{Max: bounds}.Push(gtx.Ops).Pop()
event.Op(gtx.Ops, &tag)

// Check for pointer events
for {
    ev, ok := gtx.Event(pointer.Filter{
        Target: &tag,
        Kinds:  pointer.Press | pointer.Release,
    })
    if !ok {
        break
    }
    if e, ok := ev.(pointer.Event); ok {
        if e.Kind == pointer.Press {
            if e.Buttons.Contain(pointer.ButtonPrimary) {
                // Left click
            }
            if e.Buttons.Contain(pointer.ButtonSecondary) {
                // Right click
                pos := image.Pt(int(e.Position.X), int(e.Position.Y))
            }
        }
    }
}
```

**File Reference**: [`internal/ui/renderer.go:520-532`](/Users/justyntemme/Documents/code/razor/internal/ui/renderer.go)

```go
// Right-click detection on file rows
rightClicked := false
var clickPos image.Point
for {
    ev, ok := gtx.Event(pointer.Filter{Target: &item.RightClickTag, Kinds: pointer.Press})
    if !ok {
        break
    }
    if e, ok := ev.(pointer.Event); ok && e.Buttons.Contain(pointer.ButtonSecondary) {
        rightClicked = true
        clickPos = image.Pt(int(e.Position.X), int(e.Position.Y))
    }
}
```

---

## Styling and Drawing

### Colors

```go
// Define colors as NRGBA (non-premultiplied alpha)
var (
    colWhite    = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
    colBlack    = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
    colSelected = color.NRGBA{R: 200, G: 220, B: 255, A: 255}
    colSidebar  = color.NRGBA{R: 245, G: 245, B: 245, A: 255}
)
```

### Fill Shapes

```go
// Rectangle fill
paint.FillShape(gtx.Ops, color,
    clip.Rect{Max: image.Pt(width, height)}.Op())

// Rounded rectangle
paint.FillShape(gtx.Ops, color,
    clip.RRect{
        Rect: image.Rect(0, 0, width, height),
        NE: radius, NW: radius, SE: radius, SW: radius,
    }.Op(gtx.Ops))
```

**File Reference**: [`internal/ui/layout.go:82`](/Users/justyntemme/Documents/code/razor/internal/ui/layout.go)

```go
// Sidebar background
paint.FillShape(gtx.Ops, colSidebar, clip.Rect{Max: gtx.Constraints.Max}.Op())
```

### Borders

```go
widget.Border{
    Color:        colGray,
    Width:        unit.Dp(1),
    CornerRadius: unit.Dp(4),
}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
    // Content inside border
})
```

### Clipping

```go
// Clip to rectangle (content outside is hidden)
defer clip.Rect{Max: bounds}.Push(gtx.Ops).Pop()

// Must use defer to ensure Pop() is called after drawing
```

### Material Theme

```go
// Create theme
theme := material.NewTheme()

// Customize palette (for dark mode)
theme.Palette.Bg = color.NRGBA{R: 30, G: 30, B: 30, A: 255}
theme.Palette.Fg = color.NRGBA{R: 220, G: 220, B: 220, A: 255}

// Use in widgets
lbl := material.Body1(theme, "Text")
lbl.Color = customColor
lbl.Font.Weight = font.Bold
lbl.MaxLines = 1  // Truncate with ellipsis
lbl.Layout(gtx)
```

---

## Animation

### Basic Animation Pattern

```go
// Track animation start time
var animStart time.Time

// In layout function
if shouldAnimate {
    if animStart.IsZero() {
        animStart = gtx.Now
    }

    elapsed := gtx.Now.Sub(animStart).Seconds()
    progress := float32(elapsed / duration)

    // Use progress (0.0 to 1.0) to interpolate values

    // Request next frame
    gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(16 * time.Millisecond)})
}
```

**File Reference**: [`internal/ui/layout.go:794-810`](/Users/justyntemme/Documents/code/razor/internal/ui/layout.go)

```go
// Indeterminate progress bar (sliding animation)
func (r *Renderer) layoutIndeterminateProgress(gtx layout.Context) layout.Dimensions {
    // Animate!
    if r.progressAnimStart.IsZero() {
        r.progressAnimStart = gtx.Now
    }

    elapsed := gtx.Now.Sub(r.progressAnimStart).Seconds()
    cycle := float32(elapsed) / 1.5
    pos := cycle - float32(int(cycle))

    // Ping-pong animation
    if int(cycle)%2 == 1 {
        pos = 1.0 - pos
    }

    // Draw sliding bar at calculated position
    barWidth := float32(gtx.Constraints.Max.X) * 0.3
    barX := pos * (float32(gtx.Constraints.Max.X) - barWidth)

    paint.FillShape(gtx.Ops, colProgress,
        clip.Rect{
            Min: image.Pt(int(barX), 0),
            Max: image.Pt(int(barX+barWidth), gtx.Constraints.Max.Y),
        }.Op())

    // Request next frame
    gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(16 * time.Millisecond)})

    return layout.Dimensions{Size: gtx.Constraints.Max}
}
```

---

## Common Patterns in Razor

### Modal Dialog Pattern

```go
func (r *Renderer) layoutModal(gtx layout.Context) layout.Dimensions {
    if !r.modalOpen {
        return layout.Dimensions{}
    }

    // Handle close button
    if r.closeBtn.Clicked(gtx) {
        r.modalOpen = false
    }

    return layout.Stack{}.Layout(gtx,
        // Semi-transparent backdrop
        layout.Expanded(func(gtx layout.Context) layout.Dimensions {
            paint.FillShape(gtx.Ops, color.NRGBA{A: 150},
                clip.Rect{Max: gtx.Constraints.Max}.Op())
            return material.Clickable(gtx, &r.backdropClick, func(gtx layout.Context) layout.Dimensions {
                return layout.Dimensions{Size: gtx.Constraints.Max}
            })
        }),

        // Centered dialog
        layout.Stacked(func(gtx layout.Context) layout.Dimensions {
            return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
                return r.dialogContent(gtx)
            })
        }),
    )
}
```

### Context Menu Pattern

```go
// Track menu state
type Renderer struct {
    menuVisible bool
    menuPos     image.Point
    menuPath    string
}

// Show on right-click
if rightClicked {
    r.menuVisible = true
    r.menuPos = clickPosition
    r.menuPath = item.Path
}

// Render menu
func (r *Renderer) layoutContextMenu(gtx layout.Context) layout.Dimensions {
    if !r.menuVisible {
        return layout.Dimensions{}
    }

    // Position menu at mouse location
    defer op.Offset(r.menuPos).Push(gtx.Ops).Pop()

    return r.menuShell(gtx, 150, func(gtx layout.Context) layout.Dimensions {
        return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
            layout.Rigid(func(gtx layout.Context) layout.Dimensions {
                if r.openBtn.Clicked(gtx) {
                    r.menuVisible = false
                    *eventOut = UIEvent{Action: ActionOpen, Path: r.menuPath}
                }
                return r.menuItem(gtx, &r.openBtn, "Open")
            }),
            // ... more menu items
        )
    })
}
```

### Double-Click Detection

```go
type UIEntry struct {
    LastClick time.Time  // Track last click time
    // ...
}

// In click handler
if item.Clickable.Clicked(gtx) {
    now := time.Now()
    if !item.LastClick.IsZero() && now.Sub(item.LastClick) < 500*time.Millisecond {
        // Double-click!
        handleDoubleClick(item)
    } else {
        // Single click
        handleSingleClick(item)
    }
    item.LastClick = now
}
```

### Recording and Replay

For complex custom rendering:

```go
// Record operations without executing
macro := op.Record(gtx.Ops)
dims := someContent(gtx)
call := macro.Stop()

// Draw background sized to content
paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: dims.Size}.Op())

// Replay content on top
call.Add(gtx.Ops)
```

**File Reference**: [`internal/ui/renderer.go:572-580`](/Users/justyntemme/Documents/code/razor/internal/ui/renderer.go)

```go
// Inline rename with custom background
if isRenaming {
    macro := op.Record(gtx.Ops)
    contentDims := r.renderRowContent(gtx, item, true)
    call := macro.Stop()

    // Draw selection background sized to content
    paint.FillShape(gtx.Ops, colSelected, clip.Rect{Max: contentDims.Size}.Op())
    // Replay the content on top
    call.Add(gtx.Ops)

    dims = contentDims
}
```

---

## Performance Tips

1. **Avoid allocations in layout functions** - they run every frame
2. **Use persistent widget state** - store in struct fields, not local variables
3. **Early return from invisible content** - check visibility before rendering
4. **Use layout.List for long lists** - it virtualizes rendering
5. **Batch similar operations** - Gio optimizes consecutive same-type ops

---

## Further Reading

- [Gio Documentation](https://gioui.org/doc)
- [Gio Architecture](https://gioui.org/doc/architecture)
- [Gio Examples](https://github.com/gioui/gio-example)
