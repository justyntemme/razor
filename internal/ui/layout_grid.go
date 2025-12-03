package ui

import (
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
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

	// Calculate number of rows needed
	numItems := len(state.Entries)
	rows := (numItems + cols - 1) / cols
	if rows < 1 {
		rows = 1
	}

	// Reset visible image paths for thumbnail caching
	r.visibleImagePaths = r.visibleImagePaths[:0]

	return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return r.listState.Layout(gtx, rows, func(gtx layout.Context, rowIdx int) layout.Dimensions {
			// Build row of grid items
			var children []layout.FlexChild

			startIdx := rowIdx * cols
			endIdx := startIdx + cols
			if endIdx > numItems {
				endIdx = numItems
			}

			for i := startIdx; i < endIdx; i++ {
				idx := i
				item := &state.Entries[idx]

				// Track visible images for thumbnail caching
				if !item.IsDir {
					r.trackVisibleImage(item.Path)
				}

				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.layoutGridItem(gtx, state, item, idx, itemSize, keyTag, eventOut)
				}))
			}

			// Pad remaining columns if row isn't full
			for i := endIdx - startIdx; i < cols; i++ {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(itemSize, itemSize+30)} // Icon + label height
				}))
			}

			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
		})
	})
}

func (r *Renderer) layoutGridItem(gtx layout.Context, state *State, item *UIEntry, idx, itemSize int, keyTag *layout.List, eventOut *UIEvent) layout.Dimensions {
	iconSize := itemSize - 20 // Leave room for padding
	labelHeight := 30

	isSelected := idx == state.SelectedIndex
	if state.SelectedIndices != nil && len(state.SelectedIndices) > 0 {
		isSelected = state.SelectedIndices[idx]
	}

	totalHeight := iconSize + labelHeight + 10

	// Layout the touchable widget and handle events
	dims, touchEvt := item.Touch.Layout(gtx,
		// Content widget
		func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{Alignment: layout.Center}.Layout(gtx,
				// Selection background
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					if isSelected {
						rr := gtx.Dp(4)
						paint.FillShape(gtx.Ops, colSelected,
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
							gtx.Constraints.Max.X = itemSize - 4
							gtx.Constraints.Min.X = itemSize - 4
							return layout.Inset{Top: unit.Dp(4), Left: unit.Dp(2), Right: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								lbl := material.Body2(r.Theme, truncateFilename(item.Name, 15))
								lbl.Alignment = text.Middle
								lbl.MaxLines = 2
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
		// Drag widget (nil for now - no drag support in grid view yet)
		nil,
	)

	// Handle touch events
	if touchEvt != nil {
		switch touchEvt.Type {
		case TouchClick:
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

		case TouchRightClick:
			r.menuVisible = true
			r.menuPos = r.mousePos
			r.menuPath = item.Path
			r.menuIsDir = item.IsDir
			_, r.menuIsFav = state.Favorites[item.Path]
			r.menuIsBackground = false
		}
	}

	return dims
}

func (r *Renderer) drawGridIcon(gtx layout.Context, item *UIEntry, size int) layout.Dimensions {
	s := float32(size)

	if item.IsDir {
		// Draw folder icon
		r.drawFolderIcon(gtx.Ops, size, colDirBlue)
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
				// Draw thumbnail
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

func (r *Renderer) drawFolderIcon(ops *op.Ops, size int, iconColor color.NRGBA) {
	s := float32(size)

	// Use a simple rectangle for the folder body - Gio-friendly
	bodyY := int(s * 0.3)
	bodyH := int(s * 0.55)
	bodyW := int(s * 0.8)
	bodyX := int(s * 0.1)
	paint.FillShape(ops, iconColor, clip.Rect{
		Min: image.Pt(bodyX, bodyY),
		Max: image.Pt(bodyX+bodyW, bodyY+bodyH),
	}.Op())

	// Tab on top
	tabW := int(s * 0.25)
	tabH := int(s * 0.1)
	paint.FillShape(ops, iconColor, clip.Rect{
		Min: image.Pt(bodyX, bodyY-tabH),
		Max: image.Pt(bodyX+tabW, bodyY),
	}.Op())
}

func (r *Renderer) drawFileIcon(ops *op.Ops, size int, ext string) {
	s := float32(size)

	// Simple file icon using rectangles (Gio-friendly)
	// Main file body
	fileX := int(s * 0.2)
	fileY := int(s * 0.1)
	fileW := int(s * 0.6)
	fileH := int(s * 0.8)

	// White background
	paint.FillShape(ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{
		Min: image.Pt(fileX, fileY),
		Max: image.Pt(fileX+fileW, fileY+fileH),
	}.Op())

	// Border (draw as 4 thin rectangles)
	borderColor := colGray
	borderW := 1
	// Top
	paint.FillShape(ops, borderColor, clip.Rect{
		Min: image.Pt(fileX, fileY),
		Max: image.Pt(fileX+fileW, fileY+borderW),
	}.Op())
	// Bottom
	paint.FillShape(ops, borderColor, clip.Rect{
		Min: image.Pt(fileX, fileY+fileH-borderW),
		Max: image.Pt(fileX+fileW, fileY+fileH),
	}.Op())
	// Left
	paint.FillShape(ops, borderColor, clip.Rect{
		Min: image.Pt(fileX, fileY),
		Max: image.Pt(fileX+borderW, fileY+fileH),
	}.Op())
	// Right
	paint.FillShape(ops, borderColor, clip.Rect{
		Min: image.Pt(fileX+fileW-borderW, fileY),
		Max: image.Pt(fileX+fileW, fileY+fileH),
	}.Op())

	// Folded corner indicator (small gray square in top-right)
	cornerSize := int(s * 0.15)
	paint.FillShape(ops, colLightGray, clip.Rect{
		Min: image.Pt(fileX+fileW-cornerSize, fileY),
		Max: image.Pt(fileX+fileW, fileY+cornerSize),
	}.Op())

	// Draw extension label if present
	if ext != "" && len(ext) <= 5 {
		ext = strings.TrimPrefix(ext, ".")
		ext = strings.ToUpper(ext)
		// Draw colored box for extension
		boxY := int(s * 0.55)
		boxH := int(s * 0.25)
		boxW := int(s * 0.5)
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
