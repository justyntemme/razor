package ui

import (
	"image"
	"image/color"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"github.com/justyntemme/razor/internal/debug"
)

// UI timing constants
const (
	doubleClickInterval = 200 * time.Millisecond // Maximum time between clicks for double-click
	animationFrameRate  = 16 * time.Millisecond  // ~60fps for smooth animations
)

func (r *Renderer) Layout(gtx layout.Context, state *State) UIEvent {
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()

	// ===== GLOBAL MOUSE POSITION TRACKING =====
	// Track mouse position for menu placement
	// Use PassOp so events pass through to elements underneath
	areaStack := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	passOp := pointer.PassOp{}.Push(gtx.Ops)
	event.Op(gtx.Ops, &r.mouseTag)
	passOp.Pop()
	areaStack.Pop()

	// Process mouse move events to track position
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &r.mouseTag, Kinds: pointer.Move | pointer.Press | pointer.Drag})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok {
			r.mousePos = image.Pt(int(e.Position.X), int(e.Position.Y))
			if e.Kind == pointer.Press {
				r.lastClickModifiers = e.Modifiers

				// Since we use PassOp, this handler only receives events that weren't
				// consumed by other handlers (rows, sidebar, buttons, etc.)
				// So any click reaching here is on "empty" space - treat it as background

				if e.Buttons.Contain(pointer.ButtonSecondary) {
					// Right-click on background - show context menu
					debug.Log(debug.UI, "Global right-click detected (background): pos=%v", r.mousePos)
					r.bgRightClickPending = true
					r.bgRightClickPos = r.mousePos
				} else if e.Buttons.Contain(pointer.ButtonPrimary) {
					// Left-click on background - dismiss context menu if shown
					debug.Log(debug.UI, "Global left-click detected (background): pos=%v", r.mousePos)
					r.bgLeftClickPending = true
				}
			}
		}
	}

	// ===== KEYBOARD FOCUS =====
	keyTag := &r.listState
	event.Op(gtx.Ops, keyTag)

	// Request focus on first frame
	if !r.focused {
		gtx.Execute(key.FocusCmd{Tag: keyTag})
		r.focused = true
	}

	// Check for focus events (to track when we lose focus)
	for {
		_, ok := gtx.Event(key.FocusFilter{Target: keyTag})
		if !ok {
			break
		}
	}

	eventOut := r.processGlobalInput(gtx, state, keyTag)

	// ===== MAIN LAYOUT =====
	layout.Stack{}.Layout(gtx,
		// Background click handler (for dismissing menus)
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if r.bgClick.Clicked(gtx) {
				r.onLeftClick() // Dismiss context menus, file menu, and exit path edit mode
				r.searchEditorFocused = false // Dismiss search dropdown on background click
				r.searchHistoryVisible = false
				r.CancelRename() // Cancel any active rename
				r.multiSelectMode = false // Exit multi-select mode
				r.lastClickIndex = -1 // Clear click tracking
				r.lastClickTime = time.Time{}
				if !r.settingsOpen && !r.deleteConfirmOpen && !r.createDialogOpen && !state.Conflict.Active {
					eventOut = UIEvent{Action: ActionClearSelection}
					gtx.Execute(key.FocusCmd{Tag: keyTag})
				}
			}
			return r.bgClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),

		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			// Track accumulated vertical offset for file list bounds calculation
			accumulatedHeight := 0

			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					dims := layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						if r.fileMenuBtn.Clicked(gtx) {
							r.onLeftClick()
							r.fileMenuOpen = !r.fileMenuOpen
						}
						btn := material.Button(r.Theme, &r.fileMenuBtn, "File")
						btn.Inset = layout.UniformInset(unit.Dp(6))
						btn.Background, btn.Color = color.NRGBA{}, colBlack
						return btn.Layout(gtx)
					})
					accumulatedHeight += dims.Size.Y
					return dims
				}),

				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					dims := layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(8), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return r.layoutNavBar(gtx, state, keyTag, &eventOut)
						})
					accumulatedHeight += dims.Size.Y
					return dims
				}),

				// Config error banner (shown when config.json failed to parse)
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					dims := r.layoutConfigErrorBanner(gtx)
					accumulatedHeight += dims.Size.Y
					return dims
				}),

				// Browser tab bar (if tabs are enabled)
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					dims := r.layoutBrowserTabBar(gtx, state, &eventOut)
					accumulatedHeight += dims.Size.Y
					return dims
				}),

				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					// Use dynamically calculated vertical offset from accumulated heights
					verticalOffset := accumulatedHeight

					// Calculate horizontal offset for file list (sidebar + divider width in pixels)
					// This must be calculated in the parent context before Flex lays out children
					sidebarWidthPx := gtx.Dp(180)
					dividerWidthPx := gtx.Dp(1)
					horizontalOffset := sidebarWidthPx + dividerWidthPx

					// Build flex children dynamically based on preview visibility
					children := []layout.FlexChild{
						// Sidebar
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X, gtx.Constraints.Max.X = gtx.Dp(180), gtx.Dp(180)
							paint.FillShape(gtx.Ops, colSidebar, clip.Rect{Max: gtx.Constraints.Max}.Op())
							return r.layoutSidebar(gtx, state, &eventOut)
						}),
						// Sidebar divider
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							paint.FillShape(gtx.Ops, color.NRGBA{A: 50}, clip.Rect{Max: image.Pt(gtx.Dp(1), gtx.Constraints.Max.Y)}.Op())
							return layout.Dimensions{Size: image.Pt(gtx.Dp(1), gtx.Constraints.Max.Y)}
						}),
					}

					// Calculate file list flex weight based on preview visibility
					if r.previewVisible {
						// Initialize preview width from percentage if not set
						availableWidth := gtx.Constraints.Max.X - gtx.Dp(181) // Subtract sidebar width
						if r.previewWidth == 0 {
							r.previewWidth = availableWidth * r.previewWidthPct / 100
						}
						// Ensure preview width is within bounds
						if r.previewWidth < r.previewResizeHandle.MinSize {
							r.previewWidth = r.previewResizeHandle.MinSize
						}
						maxPreviewWidth := availableWidth - 200 // Leave at least 200px for file list
						if r.previewResizeHandle.MaxSize > 0 && r.previewResizeHandle.MaxSize < maxPreviewWidth {
							maxPreviewWidth = r.previewResizeHandle.MaxSize
						}
						if r.previewWidth > maxPreviewWidth {
							r.previewWidth = maxPreviewWidth
						}

						children = append(children,
							// File list takes remaining space
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								r.fileListOffset = image.Pt(horizontalOffset, verticalOffset)
								return r.layoutFileList(gtx, state, keyTag, &eventOut)
							}),
							// Resize handle (draggable divider)
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								style := DefaultResizeHandleStyle()
								dims, newWidth := r.previewResizeHandle.Layout(gtx, style, r.previewWidth)
								if newWidth != r.previewWidth {
									r.previewWidth = newWidth
									gtx.Execute(op.InvalidateCmd{})
								}
								return dims
							}),
							// Preview pane with fixed width
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.X = r.previewWidth
								gtx.Constraints.Max.X = r.previewWidth
								return r.layoutPreviewPane(gtx, state)
							}),
						)
					} else {
						children = append(children,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								r.fileListOffset = image.Pt(horizontalOffset, verticalOffset)
								return r.layoutFileList(gtx, state, keyTag, &eventOut)
							}),
						)
					}

					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
				}),

				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.layoutProgressBar(gtx, state)
				}),
			)
		}),

		layout.Stacked(func(gtx layout.Context) layout.Dimensions { return r.layoutFileMenu(gtx, &eventOut) }),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions { return r.layoutContextMenu(gtx, state, &eventOut) }),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions { return r.layoutSearchHistoryOverlay(gtx) }),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions { return r.layoutSettingsModal(gtx, &eventOut) }),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions { return r.layoutHotkeysModal(gtx) }),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions { return r.layoutDeleteConfirm(gtx, state, &eventOut) }),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions { return r.layoutCreateDialog(gtx, state, &eventOut) }),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions { return r.layoutConflictDialog(gtx, state, &eventOut) }),
	)

	return eventOut
}
