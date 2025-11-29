package ui

import (
	"fmt"
	"image/color"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// renderMenuItem creates a standard clickable menu row (used in Context Menu, File Menu)
func (r *Renderer) renderMenuItem(gtx layout.Context, clk *widget.Clickable, text string) layout.Dimensions {
	return material.Clickable(gtx, clk, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			txt := material.Body2(r.Theme, text)
			txt.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
			return txt.Layout(gtx)
		})
	})
}

// renderFavoriteRow creates a favorite item row with proper left/right click detection.
// This is structured EXACTLY like renderRow in renderer.go:
//   - layout.Stack with two children
//   - layout.Stacked: material.Clickable wrapping the visual content
//   - layout.Expanded: right-click detection area with clip.Rect, event.Op, pointer.PassOp
func (r *Renderer) renderFavoriteRow(gtx layout.Context, fav *FavoriteItem, selected bool) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		// First child: Stacked - clickable content (exactly like renderRow)
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &fav.Clickable, func(gtx layout.Context) layout.Dimensions {
				return layout.Stack{}.Layout(gtx,
					// Selection highlight background
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						if selected {
							paint.FillShape(gtx.Ops, color.NRGBA{R: 200, G: 220, B: 255, A: 255}, clip.Rect{Max: gtx.Constraints.Min}.Op())
						}
						return layout.Dimensions{}
					}),
					// Text content
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{
							Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12),
						}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							txt := material.Body1(r.Theme, fav.Name)
							txt.Color = color.NRGBA{R: 0, G: 0, B: 128, A: 255}
							txt.MaxLines = 1
							return txt.Layout(gtx)
						})
					}),
				)
			})
		}),

		// Second child: Expanded - right-click detection area (exactly like renderRow)
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
			event.Op(gtx.Ops, &fav.RightClickTag)
			defer pointer.PassOp{}.Push(gtx.Ops).Pop()
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
	)
}

// renderMenuShell creates a white box with a grey border (used for Popups)
func (r *Renderer) renderMenuShell(gtx layout.Context, content layout.Widget) layout.Dimensions {
	return widget.Border{
		Color:        color.NRGBA{R: 200, G: 200, B: 200, A: 255},
		Width:        unit.Dp(1),
		CornerRadius: unit.Dp(4),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: gtx.Constraints.Min}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}),
			layout.Stacked(content),
		)
	})
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}