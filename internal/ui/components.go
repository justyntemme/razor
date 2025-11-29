package ui

import (
	"fmt"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// renderMenuItem creates a standard clickable menu row (used in Context Menu, File Menu, Favs)
func (r *Renderer) renderMenuItem(gtx layout.Context, clk *widget.Clickable, text string) layout.Dimensions {
	return material.Clickable(gtx, clk, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			txt := material.Body2(r.Theme, text)
			txt.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
			return txt.Layout(gtx)
		})
	})
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
