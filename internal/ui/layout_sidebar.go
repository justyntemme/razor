package ui

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// Sidebar layout - favorites, drives, recent files

func (r *Renderer) layoutSidebar(gtx layout.Context, state *State, eventOut *UIEvent) layout.Dimensions {
	// Track sidebar vertical offset for context menus
	sidebarYOffset := gtx.Dp(92)

	// Check for clicks on sidebar area to dismiss menus (for non-tabbed layouts)
	if r.sidebarClick.Clicked(gtx) {
		r.onLeftClick()
	}

	// Handle different sidebar layouts
	switch r.sidebarLayout {
	case "stacked":
		return r.layoutSidebarStacked(gtx, state, eventOut, sidebarYOffset)
	case "favorites_only":
		return invisibleClickable(gtx, &r.sidebarClick, func(gtx layout.Context) layout.Dimensions {
			return r.sidebarScroll.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
				return r.layoutFavoritesList(gtx, state, eventOut, sidebarYOffset)
			})
		})
	case "drives_only":
		return invisibleClickable(gtx, &r.sidebarClick, func(gtx layout.Context) layout.Dimensions {
			return r.sidebarScroll.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
				return r.layoutDrivesList(gtx, state, eventOut)
			})
		})
	default: // "tabbed"
		return r.layoutSidebarTabbed(gtx, state, eventOut, sidebarYOffset)
	}
}

// layoutSidebarTabbed renders the sidebar with tabs (manila or other styles)
func (r *Renderer) layoutSidebarTabbed(gtx layout.Context, state *State, eventOut *UIEvent, sidebarYOffset int) layout.Dimensions {
	// Check if using vertical tabs (manila) or horizontal tabs
	if r.sidebarTabs.Style == TabStyleManila {
		// Horizontal layout: manila tabs on left, content on right
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			// Manila folder tabs on the left side
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				dims, changed := r.sidebarTabs.LayoutVertical(gtx, r.Theme)
				if changed {
					r.onLeftClick()
				}
				return dims
			}),

			// Vertical separator
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				width := gtx.Dp(unit.Dp(1))
				paint.FillShape(gtx.Ops, colLightGray, clip.Rect{Max: image.Pt(width, gtx.Constraints.Max.Y)}.Op())
				return layout.Dimensions{Size: image.Pt(width, gtx.Constraints.Max.Y)}
			}),

			// Content area - scrollable list based on selected tab
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return r.layoutSidebarTabbedContent(gtx, state, eventOut, sidebarYOffset)
			}),
		)
	}

	// Horizontal tabs at top (underline or pill style)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Tabs at top
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims, changed := r.sidebarTabs.Layout(gtx, r.Theme)
			if changed {
				r.onLeftClick()
			}
			return dims
		}),

		// Horizontal separator
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.layoutHorizontalSeparator(gtx, colLightGray)
		}),

		// Content area
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return r.layoutSidebarTabbedContent(gtx, state, eventOut, sidebarYOffset)
		}),
	)
}

// layoutSidebarTabbedContent renders the scrollable content area for tabbed sidebar
func (r *Renderer) layoutSidebarTabbedContent(gtx layout.Context, state *State, eventOut *UIEvent, sidebarYOffset int) layout.Dimensions {
	// Check for clicks on sidebar area to dismiss menus
	if r.sidebarClick.Clicked(gtx) {
		r.onLeftClick()
	}
	return invisibleClickable(gtx, &r.sidebarClick, func(gtx layout.Context) layout.Dimensions {
		return r.sidebarScroll.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			switch r.sidebarTabs.SelectedID() {
			case "favorites":
				return r.layoutFavoritesList(gtx, state, eventOut, sidebarYOffset)
			case "drives":
				return r.layoutDrivesList(gtx, state, eventOut)
			default:
				return r.layoutFavoritesList(gtx, state, eventOut, sidebarYOffset)
			}
		})
	})
}

// layoutSidebarStacked renders both Favorites and Drives stacked vertically
func (r *Renderer) layoutSidebarStacked(gtx layout.Context, state *State, eventOut *UIEvent, sidebarYOffset int) layout.Dimensions {
	return invisibleClickable(gtx, &r.sidebarClick, func(gtx layout.Context) layout.Dimensions {
		return r.sidebarScroll.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				// Recent Files entry (above Favorites)
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.layoutRecentFilesEntry(gtx, eventOut)
				}),
				// Separator after Recent Files
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.layoutInsetSeparator(gtx, colLightGray, unit.Dp(4), unit.Dp(4))
				}),
				// Favorites section header
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8)}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							lbl := material.Body2(r.Theme, "Favorites")
							lbl.Font.Weight = 600
							lbl.Color = colGray
							return lbl.Layout(gtx)
						})
				}),
				// Favorites list
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.layoutFavoritesList(gtx, state, eventOut, sidebarYOffset)
				}),
				// Separator
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.layoutInsetSeparator(gtx, colLightGray, unit.Dp(8), unit.Dp(8))
				}),
				// Drives section header
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Bottom: unit.Dp(4), Left: unit.Dp(8)}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							lbl := material.Body2(r.Theme, "Drives")
							lbl.Font.Weight = 600
							lbl.Color = colGray
							return lbl.Layout(gtx)
						})
				}),
				// Drives list
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.layoutDrivesList(gtx, state, eventOut)
				}),
			)
		})
	})
}

// layoutFavoritesList renders the favorites list content (with Recent Files entry in tabbed mode)
func (r *Renderer) layoutFavoritesList(gtx layout.Context, state *State, eventOut *UIEvent, yOffset int) layout.Dimensions {
	// In tabbed mode, add Recent Files entry at top of favorites list
	if r.sidebarLayout == "tabbed" || r.sidebarLayout == "favorites_only" {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			// Recent Files entry
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.layoutRecentFilesEntry(gtx, eventOut)
			}),
			// Separator
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.layoutInsetSeparator(gtx, colLightGray, unit.Dp(4), unit.Dp(4))
			}),
			// Favorites list
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return r.layoutFavoritesListContent(gtx, state, eventOut, yOffset)
			}),
		)
	}

	return r.layoutFavoritesListContent(gtx, state, eventOut, yOffset)
}

// layoutFavoritesListContent renders the actual favorites list content
func (r *Renderer) layoutFavoritesListContent(gtx layout.Context, state *State, eventOut *UIEvent, yOffset int) layout.Dimensions {
	if len(state.FavList) == 0 {
		// Show empty state message
		return layout.Inset{Top: unit.Dp(16), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(r.Theme, "No favorites yet.\nRight-click a folder to add it.")
				lbl.Color = colGray
				return lbl.Layout(gtx)
			})
	}

	return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return r.favState.Layout(gtx, len(state.FavList), func(gtx layout.Context, i int) layout.Dimensions {
			fav := &state.FavList[i]

			// Render row and capture right-click event
			rowDims, leftClicked, rightClicked, _ := r.renderFavoriteRow(gtx, fav)

			// Handle right-click on favorite
			if rightClicked {
				r.menuVisible = true
				r.menuPos = r.mousePos
				r.menuPath = fav.Path
				r.menuIsDir = true
				r.menuIsFav = true
				r.menuIsBackground = false
			}

			// Handle left-click
			if leftClicked {
				r.onLeftClick()
				*eventOut = UIEvent{Action: ActionNavigate, Path: fav.Path}
			}

			return rowDims
		})
	})
}

// layoutDrivesList renders the drives list content
func (r *Renderer) layoutDrivesList(gtx layout.Context, state *State, eventOut *UIEvent) layout.Dimensions {
	if len(state.Drives) == 0 {
		// Show empty state message
		return layout.Inset{Top: unit.Dp(16), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(r.Theme, "No drives found.")
				lbl.Color = colGray
				return lbl.Layout(gtx)
			})
	}

	return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return r.driveState.Layout(gtx, len(state.Drives), func(gtx layout.Context, i int) layout.Dimensions {
			drive := &state.Drives[i]

			// Render and get click state
			rowDims, clicked := r.renderDriveRow(gtx, drive)

			// Handle click
			if clicked {
				r.onLeftClick()
				*eventOut = UIEvent{Action: ActionNavigate, Path: drive.Path}
			}

			return rowDims
		})
	})
}

// layoutRecentFilesEntry renders the "Recent Files" entry matching favorites style
func (r *Renderer) layoutRecentFilesEntry(gtx layout.Context, eventOut *UIEvent) layout.Dimensions {
	// Check for click BEFORE layout
	if r.recentFilesBtn.Clicked(gtx) {
		r.onLeftClick()
		*eventOut = UIEvent{Action: ActionShowRecentFiles}
	}

	// Match favorites styling - use material.Clickable for hover effect
	return material.Clickable(gtx, &r.recentFilesBtn, func(gtx layout.Context) layout.Dimensions {
		// Draw selection background if viewing recent (with rounded corners)
		if r.isRecentView {
			cornerRadius := gtx.Dp(4)
			rr := clip.RRect{
				Rect: image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Constraints.Max.Y),
				NE:   cornerRadius, NW: cornerRadius, SE: cornerRadius, SW: cornerRadius,
			}
			paint.FillShape(gtx.Ops, colSelected, rr.Op(gtx.Ops))
		}

		return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					// Clock icon
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						size := gtx.Dp(14)
						r.drawClockIcon(gtx.Ops, size, colAccent)
						return layout.Dimensions{Size: image.Pt(size, size)}
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					// Label
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body1(r.Theme, "Recent Files")
						lbl.Color = colDirBlue
						lbl.MaxLines = 1
						return lbl.Layout(gtx)
					}),
				)
			})
	})
}

// drawClockIcon draws a simple clock icon
func (r *Renderer) drawClockIcon(ops *op.Ops, size int, iconColor color.NRGBA) {
	s := float32(size)
	center := s / 2

	// Clock circle (outline)
	circle := clip.Ellipse{
		Min: image.Pt(1, 1),
		Max: image.Pt(size-1, size-1),
	}
	paint.FillShape(ops, iconColor, clip.Stroke{
		Path:  circle.Path(ops),
		Width: float32(size) * 0.12,
	}.Op())

	// Clock hands
	var path clip.Path
	path.Begin(ops)
	// Hour hand (short, pointing to ~10 o'clock)
	path.MoveTo(f32.Pt(center, center))
	path.LineTo(f32.Pt(center-s*0.15, center-s*0.2))
	// Minute hand (long, pointing to 12)
	path.MoveTo(f32.Pt(center, center))
	path.LineTo(f32.Pt(center, center-s*0.3))

	paint.FillShape(ops, iconColor, clip.Stroke{
		Path:  path.End(),
		Width: float32(size) * 0.1,
	}.Op())
}
