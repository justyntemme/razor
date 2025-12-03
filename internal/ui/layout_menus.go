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

	"github.com/justyntemme/razor/internal/debug"
	"github.com/justyntemme/razor/internal/trash"
)

// Context menus and file menus

func (r *Renderer) menuShell(gtx layout.Context, width unit.Dp, content layout.Widget) layout.Dimensions {
	// Draw shadow layers for depth effect - multi-layer for realistic shadow
	cornerRadius := gtx.Dp(8)
	widthPx := gtx.Dp(width)

	// We need to measure content first to know the size for shadows
	// Use a macro to record and replay
	macro := op.Record(gtx.Ops)
	// Constrain both min and max width to create a fixed-width modal
	gtx.Constraints.Min.X = widthPx
	gtx.Constraints.Max.X = widthPx
	contentDims := widget.Border{Color: colLightGray, Width: unit.Dp(1), CornerRadius: unit.Dp(8)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					// Rounded background
					rr := clip.RRect{
						Rect: image.Rect(0, 0, gtx.Constraints.Min.X, gtx.Constraints.Min.Y),
						NE:   cornerRadius, NW: cornerRadius, SE: cornerRadius, SW: cornerRadius,
					}
					paint.FillShape(gtx.Ops, colWhite, rr.Op(gtx.Ops))
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = widthPx
					gtx.Constraints.Max.X = widthPx
					return content(gtx)
				}),
			)
		})
	contentCall := macro.Stop()

	// Draw multiple shadow layers for depth (outer to inner for proper compositing)
	// Outer shadow - large, soft
	outerOffset := gtx.Dp(6)
	shadowOuter := clip.RRect{
		Rect: image.Rect(outerOffset, outerOffset, contentDims.Size.X+outerOffset, contentDims.Size.Y+outerOffset),
		NE:   cornerRadius + 2, NW: cornerRadius + 2, SE: cornerRadius + 2, SW: cornerRadius + 2,
	}
	paint.FillShape(gtx.Ops, colShadowOuter, shadowOuter.Op(gtx.Ops))

	// Middle shadow - medium spread
	midOffset := gtx.Dp(4)
	shadowMid := clip.RRect{
		Rect: image.Rect(midOffset, midOffset, contentDims.Size.X+midOffset, contentDims.Size.Y+midOffset),
		NE:   cornerRadius + 1, NW: cornerRadius + 1, SE: cornerRadius + 1, SW: cornerRadius + 1,
	}
	paint.FillShape(gtx.Ops, color.NRGBA{A: 35}, shadowMid.Op(gtx.Ops))

	// Inner shadow - tight, darker
	innerOffset := gtx.Dp(2)
	shadowInner := clip.RRect{
		Rect: image.Rect(innerOffset, innerOffset, contentDims.Size.X+innerOffset, contentDims.Size.Y+innerOffset),
		NE:   cornerRadius, NW: cornerRadius, SE: cornerRadius, SW: cornerRadius,
	}
	paint.FillShape(gtx.Ops, colShadow, shadowInner.Op(gtx.Ops))

	// Replay content on top
	contentCall.Add(gtx.Ops)

	return contentDims
}

// menuItemWithColor renders a clickable menu item with the specified text color
func (r *Renderer) menuItemWithColor(gtx layout.Context, btn *widget.Clickable, label string, textColor color.NRGBA) layout.Dimensions {
	return material.Clickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body2(r.Theme, label)
			lbl.Color = textColor
			return lbl.Layout(gtx)
		})
	})
}

// menuItem renders a clickable menu item with default (black) text color
func (r *Renderer) menuItem(gtx layout.Context, btn *widget.Clickable, label string) layout.Dimensions {
	return r.menuItemWithColor(gtx, btn, label, colBlack)
}

// menuItemDanger renders a clickable menu item with danger (red) text color
func (r *Renderer) menuItemDanger(gtx layout.Context, btn *widget.Clickable, label string) layout.Dimensions {
	return r.menuItemWithColor(gtx, btn, label, colDanger)
}

func (r *Renderer) layoutFileMenu(gtx layout.Context, eventOut *UIEvent) layout.Dimensions {
	if !r.fileMenuOpen {
		return layout.Dimensions{}
	}
	defer op.Offset(image.Pt(8, 30)).Push(gtx.Ops).Pop()
	return r.menuShell(gtx, 160, func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if r.newWindowBtn.Clicked(gtx) {
					r.onLeftClick()
					*eventOut = UIEvent{Action: ActionNewWindow}
				}
				return r.menuItem(gtx, &r.newWindowBtn, "New Window")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if r.newTabBtn.Clicked(gtx) {
					r.onLeftClick()
					*eventOut = UIEvent{Action: ActionNewTab}
				}
				return r.menuItem(gtx, &r.newTabBtn, "New Tab")
			}),
		}

		// Separator and Settings
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.layoutMenuSeparator(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if r.hotkeysBtn.Clicked(gtx) {
					r.onLeftClick()
					r.hotkeysOpen = true
				gtx.Execute(op.InvalidateCmd{})
				}
				return r.menuItem(gtx, &r.hotkeysBtn, "Keyboard Shortcuts")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if r.settingsBtn.Clicked(gtx) {
					debug.Log(debug.UI, "Settings button clicked, opening settings")
					r.onLeftClick()
					r.settingsOpen = true
				gtx.Execute(op.InvalidateCmd{})
				}
				return r.menuItem(gtx, &r.settingsBtn, "Settings")
			}),
		)

		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (r *Renderer) layoutContextMenu(gtx layout.Context, state *State, eventOut *UIEvent) layout.Dimensions {
	if !r.menuVisible {
		return layout.Dimensions{}
	}

	// Calculate menu dimensions to determine flip direction
	// Menu width is 180dp, estimate height based on menu type
	menuWidth := gtx.Dp(180)
	menuHeight := gtx.Dp(280) // Full menu approximate height
	if r.menuIsBackground {
		menuHeight = gtx.Dp(100) // Background menu is shorter
	}

	// Determine final position with flip logic
	posX := r.menuPos.X
	posY := r.menuPos.Y

	// Flip horizontally if menu would go off right edge
	if posX+menuWidth > gtx.Constraints.Max.X {
		posX = r.menuPos.X - menuWidth
	}

	// Flip vertically if menu would go off bottom edge
	if posY+menuHeight > gtx.Constraints.Max.Y {
		posY = r.menuPos.Y - menuHeight
	}

	// Ensure menu stays within bounds (clamp to edges as fallback)
	if posX < 0 {
		posX = 0
	}
	if posY < 0 {
		posY = 0
	}

	defer op.Offset(image.Pt(posX, posY)).Push(gtx.Ops).Pop()

	// Helper to close menu and trigger redraw
	closeMenu := func() {
		r.menuVisible = false
		gtx.Execute(op.InvalidateCmd{})
	}

	if r.openBtn.Clicked(gtx) {
		closeMenu()
		*eventOut = UIEvent{Action: ActionOpen, Path: r.menuPath}
	}
	if r.openWithBtn.Clicked(gtx) {
		closeMenu()
		*eventOut = UIEvent{Action: ActionOpenWith, Path: r.menuPath}
	}
	if r.openLocationBtn.Clicked(gtx) {
		closeMenu()
		*eventOut = UIEvent{Action: ActionOpenFileLocation, Path: r.menuPath}
	}
	if r.copyBtn.Clicked(gtx) {
		closeMenu()
		paths := r.collectSelectedPaths(state)
		*eventOut = UIEvent{Action: ActionCopy, Paths: paths}
	}
	if r.cutBtn.Clicked(gtx) {
		closeMenu()
		paths := r.collectSelectedPaths(state)
		*eventOut = UIEvent{Action: ActionCut, Paths: paths}
	}
	if r.pasteBtn.Clicked(gtx) {
		closeMenu()
		*eventOut = UIEvent{Action: ActionPaste}
	}
	if r.newFileBtn.Clicked(gtx) {
		closeMenu()
		r.ShowCreateDialog(false)
	}
	if r.newFolderBtn.Clicked(gtx) {
		closeMenu()
		r.ShowCreateDialog(true)
	}
	if r.deleteBtn.Clicked(gtx) {
		closeMenu()
		r.deleteConfirmOpen = true
		state.DeleteTargets = r.collectSelectedPaths(state)
	}
	if r.emptyTrashBtn.Clicked(gtx) {
		closeMenu()
		*eventOut = UIEvent{Action: ActionEmptyTrash}
	}
	if r.permanentDeleteBtn.Clicked(gtx) {
		closeMenu()
		paths := r.collectSelectedPaths(state)
		*eventOut = UIEvent{Action: ActionPermanentDelete, Paths: paths}
	}
	if r.renameBtn.Clicked(gtx) {
		closeMenu()
		// Start rename for the selected item
		if state.SelectedIndex >= 0 && state.SelectedIndex < len(state.Entries) {
			item := &state.Entries[state.SelectedIndex]
			r.StartRename(state.SelectedIndex, item.Path, item.Name, item.IsDir)
		}
	}
	if r.favBtn.Clicked(gtx) {
		closeMenu()
		action := ActionAddFavorite
		if r.menuIsFav {
			action = ActionRemoveFavorite
		}
		*eventOut = UIEvent{Action: action, Path: r.menuPath}
	}
	if r.openInNewTabBtn.Clicked(gtx) {
		closeMenu()
		*eventOut = UIEvent{Action: ActionOpenInNewTab, Path: r.menuPath}
	}
	if r.openTerminalBtn.Clicked(gtx) {
		closeMenu()
		// For favorites/drives sidebar, use the clicked item's path
		// For file list (including background clicks), always use current directory
		termPath := state.CurrentPath
		if r.menuIsFav {
			termPath = r.menuPath
		}
		*eventOut = UIEvent{Action: ActionOpenTerminal, Path: termPath}
	}

	// Background menu (right-click on empty space) shows limited options
	if r.menuIsBackground {
		return r.menuShell(gtx, 180, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.menuItem(gtx, &r.newFileBtn, "New File")
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.menuItem(gtx, &r.newFolderBtn, "New Folder")
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if state.Clipboard == nil {
						return layout.Dimensions{}
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return r.layoutMenuSeparator(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return r.menuItem(gtx, &r.pasteBtn, "Paste")
						}),
					)
				}),
				// Separator and Open Terminal Here
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.layoutMenuSeparator(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.menuItem(gtx, &r.openTerminalBtn, "Open Terminal Here")
				}),
			)
		})
	}

	// Trash view context menu - shows Copy, Cut, and Permanently Delete
	// Note: Restore is not supported on macOS/Windows as they don't track original paths
	if r.isTrashView {
		return r.menuShell(gtx, 180, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.menuItem(gtx, &r.copyBtn, "Copy")
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.menuItem(gtx, &r.cutBtn, "Cut")
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.layoutMenuSeparator(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.menuItemDanger(gtx, &r.permanentDeleteBtn, "Permanently Delete")
				}),
			)
		})
	}

	// Full menu for file/folder right-click
	return r.menuShell(gtx, 180, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if r.menuIsDir {
					return layout.Dimensions{}
				}
				return r.menuItem(gtx, &r.openBtn, "Open")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if r.menuIsDir {
					return layout.Dimensions{}
				}
				return r.menuItem(gtx, &r.openWithBtn, "Open With...")
			}),
			// "Open in New Tab" only shown for directories
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !r.menuIsDir {
					return layout.Dimensions{}
				}
				return r.menuItem(gtx, &r.openInNewTabBtn, "Open in New Tab")
			}),
			// "Open Terminal Here" - always available
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.menuItem(gtx, &r.openTerminalBtn, "Open Terminal Here")
			}),
			// "Open file location" only shown when viewing recent files
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !r.isRecentView {
					return layout.Dimensions{}
				}
				return r.menuItem(gtx, &r.openLocationBtn, "Open File Location")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.menuItem(gtx, &r.copyBtn, "Copy")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.menuItem(gtx, &r.cutBtn, "Cut")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if state.Clipboard == nil {
					return layout.Dimensions{}
				}
				return r.menuItem(gtx, &r.pasteBtn, "Paste")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.menuItem(gtx, &r.renameBtn, "Rename")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.layoutMenuSeparator(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.menuItem(gtx, &r.newFileBtn, "New File")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.menuItem(gtx, &r.newFolderBtn, "New Folder")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.layoutMenuSeparator(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !r.menuIsDir {
					return layout.Dimensions{}
				}
				label := "Add to Favorites"
				if r.menuIsFav {
					label = "Remove Favorite"
				}
				return r.menuItem(gtx, &r.favBtn, label)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := "Delete"
				if trash.IsAvailable() {
					label = trash.VerbPhrase()
				}
				return r.menuItemDanger(gtx, &r.deleteBtn, label)
			}),
		)
	})
}
