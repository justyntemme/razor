package ui

import (
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// TabStyle defines the visual style of a tab bar
type TabStyle int

const (
	// TabStyleUnderline shows an underline under the active tab (default)
	TabStyleUnderline TabStyle = iota
	// TabStylePill shows a pill/rounded background on the active tab
	TabStylePill
	// TabStyleSegmented shows segmented control style (for future use)
	TabStyleSegmented
	// TabStyleManila shows vertical manila folder tabs on the left side
	TabStyleManila
)

// Tab represents a single tab in a TabBar
type Tab struct {
	Label string         // Display label
	ID    string         // Unique identifier for the tab
	Icon  *widget.Icon   // Optional icon (nil for text-only)
	btn   widget.Clickable
}

// TabBar is a reusable tab bar component that can be used for:
// - Sidebar section switching (Favorites/Drives)
// - File browser tabs (future)
// - Split pane tabs (future)
type TabBar struct {
	Tabs         []Tab
	Selected     int       // Index of selected tab
	Style        TabStyle  // Visual style

	// Callbacks
	OnTabChanged func(index int, id string) // Called when tab selection changes

	// Appearance customization
	ActiveColor    color.NRGBA // Color for active tab indicator
	InactiveColor  color.NRGBA // Color for inactive tab text
	BackgroundColor color.NRGBA // Background color for the tab bar

	// Layout options
	Distribute bool // If true, tabs are distributed evenly; if false, tabs are sized to content
}

// NewTabBar creates a new TabBar with the given tabs
func NewTabBar(tabs ...Tab) *TabBar {
	return &TabBar{
		Tabs:           tabs,
		Selected:       0,
		Style:          TabStyleUnderline,
		ActiveColor:    color.NRGBA{R: 33, G: 150, B: 243, A: 255},  // Material blue
		InactiveColor:  color.NRGBA{R: 100, G: 100, B: 100, A: 255}, // Gray
		BackgroundColor: color.NRGBA{R: 250, G: 250, B: 250, A: 255}, // Light gray
		Distribute:     true,
	}
}

// AddTab adds a new tab to the tab bar
func (tb *TabBar) AddTab(label, id string) {
	tb.Tabs = append(tb.Tabs, Tab{Label: label, ID: id})
}

// SelectByID selects a tab by its ID
func (tb *TabBar) SelectByID(id string) bool {
	for i, tab := range tb.Tabs {
		if tab.ID == id {
			tb.Selected = i
			return true
		}
	}
	return false
}

// SelectedID returns the ID of the currently selected tab
func (tb *TabBar) SelectedID() string {
	if tb.Selected >= 0 && tb.Selected < len(tb.Tabs) {
		return tb.Tabs[tb.Selected].ID
	}
	return ""
}

// Layout renders the tab bar and returns its dimensions
// Returns true if the selection changed this frame
func (tb *TabBar) Layout(gtx layout.Context, theme *material.Theme) (layout.Dimensions, bool) {
	changed := false

	// Check for tab clicks
	for i := range tb.Tabs {
		if tb.Tabs[i].btn.Clicked(gtx) {
			if tb.Selected != i {
				tb.Selected = i
				changed = true
				if tb.OnTabChanged != nil {
					tb.OnTabChanged(i, tb.Tabs[i].ID)
				}
			}
		}
	}

	// Layout based on style
	var dims layout.Dimensions
	switch tb.Style {
	case TabStylePill:
		dims = tb.layoutPillStyle(gtx, theme)
	case TabStyleSegmented:
		dims = tb.layoutSegmentedStyle(gtx, theme)
	case TabStyleManila:
		dims = tb.layoutManilaStyle(gtx, theme)
	default:
		dims = tb.layoutUnderlineStyle(gtx, theme)
	}

	return dims, changed
}

// layoutUnderlineStyle renders tabs with an underline indicator
func (tb *TabBar) layoutUnderlineStyle(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	tabHeight := gtx.Dp(36)
	indicatorHeight := gtx.Dp(2)

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		tb.tabFlexChildren(gtx, theme, tabHeight, indicatorHeight)...,
	)
}

// layoutPillStyle renders tabs with a pill/rounded background
func (tb *TabBar) layoutPillStyle(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	tabHeight := gtx.Dp(32)

	// Background
	bgRect := clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, tabHeight)}
	paint.FillShape(gtx.Ops, tb.BackgroundColor, bgRect.Op())

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		tb.tabFlexChildrenPill(gtx, theme, tabHeight)...,
	)
}

// layoutSegmentedStyle renders tabs as a segmented control
func (tb *TabBar) layoutSegmentedStyle(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	// For future implementation - similar to iOS segmented control
	return tb.layoutUnderlineStyle(gtx, theme)
}

// tabFlexChildren creates the flex children for underline style tabs
func (tb *TabBar) tabFlexChildren(gtx layout.Context, theme *material.Theme, tabHeight, indicatorHeight int) []layout.FlexChild {
	children := make([]layout.FlexChild, len(tb.Tabs))

	for i := range tb.Tabs {
		idx := i // Capture for closure
		tab := &tb.Tabs[i]
		isSelected := idx == tb.Selected

		child := layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &tab.btn, func(gtx layout.Context) layout.Dimensions {
				return layout.Stack{Alignment: layout.S}.Layout(gtx,
					// Tab content
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = tabHeight
							lbl := material.Body2(theme, tab.Label)
							if isSelected {
								lbl.Color = tb.ActiveColor
								lbl.Font.Weight = 600
							} else {
								lbl.Color = tb.InactiveColor
							}
							return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, lbl.Layout)
						})
					}),
					// Underline indicator (only for selected tab)
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						if !isSelected {
							return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
						}
						rect := clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, indicatorHeight)}
						paint.FillShape(gtx.Ops, tb.ActiveColor, rect.Op())
						return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, indicatorHeight)}
					}),
				)
			})
		})

		if !tb.Distribute {
			child = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return material.Clickable(gtx, &tab.btn, func(gtx layout.Context) layout.Dimensions {
					return layout.Stack{Alignment: layout.S}.Layout(gtx,
						layout.Stacked(func(gtx layout.Context) layout.Dimensions {
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.Y = tabHeight
								lbl := material.Body2(theme, tab.Label)
								if isSelected {
									lbl.Color = tb.ActiveColor
									lbl.Font.Weight = 600
								} else {
									lbl.Color = tb.InactiveColor
								}
								return layout.Inset{
									Top: unit.Dp(8), Bottom: unit.Dp(8),
									Left: unit.Dp(16), Right: unit.Dp(16),
								}.Layout(gtx, lbl.Layout)
							})
						}),
						layout.Stacked(func(gtx layout.Context) layout.Dimensions {
							if !isSelected {
								return layout.Dimensions{}
							}
							rect := clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, indicatorHeight)}
							paint.FillShape(gtx.Ops, tb.ActiveColor, rect.Op())
							return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, indicatorHeight)}
						}),
					)
				})
			})
		}

		children[idx] = child
	}

	return children
}

// tabFlexChildrenPill creates the flex children for pill style tabs
func (tb *TabBar) tabFlexChildrenPill(gtx layout.Context, theme *material.Theme, tabHeight int) []layout.FlexChild {
	children := make([]layout.FlexChild, len(tb.Tabs))
	pillRadius := gtx.Dp(4)

	for i := range tb.Tabs {
		idx := i
		tab := &tb.Tabs[i]
		isSelected := idx == tb.Selected

		child := layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(2), Right: unit.Dp(2), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return material.Clickable(gtx, &tab.btn, func(gtx layout.Context) layout.Dimensions {
						// Draw pill background for selected tab
						if isSelected {
							rr := clip.RRect{
								Rect: image.Rect(0, 0, gtx.Constraints.Max.X, tabHeight-gtx.Dp(8)),
								NE:   pillRadius, NW: pillRadius, SE: pillRadius, SW: pillRadius,
							}
							paint.FillShape(gtx.Ops, colWhite, rr.Op(gtx.Ops))
						}

						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Body2(theme, tab.Label)
							if isSelected {
								lbl.Color = tb.ActiveColor
								lbl.Font.Weight = 600
							} else {
								lbl.Color = tb.InactiveColor
							}
							return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, lbl.Layout)
						})
					})
				})
		})

		children[idx] = child
	}

	return children
}

// layoutManilaStyle renders vertical folder tabs on the left side
// Tabs are stacked vertically with no overlap, with vertical text
func (tb *TabBar) layoutManilaStyle(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	tabWidth := gtx.Dp(32)    // Width of the tab (enough for rotated text height)
	tabHeight := gtx.Dp(100)  // Height of each tab (tall enough for "Favorites" text when rotated)
	gap := gtx.Dp(4)          // Gap between tabs
	cornerRadius := gtx.Dp(4)

	// Colors matching project theme
	activeColor := color.NRGBA{R: 255, G: 255, B: 255, A: 255}    // White for selected (matches content area)
	inactiveColor := color.NRGBA{R: 235, G: 235, B: 235, A: 255}  // Light gray for inactive
	borderColor := color.NRGBA{R: 200, G: 200, B: 200, A: 255}    // Light gray border (colLightGray)

	totalHeight := len(tb.Tabs)*tabHeight + (len(tb.Tabs)-1)*gap

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, tb.manilaTabChildren(gtx, theme, tabWidth, tabHeight, gap, cornerRadius, activeColor, inactiveColor, borderColor, totalHeight)...)
}

// manilaTabChildren creates flex children for folder-style tabs
func (tb *TabBar) manilaTabChildren(gtx layout.Context, theme *material.Theme, tabWidth, tabHeight, gap, cornerRadius int, activeColor, inactiveColor, borderColor color.NRGBA, totalHeight int) []layout.FlexChild {
	children := make([]layout.FlexChild, 0, len(tb.Tabs)*2)

	// Consistent text color for all tabs (gray, matching project theme)
	textColor := color.NRGBA{R: 100, G: 100, B: 100, A: 255} // colGray
	accentColor := color.NRGBA{R: 66, G: 133, B: 244, A: 255} // colAccent (blue)

	for i := range tb.Tabs {
		idx := i
		tab := &tb.Tabs[i]
		isSelected := idx == tb.Selected

		// Add tab
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// Set constraints
			gtx.Constraints.Min = image.Pt(tabWidth, tabHeight)
			gtx.Constraints.Max = image.Pt(tabWidth, tabHeight)

			return material.Clickable(gtx, &tab.btn, func(gtx layout.Context) layout.Dimensions {
				// Tab background color
				bgColor := inactiveColor
				if isSelected {
					bgColor = activeColor
				}

				// Draw the tab shape (rounded on left side only for folder look)
				tabRect := image.Rect(0, 0, tabWidth, tabHeight)
				rr := clip.RRect{
					Rect: tabRect,
					NW:   cornerRadius,
					SW:   cornerRadius,
					NE:   0,
					SE:   0,
				}
				paint.FillShape(gtx.Ops, bgColor, rr.Op(gtx.Ops))

				// Draw border
				if !isSelected {
					// Border on inactive tabs
					// Top border
					topBorder := clip.Rect{Max: image.Pt(tabWidth, 1)}
					paint.FillShape(gtx.Ops, borderColor, topBorder.Op())
					// Left border
					leftBorder := clip.Rect{Max: image.Pt(1, tabHeight)}
					paint.FillShape(gtx.Ops, borderColor, leftBorder.Op())
					// Bottom border (use separate scope to avoid defer issue)
					func() {
						defer op.Offset(image.Pt(0, tabHeight-1)).Push(gtx.Ops).Pop()
						bottomBorder := clip.Rect{Max: image.Pt(tabWidth, 1)}
						paint.FillShape(gtx.Ops, borderColor, bottomBorder.Op())
					}()
				} else {
					// Selected tab gets accent color highlight on left edge
					leftHighlight := clip.Rect{Max: image.Pt(3, tabHeight)}
					paint.FillShape(gtx.Ops, accentColor, leftHighlight.Op())
				}

				// Create the label
				labelText := tab.Label
				lbl := material.Body2(theme, labelText)
				lbl.Color = textColor
				lbl.TextSize = unit.Sp(14)
				if isSelected {
					lbl.Font.Weight = 600
				}

				// Position: move to center of tab, rotate, then offset to center text
				centerX := float32(tabWidth) / 2
				centerY := float32(tabHeight) / 2

				// Apply transformations: translate to center, rotate, translate back
				func() {
					// Move origin to center of tab
					offset1 := op.Offset(image.Pt(int(centerX), int(centerY))).Push(gtx.Ops)

					// Rotate -90 degrees (text will read from bottom to top)
					angle := float32(-math.Pi / 2)
					rotation := op.Affine(f32.Affine2D{}.Rotate(f32.Pt(0, 0), angle)).Push(gtx.Ops)

					// Offset to center the text (text will be horizontal after rotation)
					// After -90° rotation: +X moves down (toward bottom of tab), +Y moves right (toward right edge)
					// Center text horizontally (in rotated space) and vertically within tab width
					textOffset := op.Offset(image.Pt(-int(centerY)+gtx.Dp(10), -gtx.Dp(6))).Push(gtx.Ops)

					// Give the label unconstrained space so text isn't clipped
					unconstrainedGtx := gtx
					unconstrainedGtx.Constraints.Min = image.Point{}
					unconstrainedGtx.Constraints.Max = image.Pt(tabHeight*2, tabWidth*2)
					lbl.Layout(unconstrainedGtx)

					textOffset.Pop()
					rotation.Pop()
					offset1.Pop()
				}()

				return layout.Dimensions{Size: image.Pt(tabWidth, tabHeight)}
			})
		}))

		// Add gap between tabs (except after last)
		if i < len(tb.Tabs)-1 {
			children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(float32(gap) / float32(gtx.Metric.PxPerDp))}.Layout))
		}
	}

	return children
}

// LayoutVertical renders the tab bar vertically (for use alongside content)
// This is useful for manila-style tabs where tabs are on the side
func (tb *TabBar) LayoutVertical(gtx layout.Context, theme *material.Theme) (layout.Dimensions, bool) {
	changed := false

	// Check for tab clicks
	for i := range tb.Tabs {
		if tb.Tabs[i].btn.Clicked(gtx) {
			if tb.Selected != i {
				tb.Selected = i
				changed = true
				if tb.OnTabChanged != nil {
					tb.OnTabChanged(i, tb.Tabs[i].ID)
				}
			}
		}
	}

	dims := tb.layoutManilaStyle(gtx, theme)
	return dims, changed
}
