package ui

import (
	"fmt"
	"image"
	"image/color"
	"sync/atomic"
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

	"gioui.org/font"

	"github.com/justyntemme/razor/internal/debug"
)

// File list layout - main file/folder display, progress bar, error banner

func (r *Renderer) layoutFileList(gtx layout.Context, state *State, keyTag *layout.List, eventOut *UIEvent) layout.Dimensions {
	// Track file list size for background click hit-testing
	r.fileListSize = gtx.Constraints.Max

	// Track vertical offset for row bounds calculation
	headerHeight := 0

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					dims, evt := r.renderColumns(gtx)
					if evt.Action != ActionNone {
						*eventOut = evt
					}
					return dims
				})
			headerHeight = dims.Size.Y
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := widget.Border{Color: color.NRGBA{A: 50}, Width: unit.Dp(1)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Spacer{Height: unit.Dp(1), Width: unit.Dp(1)}.Layout(gtx)
				})
			headerHeight += dims.Size.Y
			return dims
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			// === BACKGROUND CLICK DETECTION ===
			// Create a hit area filling the entire available space behind the list.
			// This catches clicks that miss the rows (empty space).
			defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
			event.Op(gtx.Ops, &r.bgRightClickTag)

			// Process events for the background tag (both left and right clicks)
			// NOTE: We track if a row was clicked to avoid triggering background actions
			// when clicking on a row (since background is behind rows in z-order)
			rowRightClicked := false
			rowLeftClicked := false
			for {
				ev, ok := gtx.Event(pointer.Filter{Target: &r.bgRightClickTag, Kinds: pointer.Press})
				if !ok {
					break
				}
				if e, ok := ev.(pointer.Event); ok && e.Kind == pointer.Press {
					if e.Buttons.Contain(pointer.ButtonSecondary) {
						// Right-click - mark as pending, show menu if no row handles it
						r.bgRightClickPending = true
						r.bgRightClickPos = r.mousePos
					} else if e.Buttons.Contain(pointer.ButtonPrimary) {
						// Left-click on empty space - dismiss menus and clear selection
						r.onLeftClick()
						r.multiSelectMode = false
						r.lastClickIndex = -1
						r.lastClickTime = time.Time{}
						if !r.settingsOpen && !r.deleteConfirmOpen && !r.createDialogOpen && !state.Conflict.Active {
							*eventOut = UIEvent{Action: ActionClearSelection}
							gtx.Execute(key.FocusCmd{Tag: keyTag})
						}
					}
				}
			}

			return layout.Stack{}.Layout(gtx, layout.Stacked(func(gtx layout.Context) layout.Dimensions {

				// Layout file list
				listDims := r.listState.Layout(gtx, len(state.Entries), func(gtx layout.Context, i int) layout.Dimensions {
					item := &state.Entries[i]
					isRenaming := r.renameIndex == i

					// Check if this item should show as checked:
					// - If in multi-select mode (SelectedIndices is populated), check if it's in the map
					// - Otherwise, check if it's the primary selected item
					isChecked := false
					if state.SelectedIndices != nil && len(state.SelectedIndices) > 0 {
						isChecked = state.SelectedIndices[i]
					} else {
						isChecked = i == state.SelectedIndex
					}

					// Show checkboxes when any item is selected (allows user to enter multi-select via checkbox)
					// Only show checkboxes when in multi-select mode (entered via shift+click)
					showCheckbox := r.multiSelectMode

					// Render row and capture right-click event
					rowDims, leftClicked, rightClicked, shiftHeld, _, renameEvt, checkboxToggled, chevronEvt, dropEvt := r.renderRow(gtx, item, i, i == state.SelectedIndex, isRenaming, isChecked, showCheckbox)

					// Handle chevron click (expand/collapse directory)
					chevronClicked := false
					if chevronEvt != nil {
						debug.Log(debug.UI, "Chevron event received in layout, Action=%d Path=%s", chevronEvt.Action, chevronEvt.Path)
						*eventOut = *chevronEvt
						chevronClicked = true
					}

					// Handle drop event (file dropped onto directory)
					if dropEvt != nil {
						*eventOut = *dropEvt
					}

					// Handle checkbox toggle (only visible in multi-select mode)
					if checkboxToggled && r.multiSelectMode {
						*eventOut = UIEvent{Action: ActionToggleSelect, NewIndex: i, OldIndex: -1}
					}

					// Handle rename event
					if renameEvt != nil {
						*eventOut = *renameEvt
					}

					// Handle right-click on file/folder
					if rightClicked && !isRenaming {
						rowRightClicked = true
						r.bgRightClickPending = false // Cancel background right-click
						r.menuVisible = true
						r.menuPos = r.mousePos // Use global mouse position
						r.menuPath = item.Path
						r.menuIsDir = item.IsDir
						_, r.menuIsFav = state.Favorites[item.Path]
						r.menuIsBackground = false
						// Don't change selection on right-click - menu operations use collectSelectedPaths()
						// which will get all selected items. Selection is only cleared after copy/cut.
					}

					// Handle left-click (but not if renaming or chevron was clicked)
					if leftClicked && !isRenaming && !checkboxToggled && !chevronClicked {
						rowLeftClicked = true
						r.bgLeftClickPending = false // Cancel background left-click
						r.onLeftClick()
						r.CancelRename()
						r.isEditing = false
						gtx.Execute(key.FocusCmd{Tag: keyTag})

						now := time.Now()

						// Check if this is a double-click on the same item
						isDoubleClick := r.lastClickIndex == i &&
							!r.lastClickTime.IsZero() &&
							now.Sub(r.lastClickTime) < doubleClickInterval

						if isDoubleClick {
							// Double-click: exit multi-select mode and navigate/open
							r.multiSelectMode = false
							r.lastClickIndex = -1
							r.lastClickTime = time.Time{}
							if item.IsDir {
								*eventOut = UIEvent{Action: ActionNavigate, Path: item.Path}
							} else {
								*eventOut = UIEvent{Action: ActionOpen, Path: item.Path}
							}
						} else if shiftHeld {
							// Shift+click: toggle checkbox (alias for checkbox click)
							r.lastClickIndex = i
							r.lastClickTime = now
							if !r.multiSelectMode && state.SelectedIndex >= 0 {
								// First shift+click: add current selection to multi-select, then toggle new item
								r.multiSelectMode = true
								*eventOut = UIEvent{Action: ActionToggleSelect, NewIndex: i, OldIndex: state.SelectedIndex}
							} else {
								r.multiSelectMode = true
								*eventOut = UIEvent{Action: ActionToggleSelect, NewIndex: i, OldIndex: -1}
							}
						} else {
							// Single click: select immediately (no delay)
							r.multiSelectMode = false
							*eventOut = UIEvent{Action: ActionSelect, NewIndex: i}
							r.lastClickIndex = i
							r.lastClickTime = now
						}
					}

					return rowDims
				})

				// After processing all rows, check if we have a pending background right-click
				// that wasn't handled by any row (i.e., click was on empty space)
				if r.bgRightClickPending && !rowRightClicked {
					r.menuVisible = true
					r.menuPos = r.bgRightClickPos
					r.menuPath = state.CurrentPath
					r.menuIsDir = true
					r.menuIsFav = false
					r.menuIsBackground = true
					// Don't clear selection on background right-click
				}
				r.bgRightClickPending = false

				// Handle pending background left-click - dismiss context menu
				if r.bgLeftClickPending && !rowLeftClicked {
					r.onLeftClick() // Dismiss context menus, file menu, exit path edit mode
				}
				r.bgLeftClickPending = false

				return listDims
			}))
		}),
	)
}

func (r *Renderer) layoutProgressBar(gtx layout.Context, state *State) layout.Dimensions {
	if !state.Progress.Active {
		r.progressAnimStart = time.Time{} // Reset animation
		return layout.Dimensions{}
	}

	// Initialize animation start time
	if r.progressAnimStart.IsZero() {
		r.progressAnimStart = gtx.Now
	}

	// Read progress values atomically to avoid races with background updates
	progressCurrent := atomic.LoadInt64(&state.Progress.Current)
	progressTotal := state.Progress.Total // Total is set atomically in setProgress, not incremented

	return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					var label string
					if progressTotal > 0 {
						// Determinate progress - show percentage
						pct := float32(progressCurrent) / float32(progressTotal)
						label = fmt.Sprintf("%s - %s / %s (%.0f%%)",
							state.Progress.Label,
							formatSize(progressCurrent),
							formatSize(progressTotal),
							pct*100)
					} else {
						// Indeterminate progress - just show the label (e.g., "Found 42 files...")
						label = state.Progress.Label
					}
					lbl := material.Body2(r.Theme, label)
					lbl.Color = colGray
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					height := gtx.Dp(unit.Dp(8))
					width := gtx.Constraints.Max.X

					// Draw background
					paint.FillShape(gtx.Ops, colLightGray, clip.Rect{Max: image.Pt(width, height)}.Op())

					if progressTotal > 0 {
						// Determinate progress - fill based on percentage
						pct := float32(progressCurrent) / float32(progressTotal)
						fillWidth := int(float32(width) * pct)
						paint.FillShape(gtx.Ops, colProgress, clip.Rect{Max: image.Pt(fillWidth, height)}.Op())
					} else {
						// Indeterminate progress - animated sliding bar
						elapsed := gtx.Now.Sub(r.progressAnimStart).Seconds()

						// Create a bouncing animation: bar slides back and forth
						// Complete cycle every 1.5 seconds
						cycle := float32(elapsed) / 1.5
						pos := cycle - float32(int(cycle)) // 0.0 to 1.0

						// Ping-pong: 0->1->0
						if int(cycle)%2 == 1 {
							pos = 1.0 - pos
						}

						// Bar is 30% of the width
						barWidth := int(float32(width) * 0.3)
						barStart := int(pos * float32(width-barWidth))

						paint.FillShape(gtx.Ops, colProgress, clip.Rect{
							Min: image.Pt(barStart, 0),
							Max: image.Pt(barStart+barWidth, height),
						}.Op())

						// Request redraw for animation
						gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(animationFrameRate)})
					}
					return layout.Dimensions{Size: image.Pt(width, height)}
				}),
			)
		})
}

// layoutConfigErrorBanner renders a red error banner when config.json fails to parse
func (r *Renderer) layoutConfigErrorBanner(gtx layout.Context) layout.Dimensions {
	if r.ConfigError == "" {
		return layout.Dimensions{}
	}

	return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Bottom: unit.Dp(4)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			// Draw red background
			height := gtx.Dp(28)
			paint.FillShape(gtx.Ops, colErrorBannerBg, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, height)}.Op())

			return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(r.Theme, "Config error: "+r.ConfigError+" (using defaults)")
					lbl.Color = colErrorBannerText
					lbl.Font.Weight = font.Bold
					lbl.MaxLines = 1
					return lbl.Layout(gtx)
				})
		})
}
