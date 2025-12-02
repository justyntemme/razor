package ui

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// Search bar layout - search input, history dropdown, directive pills

// layoutSearchWithHistory renders the search box (dropdown is rendered as overlay in main Layout)
func (r *Renderer) layoutSearchWithHistory(gtx layout.Context, hasDirectivePrefix, showClearBtn bool) layout.Dimensions {
	// Check for clicks on search box area to dismiss menus
	if r.searchBoxClick.Clicked(gtx) {
		r.onLeftClick()
	}

	return invisibleClickable(gtx, &r.searchBoxClick, func(gtx layout.Context) layout.Dimensions {
		return widget.Border{Color: colLightGray, Width: unit.Dp(1), CornerRadius: unit.Dp(4)}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(6), Right: unit.Dp(4)}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							// Directive pills
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if len(r.detectedDirectives) == 0 {
									return layout.Dimensions{}
								}
								return r.layoutDirectivePills(gtx)
							}),
							// Text editor
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								placeholder := "Search..."
								if hasDirectivePrefix {
									placeholder = "press Enter to search"
								}
								ed := material.Editor(r.Theme, &r.searchEditor, placeholder)
								ed.TextSize = unit.Sp(13)
								return ed.Layout(gtx)
							}),
							// Clear button
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if !showClearBtn {
									return layout.Dimensions{}
								}
								return material.Clickable(gtx, &r.searchClearBtn, func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx,
										func(gtx layout.Context) layout.Dimensions {
											lbl := material.Body2(r.Theme, "✕")
											lbl.Color = colGray
											return lbl.Layout(gtx)
										})
								})
							}),
						)
					})
			})
	})
}

// layoutSearchHistoryOverlay renders the search history dropdown as a root-level overlay
// This ensures proper z-index ordering so clicks work correctly
func (r *Renderer) layoutSearchHistoryOverlay(gtx layout.Context) layout.Dimensions {
	if !r.searchHistoryVisible || len(r.searchHistoryItems) == 0 {
		return layout.Dimensions{}
	}

	// Calculate position: search box is 280dp wide at right side of navbar
	// Navbar is inset 8dp on each side, search box Y is ~46dp from top
	searchBoxWidth := gtx.Dp(280)
	rightMargin := gtx.Dp(8)
	topOffset := gtx.Dp(46)    // File button + navbar + spacing
	dropdownOffset := gtx.Dp(36) // Height of search box

	// Position dropdown below search box, aligned with its left edge
	xPos := gtx.Constraints.Max.X - searchBoxWidth - rightMargin
	yPos := topOffset + dropdownOffset

	defer op.Offset(image.Pt(xPos, yPos)).Push(gtx.Ops).Pop()

	return r.layoutSearchHistoryDropdown(gtx)
}

// layoutSearchHistoryDropdown renders the search history dropdown content
func (r *Renderer) layoutSearchHistoryDropdown(gtx layout.Context) layout.Dimensions {
	// Semi-transparent background color
	dropdownBg := color.NRGBA{R: 255, G: 255, B: 255, A: 240} // White with 94% opacity

	return widget.Border{Color: colLightGray, Width: unit.Dp(1), CornerRadius: unit.Dp(4)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					// Use rounded rect for the background
					rr := clip.RRect{
						Rect: image.Rect(0, 0, gtx.Constraints.Min.X, gtx.Constraints.Min.Y),
						NE:   gtx.Dp(4), NW: gtx.Dp(4), SE: gtx.Dp(4), SW: gtx.Dp(4),
					}
					paint.FillShape(gtx.Ops, dropdownBg, rr.Op(gtx.Ops))
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(280)
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx, r.searchHistoryFlexChildren()...)
				}),
			)
		})
}

// searchHistoryFlexChildren creates flex children for each history item
func (r *Renderer) searchHistoryFlexChildren() []layout.FlexChild {
	children := make([]layout.FlexChild, 0, len(r.searchHistoryItems))

	for i := range r.searchHistoryItems {
		if i >= len(r.searchHistoryBtns) {
			break
		}
		idx := i
		item := r.searchHistoryItems[i]

		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &r.searchHistoryBtns[idx], func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								// History icon
								lbl := material.Body2(r.Theme, "⏱")
								lbl.Color = colGray
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								lbl := material.Body2(r.Theme, item.Query)
								lbl.Color = colBlack
								lbl.MaxLines = 1
								return lbl.Layout(gtx)
							}),
						)
					})
			})
		}))
	}

	return children
}

// layoutDirectivePills renders detected directives as colored pill badges
func (r *Renderer) layoutDirectivePills(gtx layout.Context) layout.Dimensions {
	if len(r.detectedDirectives) == 0 {
		return layout.Dimensions{}
	}

	var children []layout.FlexChild
	for _, d := range r.detectedDirectives {
		d := d // capture for closure
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return r.renderDirectivePill(gtx, d)
			})
		}))
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

// renderDirectivePill renders a single directive as a styled pill
func (r *Renderer) renderDirectivePill(gtx layout.Context, d DetectedDirective) layout.Dimensions {
	// Choose color based on directive type
	bgColor := colDirectiveBg
	textColor := colDirective

	// Different colors for different directive types
	switch d.Type {
	case "contents":
		bgColor = color.NRGBA{R: 255, G: 243, B: 224, A: 255} // Light orange
		textColor = color.NRGBA{R: 230, G: 81, B: 0, A: 255}  // Dark orange
	case "ext":
		bgColor = color.NRGBA{R: 232, G: 245, B: 233, A: 255} // Light green
		textColor = color.NRGBA{R: 46, G: 125, B: 50, A: 255} // Dark green
	case "size":
		bgColor = color.NRGBA{R: 227, G: 242, B: 253, A: 255} // Light blue
		textColor = color.NRGBA{R: 21, G: 101, B: 192, A: 255} // Dark blue
	case "modified":
		bgColor = color.NRGBA{R: 252, G: 228, B: 236, A: 255} // Light pink
		textColor = color.NRGBA{R: 173, G: 20, B: 87, A: 255} // Dark pink
	case "recursive", "depth":
		bgColor = color.NRGBA{R: 255, G: 249, B: 196, A: 255} // Light yellow
		textColor = color.NRGBA{R: 158, G: 118, B: 0, A: 255} // Dark yellow/gold
	}

	// Short label for the directive type
	typeLabel := d.Type
	if len(typeLabel) > 4 {
		// Abbreviate long types
		switch d.Type {
		case "contents":
			typeLabel = "txt"
		case "modified":
			typeLabel = "mod"
		case "filename":
			typeLabel = "name"
		case "recursive":
			typeLabel = "rec"
		}
	}

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			rr := gtx.Dp(unit.Dp(10))
			bounds := image.Rectangle{Max: gtx.Constraints.Min}
			paint.FillShape(gtx.Ops, bgColor, clip.RRect{Rect: bounds, NE: rr, NW: rr, SE: rr, SW: rr}.Op(gtx.Ops))
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Caption(r.Theme, typeLabel+":")
							lbl.Color = textColor
							lbl.Font.Weight = font.Bold
							return lbl.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							// Truncate long values
							val := d.Value
							if len(val) > 12 {
								val = val[:10] + "…"
							}
							lbl := material.Caption(r.Theme, val)
							lbl.Color = textColor
							return lbl.Layout(gtx)
						}),
					)
				})
		}),
	)
}
