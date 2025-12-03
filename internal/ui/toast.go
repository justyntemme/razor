package ui

import (
	"image"
	"image/color"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// ToastType indicates the severity/type of toast message
type ToastType int

const (
	ToastInfo ToastType = iota
	ToastSuccess
	ToastWarning
	ToastError
)

// Toast represents a temporary notification message
type Toast struct {
	Message   string
	Type      ToastType
	Visible   bool
	ExpiresAt time.Time
	mu        sync.Mutex
}

// toastDuration is how long toasts are displayed
const toastDuration = 3 * time.Second

// ShowToast displays a toast notification that auto-dismisses
func (r *Renderer) ShowToast(message string, toastType ToastType) {
	r.toast.mu.Lock()
	defer r.toast.mu.Unlock()

	r.toast.Message = message
	r.toast.Type = toastType
	r.toast.Visible = true
	r.toast.ExpiresAt = time.Now().Add(toastDuration)
}

// ShowError is a convenience method for showing error toasts
func (r *Renderer) ShowError(message string) {
	r.ShowToast(message, ToastError)
}

// ShowSuccess is a convenience method for showing success toasts
func (r *Renderer) ShowSuccess(message string) {
	r.ShowToast(message, ToastSuccess)
}

// updateToast checks if the toast should be hidden
func (r *Renderer) updateToast() {
	r.toast.mu.Lock()
	defer r.toast.mu.Unlock()

	if r.toast.Visible && time.Now().After(r.toast.ExpiresAt) {
		r.toast.Visible = false
	}
}

// layoutToast renders the toast notification at the bottom of the screen
func (r *Renderer) layoutToast(gtx layout.Context, th *material.Theme) layout.Dimensions {
	r.updateToast()

	r.toast.mu.Lock()
	visible := r.toast.Visible
	message := r.toast.Message
	toastType := r.toast.Type
	expiresAt := r.toast.ExpiresAt
	r.toast.mu.Unlock()

	if !visible || message == "" {
		return layout.Dimensions{}
	}

	// Schedule redraw when toast should expire
	timeUntilExpiry := time.Until(expiresAt)
	if timeUntilExpiry > 0 {
		gtx.Execute(op.InvalidateCmd{At: expiresAt})
	}

	// Toast colors based on type
	var bgColor, textColor color.NRGBA
	switch toastType {
	case ToastError:
		bgColor = color.NRGBA{R: 200, G: 50, B: 50, A: 240}
		textColor = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	case ToastWarning:
		bgColor = color.NRGBA{R: 220, G: 160, B: 40, A: 240}
		textColor = color.NRGBA{R: 40, G: 40, B: 40, A: 255}
	case ToastSuccess:
		bgColor = color.NRGBA{R: 50, G: 160, B: 80, A: 240}
		textColor = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	default: // ToastInfo
		bgColor = color.NRGBA{R: 60, G: 60, B: 60, A: 240}
		textColor = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	}

	// Position toast at bottom center
	return layout.S.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{
			Bottom: unit.Dp(20),
			Left:   unit.Dp(20),
			Right:  unit.Dp(20),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			// Center the toast
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				// Limit max width
				gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(500)))

				padding := unit.Dp(12)
				cornerRadius := unit.Dp(8)

				// Measure text first
				macro := op.Record(gtx.Ops)
				textDims := layout.Inset{
					Top:    padding,
					Bottom: padding,
					Left:   unit.Dp(16),
					Right:  unit.Dp(16),
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					label := material.Body1(th, message)
					label.Color = textColor
					return label.Layout(gtx)
				})
				call := macro.Stop()

				// Draw background
				rr := gtx.Dp(cornerRadius)
				rect := image.Rect(0, 0, textDims.Size.X, textDims.Size.Y)
				paint.FillShape(gtx.Ops, bgColor, clip.RRect{
					Rect: rect,
					NE:   rr, NW: rr, SE: rr, SW: rr,
				}.Op(gtx.Ops))

				// Draw text on top
				call.Add(gtx.Ops)

				return textDims
			})
		})
	})
}
