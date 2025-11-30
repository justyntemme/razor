package ui

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (r *Renderer) Layout(gtx layout.Context, state *State) UIEvent {
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()

	// ===== TRACK GLOBAL MOUSE POSITION =====
	// Register for all pointer events at root level to track mouse position
	// Use PassOp so events pass through to elements underneath
	areaStack := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	passOp := pointer.PassOp{}.Push(gtx.Ops)
	event.Op(gtx.Ops, &r.mouseTag)
	passOp.Pop()
	areaStack.Pop()

	// Process mouse position events
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &r.mouseTag, Kinds: pointer.Move | pointer.Press})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok {
			r.mousePos = image.Pt(int(e.Position.X), int(e.Position.Y))
		}
	}

	// ===== KEYBOARD FOCUS =====
	keyTag := &r.listState
	event.Op(gtx.Ops, keyTag)
	if !r.focused {
		gtx.Execute(key.FocusCmd{Tag: keyTag})
		r.focused = true
	}

	eventOut := r.processGlobalInput(gtx, state)

	// ===== MAIN LAYOUT =====
	layout.Stack{}.Layout(gtx,
		// Background click handler (for dismissing menus)
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if r.bgClick.Clicked(gtx) {
				r.menuVisible, r.fileMenuOpen = false, false
				r.searchEditorFocused = false // Dismiss search dropdown on background click
				r.searchHistoryVisible = false
				r.CancelRename() // Cancel any active rename
				if !r.settingsOpen && !r.deleteConfirmOpen && !r.createDialogOpen && !state.Conflict.Active {
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

				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(8), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return r.layoutNavBar(gtx, state, keyTag, &eventOut)
						})
				}),

				// Config error banner (shown when config.json failed to parse)
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.layoutConfigErrorBanner(gtx)
				}),

				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					// Track vertical offset: File button (~32dp) + navbar (~44dp) + insets (~16dp) ≈ 92dp
					verticalOffset := gtx.Dp(92)
					
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X, gtx.Constraints.Max.X = gtx.Dp(180), gtx.Dp(180)
							paint.FillShape(gtx.Ops, colSidebar, clip.Rect{Max: gtx.Constraints.Max}.Op())
							return r.layoutSidebar(gtx, state, &eventOut)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							paint.FillShape(gtx.Ops, color.NRGBA{A: 50}, clip.Rect{Max: image.Pt(gtx.Dp(1), gtx.Constraints.Max.Y)}.Op())
							return layout.Dimensions{Size: image.Pt(gtx.Dp(1), gtx.Constraints.Max.Y)}
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							// Horizontal offset: sidebar (180dp) + divider (1dp) = 181dp
							r.fileListOffset = image.Pt(gtx.Dp(181), verticalOffset)
							return r.layoutFileList(gtx, state, keyTag, &eventOut)
						}),
					)
				}),

				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.layoutProgressBar(gtx, state)
				}),
			)
		}),

		layout.Stacked(func(gtx layout.Context) layout.Dimensions { return r.layoutFileMenu(gtx, &eventOut) }),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions { return r.layoutContextMenu(gtx, state, &eventOut) }),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions { return r.layoutSearchHistoryOverlay(gtx) }),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions { return r.layoutSettingsModal(gtx, &eventOut) }),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions { return r.layoutDeleteConfirm(gtx, state, &eventOut) }),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions { return r.layoutCreateDialog(gtx, state, &eventOut) }),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions { return r.layoutConflictDialog(gtx, state, &eventOut) }),
	)

	return eventOut
}

func (r *Renderer) layoutNavBar(gtx layout.Context, state *State, keyTag *layout.List, eventOut *UIEvent) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.navButton(gtx, &r.backBtn, "◀", state.CanBack, func() { *eventOut = UIEvent{Action: ActionBack} }, keyTag)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.navButton(gtx, &r.fwdBtn, "▶", state.CanForward, func() { *eventOut = UIEvent{Action: ActionForward} }, keyTag)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if r.homeBtn.Clicked(gtx) {
				*eventOut = UIEvent{Action: ActionHome}
				gtx.Execute(key.FocusCmd{Tag: keyTag})
			}
			// Circular home button
			size := gtx.Dp(32)
			return material.Clickable(gtx, &r.homeBtn, func(gtx layout.Context) layout.Dimensions {
				// Draw circular background
				circle := clip.Ellipse{Min: image.Pt(0, 0), Max: image.Pt(size, size)}.Op(gtx.Ops)
				paint.FillShape(gtx.Ops, colHomeBtnBg, circle)

				// Center the icon
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min = image.Pt(size, size)
					gtx.Constraints.Max = image.Pt(size, size)
					lbl := material.Body1(r.Theme, "⌂")
					lbl.Color = colWhite
					lbl.Alignment = text.Middle
					// Offset slightly for visual centering
					return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, lbl.Layout)
				})
			})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),

		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			// Clip the entire path area to prevent overflow into search box
			defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()

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
				// Truncate path to show start.../end (current directory)
				displayPath := truncatePathMiddle(state.CurrentPath, 50)

				// Scale font size based on display path length
				displayLen := len(displayPath)
				var fontSize unit.Sp
				switch {
				case displayLen <= 30:
					fontSize = unit.Sp(16)
				case displayLen <= 40:
					fontSize = unit.Sp(14)
				default:
					fontSize = unit.Sp(12)
				}

				lbl := material.Body1(r.Theme, displayPath)
				lbl.TextSize = fontSize
				lbl.MaxLines = 1

				// Layout with optional "Search Results" badge
				if state.IsSearchResult {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, lbl.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return widget.Border{Color: colAccent, Width: unit.Dp(1), CornerRadius: unit.Dp(4)}.Layout(gtx,
									func(gtx layout.Context) layout.Dimensions {
										return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx,
											func(gtx layout.Context) layout.Dimensions {
												badge := material.Caption(r.Theme, "Search Results")
												badge.Color = colAccent
												return badge.Layout(gtx)
											})
									})
							})
						}),
					)
				}
				return lbl.Layout(gtx)
			})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			searchBoxWidth := gtx.Dp(280)
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = searchBoxWidth, searchBoxWidth

			// Sync UI state with application state
			if !state.IsSearchResult && r.searchActive {
				r.searchActive = false
			}

			// Parse directives for visual display (cached - only re-parse when text changes)
			currentText := strings.TrimSpace(r.searchEditor.Text())
			if currentText != r.lastParsedSearchText {
				r.detectedDirectives, _ = parseDirectivesForDisplay(currentText)
				r.lastParsedSearchText = currentText
			}

			// Note: We can't easily detect focus state in Gio, so we'll show history
			// whenever the search box has content or user is typing

			// Handle search editor events
			submitPressed := false
			for {
				evt, ok := r.searchEditor.Update(gtx)
				if !ok {
					break
				}
				switch evt.(type) {
				case widget.SubmitEvent:
					submitPressed = true
					query := strings.TrimSpace(r.searchEditor.Text())
					r.lastSearchQuery = query
					r.searchActive = query != ""
					r.searchHistoryVisible = false
					r.searchEditorFocused = false // Lose focus on submit
					// Mark as submitted so history is saved
					*eventOut = UIEvent{Action: ActionSearch, Path: query, SearchSubmitted: true}
					gtx.Execute(key.FocusCmd{Tag: keyTag})
				case widget.ChangeEvent:
					// Editor has focus when it receives change events
					r.searchEditorFocused = true
				case widget.SelectEvent:
					// Editor has focus when selection changes
					r.searchEditorFocused = true
				}
			}

			// Search-as-you-type: only for simple filename searches
			// Disable if ANY directive prefix with colon is detected (even without value)
			lowerText := strings.ToLower(currentText)
			hasDirectivePrefix := strings.Contains(lowerText, "contents:") ||
				strings.Contains(lowerText, "ext:") ||
				strings.Contains(lowerText, "size:") ||
				strings.Contains(lowerText, "modified:") ||
				strings.Contains(lowerText, "filename:") ||
				strings.Contains(lowerText, "recursive:") ||
				strings.Contains(lowerText, "depth:")

			if hasDirectivePrefix {
				// Directive detected - restore directory listing once
				// BUT skip if user just pressed Enter (submit takes precedence)
				if !r.directiveRestored && !submitPressed {
					r.directiveRestored = true
					r.searchActive = false
					*eventOut = UIEvent{Action: ActionClearSearch}
				}
				r.lastSearchQuery = currentText
			} else {
				// No directive - allow search-as-you-type
				r.directiveRestored = false // Reset when no directive
				if currentText != r.lastSearchQuery && !submitPressed {
					r.lastSearchQuery = currentText
					r.searchActive = currentText != ""
					// NOT submitted - don't save to history
					*eventOut = UIEvent{Action: ActionSearch, Path: currentText, SearchSubmitted: false}
				}
			}

			// Handle clear button
			if r.searchClearBtn.Clicked(gtx) {
				r.searchEditor.SetText("")
				r.lastSearchQuery = ""
				r.searchActive = false
				r.directiveRestored = false
				r.detectedDirectives = nil
				r.searchHistoryVisible = false
				r.searchHistoryItems = nil
				r.lastHistoryQuery = ""
				r.searchEditorFocused = false
				*eventOut = UIEvent{Action: ActionClearSearch}
				gtx.Execute(key.FocusCmd{Tag: keyTag})
			}

			// Only show/fetch history when search box is focused
			if r.searchEditorFocused {
				// Request search history when text changes OR when we haven't fetched yet
				needsHistoryFetch := currentText != r.lastHistoryQuery ||
					(currentText == "" && len(r.searchHistoryItems) == 0)
				if needsHistoryFetch {
					r.lastHistoryQuery = currentText
					r.searchHistoryVisible = true
					*eventOut = UIEvent{Action: ActionRequestSearchHistory, SearchHistoryQuery: currentText}
				}
			} else {
				// Hide history when search box loses focus
				r.searchHistoryVisible = false
			}

			// Handle search history item clicks
			for i := range r.searchHistoryBtns {
				if r.searchHistoryBtns[i].Clicked(gtx) && i < len(r.searchHistoryItems) {
					query := r.searchHistoryItems[i].Query
					r.searchEditor.SetText(query)
					r.lastSearchQuery = query
					r.lastHistoryQuery = query // Prevent re-fetching
					r.searchActive = true
					r.searchHistoryVisible = false
					r.searchHistoryItems = nil
					*eventOut = UIEvent{Action: ActionSearch, Path: query}
					gtx.Execute(key.FocusCmd{Tag: keyTag})
				}
			}

			// Show clear button if there's text OR if we're showing search results
			showClearBtn := r.searchEditor.Text() != "" || state.IsSearchResult

			return r.layoutSearchWithHistory(gtx, hasDirectivePrefix, showClearBtn)
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

// layoutSearchWithHistory renders the search box (dropdown is rendered as overlay in main Layout)
func (r *Renderer) layoutSearchWithHistory(gtx layout.Context, hasDirectivePrefix, showClearBtn bool) layout.Dimensions {
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
	topOffset := gtx.Dp(46) // File button + navbar + spacing
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
		textColor = color.NRGBA{R: 46, G: 125, B: 50, A: 255}  // Dark green
	case "size":
		bgColor = color.NRGBA{R: 227, G: 242, B: 253, A: 255} // Light blue
		textColor = color.NRGBA{R: 21, G: 101, B: 192, A: 255} // Dark blue
	case "modified":
		bgColor = color.NRGBA{R: 252, G: 228, B: 236, A: 255} // Light pink
		textColor = color.NRGBA{R: 173, G: 20, B: 87, A: 255}  // Dark pink
	case "recursive", "depth":
		bgColor = color.NRGBA{R: 255, G: 249, B: 196, A: 255} // Light yellow
		textColor = color.NRGBA{R: 158, G: 118, B: 0, A: 255}  // Dark yellow/gold
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

func (r *Renderer) layoutSidebar(gtx layout.Context, state *State, eventOut *UIEvent) layout.Dimensions {
	// Track sidebar vertical offset for context menus
	sidebarYOffset := gtx.Dp(92)

	// Handle different sidebar layouts
	switch r.sidebarLayout {
	case "stacked":
		return r.layoutSidebarStacked(gtx, state, eventOut, sidebarYOffset)
	case "favorites_only":
		return r.sidebarScroll.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			return r.layoutFavoritesList(gtx, state, eventOut, sidebarYOffset)
		})
	case "drives_only":
		return r.sidebarScroll.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			return r.layoutDrivesList(gtx, state, eventOut)
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
				dims, _ := r.sidebarTabs.LayoutVertical(gtx, r.Theme)
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
			}),
		)
	}

	// Horizontal tabs at top (underline or pill style)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Tabs at top
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims, _ := r.sidebarTabs.Layout(gtx, r.Theme)
			return dims
		}),

		// Horizontal separator
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			height := gtx.Dp(unit.Dp(1))
			paint.FillShape(gtx.Ops, colLightGray, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, height)}.Op())
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
		}),

		// Content area
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
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
		}),
	)
}

// layoutSidebarStacked renders both Favorites and Drives stacked vertically
func (r *Renderer) layoutSidebarStacked(gtx layout.Context, state *State, eventOut *UIEvent, sidebarYOffset int) layout.Dimensions {
	return r.sidebarScroll.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			// Favorites section header
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(4), Left: unit.Dp(8)}.Layout(gtx,
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
				return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						height := gtx.Dp(unit.Dp(1))
						paint.FillShape(gtx.Ops, colLightGray, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, height)}.Op())
						return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
					})
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
}

// layoutFavoritesList renders the favorites list content
func (r *Renderer) layoutFavoritesList(gtx layout.Context, state *State, eventOut *UIEvent, yOffset int) layout.Dimensions {
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
		defer pointer.PassOp{}.Push(gtx.Ops).Pop()
		return r.favState.Layout(gtx, len(state.FavList), func(gtx layout.Context, i int) layout.Dimensions {
			fav := &state.FavList[i]

			// Render and get click states (position unused, we use global mousePos)
			rowDims, leftClicked, rightClicked, _ := r.renderFavoriteRow(gtx, fav)

			// Handle left-click
			if leftClicked {
				*eventOut = UIEvent{Action: ActionNavigate, Path: fav.Path}
			}

			// Handle right-click - use global mouse position for menu
			if rightClicked {
				r.menuVisible, r.menuPos, r.menuPath = true, r.mousePos, fav.Path
				r.menuIsDir, r.menuIsFav = true, true
				r.menuIsBackground = false
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
		defer pointer.PassOp{}.Push(gtx.Ops).Pop()
		return r.driveState.Layout(gtx, len(state.Drives), func(gtx layout.Context, i int) layout.Dimensions {
			drive := &state.Drives[i]

			// Render and get click state
			rowDims, clicked := r.renderDriveRow(gtx, drive)

			// Handle click
			if clicked {
				*eventOut = UIEvent{Action: ActionNavigate, Path: drive.Path}
			}

			return rowDims
		})
	})
}

func (r *Renderer) layoutFileList(gtx layout.Context, state *State, keyTag *layout.List, eventOut *UIEvent) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
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
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return widget.Border{Color: color.NRGBA{A: 50}, Width: unit.Dp(1)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Spacer{Height: unit.Dp(1), Width: unit.Dp(1)}.Layout(gtx)
				})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			// Register background right-click handler FIRST, covering entire area
			// Use PassOp so events pass through to list items on top
			bgArea := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
			passOp := pointer.PassOp{}.Push(gtx.Ops)
			event.Op(gtx.Ops, &r.bgRightClickTag)
			passOp.Pop()
			bgArea.Pop()
			
			// Track if any file row was right-clicked
			fileRightClicked := false
			
			// Use PassOp on the list so events pass through to background handler
			defer pointer.PassOp{}.Push(gtx.Ops).Pop()
			
			// Layout file list (row handlers register on top of background)
			dims := r.listState.Layout(gtx, len(state.Entries), func(gtx layout.Context, i int) layout.Dimensions {
				item := &state.Entries[i]
				isRenaming := r.renameIndex == i
				
				// Render row and get click states (position unused, we use global mousePos) + rename event
				rowDims, leftClicked, rightClicked, _, renameEvt := r.renderRow(gtx, item, i, i == state.SelectedIndex, isRenaming)
				
				// Handle rename event
				if renameEvt != nil {
					*eventOut = *renameEvt
				}
				
				// Handle left-click (but not if renaming)
				if leftClicked && !isRenaming {
					// Cancel any active rename
					r.CancelRename()
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
				
				// Handle right-click on file - use global mouse position for menu
				if rightClicked && !isRenaming {
					// Cancel any active rename
					r.CancelRename()
					fileRightClicked = true
					r.menuVisible, r.menuPos, r.menuPath = true, r.mousePos, item.Path
					r.menuIsDir = item.IsDir
					_, r.menuIsFav = state.Favorites[item.Path]
					r.menuIsBackground = false
					*eventOut = UIEvent{Action: ActionSelect, NewIndex: i}
				}
				
				return rowDims
			})
			
			// Check for background right-click AFTER processing file rows
			// Only show background menu if no file was right-clicked
			if !fileRightClicked {
				for {
					ev, ok := gtx.Event(pointer.Filter{Target: &r.bgRightClickTag, Kinds: pointer.Press})
					if !ok {
						break
					}
					if e, ok := ev.(pointer.Event); ok && e.Buttons.Contain(pointer.ButtonSecondary) {
						// Use global mouse position for menu
						r.menuVisible = true
						r.menuPos = r.mousePos
						r.menuPath = state.CurrentPath
						r.menuIsDir = true
						r.menuIsFav = false
						r.menuIsBackground = true
					}
				}
			}
			
			return dims
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
						gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(16 * time.Millisecond)})
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
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if r.newWindowBtn.Clicked(gtx) {
					r.fileMenuOpen = false
					*eventOut = UIEvent{Action: ActionNewWindow}
				}
				return r.menuItem(gtx, &r.newWindowBtn, "New Window")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					height := gtx.Dp(unit.Dp(1))
					paint.FillShape(gtx.Ops, colLightGray, clip.Rect{Max: image.Pt(gtx.Constraints.Min.X, height)}.Op())
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, height)}
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if r.settingsBtn.Clicked(gtx) {
					r.fileMenuOpen, r.settingsOpen = false, true
				}
				return r.menuItem(gtx, &r.settingsBtn, "Settings")
			}),
		)
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

	if r.openBtn.Clicked(gtx) {
		r.menuVisible = false
		*eventOut = UIEvent{Action: ActionOpen, Path: r.menuPath}
	}
	if r.openWithBtn.Clicked(gtx) {
		r.menuVisible = false
		*eventOut = UIEvent{Action: ActionOpenWith, Path: r.menuPath}
	}
	if r.copyBtn.Clicked(gtx) {
		r.menuVisible = false
		*eventOut = UIEvent{Action: ActionCopy, Path: r.menuPath}
	}
	if r.cutBtn.Clicked(gtx) {
		r.menuVisible = false
		*eventOut = UIEvent{Action: ActionCut, Path: r.menuPath}
	}
	if r.pasteBtn.Clicked(gtx) {
		r.menuVisible = false
		*eventOut = UIEvent{Action: ActionPaste}
	}
	if r.newFileBtn.Clicked(gtx) {
		r.menuVisible = false
		r.ShowCreateDialog(false)
	}
	if r.newFolderBtn.Clicked(gtx) {
		r.menuVisible = false
		r.ShowCreateDialog(true)
	}
	if r.deleteBtn.Clicked(gtx) {
		r.menuVisible = false
		r.deleteConfirmOpen = true
		state.DeleteTarget = r.menuPath
	}
	if r.renameBtn.Clicked(gtx) {
		r.menuVisible = false
		// Start rename for the selected item
		if state.SelectedIndex >= 0 && state.SelectedIndex < len(state.Entries) {
			item := &state.Entries[state.SelectedIndex]
			r.StartRename(state.SelectedIndex, item.Path, item.Name)
		}
	}
	if r.favBtn.Clicked(gtx) {
		r.menuVisible = false
		action := ActionAddFavorite
		if r.menuIsFav {
			action = ActionRemoveFavorite
		}
		*eventOut = UIEvent{Action: action, Path: r.menuPath}
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
							return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								height := gtx.Dp(unit.Dp(1))
								paint.FillShape(gtx.Ops, colLightGray, clip.Rect{Max: image.Pt(gtx.Constraints.Min.X, height)}.Op())
								return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, height)}
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return r.menuItem(gtx, &r.pasteBtn, "Paste")
						}),
					)
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
				return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					height := gtx.Dp(unit.Dp(1))
					paint.FillShape(gtx.Ops, colLightGray, clip.Rect{Max: image.Pt(gtx.Constraints.Min.X, height)}.Op())
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, height)}
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.menuItem(gtx, &r.newFileBtn, "New File")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.menuItem(gtx, &r.newFolderBtn, "New Folder")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					height := gtx.Dp(unit.Dp(1))
					paint.FillShape(gtx.Ops, colLightGray, clip.Rect{Max: image.Pt(gtx.Constraints.Min.X, height)}.Op())
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, height)}
				})
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
				return r.menuItemDanger(gtx, &r.deleteBtn, "Delete")
			}),
		)
	})
}

func (r *Renderer) layoutDeleteConfirm(gtx layout.Context, state *State, eventOut *UIEvent) layout.Dimensions {
	if !r.deleteConfirmOpen {
		return layout.Dimensions{}
	}

	if r.deleteConfirmYes.Clicked(gtx) {
		r.deleteConfirmOpen = false
		*eventOut = UIEvent{Action: ActionConfirmDelete, Path: state.DeleteTarget}
		state.DeleteTarget = ""
	}
	if r.deleteConfirmNo.Clicked(gtx) {
		r.deleteConfirmOpen = false
		state.DeleteTarget = ""
	}

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, color.NRGBA{A: 150}, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return r.menuShell(gtx, 350, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								h6 := material.H6(r.Theme, "Confirm Delete")
								h6.Color = colDanger
								return h6.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								name := filepath.Base(state.DeleteTarget)
								lbl := material.Body1(r.Theme, fmt.Sprintf("Are you sure you want to delete \"%s\"?", name))
								lbl.Color = colBlack
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := material.Body2(r.Theme, "This action cannot be undone.")
								lbl.Color = colGray
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceStart}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										btn := material.Button(r.Theme, &r.deleteConfirmNo, "Cancel")
										btn.Background = colLightGray
										btn.Color = colBlack
										return btn.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										btn := material.Button(r.Theme, &r.deleteConfirmYes, "Delete")
										btn.Background = colDanger
										return btn.Layout(gtx)
									}),
								)
							}),
						)
					})
				})
			})
		}),
	)
}

func (r *Renderer) layoutCreateDialog(gtx layout.Context, state *State, eventOut *UIEvent) layout.Dimensions {
	if !r.createDialogOpen {
		return layout.Dimensions{}
	}

	for {
		evt, ok := r.createDialogEditor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := evt.(widget.SubmitEvent); ok {
			name := strings.TrimSpace(r.createDialogEditor.Text())
			if name != "" {
				r.createDialogOpen = false
				if r.createDialogIsDir {
					*eventOut = UIEvent{Action: ActionCreateFolder, FileName: name}
				} else {
					*eventOut = UIEvent{Action: ActionCreateFile, FileName: name}
				}
			}
		}
	}

	if r.createDialogOK.Clicked(gtx) {
		name := strings.TrimSpace(r.createDialogEditor.Text())
		if name != "" {
			r.createDialogOpen = false
			if r.createDialogIsDir {
				*eventOut = UIEvent{Action: ActionCreateFolder, FileName: name}
			} else {
				*eventOut = UIEvent{Action: ActionCreateFile, FileName: name}
			}
		}
	}
	if r.createDialogCancel.Clicked(gtx) {
		r.createDialogOpen = false
	}

	title := "Create New File"
	placeholder := "filename.txt"
	if r.createDialogIsDir {
		title = "Create New Folder"
		placeholder = "folder name"
	}

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, color.NRGBA{A: 150}, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return material.Clickable(gtx, &r.createDialogCancel, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return r.menuShell(gtx, 350, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								h6 := material.H6(r.Theme, title)
								h6.Color = colBlack
								return h6.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := material.Body2(r.Theme, "Enter name:")
								lbl.Color = colGray
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return widget.Border{Color: colLightGray, Width: unit.Dp(1), CornerRadius: unit.Dp(4)}.Layout(gtx,
									func(gtx layout.Context) layout.Dimensions {
										return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
											func(gtx layout.Context) layout.Dimensions {
												ed := material.Editor(r.Theme, &r.createDialogEditor, placeholder)
												return ed.Layout(gtx)
											})
									})
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceStart}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										btn := material.Button(r.Theme, &r.createDialogCancel, "Cancel")
										btn.Background = colLightGray
										btn.Color = colBlack
										return btn.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										btn := material.Button(r.Theme, &r.createDialogOK, "Create")
										btn.Background = colSuccess
										return btn.Layout(gtx)
									}),
								)
							}),
						)
					})
				})
			})
		}),
	)
}

func (r *Renderer) layoutConflictDialog(gtx layout.Context, state *State, eventOut *UIEvent) layout.Dimensions {
	if !state.Conflict.Active {
		return layout.Dimensions{}
	}

	// Handle button clicks
	applyToAll := r.conflictApplyToAll.Value
	
	if r.conflictReplaceBtn.Clicked(gtx) {
		state.Conflict.Active = false
		state.Conflict.ApplyToAll = applyToAll
		*eventOut = UIEvent{Action: ActionConflictReplace}
	}
	if r.conflictKeepBothBtn.Clicked(gtx) {
		state.Conflict.Active = false
		state.Conflict.ApplyToAll = applyToAll
		*eventOut = UIEvent{Action: ActionConflictKeepBoth}
	}
	if r.conflictSkipBtn.Clicked(gtx) {
		state.Conflict.Active = false
		state.Conflict.ApplyToAll = applyToAll
		*eventOut = UIEvent{Action: ActionConflictSkip}
	}
	if r.conflictStopBtn.Clicked(gtx) {
		state.Conflict.Active = false
		*eventOut = UIEvent{Action: ActionConflictStop}
	}

	// Colors for the dialog
	colWarning := color.NRGBA{R: 255, G: 152, B: 0, A: 255}
	
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, color.NRGBA{A: 180}, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return r.menuShell(gtx, 450, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							// Title
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								itemType := "File"
								if state.Conflict.IsDir {
									itemType = "Folder"
								}
								h6 := material.H6(r.Theme, itemType+" Already Exists")
								h6.Color = colWarning
								return h6.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
							
							// Conflict description
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								name := filepath.Base(state.Conflict.DestPath)
								lbl := material.Body1(r.Theme, fmt.Sprintf("A file named \"%s\" already exists in this location.", name))
								lbl.Color = colBlack
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
							
							// Source file info
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := material.Body2(r.Theme, "Source:")
								lbl.Font.Weight = font.Bold
								lbl.Color = colGray
								return lbl.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								sizeStr := formatSizeForDialog(state.Conflict.SourceSize)
								timeStr := state.Conflict.SourceTime.Format("Jan 2, 2006 3:04 PM")
								lbl := material.Body2(r.Theme, fmt.Sprintf("  %s • %s", sizeStr, timeStr))
								lbl.Color = colBlack
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
							
							// Destination file info
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := material.Body2(r.Theme, "Destination:")
								lbl.Font.Weight = font.Bold
								lbl.Color = colGray
								return lbl.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								sizeStr := formatSizeForDialog(state.Conflict.DestSize)
								timeStr := state.Conflict.DestTime.Format("Jan 2, 2006 3:04 PM")
								lbl := material.Body2(r.Theme, fmt.Sprintf("  %s • %s", sizeStr, timeStr))
								lbl.Color = colBlack
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
							
							// Apply to all checkbox
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								cb := material.CheckBox(r.Theme, &r.conflictApplyToAll, "Apply to all conflicts")
								cb.Color = colBlack
								return cb.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
							
							// Buttons row 1: Replace and Keep Both
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										btn := material.Button(r.Theme, &r.conflictReplaceBtn, "Replace")
										btn.Background = colDanger
										return btn.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										btn := material.Button(r.Theme, &r.conflictKeepBothBtn, "Keep Both")
										btn.Background = colSuccess
										return btn.Layout(gtx)
									}),
								)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
							
							// Buttons row 2: Skip and Stop
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										btn := material.Button(r.Theme, &r.conflictSkipBtn, "Skip")
										btn.Background = colLightGray
										btn.Color = colBlack
										return btn.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										btn := material.Button(r.Theme, &r.conflictStopBtn, "Stop")
										btn.Background = colGray
										return btn.Layout(gtx)
									}),
								)
							}),
						)
					})
				})
			})
		}),
	)
}

func formatSizeForDialog(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d bytes", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
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

	// Check for dark mode toggle
	if r.darkModeCheck.Update(gtx) {
		r.DarkMode = r.darkModeCheck.Value
		r.applyTheme()
		*eventOut = UIEvent{Action: ActionChangeTheme, DarkMode: r.DarkMode}
	}
	
	// Check for search engine selection changes
	if r.searchEngine.Update(gtx) {
		selected := r.searchEngine.Value
		// Only allow selecting available engines
		for _, eng := range r.SearchEngines {
			if eng.ID == selected && eng.Available {
				r.SelectedEngine = selected
				*eventOut = UIEvent{Action: ActionChangeSearchEngine, SearchEngine: selected}
				break
			}
		}
		// Reset to previous if unavailable was clicked
		if r.SelectedEngine != selected {
			r.searchEngine.Value = r.SelectedEngine
		}
	}
	
	// Check for depth changes
	if r.depthDecBtn.Clicked(gtx) && r.DefaultDepth > 1 {
		r.DefaultDepth--
		*eventOut = UIEvent{Action: ActionChangeDefaultDepth, DefaultDepth: r.DefaultDepth}
	}
	if r.depthIncBtn.Clicked(gtx) && r.DefaultDepth < 20 {
		r.DefaultDepth++
		*eventOut = UIEvent{Action: ActionChangeDefaultDepth, DefaultDepth: r.DefaultDepth}
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
				return r.menuShell(gtx, 350, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = gtx.Dp(380)
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
							layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								cb := material.CheckBox(r.Theme, &r.darkModeCheck, "Dark mode")
								cb.Color = colBlack
								return cb.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
							// Default recursive depth setting
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := material.Body1(r.Theme, "Default Recursive Depth:")
								lbl.Color = colBlack
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										btn := material.Button(r.Theme, &r.depthDecBtn, "-")
										btn.Inset = layout.UniformInset(unit.Dp(8))
										if r.DefaultDepth <= 1 {
											btn.Background = colLightGray
											btn.Color = colDisabled
										}
										return btn.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										lbl := material.H6(r.Theme, fmt.Sprintf("%d", r.DefaultDepth))
										lbl.Color = colBlack
										return lbl.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										btn := material.Button(r.Theme, &r.depthIncBtn, "+")
										btn.Inset = layout.UniformInset(unit.Dp(8))
										if r.DefaultDepth >= 20 {
											btn.Background = colLightGray
											btn.Color = colDisabled
										}
										return btn.Layout(gtx)
									}),
								)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := material.Caption(r.Theme, "Used when recursive: has no value")
								lbl.Color = colGray
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := material.Body1(r.Theme, "Content Search Engine:")
								lbl.Color = colBlack
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
							// Render each search engine as a radio button
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return r.layoutSearchEngineOptions(gtx)
							}),
						)
					})
				})
			})
		}),
	)
}

func (r *Renderer) layoutSearchEngineOptions(gtx layout.Context) layout.Dimensions {
	var children []layout.FlexChild
	
	for _, eng := range r.SearchEngines {
		eng := eng // capture loop variable
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.layoutSearchEngineOption(gtx, eng)
		}))
	}
	
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (r *Renderer) layoutSearchEngineOption(gtx layout.Context, eng SearchEngineInfo) layout.Dimensions {
	// Build label with version if available
	label := eng.Name
	if eng.Version != "" && eng.Available {
		label = eng.Name + " (" + eng.Version + ")"
	}
	
	rb := material.RadioButton(r.Theme, &r.searchEngine, eng.ID, label)
	
	if !eng.Available {
		// Grey out unavailable engines
		rb.Color = colDisabled
		rb.IconColor = colDisabled
	} else if r.SelectedEngine == eng.ID {
		rb.Color = colAccent
	} else {
		rb.Color = colBlack
	}
	
	return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return rb.Layout(gtx)
	})
}
