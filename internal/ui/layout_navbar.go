package ui

import (
	"image"
	"image/color"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// Navigation bar layout - back/forward buttons, breadcrumb, search box

func (r *Renderer) layoutNavBar(gtx layout.Context, state *State, keyTag *layout.List, eventOut *UIEvent) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.navButton(gtx, &r.backBtn, "back", state.CanBack, func() { *eventOut = UIEvent{Action: ActionBack} }, keyTag)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.navButton(gtx, &r.fwdBtn, "forward", state.CanForward, func() { *eventOut = UIEvent{Action: ActionForward} }, keyTag)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if r.homeBtn.Clicked(gtx) {
				r.onLeftClick()
				*eventOut = UIEvent{Action: ActionHome}
				gtx.Execute(key.FocusCmd{Tag: keyTag})
			}
			return r.iconButton(gtx, &r.homeBtn, "home", colAccent)
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

			// Parse path into breadcrumb segments (max 6 visible segments)
			r.breadcrumbSegments = parseBreadcrumbSegments(state.CurrentPath, 6)

			// Apply width-based collapsing to prevent overflow
			r.breadcrumbSegments = r.collapseBreadcrumbsForWidth(gtx, r.breadcrumbSegments)

			// Ensure we have enough buttons and click trackers
			for len(r.breadcrumbBtns) < len(r.breadcrumbSegments) {
				r.breadcrumbBtns = append(r.breadcrumbBtns, widget.Clickable{})
				r.breadcrumbLastClicks = append(r.breadcrumbLastClicks, time.Time{})
			}

			// Check for segment clicks - single click navigates, double click edits
			segmentClicked := false
			for i, seg := range r.breadcrumbSegments {
				if !seg.IsEllipsis && r.breadcrumbBtns[i].Clicked(gtx) {
					segmentClicked = true
					now := time.Now()
					if !r.breadcrumbLastClicks[i].IsZero() && now.Sub(r.breadcrumbLastClicks[i]) < doubleClickInterval {
						// Double-click: enter edit mode
						r.isEditing = true
						r.pathEditor.SetText(state.CurrentPath)
						gtx.Execute(key.FocusCmd{Tag: &r.pathEditor})
						r.breadcrumbLastClicks[i] = time.Time{} // Reset
					} else {
						// Single click: navigate
						r.onLeftClick()
						*eventOut = UIEvent{Action: ActionNavigate, Path: seg.Path}
						r.breadcrumbLastClicks[i] = now
					}
				}
			}

			// Single click on breadcrumb area (not on a segment) enters edit mode
			if r.pathClick.Clicked(gtx) && !segmentClicked {
				r.isEditing = true
				r.pathEditor.SetText(state.CurrentPath)
				gtx.Execute(key.FocusCmd{Tag: &r.pathEditor})
			}

			// Build the breadcrumb layout
			return r.layoutBreadcrumb(gtx, state, eventOut)
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

			// Focus the search editor if requested (via Cmd+F or Ctrl+F)
			if r.searchFocusRequested {
				gtx.Execute(key.FocusCmd{Tag: &r.searchEditor})
				r.searchFocusRequested = false // One-shot, clear after use
			}

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
					if !r.searchEditorFocused {
						r.onLeftClick() // Clear menus when search box gains focus
						r.multiSelectMode = false
						*eventOut = UIEvent{Action: ActionClearSelection}
					}
					r.searchEditorFocused = true
				case widget.SelectEvent:
					// Editor has focus when selection changes
					if !r.searchEditorFocused {
						r.onLeftClick() // Clear menus when search box gains focus
						r.multiSelectMode = false
						*eventOut = UIEvent{Action: ActionClearSelection}
					}
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
				r.onLeftClick()
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
			// AND we haven't already set a search event (don't overwrite search-as-you-type)
			if r.searchEditorFocused && eventOut.Action == ActionNone {
				// Request search history when text changes OR when we haven't fetched yet
				needsHistoryFetch := currentText != r.lastHistoryQuery ||
					(currentText == "" && len(r.searchHistoryItems) == 0)
				if needsHistoryFetch {
					r.lastHistoryQuery = currentText
					r.searchHistoryVisible = true
					*eventOut = UIEvent{Action: ActionRequestSearchHistory, SearchHistoryQuery: currentText}
				}
			} else if !r.searchEditorFocused {
				// Hide history when search box loses focus
				r.searchHistoryVisible = false
			}

			// Handle search history item clicks
			for i := range r.searchHistoryBtns {
				if r.searchHistoryBtns[i].Clicked(gtx) && i < len(r.searchHistoryItems) {
					r.onLeftClick()
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

// collapseBreadcrumbsForWidth collapses breadcrumb segments if they exceed available width
func (r *Renderer) collapseBreadcrumbsForWidth(gtx layout.Context, segments []BreadcrumbSegment) []BreadcrumbSegment {
	// gtx.Constraints.Max.X here is already the available width for breadcrumbs
	// (since we're inside a layout.Flexed area that gets remaining space)
	// Only add a small margin to avoid touching the edge
	availableWidth := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(8))
	if availableWidth < gtx.Dp(unit.Dp(100)) {
		availableWidth = gtx.Dp(unit.Dp(100)) // Minimum breadcrumb width
	}

	separatorWidth := gtx.Dp(unit.Dp(20)) // Approximate width of " › "
	ellipsisWidth := gtx.Dp(unit.Dp(25))  // Approximate width of "..."

	// Measure approximate width of each segment (rough estimate based on character count)
	// Using ~7px per character as a rough estimate for Body2 font
	charWidth := gtx.Dp(unit.Dp(7))
	measureSegment := func(name string) int {
		return len(name) * charWidth
	}

	// Calculate total width needed
	totalWidth := 0
	for i, seg := range segments {
		if i > 0 {
			totalWidth += separatorWidth
		}
		if seg.IsEllipsis {
			totalWidth += ellipsisWidth
		} else {
			totalWidth += measureSegment(seg.Name)
		}
	}

	// If we need to collapse further due to width constraints
	// Keep collapsing until we fit (keep first and last segments)
	for totalWidth > availableWidth && len(segments) > 2 {
		// Find if there's already an ellipsis
		hasEllipsis := false
		ellipsisIdx := -1
		for i, seg := range segments {
			if seg.IsEllipsis {
				hasEllipsis = true
				ellipsisIdx = i
				break
			}
		}

		if !hasEllipsis {
			// No ellipsis yet - remove second segment and add ellipsis
			if len(segments) > 2 {
				removedWidth := measureSegment(segments[1].Name) + separatorWidth
				newSegments := make([]BreadcrumbSegment, 0, len(segments))
				newSegments = append(newSegments, segments[0])
				newSegments = append(newSegments, BreadcrumbSegment{Name: "...", IsEllipsis: true})
				newSegments = append(newSegments, segments[2:]...)
				segments = newSegments
				totalWidth = totalWidth - removedWidth + ellipsisWidth + separatorWidth
			}
		} else if ellipsisIdx >= 0 && ellipsisIdx < len(segments)-2 {
			// Ellipsis exists - remove segment after ellipsis (collapse more)
			removeIdx := ellipsisIdx + 1
			if removeIdx < len(segments)-1 { // Don't remove the last segment
				removedWidth := measureSegment(segments[removeIdx].Name) + separatorWidth
				newSegments := make([]BreadcrumbSegment, 0, len(segments)-1)
				newSegments = append(newSegments, segments[:removeIdx]...)
				newSegments = append(newSegments, segments[removeIdx+1:]...)
				segments = newSegments
				totalWidth -= removedWidth
			} else {
				break // Can't collapse further
			}
		} else {
			break // Can't collapse further
		}
	}

	return segments
}

// layoutBreadcrumb renders the clickable path breadcrumb
func (r *Renderer) layoutBreadcrumb(gtx layout.Context, state *State, eventOut *UIEvent) layout.Dimensions {
	// Wrap in pathClick for double-click to edit
	return material.Clickable(gtx, &r.pathClick, func(gtx layout.Context) layout.Dimensions {
		// Use the already-stored segments (click detection uses these too)
		segments := r.breadcrumbSegments

		// Build flex children for the segments
		var children []layout.FlexChild

		for i, seg := range segments {
			idx := i
			segment := seg

			// Add separator before all but first segment
			if i > 0 {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(r.Theme, " › ")
					lbl.Color = colGray
					lbl.MaxLines = 1
					return lbl.Layout(gtx)
				}))
			}

			if segment.IsEllipsis {
				// Ellipsis is not clickable
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(r.Theme, "...")
					lbl.Color = colGray
					lbl.MaxLines = 1
					return lbl.Layout(gtx)
				}))
			} else {
				// Clickable segment
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					// Check if this is the last segment (current directory)
					isLast := idx == len(segments)-1

					return material.Clickable(gtx, &r.breadcrumbBtns[idx], func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body2(r.Theme, segment.Name)
						lbl.MaxLines = 1
						if isLast {
							// Current directory is bold and darker
							lbl.Font.Weight = font.Bold
							lbl.Color = colBlack
						} else {
							// Parent directories are clickable links
							lbl.Color = colAccent
						}
						return lbl.Layout(gtx)
					})
				}))
			}
		}

		// Add search results badge if applicable
		if state.IsSearchResult {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
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
			}))
		}

		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
	})
}

// iconButton renders a clickable icon without background
// iconType: "back", "forward", "home" - draws appropriate icon shape
func (r *Renderer) iconButton(gtx layout.Context, btn *widget.Clickable, iconType string, iconColor color.NRGBA) layout.Dimensions {
	size := gtx.Dp(24)

	return material.Clickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
		// Draw the icon
		r.drawIcon(gtx.Ops, iconType, size, iconColor)
		return layout.Dimensions{Size: image.Pt(size, size)}
	})
}

// drawIcon draws an icon shape at the given size
func (r *Renderer) drawIcon(ops *op.Ops, iconType string, size int, iconColor color.NRGBA) {
	s := float32(size)

	var path clip.Path
	path.Begin(ops)

	switch iconType {
	case "back":
		// Left-pointing chevron/arrow
		path.MoveTo(f32.Pt(s*0.7, s*0.15))
		path.LineTo(f32.Pt(s*0.25, s*0.5))
		path.LineTo(f32.Pt(s*0.7, s*0.85))
		path.LineTo(f32.Pt(s*0.55, s*0.5))
		path.Close()

	case "forward":
		// Right-pointing chevron/arrow
		path.MoveTo(f32.Pt(s*0.3, s*0.15))
		path.LineTo(f32.Pt(s*0.75, s*0.5))
		path.LineTo(f32.Pt(s*0.3, s*0.85))
		path.LineTo(f32.Pt(s*0.45, s*0.5))
		path.Close()

	case "home":
		// House shape: roof + base (scaled down to fit within bounds)
		// Roof (triangle)
		path.MoveTo(f32.Pt(s*0.5, s*0.18))  // Top peak
		path.LineTo(f32.Pt(s*0.15, s*0.48)) // Bottom left
		path.LineTo(f32.Pt(s*0.85, s*0.48)) // Bottom right
		path.Close()
		paint.FillShape(ops, iconColor, clip.Outline{Path: path.End()}.Op())

		// Base (rectangle)
		path.Begin(ops)
		path.MoveTo(f32.Pt(s*0.25, s*0.48))
		path.LineTo(f32.Pt(s*0.25, s*0.82))
		path.LineTo(f32.Pt(s*0.75, s*0.82))
		path.LineTo(f32.Pt(s*0.75, s*0.48))
		path.Close()

	case "chevron-right":
		// Small right-pointing triangle for collapsed directories
		path.MoveTo(f32.Pt(s*0.35, s*0.2))
		path.LineTo(f32.Pt(s*0.75, s*0.5))
		path.LineTo(f32.Pt(s*0.35, s*0.8))
		path.Close()

	case "chevron-down":
		// Small down-pointing triangle for expanded directories
		path.MoveTo(f32.Pt(s*0.2, s*0.35))
		path.LineTo(f32.Pt(s*0.5, s*0.75))
		path.LineTo(f32.Pt(s*0.8, s*0.35))
		path.Close()
	}

	paint.FillShape(ops, iconColor, clip.Outline{Path: path.End()}.Op())
}

func (r *Renderer) navButton(gtx layout.Context, btn *widget.Clickable, iconType string, enabled bool, action func(), keyTag *layout.List) layout.Dimensions {
	if enabled && btn.Clicked(gtx) {
		r.onLeftClick()
		action()
		gtx.Execute(key.FocusCmd{Tag: keyTag})
	}

	iconColor := colAccent
	if !enabled {
		iconColor = colDisabled
	}

	return r.iconButton(gtx, btn, iconType, iconColor)
}
