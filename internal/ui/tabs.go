package ui

import (
	"image"
	"image/color"

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

// layoutManilaStyle renders vertical manila folder tabs on the left side
// Tabs are stacked vertically with no overlap, with rotated text
func (tb *TabBar) layoutManilaStyle(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	tabWidth := gtx.Dp(32)    // Width of the tab (narrow since text is rotated)
	tabHeight := gtx.Dp(90)   // Height of each tab (tall for vertical text)
	gap := gtx.Dp(2)          // Gap between tabs
	cornerRadius := gtx.Dp(6)

	// Manila folder colors
	manilaActive := color.NRGBA{R: 245, G: 222, B: 179, A: 255}   // Warm manila/buff color
	manilaInactive := color.NRGBA{R: 230, G: 215, B: 185, A: 255} // Slightly lighter manila
	borderColor := color.NRGBA{R: 180, G: 160, B: 120, A: 255}    // Brown border

	totalHeight := len(tb.Tabs)*tabHeight + (len(tb.Tabs)-1)*gap

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, tb.manilaTabChildren(gtx, theme, tabWidth, tabHeight, gap, cornerRadius, manilaActive, manilaInactive, borderColor, totalHeight)...)
}

// manilaTabChildren creates flex children for manila tabs
func (tb *TabBar) manilaTabChildren(gtx layout.Context, theme *material.Theme, tabWidth, tabHeight, gap, cornerRadius int, activeColor, inactiveColor, borderColor color.NRGBA, totalHeight int) []layout.FlexChild {
	children := make([]layout.FlexChild, 0, len(tb.Tabs)*2)

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

				// Draw the manila tab shape (rounded on left side only)
				tabRect := image.Rect(0, 0, tabWidth, tabHeight)
				rr := clip.RRect{
					Rect: tabRect,
					NW:   cornerRadius,
					SW:   cornerRadius,
					NE:   0,
					SE:   0,
				}
				paint.FillShape(gtx.Ops, bgColor, rr.Op(gtx.Ops))

				// Draw border on inactive tabs
				if !isSelected {
					// Top border
					topBorder := clip.Rect{Max: image.Pt(tabWidth, 1)}
					paint.FillShape(gtx.Ops, borderColor, topBorder.Op())
					// Left border
					leftBorder := clip.Rect{Max: image.Pt(1, tabHeight)}
					paint.FillShape(gtx.Ops, borderColor, leftBorder.Op())
					// Bottom border
					defer op.Offset(image.Pt(0, tabHeight-1)).Push(gtx.Ops).Pop()
					bottomBorder := clip.Rect{Max: image.Pt(tabWidth, 1)}
					paint.FillShape(gtx.Ops, borderColor, bottomBorder.Op())
				} else {
					// Selected tab gets a highlight on the left edge
					highlightColor := color.NRGBA{R: 139, G: 90, B: 43, A: 255} // Darker brown
					leftHighlight := clip.Rect{Max: image.Pt(3, tabHeight)}
					paint.FillShape(gtx.Ops, highlightColor, leftHighlight.Op())
				}

				// Draw vertical text using rotation
				// Position at center of tab, then rotate
				centerX := tabWidth / 2
				centerY := tabHeight / 2

				// Create rotated text by drawing each character vertically
				labelText := tab.Label
				textColor := color.NRGBA{R: 100, G: 80, B: 50, A: 255} // Medium brown
				if isSelected {
					textColor = color.NRGBA{R: 60, G: 40, B: 20, A: 255} // Dark brown
				}

				// Draw text character by character vertically
				charHeight := gtx.Dp(12)
				startY := centerY - (len(labelText)*charHeight)/2

				for j, ch := range labelText {
					charY := startY + j*charHeight
					func() {
						defer op.Offset(image.Pt(centerX-gtx.Dp(5), charY)).Push(gtx.Ops).Pop()
						lbl := material.Body2(theme, string(ch))
						lbl.Color = textColor
						lbl.TextSize = unit.Sp(11)
						if isSelected {
							lbl.Font.Weight = 600
						}
						lbl.Layout(gtx)
					}()
				}

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
