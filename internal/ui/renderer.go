package ui

import (
	"image"
	"image/color"
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
	ActionSearch
	ActionOpen
	ActionAddFavorite
	ActionRemoveFavorite
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
	RightClickTag int
	LastClick     time.Time
}

type FavoriteItem struct {
	Name          string
	Path          string
	Clickable     widget.Clickable
	RightClickTag int
}

type State struct {
	CurrentPath   string
	Entries       []UIEntry
	SelectedIndex int
	CanBack       bool
	CanForward    bool
	Favorites     map[string]bool
	FavList       []FavoriteItem
}

type Renderer struct {
	Theme     *material.Theme
	listState layout.List
	favState  layout.List
	backBtn   widget.Clickable
	fwdBtn    widget.Clickable

	bgClick widget.Clickable
	focused bool
	Debug   bool

	pathEditor widget.Editor
	pathClick  widget.Clickable
	isEditing  bool

	searchEditor widget.Editor

	menuVisible bool
	menuPos     image.Point
	menuPath    string
	menuIsDir   bool
	menuIsFav   bool

	openBtn widget.Clickable
	copyBtn widget.Clickable
	favBtn  widget.Clickable

	fileMenuBtn  widget.Clickable
	fileMenuOpen bool
	settingsBtn  widget.Clickable

	settingsOpen     bool
	settingsCloseBtn widget.Clickable
	searchEngine     widget.Enum

	mousePos image.Point
	mouseTag struct{}
}

func NewRenderer() *Renderer {
	r := &Renderer{
		Theme: material.NewTheme(),
	}
	r.listState.Axis = layout.Vertical
	r.favState.Axis = layout.Vertical
	r.pathEditor.SingleLine = true
	r.pathEditor.Submit = true

	r.searchEditor.SingleLine = true
	r.searchEditor.Submit = true

	r.searchEngine.Value = "default"

	return r
}

// detectRightClick checks for secondary button presses on a specific tag
func (r *Renderer) detectRightClick(gtx layout.Context, tag event.Tag) bool {
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Press | pointer.Release})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok {
			if e.Kind == pointer.Press && e.Buttons.Contain(pointer.ButtonSecondary) {
				return true
			}
		}
	}
	return false
}

// processGlobalInput handles keyboard shortcuts and global mouse tracking
func (r *Renderer) processGlobalInput(gtx layout.Context, state *State) UIEvent {
	var eventOut UIEvent

	// Mouse Tracking
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

	// Keyboard Shortcuts
	for {
		e, ok := gtx.Event(key.Filter{Focus: true, Name: ""})
		if !ok {
			break
		}
		if r.isEditing || r.settingsOpen {
			continue
		}
		if k, ok := e.(key.Event); ok && k.State == key.Press {
			switch k.Name {
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
				if k.Modifiers.Contain(key.ModAlt) && state.CanBack {
					eventOut = UIEvent{Action: ActionBack}
				}
			case "Right":
				if k.Modifiers.Contain(key.ModAlt) && state.CanForward {
					eventOut = UIEvent{Action: ActionForward}
				}
			case "Return", "Enter":
				if state.SelectedIndex >= 0 && state.SelectedIndex < len(state.Entries) {
					item := state.Entries[state.SelectedIndex]
					if item.IsDir {
						eventOut = UIEvent{Action: ActionNavigate, Path: item.Path}
					} else {
						eventOut = UIEvent{Action: ActionOpen, Path: item.Path}
					}
				}
			}
		}
	}
	return eventOut
}

// --- MAIN LAYOUT ---
func (r *Renderer) renderColumns(gtx layout.Context) layout.Dimensions {
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
			defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
			event.Op(gtx.Ops, &item.RightClickTag)
			defer pointer.PassOp{}.Push(gtx.Ops).Pop()
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
	)
}
