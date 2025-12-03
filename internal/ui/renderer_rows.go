package ui

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"path/filepath"
	"strings"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/justyntemme/razor/internal/debug"
)

// File list row rendering - columns, rows, favorites, drives

func (r *Renderer) renderColumns(gtx layout.Context) (layout.Dimensions, UIEvent) {
	availWidth := gtx.Constraints.Max.X
	headerHeight := gtx.Dp(24)
	handleWidth := gtx.Dp(6)

	// Initialize column widths on first render
	// Account for row insets (12dp left + 12dp right) so columns fit properly
	if !r.columnWidthsInited {
		effectiveWidth := availWidth - gtx.Dp(24) // Subtract row insets
		r.columnWidths = [4]int{
			effectiveWidth * 40 / 100,
			effectiveWidth * 25 / 100,
			effectiveWidth * 15 / 100,
			effectiveWidth * 20 / 100,
		}
		r.colDragActive = -1
		r.columnWidthsInited = true
	}

	// Calculate divider positions (cumulative column widths + handle widths)
	// Divider 0 is after col0, Divider 1 is after col1, Divider 2 is after col2
	dividerX := [3]int{
		r.columnWidths[0],
		r.columnWidths[0] + handleWidth + r.columnWidths[1],
		r.columnWidths[0] + handleWidth + r.columnWidths[1] + handleWidth + r.columnWidths[2],
	}

	// Process drag events using SCREEN coordinates
	// Events are processed on the whole header area
	for i := 0; i < 3; i++ {
		for {
			ev, ok := gtx.Event(pointer.Filter{
				Target: &r.colDragTag[i],
				Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel,
			})
			if !ok {
				break
			}
			if e, ok := ev.(pointer.Event); ok {
				switch e.Kind {
				case pointer.Press:
					if e.Buttons.Contain(pointer.ButtonPrimary) {
						r.colDragActive = i
						r.colDragID = e.PointerID
					}
				case pointer.Drag:
					if r.colDragActive == i && e.PointerID == r.colDragID {
						// Grab pointer for exclusive access
						if e.Priority < pointer.Grabbed {
							gtx.Execute(pointer.GrabCmd{Tag: &r.colDragTag[i], ID: r.colDragID})
						}
						// e.Position.X is relative to the handle's position
						// The handle is at dividerX[i], so absolute screen X = dividerX[i] + e.Position.X
						// But we want the new divider position to be at the mouse cursor
						// New column width = current position of this divider + mouse offset from handle center
						handleCenter := float32(handleWidth) / 2
						newDividerX := dividerX[i] + int(e.Position.X-handleCenter)

						// Calculate new column width based on divider position
						var newWidth int
						switch i {
						case 0:
							newWidth = newDividerX
						case 1:
							newWidth = newDividerX - r.columnWidths[0] - handleWidth
						case 2:
							newWidth = newDividerX - r.columnWidths[0] - handleWidth - r.columnWidths[1] - handleWidth
						}

						// Enforce minimum width
						if newWidth >= 50 {
							r.columnWidths[i] = newWidth
						}
					}
				case pointer.Release, pointer.Cancel:
					if r.colDragActive == i {
						r.colDragActive = -1
					}
				}
			}
		}
	}

	// Recalculate last column to fill remaining space
	// Account for row insets (12dp left + 12dp right) so Size column doesn't get cut off
	rowInsetPx := gtx.Dp(24) // 12dp left + 12dp right inset from renderRowContent
	usedWidth := r.columnWidths[0] + r.columnWidths[1] + r.columnWidths[2] + handleWidth*3
	r.columnWidths[3] = availWidth - usedWidth - rowInsetPx
	if r.columnWidths[3] < 50 {
		r.columnWidths[3] = 50
	}

	type colDef struct {
		label string
		col   SortColumn
		align text.Alignment
	}
	cols := []colDef{
		{"Name", SortByName, text.Start},
		{"Date Modified", SortByDate, text.Start},
		{"Type", SortByType, text.Start},
		{"Size", SortBySize, text.End},
	}

	var evt UIEvent

	// Handle header button clicks for sorting
	for i, c := range cols {
		if r.headerBtns[i].Clicked(gtx) {
			r.onLeftClick()
			if r.SortColumn == c.col {
				r.SortAscending = !r.SortAscending
			} else {
				r.SortColumn, r.SortAscending = c.col, true
			}
			evt = UIEvent{Action: ActionSort, SortColumn: r.SortColumn, SortAscending: r.SortAscending}
		}
	}

	// Helper to render column header
	renderCol := func(idx int) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			width := r.columnWidths[idx]
			label := cols[idx].label
			if r.SortColumn == cols[idx].col {
				if r.SortAscending {
					label += " ▲"
				} else {
					label += " ▼"
				}
			}
			textColor, weight := colGray, font.Normal
			if r.SortColumn == cols[idx].col {
				textColor, weight = colBlack, font.Medium
			}
			gtx.Constraints = layout.Exact(image.Pt(width, headerHeight))
			defer clip.Rect{Max: image.Pt(width, headerHeight)}.Push(gtx.Ops).Pop()
			material.Clickable(gtx, &r.headerBtns[idx], func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(r.Theme, label)
				lbl.Color, lbl.Font.Weight, lbl.Alignment, lbl.MaxLines = textColor, weight, cols[idx].align, 1
				return lbl.Layout(gtx)
			})
			return layout.Dimensions{Size: image.Pt(width, headerHeight)}
		})
	}

	// Helper to render resize handle
	renderHandle := func(idx int) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(handleWidth, headerHeight)
			defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
			event.Op(gtx.Ops, &r.colDragTag[idx])
			pointer.CursorColResize.Add(gtx.Ops)
			// Draw handle indicator when dragging
			if r.colDragActive == idx {
				paint.FillShape(gtx.Ops, color.NRGBA{R: 100, G: 100, B: 100, A: 128},
					clip.Rect{Max: size}.Op())
			}
			return layout.Dimensions{Size: size}
		})
	}

	children := []layout.FlexChild{
		renderCol(0),
		renderHandle(0),
		renderCol(1),
		renderHandle(1),
		renderCol(2),
		renderHandle(2),
		renderCol(3),
	}

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...), evt
}

// renderRow renders a file/folder row. Returns dimensions, left-clicked, right-clicked, shift held, click position, rename event, checkbox toggled, chevron event, and drop event.
func (r *Renderer) renderRow(gtx layout.Context, item *UIEntry, index int, selected bool, isRenaming bool, isChecked bool, showCheckbox bool) (layout.Dimensions, bool, bool, bool, image.Point, *UIEvent, bool, *UIEvent, *UIEvent) {
	// Track click state - will be set from Touchable.Layout
	leftClicked := false
	rightClicked := false
	shiftHeld := false
	var clickPos image.Point

	// Check for checkbox toggle (only if checkboxes are visible)
	checkboxToggled := false
	if showCheckbox && item.Checkbox.Update(gtx) {
		checkboxToggled = true
	}

	// Check for chevron click (expand/collapse directory)
	var chevronEvent *UIEvent
	if item.IsDir && item.ExpandBtn.Clicked(gtx) {
		debug.Log(debug.UI, "Chevron clicked for directory: %s (IsExpanded=%v)", item.Path, item.IsExpanded)
		if item.IsExpanded {
			chevronEvent = &UIEvent{Action: ActionCollapseDir, Path: item.Path}
			debug.Log(debug.UI, "Creating ActionCollapseDir event for: %s", item.Path)
		} else {
			chevronEvent = &UIEvent{Action: ActionExpandDir, Path: item.Path}
			debug.Log(debug.UI, "Creating ActionExpandDir event for: %s", item.Path)
		}
	}

	// Check for rename submission or cancellation
	var renameEvent *UIEvent
	if isRenaming {
		// Handle Enter to submit
		for {
			ev, ok := r.renameEditor.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.SubmitEvent); ok {
				newName := r.renameEditor.Text()
				if newName != "" && newName != item.Name {
					renameEvent = &UIEvent{
						Action:  ActionRename,
						Path:    filepath.Join(filepath.Dir(r.renamePath), newName),
						OldPath: r.renamePath,
					}
				}
				r.CancelRename()
			}
		}

		// Handle Escape to cancel - check focused key events
		for {
			ev, ok := gtx.Event(key.Filter{Focus: true, Name: key.NameEscape})
			if !ok {
				break
			}
			if e, ok := ev.(key.Event); ok && e.State == key.Press {
				r.CancelRename()
			}
		}
	}

	// Determine if row should show as selected
	// In multi-select mode, only use isChecked (from SelectedIndices)
	// In single-select mode, use the primary selection
	showSelected := isChecked

	// Set MIME type for drag operations - must be set before Update() and Layout()
	item.Touch.Type = FileDragMIME

	// Handle drop events for directories (they are drop targets)
	var dropEvent *UIEvent
	if item.IsDir {
		// Check for transfer events on this directory
		for {
			ev, ok := gtx.Event(transfer.TargetFilter{Target: &item.DropTag, Type: FileDragMIME})
			if !ok {
				break
			}
			switch e := ev.(type) {
			case transfer.InitiateEvent:
				// Drag started and this directory is a potential target
				// Check if it's a valid drop target (not the source or its parent)
				if r.dragSourcePath != "" && r.dragSourcePath != item.Path && filepath.Dir(r.dragSourcePath) != item.Path {
					// We can't set dropTargetPath here because InitiateEvent is sent to ALL potential targets
					// We need pointer position to determine which one is actually hovered
				}
			case transfer.DataEvent:
				if e.Type == FileDragMIME {
					// Read the dropped file paths (newline-separated for multi-select)
					reader := e.Open()
					data, err := io.ReadAll(reader)
					reader.Close()
					if err == nil {
						pathsData := string(data)
						sourcePaths := strings.Split(pathsData, "\n")
						// Filter out invalid paths (self or parent of destination)
						validPaths := make([]string, 0, len(sourcePaths))
						for _, p := range sourcePaths {
							if p != "" && p != item.Path && filepath.Dir(p) != item.Path {
								validPaths = append(validPaths, p)
							}
						}
						if len(validPaths) > 0 {
							dropEvent = &UIEvent{
								Action: ActionMove,
								Paths:  validPaths,  // Source paths
								Path:   item.Path,   // Destination directory
							}
						}
					}
				}
			}
		}
	}

	// Layout the row (but not if renaming - we handle clicks differently)
	var dims layout.Dimensions
	cornerRadius := gtx.Dp(4)
	if isRenaming {
		// For renaming, render content first to get proper size, then draw background
		// Use a macro to record the content, measure it, then draw background + content
		macro := op.Record(gtx.Ops)
		contentDims := r.renderRowContent(gtx, item, true, false, false)
		call := macro.Stop()

		// Draw selection background with rounded corners
		rr := clip.RRect{
			Rect: image.Rect(0, 0, contentDims.Size.X, contentDims.Size.Y),
			NE:   cornerRadius, NW: cornerRadius, SE: cornerRadius, SW: cornerRadius,
		}
		paint.FillShape(gtx.Ops, colSelected, rr.Op(gtx.Ops))
		// Replay the content on top
		call.Add(gtx.Ops)

		dims = contentDims
	} else {
		// Use Touchable.Layout for combined click, right-click, and drag handling
		// This allows click/double-click, right-click, AND drag with visual shadow
		var touchEvt *TouchEvent

		// Check hover state for this item (works when not dragging)
		isHovered := item.Touch.Hovered()

		// Track if this item is being dragged
		if item.Touch.Dragging() {
			r.dragSourcePath = item.Path
		}

		// Check if this is a valid drop target:
		// - Must be a directory
		// - Something must be being dragged (dragSourcePath is set)
		// - Can't drop on the item being dragged
		// - Can't drop into parent directory of dragged item (it's already there)
		// For hover detection during drag, we check if this is the current dropTargetPath
		isValidDropTarget := item.IsDir &&
			r.dragSourcePath != "" &&
			r.dragSourcePath != item.Path &&
			filepath.Dir(r.dragSourcePath) != item.Path

		// Determine if this row should show as drop target
		// When not dragging, use regular hover. When dragging, use dropTargetPath match.
		isDropTarget := isValidDropTarget && (isHovered || r.dropTargetPath == item.Path)

		dims, touchEvt = item.Touch.Layout(gtx,
			// Normal content - the row with selection background
			func(gtx layout.Context) layout.Dimensions {
				// First render content to get actual dimensions
				macro := op.Record(gtx.Ops)
				contentDims := r.renderRowContent(gtx, item, false, showCheckbox, isChecked)
				call := macro.Stop()

				// Draw background based on state (priority: selected > drop target > hover)
				var bgColor color.NRGBA
				drawBg := false

				if showSelected {
					bgColor = colSelected
					drawBg = true
				} else if isDropTarget {
					bgColor = colDropTarget
					drawBg = true
				} else if isHovered {
					bgColor = colHover
					drawBg = true
				}

				if drawBg {
					rr := clip.RRect{
						Rect: image.Rect(0, 0, contentDims.Size.X, contentDims.Size.Y),
						NE:   cornerRadius, NW: cornerRadius, SE: cornerRadius, SW: cornerRadius,
					}
					paint.FillShape(gtx.Ops, bgColor, rr.Op(gtx.Ops))
				}

				// Replay the content on top of the background
				call.Add(gtx.Ops)

				return contentDims
			},
			// Drag appearance - simplified visual during drag (the shadow)
			func(gtx layout.Context) layout.Dimensions {
				// Show a semi-transparent version of the item being dragged
				cornerRadius := gtx.Dp(4)
				rr := clip.RRect{
					Rect: image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Dp(32)),
					NE:   cornerRadius, NW: cornerRadius, SE: cornerRadius, SW: cornerRadius,
				}
				paint.FillShape(gtx.Ops, color.NRGBA{R: 200, G: 220, B: 255, A: 200}, rr.Op(gtx.Ops))

				return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12)}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body2(r.Theme, item.Name)
						lbl.Color = colBlack
						lbl.MaxLines = 1
						return lbl.Layout(gtx)
					})
			},
		)

		// Register drop target for directories AFTER Touch.Layout with same dimensions
		// PassOp is critical: without it, the drop target clip area would block pointer
		// events from reaching the underlying Touchable, breaking hover detection.
		// PassOp must wrap the clip area to allow events to "pass through" to lower layers.
		if item.IsDir {
			passStack := pointer.PassOp{}.Push(gtx.Ops)
			stack := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
			event.Op(gtx.Ops, &item.DropTag)
			stack.Pop()
			passStack.Pop()
		}

		// Process touch event from Touchable (handles both left-click and right-click)
		if touchEvt != nil {
			switch touchEvt.Type {
			case TouchClick:
				leftClicked = true
				clickPos = touchEvt.Position
				shiftHeld = touchEvt.Modifiers.Contain(key.ModShift)
			case TouchRightClick:
				rightClicked = true
				clickPos = touchEvt.Position
			}
		}

		// Handle drag data request - call Update() AFTER Layout() to process transfer events
		// This receives RequestEvent when a drop happens on a target
		if mime, ok := item.Touch.Update(gtx); ok && mime == FileDragMIME {
			// Send all selected paths (for multi-select drag), separated by newlines
			pathsData := strings.Join(r.dragSourcePaths, "\n")
			item.Touch.Offer(gtx, mime, io.NopCloser(strings.NewReader(pathsData)))
		}
	}

	return dims, leftClicked, rightClicked, shiftHeld, clickPos, renameEvent, checkboxToggled, chevronEvent, dropEvent
}

// renderRowContent renders the content of a row (shared between normal and rename mode)
func (r *Renderer) renderRowContent(gtx layout.Context, item *UIEntry, isRenaming bool, showCheckbox bool, isChecked bool) layout.Dimensions {
	name, typeStr, sizeStr := item.Name, "File", formatSize(item.Size)
	dateStr := item.ModTime.Format("01/02/06 03:04 PM")
	textColor, weight := colBlack, font.Normal

	if item.IsDir {
		if !isRenaming {
			name = item.Name + "/"
		}
		typeStr, sizeStr = "File Folder", ""
		textColor, weight = colDirBlue, font.Bold
	} else if ext := filepath.Ext(item.Name); len(ext) > 1 {
		typeStr = strings.ToUpper(ext[1:]) + " File"
	}

	// Sync checkbox state with selection state
	item.Checkbox.Value = isChecked

	// Calculate indentation based on depth
	indentWidth := unit.Dp(float32(item.Depth * r.treeIndent))

	return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			var children []layout.FlexChild

			// Add indentation for tree view
			if item.Depth > 0 {
				children = append(children,
					layout.Rigid(layout.Spacer{Width: indentWidth}.Layout),
				)
			}

			// Add chevron for directories (expand/collapse button)
			if item.IsDir {
				children = append(children,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						chevronSize := gtx.Dp(16)
						return material.Clickable(gtx, &item.ExpandBtn, func(gtx layout.Context) layout.Dimensions {
							iconType := "chevron-right"
							if item.IsExpanded {
								iconType = "chevron-down"
							}
							r.drawIcon(gtx.Ops, iconType, chevronSize, colGray)
							return layout.Dimensions{Size: image.Pt(chevronSize, chevronSize)}
						})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
					// Small divider between chevron and directory name
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						dividerHeight := gtx.Dp(unit.Dp(12))
						dividerWidth := gtx.Dp(unit.Dp(1))
						// Center the divider vertically
						return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								size := image.Pt(dividerWidth, dividerHeight)
								paint.FillShape(gtx.Ops, color.NRGBA{R: 200, G: 200, B: 200, A: 180},
									clip.Rect{Max: size}.Op())
								return layout.Dimensions{Size: size}
							})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				)
			} else {
				// For files, add spacer to align with directories that have chevrons + divider
				children = append(children,
					layout.Rigid(layout.Spacer{Width: unit.Dp(27)}.Layout), // chevron width + spacing + divider + spacing
				)
			}

			// Add checkbox if in multi-select mode
			if showCheckbox {
				children = append(children,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						cb := material.CheckBox(r.Theme, &item.Checkbox, "")
						cb.Size = unit.Dp(14) // Smaller checkbox
						if isChecked {
							cb.Color = colAccent
							cb.IconColor = colAccent
						} else {
							cb.Color = colGray
							cb.IconColor = colGray
						}
						return cb.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					// Divider between checkbox and filename
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						size := image.Pt(gtx.Dp(unit.Dp(1)), gtx.Dp(unit.Dp(16)))
						paint.FillShape(gtx.Ops, color.NRGBA{R: 200, G: 200, B: 200, A: 255},
							clip.Rect{Max: size}.Op())
						return layout.Dimensions{Size: size}
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				)
			}

			// Use stored column widths (in pixels) for consistent alignment with header
			// Add 6dp spacers between columns to match the resize handle widths in the header
			colWidths := r.columnWidths

			// Name column
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = colWidths[0]
					gtx.Constraints.Max.X = colWidths[0]
					if isRenaming {
						// Show editor for renaming
						gtx.Execute(key.FocusCmd{Tag: &r.renameEditor})
						ed := material.Editor(r.Theme, &r.renameEditor, "")
						ed.TextSize = unit.Sp(14)
						ed.Color = textColor
						ed.Font.Weight = weight
						return widget.Border{Color: colAccent, Width: unit.Dp(1)}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, ed.Layout)
							})
					}
					lbl := material.Body1(r.Theme, name)
					lbl.Color, lbl.Font.Weight, lbl.MaxLines = textColor, weight, 1
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout), // Match header resize handle width
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = colWidths[1]
					gtx.Constraints.Max.X = colWidths[1]
					lbl := material.Body2(r.Theme, dateStr)
					lbl.Color, lbl.MaxLines = colGray, 1
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout), // Match header resize handle width
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = colWidths[2]
					gtx.Constraints.Max.X = colWidths[2]
					lbl := material.Body2(r.Theme, typeStr)
					lbl.Color, lbl.MaxLines = colGray, 1
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout), // Match header resize handle width
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = colWidths[3]
					gtx.Constraints.Max.X = colWidths[3]
					lbl := material.Body2(r.Theme, sizeStr)
					lbl.Color, lbl.Alignment, lbl.MaxLines = colGray, text.End, 1
					return lbl.Layout(gtx)
				}),
			)

			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
		})
}

// renderFavoriteRow renders a favorite item. Returns dimensions, left-clicked, right-clicked, click position, and drop event.
func (r *Renderer) renderFavoriteRow(gtx layout.Context, fav *FavoriteItem, isDropHover bool) (layout.Dimensions, bool, bool, image.Point, *UIEvent) {
	// Check for left-click BEFORE layout
	leftClicked := fav.Clickable.Clicked(gtx)

	// Check for right-click and capture position
	rightClicked := false
	var clickPos image.Point
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &fav.RightClickTag, Kinds: pointer.Press})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok && e.Buttons.Contain(pointer.ButtonSecondary) {
			rightClicked = true
			clickPos = image.Pt(int(e.Position.X), int(e.Position.Y))
		}
	}

	// Handle drop events (favorites are drop targets)
	var dropEvent *UIEvent
	for {
		ev, ok := gtx.Event(transfer.TargetFilter{Target: &fav.DropTag, Type: FileDragMIME})
		if !ok {
			break
		}
		if e, ok := ev.(transfer.DataEvent); ok && e.Type == FileDragMIME {
			reader := e.Open()
			data, _ := io.ReadAll(reader)
			reader.Close()
			pathsData := string(data)
			sourcePaths := strings.Split(pathsData, "\n")
			// Filter out invalid paths
			validPaths := make([]string, 0, len(sourcePaths))
			for _, p := range sourcePaths {
				if p != "" && p != fav.Path && filepath.Dir(p) != fav.Path {
					validPaths = append(validPaths, p)
				}
			}
			if len(validPaths) > 0 {
				if fav.Type == FavoriteTypeTrash {
					// Dropping on trash = delete (move to trash)
					dropEvent = &UIEvent{
						Action: ActionConfirmDelete,
						Paths:  validPaths,
					}
				} else {
					// Dropping on favorite folder = move
					dropEvent = &UIEvent{
						Action: ActionMove,
						Paths:  validPaths,
						Path:   fav.Path,
					}
				}
			}
		}
	}

	dims := material.Clickable(gtx, &fav.Clickable, func(gtx layout.Context) layout.Dimensions {
		// Register right-click handler
		defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
		event.Op(gtx.Ops, &fav.RightClickTag)

		// Register as drop target with PassOp so clicks still work
		passStack := pointer.PassOp{}.Push(gtx.Ops)
		event.Op(gtx.Ops, &fav.DropTag)
		passStack.Pop()

		// Highlight if viewing trash or if being hovered during drag
		showHighlight := (fav.Type == FavoriteTypeTrash && r.isTrashView) || isDropHover
		if showHighlight {
			cornerRadius := gtx.Dp(4)
			rr := clip.RRect{
				Rect: image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Constraints.Max.Y),
				NE:   cornerRadius, NW: cornerRadius, SE: cornerRadius, SW: cornerRadius,
			}
			bgColor := colSelected
			if isDropHover {
				bgColor = colDropTarget
			}
			paint.FillShape(gtx.Ops, bgColor, rr.Op(gtx.Ops))
		}

		return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					// Icon for trash
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if fav.Type == FavoriteTypeTrash {
							size := gtx.Dp(14)
							r.drawTrashIcon(gtx.Ops, size, colGray)
							return layout.Dimensions{Size: image.Pt(size, size)}
						}
						return layout.Dimensions{}
					}),
					// Spacer between icon and text (only for trash)
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if fav.Type == FavoriteTypeTrash {
							return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx)
						}
						return layout.Dimensions{}
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body1(r.Theme, fav.Name)
						lbl.Color, lbl.MaxLines = colDirBlue, 1
						return lbl.Layout(gtx)
					}),
				)
			})
	})

	return dims, leftClicked, rightClicked, clickPos, dropEvent
}

// drawTrashIcon draws a simple trash can icon
func (r *Renderer) drawTrashIcon(ops *op.Ops, size int, iconColor color.NRGBA) {
	s := float32(size)

	// Trash can body (rectangle)
	bodyTop := int(s * 0.3)
	bodyLeft := int(s * 0.15)
	bodyRight := int(s * 0.85)
	bodyBottom := int(s * 0.95)
	paint.FillShape(ops, iconColor, clip.Rect{
		Min: image.Pt(bodyLeft, bodyTop),
		Max: image.Pt(bodyRight, bodyBottom),
	}.Op())

	// Lid (horizontal line at top)
	lidTop := int(s * 0.15)
	lidBottom := int(s * 0.25)
	paint.FillShape(ops, iconColor, clip.Rect{
		Min: image.Pt(int(s*0.1), lidTop),
		Max: image.Pt(int(s*0.9), lidBottom),
	}.Op())

	// Handle on lid
	handleLeft := int(s * 0.35)
	handleRight := int(s * 0.65)
	handleTop := int(s * 0.05)
	handleBottom := int(s * 0.15)
	paint.FillShape(ops, iconColor, clip.Rect{
		Min: image.Pt(handleLeft, handleTop),
		Max: image.Pt(handleRight, handleBottom),
	}.Op())
}

func (r *Renderer) renderDriveRow(gtx layout.Context, drive *DriveItem) (layout.Dimensions, bool) {
	// Check for click BEFORE layout
	clicked := drive.Clickable.Clicked(gtx)

	dims := material.Clickable(gtx, &drive.Clickable, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(r.Theme, drive.Name)
				lbl.Color, lbl.MaxLines = colDriveIcon, 1
				return lbl.Layout(gtx)
			})
	})

	return dims, clicked
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// truncatePathMiddle truncates a path to maxLen characters, showing start.../end
// The end portion (current directory) is prioritized to always be visible
func truncatePathMiddle(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}

	// Find the last path separator to get the current directory
	lastSep := strings.LastIndex(path, string(filepath.Separator))
	if lastSep <= 0 || lastSep >= len(path)-1 {
		// No separator or at start/end, just truncate end
		return path[:maxLen-3] + "..."
	}

	// Get the end part (current directory with separator)
	endPart := path[lastSep:]

	// Calculate space for start
	// Reserve: 3 for "...", endPart length
	startLen := maxLen - 3 - len(endPart)

	if startLen < 5 {
		// Not enough room for meaningful start, just show end
		if len(endPart) > maxLen-3 {
			return "..." + endPart[len(endPart)-(maxLen-3):]
		}
		return "..." + endPart
	}

	return path[:startLen] + "..." + endPart
}
