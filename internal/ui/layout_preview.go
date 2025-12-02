package ui

import (
	"image"
	"path/filepath"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/justyntemme/razor/internal/debug"
)

// File preview pane - images, text, markdown

// layoutPreviewPane renders the file preview pane
func (r *Renderer) layoutPreviewPane(gtx layout.Context, state *State) layout.Dimensions {
	if !r.previewVisible {
		return layout.Dimensions{}
	}

	// Handle close button click
	if r.previewCloseBtn.Clicked(gtx) {
		r.onLeftClick()
		r.HidePreview()
	}

	// Background
	paint.FillShape(gtx.Ops, colWhite, clip.Rect{Max: gtx.Constraints.Max}.Op())

	// Handle markdown toggle click
	if r.previewMdToggleBtn.Clicked(gtx) {
		r.onLeftClick()
		r.previewMarkdownRender = !r.previewMarkdownRender
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Header with filename, toggle (for markdown), and close button
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(8)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							filename := filepath.Base(r.previewPath)
							lbl := material.Body1(r.Theme, filename)
							lbl.Font.Weight = font.Bold
							lbl.MaxLines = 1
							return lbl.Layout(gtx)
						}),
						// Markdown Raw/Preview toggle (only for markdown files)
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if !r.previewIsMarkdown {
								return layout.Dimensions{}
							}
							return material.Clickable(gtx, &r.previewMdToggleBtn, func(gtx layout.Context) layout.Dimensions {
								label := "Raw"
								bgColor := colLightGray
								if r.previewMarkdownRender {
									label = "Preview"
									bgColor = colAccent
								}
								return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx,
									func(gtx layout.Context) layout.Dimensions {
										// Button background
										return layout.Stack{}.Layout(gtx,
											layout.Expanded(func(gtx layout.Context) layout.Dimensions {
												rr := gtx.Dp(4)
												paint.FillShape(gtx.Ops, bgColor, clip.RRect{
													Rect: image.Rect(0, 0, gtx.Constraints.Min.X, gtx.Constraints.Min.Y),
													NE:   rr, NW: rr, SE: rr, SW: rr,
												}.Op(gtx.Ops))
												return layout.Dimensions{Size: gtx.Constraints.Min}
											}),
											layout.Stacked(func(gtx layout.Context) layout.Dimensions {
												return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx,
													func(gtx layout.Context) layout.Dimensions {
														lbl := material.Body2(r.Theme, label)
														if r.previewMarkdownRender {
															lbl.Color = colWhite
														}
														return lbl.Layout(gtx)
													})
											}),
										)
									})
							})
						}),
						// Spacer between toggle and close button
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if !r.previewIsMarkdown {
								return layout.Dimensions{}
							}
							return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx)
						}),
						// Close button
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return material.Clickable(gtx, &r.previewCloseBtn, func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(4), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx,
									func(gtx layout.Context) layout.Dimensions {
										lbl := material.Body1(r.Theme, "✕")
										lbl.Color = colGray
										return lbl.Layout(gtx)
									})
							})
						}),
					)
				})
		}),
		// Divider
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, colLightGray, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(1))}.Op())
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(1))}
		}),
		// Error message if any
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if r.previewError == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(r.Theme, r.previewError)
					lbl.Color = colDanger
					return lbl.Layout(gtx)
				})
		}),
		// Content area
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			debug.Log(debug.UI, "layoutPreviewPane content: previewIsImage=%v, previewVisible=%v", r.previewIsImage, r.previewVisible)
			if r.previewIsImage {
				return r.layoutImagePreview(gtx)
			}
			// Render markdown if enabled and this is a markdown file
			if r.previewIsMarkdown && r.previewMarkdownRender {
				return r.layoutMarkdownPreview(gtx)
			}
			return r.layoutTextPreview(gtx)
		}),
	)
}

// layoutImagePreview renders an image in the preview pane
func (r *Renderer) layoutImagePreview(gtx layout.Context) layout.Dimensions {
	debug.Log(debug.UI, "layoutImagePreview: previewImageSize=%v", r.previewImageSize)
	if r.previewImageSize.X == 0 || r.previewImageSize.Y == 0 {
		debug.Log(debug.UI, "layoutImagePreview: returning empty (zero size)")
		return layout.Dimensions{}
	}

	return layout.Inset{Top: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12), Bottom: unit.Dp(8)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			// Calculate scale to fit image within available space while maintaining aspect ratio
			availWidth := float32(gtx.Constraints.Max.X)
			availHeight := float32(gtx.Constraints.Max.Y)
			imgWidth := float32(r.previewImageSize.X)
			imgHeight := float32(r.previewImageSize.Y)
			debug.Log(debug.UI, "layoutImagePreview: avail=(%v,%v) img=(%v,%v)", availWidth, availHeight, imgWidth, imgHeight)

			// Calculate scale factor to fit
			scaleX := availWidth / imgWidth
			scaleY := availHeight / imgHeight
			scale := scaleX
			if scaleY < scaleX {
				scale = scaleY
			}
			// Don't scale up, only down
			if scale > 1 {
				scale = 1
			}

			// Calculate final dimensions
			finalWidth := int(imgWidth * scale)
			finalHeight := int(imgHeight * scale)

			// Center the image
			offsetX := (int(availWidth) - finalWidth) / 2
			offsetY := (int(availHeight) - finalHeight) / 2
			if offsetX < 0 {
				offsetX = 0
			}
			if offsetY < 0 {
				offsetY = 0
			}

			// Use widget.Image for proper scaling
			img := widget.Image{
				Src:   r.previewImage,
				Fit:   widget.Contain,
				Scale: 1.0 / scale, // Inverse because Scale is pixels per dp
			}

			// Constrain to calculated size and center
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(image.Pt(finalWidth, finalHeight))
				return img.Layout(gtx)
			})
		})
}

// layoutTextPreview renders text content in the preview pane
func (r *Renderer) layoutTextPreview(gtx layout.Context) layout.Dimensions {
	if r.previewContent == "" {
		return layout.Dimensions{}
	}

	// Split content into lines for scrollable rendering
	lines := strings.Split(r.previewContent, "\n")

	return layout.Inset{Top: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12), Bottom: unit.Dp(8)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return r.previewScroll.Layout(gtx, len(lines), func(gtx layout.Context, i int) layout.Dimensions {
				line := lines[i]
				if line == "" {
					line = " " // Preserve empty lines
				}

				lbl := material.Body2(r.Theme, line)
				lbl.Font.Typeface = "monospace"
				lbl.TextSize = unit.Sp(12)

				// Syntax coloring for JSON
				if r.previewIsJSON {
					// Color keys vs values (simple heuristic)
					trimmed := strings.TrimSpace(line)
					if strings.HasPrefix(trimmed, "\"") && strings.Contains(line, ":") {
						lbl.Color = colAccent // Keys in blue
					}
				}

				return lbl.Layout(gtx)
			})
		})
}

// layoutMarkdownPreview renders parsed markdown content
func (r *Renderer) layoutMarkdownPreview(gtx layout.Context) layout.Dimensions {
	if len(r.previewMarkdownBlocks) == 0 {
		return layout.Dimensions{}
	}

	return layout.Inset{Top: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12), Bottom: unit.Dp(8)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return r.previewScroll.Layout(gtx, len(r.previewMarkdownBlocks), func(gtx layout.Context, i int) layout.Dimensions {
				return r.LayoutMarkdownBlock(gtx, r.previewMarkdownBlocks[i])
			})
		})
}
