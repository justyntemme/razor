package ui

import (
	"fmt"
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
)

type UIEvent struct {
	Action   UIAction
	Path     string
	NewIndex int
}

type UIEntry struct {
	Name      string
	Path      string
	IsDir     bool
	Size      int64
	ModTime   time.Time
	Clickable widget.Clickable
	LastClick time.Time
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
}

func NewRenderer() *Renderer {
	r := &Renderer{
		Theme: material.NewTheme(),
	}
	r.listState.Axis = layout.Vertical
	r.pathEditor.SingleLine = true
	r.pathEditor.Submit = true
	return r
}

func (r *Renderer) Layout(gtx layout.Context, state *State) UIEvent {
	var eventOut UIEvent

	// 1. Interactive Area Definition
	area := clip.Rect{Max: gtx.Constraints.Max}
	defer area.Push(gtx.Ops).Pop()

	// 2. Global Key Listener on listState
	keyTag := &r.listState
	event.Op(gtx.Ops, keyTag)

	// Auto-Focus
	if !r.focused {
		if r.Debug {
			log.Println("[DEBUG] Executing Initial FocusCmd on listState")
		}
		gtx.Execute(key.FocusCmd{Tag: keyTag})
		r.focused = true
	}

	// 3. Process Global Events
	for {
		e, ok := gtx.Event(
			key.Filter{Focus: true, Name: ""}, 
		)
		if !ok {
			break
		}

		if r.Debug {
			log.Printf("[DEBUG] Global Event: %T %+v", e, e)
		}

		switch e := e.(type) {
		case key.FocusEvent:
			if r.Debug {
				log.Printf("[DEBUG] Focus State: %v", e.Focus)
			}
		case key.Event:
			// If editing, ignore global navigation keys (Editor handles them)
			if r.isEditing {
				continue
			}

			if e.State == key.Press {
				if r.Debug {
					log.Printf("[DEBUG] Processing Key: '%s' (Mod: %v)", e.Name, e.Modifiers)
				}

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

	menuBar := func(gtx layout.Context) layout.Dimensions {
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

			// Editable Path Label / Editor
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if r.isEditing {
					// Handle Editor Events
					for {
						evt, ok := r.pathEditor.Update(gtx)
						if !ok {
							break
						}
						// Check for Submit (Enter Key)
						if s, ok := evt.(widget.SubmitEvent); ok {
							r.isEditing = false
							cleanPath := strings.TrimSpace(s.Text)
							
							if r.Debug {
								log.Printf("[DEBUG] Editor Submitted. Raw: '%s', Clean: '%s'", s.Text, cleanPath)
							}
							
							eventOut = UIEvent{Action: ActionNavigate, Path: cleanPath}
							gtx.Execute(key.FocusCmd{Tag: keyTag}) // Return focus to list
						}
					}
					
					ed := material.Editor(r.Theme, &r.pathEditor, "Enter path...")
					ed.TextSize = unit.Sp(16)
					ed.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
					border := widget.Border{Color: color.NRGBA{R: 0, G: 0, B: 0, A: 255}, Width: unit.Dp(1), CornerRadius: unit.Dp(4)}
					return border.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						// Grab focus for editor if just enabled
						if r.pathClick.Clicked(gtx) { // Safety fallback
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
		)
	}

	list := func(gtx layout.Context) layout.Dimensions {
		if r.Debug {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 240, G: 240, B: 240, A: 255}, clip.Rect{Max: gtx.Constraints.Max}.Op())
		}

		return r.listState.Layout(gtx, len(state.Entries), func(gtx layout.Context, index int) layout.Dimensions {
			item := &state.Entries[index]
			isSelected := index == state.SelectedIndex

			if item.Clickable.Clicked(gtx) {
				if r.Debug {
					log.Printf("[DEBUG] Item %d Clicked", index)
				}
				
				if r.isEditing {
					r.isEditing = false
				}
				
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

			return r.renderRow(gtx, item, isSelected)
		})
	}

	layout.Stack{}.Layout(gtx,
		// Global Background Listener
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if r.bgClick.Clicked(gtx) {
				if r.Debug {
					log.Println("[DEBUG] Background Clicked -> Deselecting")
				}
				
				if r.isEditing {
					r.isEditing = false
				}
				
				eventOut = UIEvent{Action: ActionSelect, NewIndex: -1}
				gtx.Execute(key.FocusCmd{Tag: keyTag})
			}
			
			return r.bgClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			defer pointer.PassOp{}.Push(gtx.Ops).Pop()

			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, menuBar)
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
	)

	return eventOut
}

func (r *Renderer) renderRow(gtx layout.Context, item *UIEntry, selected bool) layout.Dimensions {
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