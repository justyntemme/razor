package ui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// layoutFileMenu renders the dropdown under the "File" button
func (r *Renderer) layoutFileMenu(gtx layout.Context) (layout.Dimensions, UIEvent) {
	if !r.fileMenuOpen {
		return layout.Dimensions{}, UIEvent{}
	}

	// Positioning offset
	offset := op.Offset(image.Point{X: 8, Y: 30}).Push(gtx.Ops)
	defer offset.Pop()

	return r.renderMenuShell(gtx, func(gtx layout.Context) layout.Dimensions {
		if r.settingsBtn.Clicked(gtx) {
			r.fileMenuOpen = false
			r.settingsOpen = true
		}
		gtx.Constraints.Min.X = gtx.Dp(120)
		return r.renderMenuItem(gtx, &r.settingsBtn, "Settings")
	}), UIEvent{}
}

// layoutContextMenu renders the right-click popup
func (r *Renderer) layoutContextMenu(gtx layout.Context) (layout.Dimensions, UIEvent) {
	if !r.menuVisible {
		return layout.Dimensions{}, UIEvent{}
	}
	var eventOut UIEvent

	offset := op.Offset(r.menuPos).Push(gtx.Ops)
	defer offset.Pop()

	// Handle Button Clicks
	if r.openBtn.Clicked(gtx) {
		r.menuVisible = false
		eventOut = UIEvent{Action: ActionOpen, Path: r.menuPath}
	}
	if r.copyBtn.Clicked(gtx) {
		r.menuVisible = false
	}
	if r.favBtn.Clicked(gtx) {
		r.menuVisible = false
		if r.menuIsFav {
			eventOut = UIEvent{Action: ActionRemoveFavorite, Path: r.menuPath}
		} else {
			eventOut = UIEvent{Action: ActionAddFavorite, Path: r.menuPath}
		}
	}

	dims := r.renderMenuShell(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Dp(180)
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.renderMenuItem(gtx, &r.openBtn, "Open")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.renderMenuItem(gtx, &r.copyBtn, "Copy (noop)")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !r.menuIsDir {
					return layout.Dimensions{}
				}
				text := "Add to Favorites"
				if r.menuIsFav {
					text = "Remove Favorite"
				}
				return r.renderMenuItem(gtx, &r.favBtn, text)
			}),
		)
	})

	return dims, eventOut
}

// layoutSettingsModal renders the centered settings dialog
func (r *Renderer) layoutSettingsModal(gtx layout.Context) layout.Dimensions {
	if !r.settingsOpen {
		return layout.Dimensions{}
	}
	if r.settingsCloseBtn.Clicked(gtx) {
		r.settingsOpen = false
	}

	return layout.Stack{}.Layout(gtx,
		// Semi-transparent backdrop
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, color.NRGBA{A: 150}, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return material.Clickable(gtx, &r.settingsCloseBtn, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		// The Dialog
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return r.renderMenuShell(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(300)
					gtx.Constraints.Min.Y = gtx.Dp(200)
					return layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								h6 := material.H6(r.Theme, "Settings")
								h6.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
								return h6.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := material.Body1(r.Theme, "Search Engine:")
								lbl.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
								return lbl.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.RadioButton(r.Theme, &r.searchEngine, "default", "Default").Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.RadioButton(r.Theme, &r.searchEngine, "ripgrep", "ripgrep").Layout(gtx)
							}),
						)
					})
				})
			})
		}),
	)
}
