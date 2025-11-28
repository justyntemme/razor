package ui

import (
	"image/color"
	"log"
	"time"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
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
	Debug     bool // New flag for internal UI debugging
}

func NewRenderer() *Renderer {
	r := &Renderer{
		Theme: material.NewTheme(),
	}
	r.listState.Axis = layout.Vertical
	return r
}

func (r *Renderer) Layout(gtx layout.Context, state *State) UIEvent {
	var eventOut UIEvent

	// 1. Process Global Key Events
	event.Op(gtx.Ops, &r.listState)
	for {
		e, ok := gtx.Event(
			key.Filter{Name: "Up", Optional: key.ModShift},
			key.Filter{Name: "Down", Optional: key.ModShift},
			key.Filter{Name: key.NameReturn},
			key.Filter{Name: key.NameEnter},
			key.Filter{Name: "Left", Optional: key.ModAlt},
			key.Filter{Name: "Right", Optional: key.ModAlt},
		)
		if !ok {
			break
		}

		if k, ok := e.(key.Event); ok && k.State == key.Press {
			// Verbose logging for key events
			if r.Debug {
				log.Printf("[DEBUG] Key Pressed: %s (Mod: %v)", k.Name, k.Modifiers)
			}

			switch k.Name {
			case "Up":
				if state.SelectedIndex > 0 {
					eventOut = UIEvent{Action: ActionSelect, NewIndex: state.SelectedIndex - 1}
				} else if state.SelectedIndex == -1 && len(state.Entries) > 0 {
					eventOut = UIEvent{Action: ActionSelect, NewIndex: len(state.Entries) - 1}
				}
			case "Down":
				if state.SelectedIndex < len(state.Entries)-1 {
					eventOut = UIEvent{Action: ActionSelect, NewIndex: state.SelectedIndex + 1}
				} else if state.SelectedIndex == -1 && len(state.Entries) > 0 {
					eventOut = UIEvent{Action: ActionSelect, NewIndex: 0}
				}
			case "Left":
				if k.Modifiers.Contain(key.ModAlt) && state.CanBack {
					eventOut = UIEvent{Action: ActionBack}
				}
			case "Right":
				if k.Modifiers.Contain(key.ModAlt) && state.CanForward {
					eventOut = UIEvent{Action: ActionForward}
				}
			case key.NameReturn, key.NameEnter:
				if state.SelectedIndex >= 0 && state.SelectedIndex < len(state.Entries) {
					item := state.Entries[state.SelectedIndex]
					if item.IsDir {
						eventOut = UIEvent{Action: ActionNavigate, Path: item.Path}
					}
				}
			}
		}
	}

	// 2. Menu Bar
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
				}
				btn := material.Button(r.Theme, &r.fwdBtn, ">")
				btn.Inset = layout.UniformInset(unit.Dp(10))
				return btn.Layout(btnGtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),

			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				txt := state.CurrentPath
				if txt == "" {
					txt = "Loading..."
				}
				label := material.H6(r.Theme, txt)
				label.Color = color.NRGBA{R: 50, G: 50, B: 50, A: 255}
				label.MaxLines = 1
				return label.Layout(gtx)
			}),
		)
	}

	// 3. List Layout
	list := func(gtx layout.Context) layout.Dimensions {
		return r.listState.Layout(gtx, len(state.Entries), func(gtx layout.Context, index int) layout.Dimensions {
			item := &state.Entries[index]
			isSelected := index == state.SelectedIndex

			if item.Clickable.Clicked(gtx) {
				eventOut = UIEvent{Action: ActionSelect, NewIndex: index}
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

	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, menuBar)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return widget.Border{Color: color.NRGBA{A: 50}, Width: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Spacer{Height: unit.Dp(1), Width: unit.Dp(1)}.Layout(gtx)
			})
		}),
		layout.Flexed(1, list),
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
				txt := item.Name
				textColor := color.NRGBA{R: 0, G: 0, B: 0, A: 255}
				weight := font.Normal

				if item.IsDir {
					txt = item.Name + "/"
					textColor = color.NRGBA{R: 0, G: 0, B: 128, A: 255}
					weight = font.Bold
				}

				lbl := material.Body1(r.Theme, txt)
				lbl.Color = textColor
				lbl.Font.Weight = weight

				return layout.Inset{
					Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12),
				}.Layout(gtx, lbl.Layout)
			}),
		)
	})
}