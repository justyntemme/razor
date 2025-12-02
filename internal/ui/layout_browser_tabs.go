package ui

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget/material"
)

// Browser tab bar layout

// layoutBrowserTabBar renders the browser tab bar with tabs and close buttons
func (r *Renderer) layoutBrowserTabBar(gtx layout.Context, state *State, eventOut *UIEvent) layout.Dimensions {
	if !r.tabsEnabled || len(r.browserTabs) == 0 {
		return layout.Dimensions{}
	}

	tabHeight := gtx.Dp(32)
	tabMinWidth := gtx.Dp(120)
	tabMaxWidth := gtx.Dp(200)
	closeSize := gtx.Dp(16)

	// Background
	bgRect := clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, tabHeight)}
	paint.FillShape(gtx.Ops, colSidebar, bgRect.Op())

	// Process tab clicks and close button clicks
	for i := range r.browserTabs {
		if r.browserTabs[i].tabBtn.Clicked(gtx) {
			if i != r.activeTabIndex {
				*eventOut = UIEvent{Action: ActionSwitchTab, TabIndex: i}
			}
		}
		if r.browserTabs[i].closeBtn.Clicked(gtx) {
			*eventOut = UIEvent{Action: ActionCloseTab, TabIndex: i}
		}
	}

	// Layout tabs horizontally
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, r.browserTabChildren(gtx, tabHeight, tabMinWidth, tabMaxWidth, closeSize)...)
		}),
	)
}

// browserTabChildren creates flex children for each browser tab
func (r *Renderer) browserTabChildren(gtx layout.Context, tabHeight, tabMinWidth, tabMaxWidth, closeSize int) []layout.FlexChild {
	children := make([]layout.FlexChild, len(r.browserTabs))
	closeBtnWidth := gtx.Dp(28) // Width for close button area including padding
	textPadding := gtx.Dp(16)   // Padding around text

	for i := range r.browserTabs {
		idx := i
		tab := &r.browserTabs[i]
		isActive := idx == r.activeTabIndex

		children[idx] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// Tab background
			bgColor := colSidebar
			if isActive {
				bgColor = colWhite
			}

			title := tab.Title
			if title == "" {
				title = "New Tab"
			}

			// Measure text width to size tab appropriately
			lbl := material.Body2(r.Theme, title)
			if isActive {
				lbl.Font.Weight = 600
			}
			macro := op.Record(gtx.Ops)
			dims := lbl.Layout(gtx)
			macro.Stop()

			// Calculate tab width: text + padding + close button area
			tabWidth := dims.Size.X + textPadding + closeBtnWidth
			if tabWidth < tabMinWidth {
				tabWidth = tabMinWidth
			}
			if tabWidth > tabMaxWidth {
				tabWidth = tabMaxWidth
			}

			// Width available for centered text (total width minus close button area)
			textAreaWidth := tabWidth - closeBtnWidth

			return material.Clickable(gtx, &tab.tabBtn, func(gtx layout.Context) layout.Dimensions {
				// Draw background
				rect := clip.Rect{Max: image.Pt(tabWidth, tabHeight)}
				paint.FillShape(gtx.Ops, bgColor, rect.Op())

				// Draw bottom border for inactive tabs
				if !isActive {
					stack := op.Offset(image.Pt(0, tabHeight-1)).Push(gtx.Ops)
					borderRect := clip.Rect{Max: image.Pt(tabWidth, 1)}
					paint.FillShape(gtx.Ops, colLightGray, borderRect.Op())
					stack.Pop()
				}

				// Draw right border between tabs
				stack := op.Offset(image.Pt(tabWidth-1, 0)).Push(gtx.Ops)
				borderRect := clip.Rect{Max: image.Pt(1, tabHeight)}
				paint.FillShape(gtx.Ops, colLightGray, borderRect.Op())
				stack.Pop()

				// Draw divider line before close button
				dividerX := tabWidth - closeBtnWidth
				dividerPadding := gtx.Dp(6)
				divStack := op.Offset(image.Pt(dividerX, dividerPadding)).Push(gtx.Ops)
				divRect := clip.Rect{Max: image.Pt(1, tabHeight-dividerPadding*2)}
				divColor := colLightGray
				if !isActive {
					divColor = color.NRGBA{R: 200, G: 200, B: 200, A: 255}
				}
				paint.FillShape(gtx.Ops, divColor, divRect.Op())
				divStack.Pop()

				// Draw centered title in the text area
				lbl := material.Body2(r.Theme, title)
				lbl.Color = colBlack
				if isActive {
					lbl.Font.Weight = 600
				}

				// Center text horizontally and vertically in text area
				textX := (textAreaWidth - dims.Size.X) / 2
				if textX < gtx.Dp(4) {
					textX = gtx.Dp(4) // Minimum left padding
				}
				textY := (tabHeight - dims.Size.Y) / 2
				textStack := op.Offset(image.Pt(textX, textY)).Push(gtx.Ops)
				lbl.Layout(gtx)
				textStack.Pop()

				// Draw close button at the right side, vertically centered
				closeBtnX := tabWidth - closeBtnWidth + (closeBtnWidth-closeSize)/2
				closeBtnY := (tabHeight - closeSize) / 2
				closeStack := op.Offset(image.Pt(closeBtnX, closeBtnY)).Push(gtx.Ops)
				material.Clickable(gtx, &tab.closeBtn, func(gtx layout.Context) layout.Dimensions {
					// Draw X
					centerX := float32(closeSize) / 2
					centerY := float32(closeSize) / 2
					armLen := float32(closeSize) / 4

					xColor := colGray
					// Draw X lines
					var p clip.Path
					p.Begin(gtx.Ops)
					p.MoveTo(f32.Pt(centerX-armLen, centerY-armLen))
					p.LineTo(f32.Pt(centerX+armLen, centerY+armLen))
					p.MoveTo(f32.Pt(centerX+armLen, centerY-armLen))
					p.LineTo(f32.Pt(centerX-armLen, centerY+armLen))
					paint.FillShape(gtx.Ops, xColor, clip.Stroke{Path: p.End(), Width: 1.5}.Op())

					return layout.Dimensions{Size: image.Pt(closeSize, closeSize)}
				})
				closeStack.Pop()

				return layout.Dimensions{Size: image.Pt(tabWidth, tabHeight)}
			})
		})
	}

	return children
}
