package ui

import (
	"image"
	"io"

	"gioui.org/f32"
	"gioui.org/gesture"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
)

// Note: transfer is used in Update() for SourceFilter

// ClickAndDraggable is a custom widget that handles both click and drag gestures
// on the same area. It combines the functionality of gesture.Click for click/double-click
// detection and gesture.Drag for drag operations with visual feedback.
//
// The key insight is that gesture.Drag has a built-in 3dp movement threshold before
// it activates. By processing click events first and only allowing drag to grab the
// pointer after movement, we can support both interactions on the same widget.
type ClickAndDraggable struct {
	// Type contains the MIME type for drag-and-drop transfers
	Type string

	click gesture.Click
	drag  gesture.Drag

	// Drag state for rendering the drag shadow
	clickPos f32.Point // Position where drag started
	dragPos  f32.Point // Current drag position (relative to start)

	// pid tracks the pointer ID for coordinating click vs drag
	pid pointer.ID
	// dragStarted indicates drag threshold was exceeded
	dragStarted bool
}

// Dragging reports whether a drag is in progress.
func (c *ClickAndDraggable) Dragging() bool {
	return c.drag.Dragging()
}

// Pressed reports whether a pointer is pressing.
func (c *ClickAndDraggable) Pressed() bool {
	return c.drag.Pressed()
}

// Hovered reports whether a pointer is inside the area.
func (c *ClickAndDraggable) Hovered() bool {
	return c.click.Hovered()
}

// Pos returns the current drag position relative to the start.
func (c *ClickAndDraggable) Pos() f32.Point {
	return c.dragPos
}

// Update processes drag events and returns the MIME type if a drag data request was made.
// Call this before Layout to handle transfer.RequestEvent.
func (c *ClickAndDraggable) Update(gtx layout.Context) (mime string, requested bool) {
	for {
		ev, ok := gtx.Event(transfer.SourceFilter{Target: c, Type: c.Type})
		if !ok {
			break
		}
		if e, ok := ev.(transfer.RequestEvent); ok {
			return e.Type, true
		}
	}
	return "", false
}

// Offer provides data for a drag-and-drop transfer.
func (c *ClickAndDraggable) Offer(gtx layout.Context, mime string, data io.ReadCloser) {
	gtx.Execute(transfer.OfferCmd{Tag: c, Type: mime, Data: data})
}

// Clicked reports whether a click occurred. This consumes the click event.
// For more control, use Layout and check the returned events.
func (c *ClickAndDraggable) Clicked(gtx layout.Context) bool {
	for {
		e, ok := c.click.Update(gtx.Source)
		if !ok {
			break
		}
		if e.Kind == gesture.KindClick {
			return true
		}
	}
	return false
}

// ClickEvent represents a click event with position and modifier information
type ClickEvent struct {
	Position  image.Point
	Modifiers key.Modifiers
	NumClicks int
}

// Layout renders the widget and handles click/drag interactions.
// The w widget is rendered normally. The drag widget is rendered at the drag
// position when a drag is in progress, creating the visual "shadow" effect.
// Returns the dimensions and any click event that occurred.
func (c *ClickAndDraggable) Layout(gtx layout.Context, w, drag layout.Widget) (layout.Dimensions, *ClickEvent) {
	if !gtx.Enabled() {
		return w(gtx), nil
	}

	var clickEvent *ClickEvent

	// Process click events BEFORE layout (Gio pattern)
	// Events are delivered based on previous frame's hit area registration
	for {
		e, ok := c.click.Update(gtx.Source)
		if !ok {
			break
		}
		switch e.Kind {
		case gesture.KindClick:
			// Only report click if we didn't start a drag
			if !c.dragStarted {
				clickEvent = &ClickEvent{
					Position:  e.Position,
					Modifiers: e.Modifiers,
					NumClicks: e.NumClicks,
				}
			}
		case gesture.KindCancel:
			c.dragStarted = false
		}
	}

	// Process drag events
	for {
		e, ok := c.drag.Update(gtx.Metric, gtx.Source, gesture.Both)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			c.clickPos = e.Position
			c.dragPos = f32.Point{}
			c.pid = e.PointerID
			c.dragStarted = false
		case pointer.Drag:
			if e.PointerID == c.pid {
				c.dragStarted = true
				c.dragPos = e.Position.Sub(c.clickPos)
			}
		case pointer.Release, pointer.Cancel:
			c.dragStarted = false
		}
	}

	// Render the widget content
	dims := w(gtx)

	// Set up hit area for gesture handlers (for next frame's events)
	// The clip rect bounds the area where pointer events are registered
	// Using defer ensures the clip is properly scoped
	defer clip.Rect{Max: dims.Size}.Push(gtx.Ops).Pop()
	c.click.Add(gtx.Ops)
	c.drag.Add(gtx.Ops)
	event.Op(gtx.Ops, c)

	// Render the drag shadow if dragging
	if drag != nil && c.drag.Pressed() && c.dragStarted {
		rec := op.Record(gtx.Ops)
		op.Offset(c.dragPos.Round()).Add(gtx.Ops)
		drag(gtx)
		op.Defer(gtx.Ops, rec.Stop())
	}

	return dims, clickEvent
}
