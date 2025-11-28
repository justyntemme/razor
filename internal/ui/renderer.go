package ui

import (
	"image/color"

	"gioui.org/font" // Added this import
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// UIEntry duplicates minimal data needed for rendering to decouple from FS.
type UIEntry struct {
	Name  string
	IsDir bool
}

type State struct {
	CurrentPath string
	Entries     []UIEntry
}

type Renderer struct {
	Theme *material.Theme
	// listState persists scroll position across frames
	listState layout.List
}

func NewRenderer() *Renderer {
	r := &Renderer{
		Theme: material.NewTheme(),
	}
	// Vertical list setup
	r.listState.Axis = layout.Vertical
	return r
}

func (r *Renderer) Layout(gtx layout.Context, state State) layout.Dimensions {
	// 1. Header Layout
	header := func(gtx layout.Context) layout.Dimensions {
		txt := "Loading..."
		if state.CurrentPath != "" {
			txt = state.CurrentPath
		}
		h2 := material.H6(r.Theme, txt)
		h2.Color = color.NRGBA{R: 50, G: 50, B: 50, A: 255}
		return layout.Inset{
			Top: unit.Dp(16), Bottom: unit.Dp(16), Left: unit.Dp(8),
		}.Layout(gtx, h2.Layout)
	}

	// 2. List Layout (Virtualization happens here)
	list := func(gtx layout.Context) layout.Dimensions {
		return r.listState.Layout(gtx, len(state.Entries), func(gtx layout.Context, index int) layout.Dimensions {
			item := state.Entries[index]
			return r.renderRow(gtx, item)
		})
	}

	// Combine vertical flex
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(header),
		layout.Flexed(1, list),
	)
}

// renderRow draws a single file item. 
// This is a "hot path" function; keep it allocation-light.
func (r *Renderer) renderRow(gtx layout.Context, item UIEntry) layout.Dimensions {
	// Determine styling based on type
	txt := item.Name
	col := color.NRGBA{R: 0, G: 0, B: 0, A: 255} // Black text
	weight := font.Normal                          // Corrected: using font package

	if item.IsDir {
		txt = "[DIR] " + item.Name
		col = color.NRGBA{R: 0, G: 0, B: 128, A: 255} // Navy for folders
		weight = font.Bold                            // Corrected: using font package
	}

	lbl := material.Body1(r.Theme, txt)
	lbl.Color = col
	lbl.Font.Weight = weight

	return layout.Inset{
		Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8),
	}.Layout(gtx, lbl.Layout)
}