package ui

import (
	"image"
	"image/color"
	"io"
	"path/filepath"
	"strings"
	"time"

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
	"github.com/justyntemme/razor/internal/platform"
)

// Grid view layout for file list

func (r *Renderer) layoutFileGrid(gtx layout.Context, state *State, keyTag *layout.List, eventOut *UIEvent) layout.Dimensions {
	// Track file list size for background click hit-testing
	r.fileListSize = gtx.Constraints.Max

	itemSize := r.gridItemSize
	padding := gtx.Dp(8)
	availWidth := gtx.Constraints.Max.X - padding*2

	// Calculate number of columns that fit
	cols := availWidth / itemSize
	if cols < 1 {
		cols = 1
	}
	r.gridColumns = cols // Store for keyboard navigation

	// Calculate number of rows needed
	numItems := len(state.Entries)
	rows := (numItems + cols - 1) / cols
	if rows < 1 {
		rows = 1
	}

	// Reset visible image paths for thumbnail caching
	r.visibleImagePaths = r.visibleImagePaths[:0]

	// Reset drag hover candidates for this frame
	r.dragHoverCandidates = r.dragHoverCandidates[:0]

	// Track if any item is being dragged
	anyDragging := false

	return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// === BACKGROUND CLICK DETECTION ===
		// Create a hit area filling the entire available space behind the grid.
		// This catches clicks that miss the items (empty space).
		defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
		event.Op(gtx.Ops, &r.bgRightClickTag)

		// Process events for the background tag (both left and right clicks)
		// NOTE: We track if an item was clicked to avoid triggering background actions
		// when clicking on an item (since background is behind items in z-order with PassOp)
		for {
			ev, ok := gtx.Event(pointer.Filter{Target: &r.bgRightClickTag, Kinds: pointer.Press})
			if !ok {
				break
			}
			if e, ok := ev.(pointer.Event); ok && e.Kind == pointer.Press {
				if e.Buttons.Contain(pointer.ButtonSecondary) {
					// Right-click - mark as pending, show menu if no item handles it
					r.bgRightClickPending = true
					r.bgRightClickPos = r.mousePos
				} else if e.Buttons.Contain(pointer.ButtonPrimary) {
					// Left-click - mark as pending, process if no item handles it
					r.bgLeftClickPending = true
				}
			}
		}

		// Track cumulative Y position for row bounds
		cumulativeY := 0

		dims := r.listState.Layout(gtx, rows, func(gtx layout.Context, rowIdx int) layout.Dimensions {
			// Build row of grid items
			var children []layout.FlexChild
			rowHeight := 0

			startIdx := rowIdx * cols
			endIdx := startIdx + cols
			if endIdx > numItems {
				endIdx = numItems
			}

			// Track X position within row
			cumulativeX := 0

			for i := startIdx; i < endIdx; i++ {
				idx := i
				item := &state.Entries[idx]
				itemX := cumulativeX // Capture for closure

				// Track visible images for thumbnail caching
				if !item.IsDir {
					r.trackVisibleImage(item.Path)
				}

				// Track if this item is being dragged
				if item.Touch.Dragging() {
					anyDragging = true
					dragPos := item.Touch.CurrentPos()
					r.dragCurrentX = r.fileListOffset.X + itemX + int(dragPos.X)
					r.dragCurrentY = cumulativeY + int(dragPos.Y)
					r.dragWindowY = r.fileListOffset.Y + cumulativeY + int(dragPos.Y) - r.listState.Position.Offset
					r.dragSourcePath = item.Path
					r.dragSourcePaths = []string{item.Path}
				}

				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					itemDims := r.layoutGridItem(gtx, state, item, idx, itemSize, keyTag, eventOut)
					if itemDims.Size.Y > rowHeight {
						rowHeight = itemDims.Size.Y
					}

					// Track this item as drop target candidate if it's a valid directory
					// For internal drag: check it's not self or parent
					// For external drag: all directories are valid
					isInternalDragCandidate := item.IsDir && r.dragSourcePath != "" &&
						r.dragSourcePath != item.Path &&
						filepath.Dir(r.dragSourcePath) != item.Path
					isExternalDragCandidate := item.IsDir && state.ExternalDragActive

					if isInternalDragCandidate || isExternalDragCandidate {
						r.dragHoverCandidates = append(r.dragHoverCandidates, dragHoverCandidate{
							Path: item.Path,
							MinX: itemX,
							MaxX: itemX + itemSize,
							MinY: cumulativeY,
							MaxY: cumulativeY + itemDims.Size.Y,
						})
					}

					return itemDims
				}))
				cumulativeX += itemSize
			}

			// Pad remaining columns if row isn't full
			for i := endIdx - startIdx; i < cols; i++ {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(itemSize, itemSize+30)} // Icon + label height
				}))
			}

			rowDims := layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
			cumulativeY += rowDims.Size.Y
			return rowDims
		})

		// Clear drag state if nothing is being dragged (internal or external)
		if !anyDragging && !state.ExternalDragActive {
			r.dragSourcePath = ""
			r.dropTargetPath = ""
			platform.SetCurrentDropTarget("")
		} else if anyDragging {
			// Internal drag: find drop target based on cursor position
			r.dropTargetPath = ""
			for _, candidate := range r.dragHoverCandidates {
				if r.dragCurrentX >= candidate.MinX && r.dragCurrentX < candidate.MaxX &&
					r.dragCurrentY >= candidate.MinY && r.dragCurrentY < candidate.MaxY {
					r.dropTargetPath = candidate.Path
					break
				}
			}
			// Request redraw to keep tracking drag position
			gtx.Execute(op.InvalidateCmd{})
		} else if state.ExternalDragActive {
			// External drag: ExternalDragPos is in view coordinates from AppKit
			// The Gio view fills the entire window content area
			// AppKit reports (0,0) at view origin which equals window content origin
			//
			// Candidates are stored with coordinates relative to grid content:
			// - X starts at 0 for first column (relative to inside 8dp padding)
			// - Y starts at 0 for first row (relative to inside 8dp padding)
			//
			// Grid content area starts at:
			// - X: fileListOffset.X + padding (sidebar + divider + padding)
			// - Y: fileListOffset.Y + padding (navbar + padding)
			//
			// So to convert extPos to grid-local:
			// localX = extPos.X - (fileListOffset.X + padding)
			// localY = extPos.Y - (fileListOffset.Y + padding) + scroll
			padding := gtx.Dp(8)

			localX := state.ExternalDragPos.X - r.fileListOffset.X - padding
			localY := state.ExternalDragPos.Y - r.fileListOffset.Y - padding + r.listState.Position.Offset

			debug.Log(debug.UI, "ExtDrag: extPos=(%d,%d) fileListOffset=(%d,%d) padding=%d scroll=%d -> local=(%d,%d) numCandidates=%d",
				state.ExternalDragPos.X, state.ExternalDragPos.Y,
				r.fileListOffset.X, r.fileListOffset.Y,
				padding, r.listState.Position.Offset,
				localX, localY, len(r.dragHoverCandidates))

			r.dropTargetPath = ""
			// Log all candidates once
			if len(r.dragHoverCandidates) > 0 {
				debug.Log(debug.UI, "  All %d candidates:", len(r.dragHoverCandidates))
				for i, c := range r.dragHoverCandidates {
					debug.Log(debug.UI, "    [%d] %s X=[%d,%d] Y=[%d,%d]",
						i, filepath.Base(c.Path), c.MinX, c.MaxX, c.MinY, c.MaxY)
				}
			}
			for _, candidate := range r.dragHoverCandidates {
				if localX >= candidate.MinX && localX < candidate.MaxX &&
					localY >= candidate.MinY && localY < candidate.MaxY {
					r.dropTargetPath = candidate.Path
					platform.SetCurrentDropTarget(candidate.Path)
					debug.Log(debug.UI, "  -> MATCH: %s at local=(%d,%d)", filepath.Base(candidate.Path), localX, localY)
					break
				}
			}
			// Clear target if not over any folder
			if r.dropTargetPath == "" {
				platform.SetCurrentDropTarget("")
			}
			// Request redraw to keep tracking drag position
			gtx.Execute(op.InvalidateCmd{})
		}

		// After processing all items, check if we have pending background clicks
		// that weren't handled by any item (i.e., click was on empty space)
		if r.bgRightClickPending {
			r.menuVisible = true
			r.menuPos = r.bgRightClickPos
			r.menuPath = state.CurrentPath
			r.menuIsDir = true
			r.menuIsFav = false
			r.menuIsBackground = true
			gtx.Execute(op.InvalidateCmd{})
		}
		r.bgRightClickPending = false

		// Handle pending background left-click - dismiss context menu and clear selection
		if r.bgLeftClickPending {
			r.onLeftClick()
			r.multiSelectMode = false
			r.lastClickIndex = -1
			r.lastClickTime = time.Time{}
			if !r.settingsOpen && !r.deleteConfirmOpen && !r.createDialogOpen && !state.Conflict.Active {
				*eventOut = UIEvent{Action: ActionClearSelection}
				gtx.Execute(key.FocusCmd{Tag: keyTag})
			}
		}
		r.bgLeftClickPending = false

		return dims
	})
}

func (r *Renderer) layoutGridItem(gtx layout.Context, state *State, item *UIEntry, idx, itemSize int, keyTag *layout.List, eventOut *UIEvent) layout.Dimensions {
	iconSize := itemSize - 20 // Leave room for padding
	labelHeight := 30

	isSelected := idx == state.SelectedIndex
	if state.SelectedIndices != nil && len(state.SelectedIndices) > 0 {
		isSelected = state.SelectedIndices[idx]
	}

	// Check hover state for this item (works when not dragging)
	isHovered := item.Touch.Hovered()

	// Track if this item is being dragged
	if item.Touch.Dragging() {
		r.dragSourcePath = item.Path
	}

	// Check if this is a valid drop target:
	// - Must be a directory
	// - For internal drag: something must be being dragged, can't drop on self or parent
	// - For external drag: all directories are valid targets
	isValidDropTarget := item.IsDir &&
		(r.dragSourcePath != "" &&
			r.dragSourcePath != item.Path &&
			filepath.Dir(r.dragSourcePath) != item.Path)

	// For external drags, all directories are valid drop targets
	isExternalDropTarget := item.IsDir && state.ExternalDragActive

	// Determine if this item should show as drop target
	// For internal drag: use hover OR dropTargetPath match
	// For external drag: ONLY use dropTargetPath match (hover is stale during external drag)
	var isDropTarget bool
	if state.ExternalDragActive {
		isDropTarget = isExternalDropTarget && r.dropTargetPath == item.Path
	} else {
		isDropTarget = isValidDropTarget && (isHovered || r.dropTargetPath == item.Path)
	}

	totalHeight := iconSize + labelHeight + 10

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
								Paths:  validPaths, // Source paths
								Path:   item.Path,  // Destination directory
							}
						}
					}
				}
			}
		}
	}

	// Layout the touchable widget and handle events
	dims, touchEvt := item.Touch.Layout(gtx,
		// Content widget
		func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{Alignment: layout.Center}.Layout(gtx,
				// Selection/drop target background
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					var bgColor color.NRGBA
					drawBg := false

					if isDropTarget {
						bgColor = colDropTarget
						drawBg = true
					} else if isSelected {
						bgColor = colSelected
						drawBg = true
					}

					if drawBg {
						rr := gtx.Dp(4)
						paint.FillShape(gtx.Ops, bgColor,
							clip.RRect{
								Rect: image.Rect(0, 0, itemSize, totalHeight),
								NE:   rr, NW: rr, SE: rr, SW: rr,
							}.Op(gtx.Ops))
					}
					return layout.Dimensions{Size: image.Pt(itemSize, totalHeight)}
				}),
				// Content
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
						// Icon
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return r.drawGridIcon(gtx, item, iconSize)
							})
						}),
						// Filename label
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							// Use full item width for text
							gtx.Constraints.Max.X = itemSize
							gtx.Constraints.Min.X = itemSize
							return layout.Inset{Top: unit.Dp(4), Left: unit.Dp(2), Right: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								// Show full name, let Gio handle wrapping
								lbl := material.Body2(r.Theme, item.Name)
								lbl.Alignment = text.Middle
								lbl.MaxLines = 2 // Allow 2 lines for longer names
								if item.IsDir {
									lbl.Color = colBlack
								} else {
									lbl.Color = colGray
								}
								return lbl.Layout(gtx)
							})
						}),
					)
				}),
			)
		},
		// Drag appearance - show small icon + name
		func(gtx layout.Context) layout.Dimensions {
			dragHeight := gtx.Dp(36)
			dragWidth := gtx.Dp(160)

			cornerRadius := gtx.Dp(4)
			rr := clip.RRect{
				Rect: image.Rect(0, 0, dragWidth, dragHeight),
				NE:   cornerRadius, NW: cornerRadius, SE: cornerRadius, SW: cornerRadius,
			}
			paint.FillShape(gtx.Ops, color.NRGBA{R: 200, G: 220, B: 255, A: 220}, rr.Op(gtx.Ops))

			return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						// Small icon
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							iconSz := gtx.Dp(24)
							if item.IsDir {
								r.drawFolderIcon(gtx.Ops, iconSz, colAccent, colDirBlue)
							} else {
								ext := strings.ToLower(filepath.Ext(item.Path))
								r.drawFileIcon(gtx.Ops, iconSz, ext)
							}
							return layout.Dimensions{Size: image.Pt(iconSz, iconSz)}
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						// Name
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Body2(r.Theme, item.Name)
							lbl.Color = colBlack
							lbl.MaxLines = 1
							return lbl.Layout(gtx)
						}),
					)
				})
		},
	)

	// Handle touch events
	if touchEvt != nil {
		switch touchEvt.Type {
		case TouchClick:
			r.bgLeftClickPending = false // Cancel background left-click
			r.onLeftClick()
			now := time.Now()

			isDoubleClick := r.lastClickIndex == idx &&
				!r.lastClickTime.IsZero() &&
				now.Sub(r.lastClickTime) < doubleClickInterval

			if isDoubleClick {
				r.multiSelectMode = false
				r.lastClickIndex = -1
				r.lastClickTime = time.Time{}
				if item.IsDir {
					*eventOut = UIEvent{Action: ActionNavigate, Path: item.Path}
				} else {
					*eventOut = UIEvent{Action: ActionOpen, Path: item.Path}
				}
			} else {
				r.multiSelectMode = false
				*eventOut = UIEvent{Action: ActionSelect, NewIndex: idx}
				r.lastClickIndex = idx
				r.lastClickTime = now
			}
			gtx.Execute(key.FocusCmd{Tag: keyTag})
			gtx.Execute(op.InvalidateCmd{})

		case TouchRightClick:
			r.bgRightClickPending = false // Cancel background right-click
			r.menuVisible = true
			r.menuPos = r.mousePos
			r.menuPath = item.Path
			r.menuIsDir = item.IsDir
			_, r.menuIsFav = state.Favorites[item.Path]
			r.menuIsBackground = false
			gtx.Execute(op.InvalidateCmd{})
		}
	}

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

	// Handle drag data request - call Update() AFTER Layout() to process transfer events
	// This receives RequestEvent when a drop happens on a target
	if mime, ok := item.Touch.Update(gtx); ok && mime == FileDragMIME {
		// Send all selected paths (for multi-select drag), separated by newlines
		pathsData := strings.Join(r.dragSourcePaths, "\n")
		item.Touch.Offer(gtx, mime, io.NopCloser(strings.NewReader(pathsData)))
	}

	// Handle drop event
	if dropEvent != nil {
		*eventOut = *dropEvent
	}

	return dims
}

func (r *Renderer) drawGridIcon(gtx layout.Context, item *UIEntry, size int) layout.Dimensions {
	s := float32(size)

	if item.IsDir {
		// Draw folder icon with blue theme (accent blue inner, darker blue outer)
		r.drawFolderIcon(gtx.Ops, size, colAccent, colDirBlue)
	} else {
		// Check if it's an image file - try to show thumbnail
		ext := strings.ToLower(filepath.Ext(item.Path))
		isImage := false
		for _, imgExt := range r.previewImageExts {
			if ext == strings.ToLower(imgExt) {
				isImage = true
				break
			}
		}

		if isImage {
			// Try to get cached thumbnail
			if thumb, _, ok := r.thumbnailCache.Get(item.Path); ok {
				// Constrain the image to the icon size
				gtx.Constraints.Min = image.Pt(size, size)
				gtx.Constraints.Max = image.Pt(size, size)
				widget.Image{
					Src:      thumb,
					Fit:      widget.Contain,
					Position: layout.Center,
				}.Layout(gtx)
				return layout.Dimensions{Size: image.Pt(size, size)}
			}
		}

		// Draw file icon
		r.drawFileIcon(gtx.Ops, size, ext)
	}

	return layout.Dimensions{Size: image.Pt(int(s), int(s))}
}

func (r *Renderer) drawFolderIcon(ops *op.Ops, size int, innerColor, outerColor color.NRGBA) {
	s := float32(size)

	// Material Design style folder - inner color fill, outer color border/tab
	bodyY := int(s * 0.28)
	bodyH := int(s * 0.58)
	bodyW := int(s * 0.76)
	bodyX := int(s * 0.12)

	// Light version of inner color for folder body fill
	lightInner := color.NRGBA{
		R: uint8(min(255, int(innerColor.R)+180)),
		G: uint8(min(255, int(innerColor.G)+180)),
		B: uint8(min(255, int(innerColor.B)+180)),
		A: 255,
	}
	paint.FillShape(ops, lightInner, clip.Rect{
		Min: image.Pt(bodyX, bodyY),
		Max: image.Pt(bodyX+bodyW, bodyY+bodyH),
	}.Op())

	// Folder outline with outer color
	borderW := 2
	// Top
	paint.FillShape(ops, outerColor, clip.Rect{
		Min: image.Pt(bodyX, bodyY),
		Max: image.Pt(bodyX+bodyW, bodyY+borderW),
	}.Op())
	// Bottom
	paint.FillShape(ops, outerColor, clip.Rect{
		Min: image.Pt(bodyX, bodyY+bodyH-borderW),
		Max: image.Pt(bodyX+bodyW, bodyY+bodyH),
	}.Op())
	// Left
	paint.FillShape(ops, outerColor, clip.Rect{
		Min: image.Pt(bodyX, bodyY),
		Max: image.Pt(bodyX+borderW, bodyY+bodyH),
	}.Op())
	// Right
	paint.FillShape(ops, outerColor, clip.Rect{
		Min: image.Pt(bodyX+bodyW-borderW, bodyY),
		Max: image.Pt(bodyX+bodyW, bodyY+bodyH),
	}.Op())

	// Tab on top (solid outer color - the manila tab)
	tabW := int(s * 0.30)
	tabH := int(s * 0.12)
	paint.FillShape(ops, outerColor, clip.Rect{
		Min: image.Pt(bodyX, bodyY-tabH),
		Max: image.Pt(bodyX+tabW, bodyY+borderW),
	}.Op())
}

func (r *Renderer) drawFileIcon(ops *op.Ops, size int, ext string) {
	s := float32(size)

	// Material Design style file icon - clean and light
	// Main file body (centered, slightly smaller)
	fileX := int(s * 0.22)
	fileY := int(s * 0.08)
	fileW := int(s * 0.56)
	fileH := int(s * 0.78)

	// Light accent background (very light blue, Material style)
	lightAccent := color.NRGBA{R: 227, G: 242, B: 253, A: 255} // Light blue background
	paint.FillShape(ops, lightAccent, clip.Rect{
		Min: image.Pt(fileX, fileY),
		Max: image.Pt(fileX+fileW, fileY+fileH),
	}.Op())

	// Accent color border (2px, same as home/nav buttons)
	borderW := 2
	// Top
	paint.FillShape(ops, colAccent, clip.Rect{
		Min: image.Pt(fileX, fileY),
		Max: image.Pt(fileX+fileW, fileY+borderW),
	}.Op())
	// Bottom
	paint.FillShape(ops, colAccent, clip.Rect{
		Min: image.Pt(fileX, fileY+fileH-borderW),
		Max: image.Pt(fileX+fileW, fileY+fileH),
	}.Op())
	// Left
	paint.FillShape(ops, colAccent, clip.Rect{
		Min: image.Pt(fileX, fileY),
		Max: image.Pt(fileX+borderW, fileY+fileH),
	}.Op())
	// Right
	paint.FillShape(ops, colAccent, clip.Rect{
		Min: image.Pt(fileX+fileW-borderW, fileY),
		Max: image.Pt(fileX+fileW, fileY+fileH),
	}.Op())

	// Folded corner indicator (accent colored triangle effect)
	cornerSize := int(s * 0.12)
	paint.FillShape(ops, colAccent, clip.Rect{
		Min: image.Pt(fileX+fileW-cornerSize, fileY),
		Max: image.Pt(fileX+fileW, fileY+cornerSize),
	}.Op())

	// Draw extension label if present
	if ext != "" && len(ext) <= 5 {
		ext = strings.TrimPrefix(ext, ".")
		ext = strings.ToUpper(ext)
		// Draw colored badge for extension (centered, rounded look)
		boxY := int(s * 0.50)
		boxH := int(s * 0.22)
		boxW := int(s * 0.44)
		boxX := int(s*0.5) - boxW/2

		extColor := getExtensionColor(ext)
		paint.FillShape(ops, extColor, clip.Rect{
			Min: image.Pt(boxX, boxY),
			Max: image.Pt(boxX+boxW, boxY+boxH),
		}.Op())
	}
}

func getExtensionColor(ext string) color.NRGBA {
	switch strings.ToLower(ext) {
	case "go":
		return color.NRGBA{R: 0, G: 173, B: 216, A: 255} // Go blue
	case "js", "ts", "jsx", "tsx":
		return color.NRGBA{R: 247, G: 223, B: 30, A: 255} // JavaScript yellow
	case "py":
		return color.NRGBA{R: 55, G: 118, B: 171, A: 255} // Python blue
	case "rs":
		return color.NRGBA{R: 222, G: 165, B: 132, A: 255} // Rust orange
	case "md", "txt":
		return color.NRGBA{R: 100, G: 100, B: 100, A: 255} // Gray
	case "json", "yaml", "yml", "toml":
		return color.NRGBA{R: 130, G: 80, B: 160, A: 255} // Purple
	case "html", "css":
		return color.NRGBA{R: 228, G: 77, B: 38, A: 255} // Orange
	case "png", "jpg", "jpeg", "gif", "webp", "heic":
		return color.NRGBA{R: 76, G: 175, B: 80, A: 255} // Green
	case "pdf":
		return color.NRGBA{R: 244, G: 67, B: 54, A: 255} // Red
	default:
		return colAccent
	}
}

func truncateFilename(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	// Show first part ... last part
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if len(base) > maxLen-3-len(ext) {
		base = base[:maxLen-3-len(ext)]
	}
	return base + "..." + ext
}
