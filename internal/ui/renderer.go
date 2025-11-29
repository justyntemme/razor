package ui

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"path/filepath"
	"strings"
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

type UIAction int

const (
	ActionNone UIAction = iota
	ActionNavigate
	ActionBack
	ActionForward
	ActionSelect
	ActionSearch // Added
)

type UIEvent struct {
	Action   UIAction
	Path     string
	NewIndex int
}

type UIEntry struct {
	Name          string
	Path          string
	IsDir         bool
	Size          int64
	ModTime       time.Time
	Clickable     widget.Clickable
	RightClickTag struct{}
	LastClick     time.Time
}

type State struct {
	CurrentPath   string
	Entries       []UIEntry
	SelectedIndex int
	CanBack       bool
	CanForward    bool
}

type Renderer struct {
	Theme     *material.Theme
	listState layout.List
	backBtn   widget.Clickable
	fwdBtn    widget.Clickable
	
	bgClick   widget.Clickable
	focused   bool
	Debug     bool

	// Editable Path Fields
	pathEditor widget.Editor
	pathClick  widget.Clickable
	isEditing  bool

	// Search Box
	searchEditor widget.Editor

	// Context Menu
	menuVisible bool
	menuPos     image.Point
	menuIndex   int
	copyBtn     widget.Clickable
	
	// Top File Menu
	fileMenuBtn  widget.Clickable
	fileMenuOpen bool
	settingsBtn  widget.Clickable

	// Settings Modal
	settingsOpen     bool
	settingsCloseBtn widget.Clickable
	searchEngine     widget.Enum
	
	// Global Mouse Tracking
	mousePos image.Point 
	mouseTag struct{}
}

func NewRenderer() *Renderer {
	r := &Renderer{
		Theme: material.NewTheme(),
	}
	r.listState.Axis = layout.Vertical
	r.pathEditor.SingleLine = true
	r.pathEditor.Submit = true
	
	r.searchEditor.SingleLine = true
	r.searchEditor.Submit = true
	
	r.searchEngine.Value = "default" 
	
	return r
}

func (r *Renderer) Layout(gtx layout.Context, state *State) UIEvent {
	var eventOut UIEvent

	// 1. Interactive Area Definition
	area := clip.Rect{Max: gtx.Constraints.Max}
	defer area.Push(gtx.Ops).Pop()

	// 2. Global Key Listener
	keyTag := &r.listState
	event.Op(gtx.Ops, keyTag)

	if !r.focused {
		if r.Debug {
			log.Println("[DEBUG] Executing Initial FocusCmd on listState")
		}
		gtx.Execute(key.FocusCmd{Tag: keyTag})
		r.focused = true
	}

	// 3. Global Mouse Tracking
	mouseTag := &r.mouseTag
	event.Op(gtx.Ops, mouseTag)
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: mouseTag, Kinds: pointer.Move})
		if !ok {
			break
		}
		if x, ok := ev.(pointer.Event); ok {
			r.mousePos = image.Point{X: int(x.Position.X), Y: int(x.Position.Y)}
		}
	}

	// 4. Global Key Events
	for {
		e, ok := gtx.Event(key.Filter{Focus: true, Name: ""})
		if !ok {
			break
		}
		if r.Debug {
			log.Printf("[DEBUG] Global Event: %T %+v", e, e)
		}
		switch e := e.(type) {
		case key.Event:
			if r.isEditing || r.settingsOpen {
				continue
			}
			if e.State == key.Press {
				switch e.Name {
				case "Up":
					newIndex := -1
					if state.SelectedIndex > 0 {
						newIndex = state.SelectedIndex - 1
					} else if state.SelectedIndex == -1 && len(state.Entries) > 0 {
						newIndex = len(state.Entries) - 1
					}
					if newIndex != -1 {
						eventOut = UIEvent{Action: ActionSelect, NewIndex: newIndex}
						r.listState.ScrollTo(newIndex)
					}
				case "Down":
					newIndex := -1
					if state.SelectedIndex < len(state.Entries)-1 {
						newIndex = state.SelectedIndex + 1
					} else if state.SelectedIndex == -1 && len(state.Entries) > 0 {
						newIndex = 0
					}
					if newIndex != -1 {
						eventOut = UIEvent{Action: ActionSelect, NewIndex: newIndex}
						r.listState.ScrollTo(newIndex)
					}
				case "Left":
					if e.Modifiers.Contain(key.ModAlt) && state.CanBack {
						eventOut = UIEvent{Action: ActionBack}
					}
				case "Right":
					if e.Modifiers.Contain(key.ModAlt) && state.CanForward {
						eventOut = UIEvent{Action: ActionForward}
					}
				case "Return", "Enter":
					if state.SelectedIndex >= 0 && state.SelectedIndex < len(state.Entries) {
						item := state.Entries[state.SelectedIndex]
						if item.IsDir {
							eventOut = UIEvent{Action: ActionNavigate, Path: item.Path}
						}
					}
				}
			}
		}
	}

	// --- LAYOUT COMPONENTS ---

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
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
			Spacing:   layout.SpaceEnd,
		}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				btnGtx := gtx
				if !state.CanBack {
					btnGtx = gtx.Disabled()
				}
				if r.backBtn.Clicked(btnGtx) {
					eventOut = UIEvent{Action: ActionBack}
					gtx.Execute(key.FocusCmd{Tag: keyTag})
				}
				btn := material.Button(r.Theme, &r.backBtn, "<")
				btn.Inset = layout.UniformInset(unit.Dp(10))
				return btn.Layout(btnGtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				btnGtx := gtx
				if !state.CanForward {
					btnGtx = gtx.Disabled()
				}
				if r.fwdBtn.Clicked(btnGtx) {
					eventOut = UIEvent{Action: ActionForward}
					gtx.Execute(key.FocusCmd{Tag: keyTag})
				}
				btn := material.Button(r.Theme, &r.fwdBtn, ">")
				btn.Inset = layout.UniformInset(unit.Dp(10))
				return btn.Layout(btnGtx)
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
							cleanPath := strings.TrimSpace(s.Text)
							eventOut = UIEvent{Action: ActionNavigate, Path: cleanPath}
							gtx.Execute(key.FocusCmd{Tag: keyTag})
						}
					}
					ed := material.Editor(r.Theme, &r.pathEditor, "Enter path...")
					ed.TextSize = unit.Sp(16)
					ed.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
					border := widget.Border{Color: color.NRGBA{R: 0, G: 0, B: 0, A: 255}, Width: unit.Dp(1), CornerRadius: unit.Dp(4)}
					return border.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						if r.pathClick.Clicked(gtx) { 
							gtx.Execute(key.FocusCmd{Tag: &r.pathEditor})
						}
						return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, ed.Layout)
					})
				} else {
					if r.pathClick.Clicked(gtx) {
						r.isEditing = true
						r.pathEditor.SetText(state.CurrentPath)
						gtx.Execute(key.FocusCmd{Tag: &r.pathEditor})
					}
					return material.Clickable(gtx, &r.pathClick, func(gtx layout.Context) layout.Dimensions {
						txt := state.CurrentPath
						if txt == "" {
							txt = "Loading..."
						}
						label := material.H6(r.Theme, txt)
						label.Color = color.NRGBA{R: 50, G: 50, B: 50, A: 255}
						label.MaxLines = 1
						return label.Layout(gtx)
					})
				}
			}),

			layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),

			// Search Box (Rigid Width)
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(unit.Dp(200))
				gtx.Constraints.Max.X = gtx.Dp(unit.Dp(200))
				
				// Handle Search Submit
				for {
					evt, ok := r.searchEditor.Update(gtx)
					if !ok {
						break
					}
					if s, ok := evt.(widget.SubmitEvent); ok {
						if r.Debug {
							log.Printf("[DEBUG] Search Submitted: %s", s.Text)
						}
						eventOut = UIEvent{Action: ActionSearch, Path: s.Text}
						gtx.Execute(key.FocusCmd{Tag: keyTag}) // Return focus to list
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

	columnHeader := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
			layout.Flexed(0.5, func(gtx layout.Context) layout.Dimensions {
				return material.Body2(r.Theme, "Name").Layout(gtx)
			}),
			layout.Flexed(0.25, func(gtx layout.Context) layout.Dimensions {
				return material.Body2(r.Theme, "Date Modified").Layout(gtx)
			}),
			layout.Flexed(0.15, func(gtx layout.Context) layout.Dimensions {
				return material.Body2(r.Theme, "Type").Layout(gtx)
			}),
			layout.Flexed(0.10, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(r.Theme, "Size")
				lbl.Alignment = text.End
				return lbl.Layout(gtx)
			}),
		)
	}

	list := func(gtx layout.Context) layout.Dimensions {
		defer pointer.PassOp{}.Push(gtx.Ops).Pop()
		if r.Debug {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 240, G: 240, B: 240, A: 255}, clip.Rect{Max: gtx.Constraints.Max}.Op())
		}
		return r.listState.Layout(gtx, len(state.Entries), func(gtx layout.Context, index int) layout.Dimensions {
			item := &state.Entries[index]
			isSelected := index == state.SelectedIndex

			if item.Clickable.Clicked(gtx) {
				if r.isEditing { r.isEditing = false }
				if r.fileMenuOpen { r.fileMenuOpen = false }
				
				eventOut = UIEvent{Action: ActionSelect, NewIndex: index}
				gtx.Execute(key.FocusCmd{Tag: keyTag})

				now := time.Now()
				if !item.LastClick.IsZero() && now.Sub(item.LastClick) < 500*time.Millisecond {
					if item.IsDir {
						eventOut = UIEvent{Action: ActionNavigate, Path: item.Path}
					}
				}
				item.LastClick = now
			}

			rightClickTag := &item.RightClickTag
			event.Op(gtx.Ops, rightClickTag) 
			for {
				ev, ok := gtx.Event(pointer.Filter{Target: rightClickTag, Kinds: pointer.Press | pointer.Release})
				if !ok { break }
				if e, ok := ev.(pointer.Event); ok {
					if e.Kind == pointer.Press && e.Buttons.Contain(pointer.ButtonSecondary) {
						r.menuVisible = true
						r.menuPos = r.mousePos
						r.menuIndex = index
						eventOut = UIEvent{Action: ActionSelect, NewIndex: index}
					}
				}
			}
			return r.renderRow(gtx, item, isSelected)
		})
	}

	// --- ROOT STACK ---
	layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if r.bgClick.Clicked(gtx) {
				if r.Debug { log.Println("[DEBUG] Background Clicked") }
				if r.isEditing { r.isEditing = false }
				if r.menuVisible { r.menuVisible = false }
				if r.fileMenuOpen { r.fileMenuOpen = false }
				
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
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, columnHeader)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return widget.Border{Color: color.NRGBA{A: 50}, Width: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Spacer{Height: unit.Dp(1), Width: unit.Dp(1)}.Layout(gtx)
					})
				}),
				layout.Flexed(1, list),
			)
		}),

		// File Menu Overlay
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if !r.fileMenuOpen {
				return layout.Dimensions{}
			}
			offset := op.Offset(image.Point{X: 8, Y: 30}).Push(gtx.Ops)
			defer offset.Pop()

			menuSize := image.Pt(120, 40)
			return widget.Border{Color: color.NRGBA{R: 200, G: 200, B: 200, A: 255}, Width: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Stack{}.Layout(gtx,
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: menuSize}.Op())
						return layout.Dimensions{Size: menuSize}
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						if r.settingsBtn.Clicked(gtx) {
							r.fileMenuOpen = false
							r.settingsOpen = true
						}
						return material.Clickable(gtx, &r.settingsBtn, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min = menuSize
							return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								txt := material.Body2(r.Theme, "Settings")
								txt.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
								return txt.Layout(gtx)
							})
						})
					}),
				)
			})
		}),

		// Context Menu Overlay
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if !r.menuVisible {
				return layout.Dimensions{}
			}
			
			offset := op.Offset(r.menuPos).Push(gtx.Ops)
			defer offset.Pop()

			if r.copyBtn.Clicked(gtx) {
				r.menuVisible = false
			}

			menuSize := image.Pt(150, 40)
			return widget.Border{Color: color.NRGBA{R: 200, G: 200, B: 200, A: 255}, Width: unit.Dp(1), CornerRadius: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Stack{}.Layout(gtx,
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: menuSize}.Op())
						return layout.Dimensions{Size: gtx.Constraints.Min}
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						return material.Clickable(gtx, &r.copyBtn, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min = menuSize
							gtx.Constraints.Max = menuSize
							return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								txt := material.Body2(r.Theme, "Copy (noop)")
								txt.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
								return txt.Layout(gtx)
							})
						})
					}),
				)
			})
		}),

		// Settings Modal Overlay
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if !r.settingsOpen {
				return layout.Dimensions{}
			}

			if r.settingsCloseBtn.Clicked(gtx) {
				r.settingsOpen = false
			}
			
			layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					paint.FillShape(gtx.Ops, color.NRGBA{A: 150}, clip.Rect{Max: gtx.Constraints.Max}.Op())
					return material.Clickable(gtx, &r.settingsCloseBtn, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: gtx.Constraints.Max}
					})
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return widget.Border{Color: color.NRGBA{A: 100}, Width: unit.Dp(1), CornerRadius: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Stack{}.Layout(gtx,
								layout.Expanded(func(gtx layout.Context) layout.Dimensions {
									paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: gtx.Constraints.Min}.Op())
									return layout.Dimensions{Size: gtx.Constraints.Min}
								}),
								layout.Stacked(func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.X = gtx.Dp(300)
									gtx.Constraints.Min.Y = gtx.Dp(200)
									return layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												h6 := material.H6(r.Theme, "Settings")
												h6.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
												return h6.Layout(gtx)
											}),
											layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												lbl := material.Body1(r.Theme, "Search Engine:")
												lbl.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
												return lbl.Layout(gtx)
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return material.RadioButton(r.Theme, &r.searchEngine, "default", "Default").Layout(gtx)
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return material.RadioButton(r.Theme, &r.searchEngine, "ripgrep", "ripgrep").Layout(gtx)
											}),
										)
									})
								}),
							)
						})
					})
				}),
			)
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
	)

	return eventOut
}

func (r *Renderer) renderRow(gtx layout.Context, item *UIEntry, selected bool) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &item.Clickable, func(gtx layout.Context) layout.Dimensions {
				return layout.Stack{}.Layout(gtx,
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						if selected {
							paint.FillShape(gtx.Ops, color.NRGBA{R: 200, G: 220, B: 255, A: 255}, clip.Rect{Max: gtx.Constraints.Min}.Op())
						}
						return layout.Dimensions{}
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						name := item.Name
						typeStr := "File"
						sizeStr := formatSize(item.Size)
						dateStr := item.ModTime.Format("01/02/06 03:04 PM")
						textColor := color.NRGBA{R: 0, G: 0, B: 0, A: 255}
						weight := font.Normal

						if item.IsDir {
							name = item.Name + "/"
							typeStr = "File Folder"
							sizeStr = ""
							textColor = color.NRGBA{R: 0, G: 0, B: 128, A: 255}
							weight = font.Bold
						} else {
							ext := filepath.Ext(item.Name)
							if len(ext) > 1 {
								typeStr = strings.ToUpper(ext[1:]) + " File"
							}
						}

						return layout.Inset{
							Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12),
						}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween, Alignment: layout.Middle}.Layout(gtx,
								layout.Flexed(0.5, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Body1(r.Theme, name)
									lbl.Color = textColor
									lbl.Font.Weight = weight
									lbl.MaxLines = 1
									return lbl.Layout(gtx)
								}),
								layout.Flexed(0.25, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Body2(r.Theme, dateStr)
									lbl.Color = color.NRGBA{R: 100, G: 100, B: 100, A: 255}
									lbl.MaxLines = 1
									return lbl.Layout(gtx)
								}),
								layout.Flexed(0.15, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Body2(r.Theme, typeStr)
									lbl.Color = color.NRGBA{R: 100, G: 100, B: 100, A: 255}
									lbl.MaxLines = 1
									return lbl.Layout(gtx)
								}),
								layout.Flexed(0.10, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Body2(r.Theme, sizeStr)
									lbl.Color = color.NRGBA{R: 100, G: 100, B: 100, A: 255}
									lbl.Alignment = text.End
									lbl.MaxLines = 1
									return lbl.Layout(gtx)
								}),
							)
						})
					}),
				)
			})
		}),

		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			event.Op(gtx.Ops, &item.RightClickTag)
			clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
			defer pointer.PassOp{}.Push(gtx.Ops).Pop()
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
	)
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