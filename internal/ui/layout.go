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
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (r *Renderer) Layout(gtx layout.Context, state *State) UIEvent {
	area := clip.Rect{Max: gtx.Constraints.Max}
	defer area.Push(gtx.Ops).Pop()

	keyTag := &r.listState
	event.Op(gtx.Ops, keyTag)

	if !r.focused {
		gtx.Execute(key.FocusCmd{Tag: keyTag})
		r.focused = true
	}

	// 1. Process Input
	eventOut := r.processGlobalInput(gtx, state)

	// --- VIEW DEFINITIONS ---

	appBar := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if r.fileMenuBtn.Clicked(gtx) {
					r.fileMenuOpen = !r.fileMenuOpen
				}
				btn := material.Button(r.Theme, &r.fileMenuBtn, "File")
				btn.Inset = layout.UniformInset(unit.Dp(6))
				btn.Background = color.NRGBA{}
				btn.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
				return btn.Layout(gtx)
			}),
		)
	}

	navBar := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if state.CanBack && r.backBtn.Clicked(gtx) {
					eventOut = UIEvent{Action: ActionBack}
					gtx.Execute(key.FocusCmd{Tag: keyTag})
				}
				btn := material.Button(r.Theme, &r.backBtn, "<")
				if !state.CanBack {
					btn.Background = color.NRGBA{R: 200, G: 200, B: 200, A: 255}
					btn.Color = color.NRGBA{R: 150, G: 150, B: 150, A: 255}
				}
				return btn.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if state.CanForward && r.fwdBtn.Clicked(gtx) {
					eventOut = UIEvent{Action: ActionForward}
					gtx.Execute(key.FocusCmd{Tag: keyTag})
				}
				btn := material.Button(r.Theme, &r.fwdBtn, ">")
				if !state.CanForward {
					btn.Background = color.NRGBA{R: 200, G: 200, B: 200, A: 255}
					btn.Color = color.NRGBA{R: 150, G: 150, B: 150, A: 255}
				}
				return btn.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if r.isEditing {
					for {
						evt, ok := r.pathEditor.Update(gtx)
						if !ok {
							break
						}
						if s, ok := evt.(widget.SubmitEvent); ok {
							r.isEditing = false
							eventOut = UIEvent{Action: ActionNavigate, Path: strings.TrimSpace(s.Text)}
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
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(unit.Dp(200))
				gtx.Constraints.Max.X = gtx.Dp(unit.Dp(200))
				for {
					evt, ok := r.searchEditor.Update(gtx)
					if !ok {
						break
					}
					if s, ok := evt.(widget.SubmitEvent); ok {
						eventOut = UIEvent{Action: ActionSearch, Path: s.Text}
						gtx.Execute(key.FocusCmd{Tag: keyTag})
					}
				}
				ed := material.Editor(r.Theme, &r.searchEditor, "Search...")
				ed.TextSize = unit.Sp(14)
				border := widget.Border{Color: color.NRGBA{R: 200, G: 200, B: 200, A: 255}, Width: unit.Dp(1), CornerRadius: unit.Dp(4)}
				return border.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, ed.Layout)
				})
			}),
		)
	}

	// Sidebar now mirrors mainList structure exactly
	sidebar := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: unit.Dp(8), Top: unit.Dp(8), Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.Body2(r.Theme, "FAVORITES").Layout(gtx)
				})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				// KEY: Add pointer.PassOp at top level, exactly like mainList does
				defer pointer.PassOp{}.Push(gtx.Ops).Pop()

				return r.favState.Layout(gtx, len(state.FavList), func(gtx layout.Context, index int) layout.Dimensions {
					fav := &state.FavList[index]

					// Handle left-click - check BEFORE renderFavoriteRow, exactly like mainList
					if fav.Clickable.Clicked(gtx) {
						eventOut = UIEvent{Action: ActionNavigate, Path: fav.Path}
					}

					// Handle right-click - check BEFORE renderFavoriteRow, exactly like mainList
					if r.detectRightClick(gtx, &fav.RightClickTag) {
						r.menuVisible = true
						r.menuPos = r.mousePos
						r.menuPath = fav.Path
						r.menuIsDir = true // Favorites are always directories
						r.menuIsFav = true
					}

					return r.renderFavoriteRow(gtx, fav, false)
				})
			}),
		)
	}

	mainList := func(gtx layout.Context) layout.Dimensions {
		defer pointer.PassOp{}.Push(gtx.Ops).Pop()
		if r.Debug {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 240, G: 240, B: 240, A: 255}, clip.Rect{Max: gtx.Constraints.Max}.Op())
		}
		return r.listState.Layout(gtx, len(state.Entries), func(gtx layout.Context, index int) layout.Dimensions {
			item := &state.Entries[index]
			isSelected := index == state.SelectedIndex

			if item.Clickable.Clicked(gtx) {
				if r.isEditing {
					r.isEditing = false
				}
				if r.fileMenuOpen {
					r.fileMenuOpen = false
				}
				eventOut = UIEvent{Action: ActionSelect, NewIndex: index}
				gtx.Execute(key.FocusCmd{Tag: keyTag})
				now := time.Now()
				if !item.LastClick.IsZero() && now.Sub(item.LastClick) < 500*time.Millisecond {
					if item.IsDir {
						eventOut = UIEvent{Action: ActionNavigate, Path: item.Path}
					} else {
						eventOut = UIEvent{Action: ActionOpen, Path: item.Path}
					}
				}
				item.LastClick = now
			}

			// Use shared Right Click helper
			if r.detectRightClick(gtx, &item.RightClickTag) {
				r.menuVisible = true
				r.menuPos = r.mousePos
				r.menuPath = item.Path
				r.menuIsDir = item.IsDir
				_, r.menuIsFav = state.Favorites[item.Path]
				eventOut = UIEvent{Action: ActionSelect, NewIndex: index}
			}

			return r.renderRow(gtx, item, isSelected)
		})
	}

	// 2. Main Layout Stack
	layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if r.bgClick.Clicked(gtx) {
				if r.menuVisible {
					r.menuVisible = false
				}
				if r.fileMenuOpen {
					r.fileMenuOpen = false
				}
				if !r.settingsOpen {
					eventOut = UIEvent{Action: ActionSelect, NewIndex: -1}
					gtx.Execute(key.FocusCmd{Tag: keyTag})
				}
			}
			return r.bgClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(0), Bottom: unit.Dp(0), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, appBar)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(8), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, navBar)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Dp(180)
							gtx.Constraints.Max.X = gtx.Dp(180)
							paint.FillShape(gtx.Ops, color.NRGBA{R: 245, G: 245, B: 245, A: 255}, clip.Rect{Max: gtx.Constraints.Max}.Op())
							return sidebar(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							paint.FillShape(gtx.Ops, color.NRGBA{A: 50}, clip.Rect{Max: image.Pt(gtx.Dp(1), gtx.Constraints.Max.Y)}.Op())
							return layout.Dimensions{Size: image.Pt(gtx.Dp(1), gtx.Constraints.Max.Y)}
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										dims, evt := r.renderColumns(gtx)
										if evt.Action != ActionNone {
											eventOut = evt
										}
										return dims
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return widget.Border{Color: color.NRGBA{A: 50}, Width: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Spacer{Height: unit.Dp(1), Width: unit.Dp(1)}.Layout(gtx)
									})
								}),
								layout.Flexed(1, mainList),
							)
						}),
					)
				}),
			)
		}),

		// --- POPUPS & MENUS (Moved to menus.go) ---

		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			// Logic now lives in layoutFileMenu
			dims, evt := r.layoutFileMenu(gtx)
			if evt.Action != ActionNone {
				eventOut = evt
			}
			return dims
		}),

		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			// Logic now lives in layoutContextMenu
			dims, evt := r.layoutContextMenu(gtx)
			if evt.Action != ActionNone {
				eventOut = evt
			}
			return dims
		}),

		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			// Logic now lives in layoutSettingsModal
			return r.layoutSettingsModal(gtx)
		}),
	)

	return eventOut
}