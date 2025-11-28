package ui

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/widget/material"
)

// Renderer holds the visual state and theme.
type Renderer struct {
	Theme *material.Theme
	// Add reusable widgets here to avoid allocation in the hot path
}

// NewRenderer creates the theme.
func NewRenderer() *Renderer {
	return &Renderer{
		Theme: material.NewTheme(),
	}
}

// State represents the data the UI needs to render.
type State struct {
	CurrentPath string
	Files       []string
}

// Layout draws the interface.
// It follows the DRY principle and avoids complex logic.
func (r *Renderer) Layout(gtx layout.Context, state State) layout.Dimensions {
	// Define the text content based on state
	txt := "Hello, Gio"
	if state.CurrentPath != "" {
		txt = "Browsing: " + state.CurrentPath
	}

	title := material.H1(r.Theme, txt)
	
	// Maroon color as requested in original spec
	title.Color = color.NRGBA{R: 127, G: 0, B: 0, A: 255}
	title.Alignment = text.Middle

	return layout.Flex{
		Axis:    layout.Vertical,
		Spacing: layout.SpaceAround,
	}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return title.Layout(gtx)
		}),
	)
}