package ui

import (
	"image"
	"image/color"

	"gioui.org/layout"
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
