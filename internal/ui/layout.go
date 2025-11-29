package ui

import (
	"image"
	"image/color"
	"strings"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (r *Renderer) Layout(gtx layout.Context, state *State) UIEvent {
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()

	keyTag := &r.listState
	event.Op(gtx.Ops, keyTag)
	if !r.focused {
		gtx.Execute(key.FocusCmd{Tag: keyTag})
		r.focused = true
	}

	eventOut := r.processGlobalInput(gtx, state)

	// Main layout stack
	layout.Stack{}.Layout(gtx,
		// Background click handler
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if r.bgClick.Clicked(gtx) {
				r.menuVisible, r.fileMenuOpen = false, false
				if !r.settingsOpen {
					eventOut = UIEvent{Action: ActionSelect, NewIndex: -1}
					gtx.Execute(key.FocusCmd{Tag: keyTag})
				}
			}
			return r.bgClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),

		// Main content
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				// App bar
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						if r.fileMenuBtn.Clicked(gtx) {
							r.fileMenuOpen = !r.fileMenuOpen
						}
						btn := material.Button(r.Theme, &r.fileMenuBtn, "File")
						btn.Inset = layout.UniformInset(unit.Dp(6))
						btn.Background, btn.Color = color.NRGBA{}, colBlack
						return btn.Layout(gtx)
					})
				}),

				// Nav bar
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(8), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return r.layoutNavBar(gtx, state, keyTag, &eventOut)
						})
				}),

				// Main area (sidebar + file list)
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						// Sidebar
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X, gtx.Constraints.Max.X = gtx.Dp(180), gtx.Dp(180)
							paint.FillShape(gtx.Ops, colSidebar, clip.Rect{Max: gtx.Constraints.Max}.Op())
							return r.layoutSidebar(gtx, state, &eventOut)
						}),
						// Divider
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							paint.FillShape(gtx.Ops, color.NRGBA{A: 50}, clip.Rect{Max: image.Pt(gtx.Dp(1), gtx.Constraints.Max.Y)}.Op())
							return layout.Dimensions{Size: image.Pt(gtx.Dp(1), gtx.Constraints.Max.Y)}
						}),
						// File list
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return r.layoutFileList(gtx, state, keyTag, &eventOut)
						}),
					)
				}),
			)
		}),

		// Popups
		layout.Stacked(func(gtx layout.Context) layout.Dimensions { return r.layoutFileMenu(gtx) }),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions { return r.layoutContextMenu(gtx, &eventOut) }),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions { return r.layoutSettingsModal(gtx, &eventOut) }),
	)

	return eventOut
}

func (r *Renderer) layoutNavBar(gtx layout.Context, state *State, keyTag *layout.List, eventOut *UIEvent) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		// Back/Forward buttons - unified helper
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.navButton(gtx, &r.backBtn, "<", state.CanBack, func() { *eventOut = UIEvent{Action: ActionBack} }, keyTag)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.navButton(gtx, &r.fwdBtn, ">", state.CanForward, func() { *eventOut = UIEvent{Action: ActionForward} }, keyTag)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),

		// Path bar
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if r.isEditing {
				for {
					evt, ok := r.pathEditor.Update(gtx)
					if !ok {
						break
					}
					if s, ok := evt.(widget.SubmitEvent); ok {
						r.isEditing = false
						*eventOut = UIEvent{Action: ActionNavigate, Path: strings.TrimSpace(s.Text)}
						gtx.Execute(key.FocusCmd{Tag: keyTag})
					}
				}
				return material.Editor(r.Theme, &r.pathEditor, "Path").Layout(gtx)
			}
			if r.pathClick.Clicked(gtx) {
				r.isEditing = true
				r.pathEditor.SetText(state.CurrentPath)
				gtx.Execute(key.FocusCmd{Tag: &r.pathEditor})
			}
			return material.Clickable(gtx, &r.pathClick, func(gtx layout.Context) layout.Dimensions {
				return material.H6(r.Theme, state.CurrentPath).Layout(gtx)
			})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),

		// Search box
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = gtx.Dp(200), gtx.Dp(200)
			for {
				evt, ok := r.searchEditor.Update(gtx)
				if !ok {
					break
				}
				if s, ok := evt.(widget.SubmitEvent); ok {
					*eventOut = UIEvent{Action: ActionSearch, Path: s.Text}
					gtx.Execute(key.FocusCmd{Tag: keyTag})
				}
			}
			ed := material.Editor(r.Theme, &r.searchEditor, "Search...")
			ed.TextSize = unit.Sp(14)
			return widget.Border{Color: colLightGray, Width: unit.Dp(1), CornerRadius: unit.Dp(4)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, ed.Layout)
				})
		}),
	)
}

func (r *Renderer) navButton(gtx layout.Context, btn *widget.Clickable, label string, enabled bool, action func(), keyTag *layout.List) layout.Dimensions {
	if enabled && btn.Clicked(gtx) {
		action()
		gtx.Execute(key.FocusCmd{Tag: keyTag})
	}
	b := material.Button(r.Theme, btn, label)
	if !enabled {
		b.Background, b.Color = colLightGray, colDisabled
	}
	return b.Layout(gtx)
}

func (r *Renderer) layoutSidebar(gtx layout.Context, state *State, eventOut *UIEvent) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(8)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return material.Body2(r.Theme, "FAVORITES").Layout(gtx)
				})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			defer pointer.PassOp{}.Push(gtx.Ops).Pop()
			return r.favState.Layout(gtx, len(state.FavList), func(gtx layout.Context, i int) layout.Dimensions {
				fav := &state.FavList[i]
				if fav.Clickable.Clicked(gtx) {
					*eventOut = UIEvent{Action: ActionNavigate, Path: fav.Path}
				}
				if r.detectRightClick(gtx, &fav.RightClickTag) {
					r.menuVisible, r.menuPos, r.menuPath = true, r.mousePos, fav.Path
					r.menuIsDir, r.menuIsFav = true, true
				}
				return r.renderFavoriteRow(gtx, fav)
			})
		}),
	)
}

func (r *Renderer) layoutFileList(gtx layout.Context, state *State, keyTag *layout.List, eventOut *UIEvent) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Column headers
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					dims, evt := r.renderColumns(gtx)
					if evt.Action != ActionNone {
						*eventOut = evt
					}
					return dims
				})
		}),
		// Divider
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return widget.Border{Color: color.NRGBA{A: 50}, Width: unit.Dp(1)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Spacer{Height: unit.Dp(1), Width: unit.Dp(1)}.Layout(gtx)
				})
		}),
		// File list
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			defer pointer.PassOp{}.Push(gtx.Ops).Pop()
			return r.listState.Layout(gtx, len(state.Entries), func(gtx layout.Context, i int) layout.Dimensions {
				item := &state.Entries[i]
				if item.Clickable.Clicked(gtx) {
					r.isEditing, r.fileMenuOpen = false, false
					*eventOut = UIEvent{Action: ActionSelect, NewIndex: i}
					gtx.Execute(key.FocusCmd{Tag: keyTag})
					if now := time.Now(); !item.LastClick.IsZero() && now.Sub(item.LastClick) < 500*time.Millisecond {
						if item.IsDir {
							*eventOut = UIEvent{Action: ActionNavigate, Path: item.Path}
						} else {
							*eventOut = UIEvent{Action: ActionOpen, Path: item.Path}
						}
					}
					item.LastClick = time.Now()
				}
				if r.detectRightClick(gtx, &item.RightClickTag) {
					r.menuVisible, r.menuPos, r.menuPath = true, r.mousePos, item.Path
					r.menuIsDir = item.IsDir
					_, r.menuIsFav = state.Favorites[item.Path]
					*eventOut = UIEvent{Action: ActionSelect, NewIndex: i}
				}
				return r.renderRow(gtx, item, i == state.SelectedIndex)
			})
		}),
	)
}

// --- MENUS ---

func (r *Renderer) menuShell(gtx layout.Context, width unit.Dp, content layout.Widget) layout.Dimensions {
	return widget.Border{Color: colLightGray, Width: unit.Dp(1), CornerRadius: unit.Dp(4)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					paint.FillShape(gtx.Ops, colWhite, clip.Rect{Max: gtx.Constraints.Min}.Op())
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(width)
					return content(gtx)
				}),
			)
		})
}

func (r *Renderer) menuItem(gtx layout.Context, btn *widget.Clickable, label string) layout.Dimensions {
	return material.Clickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body2(r.Theme, label)
			lbl.Color = colBlack
			return lbl.Layout(gtx)
		})
	})
}

func (r *Renderer) layoutFileMenu(gtx layout.Context) layout.Dimensions {
	if !r.fileMenuOpen {
		return layout.Dimensions{}
	}
	defer op.Offset(image.Pt(8, 30)).Push(gtx.Ops).Pop()
	return r.menuShell(gtx, 120, func(gtx layout.Context) layout.Dimensions {
		if r.settingsBtn.Clicked(gtx) {
			r.fileMenuOpen, r.settingsOpen = false, true
		}
		return r.menuItem(gtx, &r.settingsBtn, "Settings")
	})
}

func (r *Renderer) layoutContextMenu(gtx layout.Context, eventOut *UIEvent) layout.Dimensions {
	if !r.menuVisible {
		return layout.Dimensions{}
	}
	defer op.Offset(r.menuPos).Push(gtx.Ops).Pop()

	if r.openBtn.Clicked(gtx) {
		r.menuVisible = false
		*eventOut = UIEvent{Action: ActionOpen, Path: r.menuPath}
	}
	if r.copyBtn.Clicked(gtx) {
		r.menuVisible = false
	}
	if r.favBtn.Clicked(gtx) {
		r.menuVisible = false
		action := ActionAddFavorite
		if r.menuIsFav {
			action = ActionRemoveFavorite
		}
		*eventOut = UIEvent{Action: action, Path: r.menuPath}
	}

	return r.menuShell(gtx, 180, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if r.menuIsDir {
					return layout.Dimensions{}
				}
				return r.menuItem(gtx, &r.openBtn, "Open")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.menuItem(gtx, &r.copyBtn, "Copy (noop)")
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
		)
	})
}

func (r *Renderer) layoutSettingsModal(gtx layout.Context, eventOut *UIEvent) layout.Dimensions {
	if !r.settingsOpen {
		return layout.Dimensions{}
	}
	if r.settingsCloseBtn.Clicked(gtx) {
		r.settingsOpen = false
	}
	if r.showDotfilesCheck.Update(gtx) {
		r.ShowDotfiles = r.showDotfilesCheck.Value
		*eventOut = UIEvent{Action: ActionToggleDotfiles, ShowDotfiles: r.ShowDotfiles}
	}

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, color.NRGBA{A: 150}, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return material.Clickable(gtx, &r.settingsCloseBtn, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return r.menuShell(gtx, 300, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = gtx.Dp(200)
					return layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								h6 := material.H6(r.Theme, "Settings")
								h6.Color = colBlack
								return h6.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								cb := material.CheckBox(r.Theme, &r.showDotfilesCheck, "Show dotfiles")
								cb.Color = colBlack
								return cb.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := material.Body1(r.Theme, "Search Engine:")
								lbl.Color = colBlack
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