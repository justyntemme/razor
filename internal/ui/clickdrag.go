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

// Touchable is a widget that handles click, double-click, right-click, and drag gestures
// on the same area. It follows Gio's naming convention (Clickable, Draggable, Touchable).
//
// It combines:
// - gesture.Click for left-click and double-click detection
// - gesture.Drag for drag operations with visual feedback
// - pointer.Filter for right-click detection
//
// The gesture.Drag has a built-in 3dp movement threshold before it activates,
// allowing taps to fall through to gesture.Click.
type Touchable struct {
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

	// rightClicked tracks if a right-click occurred this frame
	rightClicked bool
	// rightClickPos stores the position of the right-click
	rightClickPos image.Point
}

// Dragging reports whether a drag is in progress.
func (t *Touchable) Dragging() bool {
	return t.drag.Dragging()
}

// Pressed reports whether a pointer is pressing.
func (t *Touchable) Pressed() bool {
	return t.drag.Pressed()
}

// Hovered reports whether a pointer is inside the area.
func (t *Touchable) Hovered() bool {
	return t.click.Hovered()
}

// Pos returns the current drag position relative to the start.
func (t *Touchable) Pos() f32.Point {
	return t.dragPos
}

// Update processes drag events and returns the MIME type if a drag data request was made.
// Call this before Layout to handle transfer.RequestEvent.
func (t *Touchable) Update(gtx layout.Context) (mime string, requested bool) {
	for {
		ev, ok := gtx.Event(transfer.SourceFilter{Target: t, Type: t.Type})
		if !ok {
			break
		}
		switch e := ev.(type) {
		case transfer.RequestEvent:
			return e.Type, true
		case transfer.InitiateEvent:
			// Drag initiated - we're a source
		case transfer.CancelEvent:
			// Drag cancelled
		}
	}
	return "", false
}

// Offer provides data for a drag-and-drop transfer.
func (t *Touchable) Offer(gtx layout.Context, mime string, data io.ReadCloser) {
	gtx.Execute(transfer.OfferCmd{Tag: t, Type: mime, Data: data})
}

// TouchEvent represents an interaction event from Touchable
type TouchEvent struct {
	Type      TouchEventType
	Position  image.Point
	Modifiers key.Modifiers
	NumClicks int // For click events, number of successive clicks (1 = single, 2 = double)
}

// TouchEventType indicates the type of touch event
type TouchEventType uint8

const (
	TouchNone       TouchEventType = iota
	TouchClick                     // Left-click (or tap)
	TouchRightClick                // Right-click (or secondary button)
)

// Layout renders the widget and handles click/right-click/drag interactions.
// The w widget is rendered normally. The drag widget is rendered at the drag
// position when a drag is in progress, creating the visual "shadow" effect.
// Returns the dimensions and any touch event that occurred (click or right-click).
func (t *Touchable) Layout(gtx layout.Context, w, drag layout.Widget) (layout.Dimensions, *TouchEvent) {
	if !gtx.Enabled() {
		return w(gtx), nil
	}

	var touchEvent *TouchEvent

	// Process right-click events BEFORE layout
	// These were registered in previous frame via event.Op
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: t, Kinds: pointer.Press})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok && e.Kind == pointer.Press {
			if e.Buttons.Contain(pointer.ButtonSecondary) {
				touchEvent = &TouchEvent{
					Type:      TouchRightClick,
					Position:  e.Position.Round(),
					Modifiers: e.Modifiers,
				}
			}
		}
	}

	// Process left-click events BEFORE layout (Gio pattern)
	// Events are delivered based on previous frame's hit area registration
	for {
		e, ok := t.click.Update(gtx.Source)
		if !ok {
			break
		}
		switch e.Kind {
		case gesture.KindClick:
			// Only report click if we didn't start a drag
			if !t.dragStarted {
				touchEvent = &TouchEvent{
					Type:      TouchClick,
					Position:  e.Position,
					Modifiers: e.Modifiers,
					NumClicks: e.NumClicks,
				}
			}
		case gesture.KindCancel:
			t.dragStarted = false
		}
	}

	// Process drag events
	for {
		e, ok := t.drag.Update(gtx.Metric, gtx.Source, gesture.Both)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			t.clickPos = e.Position
			t.dragPos = f32.Point{}
			t.pid = e.PointerID
			t.dragStarted = false
		case pointer.Drag:
			if e.PointerID == t.pid {
				t.dragStarted = true
				t.dragPos = e.Position.Sub(t.clickPos)
			}
		case pointer.Release, pointer.Cancel:
			t.dragStarted = false
		}
	}

	// Render the widget content
	dims := w(gtx)

	// Set up hit area for gesture handlers (for next frame's events)
	// The clip rect bounds the area where pointer events are registered
	defer clip.Rect{Max: dims.Size}.Push(gtx.Ops).Pop()

	// Register click handler
	t.click.Add(gtx.Ops)

	// Register drag handler
	t.drag.Add(gtx.Ops)

	// Register for right-click and other pointer events
	event.Op(gtx.Ops, t)

	// Render the drag shadow if dragging
	if drag != nil && t.drag.Pressed() && t.dragStarted {
		rec := op.Record(gtx.Ops)
		op.Offset(t.dragPos.Round()).Add(gtx.Ops)
		drag(gtx)
		op.Defer(gtx.Ops, rec.Stop())
	}

	return dims, touchEvent
}
