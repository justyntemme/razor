package ui

import (
	"fmt"
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
	ActionHome
	ActionNewWindow
	ActionSelect
	ActionSearch
	ActionOpen
	ActionAddFavorite
	ActionRemoveFavorite
	ActionSort
	ActionToggleDotfiles
	ActionCopy
	ActionCut
	ActionPaste
	ActionDelete
	ActionConfirmDelete
	ActionCancelDelete
)

type ClipOp int

const (
	ClipCopy ClipOp = iota
	ClipCut
)

type SortColumn int

const (
	SortByName SortColumn = iota
	SortByDate
	SortByType
	SortBySize
)

type Clipboard struct {
	Path string
	Op   ClipOp
}

type UIEvent struct {
	Action        UIAction
	Path          string
	NewIndex      int
	SortColumn    SortColumn
	SortAscending bool
	ShowDotfiles  bool
	ClipOp        ClipOp
}

type UIEntry struct {
	Name, Path    string
	IsDir         bool
	Size          int64
	ModTime       time.Time
	Clickable     widget.Clickable
	RightClickTag int
	LastClick     time.Time
}

type FavoriteItem struct {
	Name, Path    string
	Clickable     widget.Clickable
	RightClickTag int
}

type ProgressState struct {
	Active  bool
	Label   string
	Current int64
	Total   int64
}

type DriveItem struct {
	Name, Path string
	Clickable  widget.Clickable
}

type State struct {
	CurrentPath   string
	Entries       []UIEntry
	SelectedIndex int
	CanBack       bool
	CanForward    bool
	Favorites     map[string]bool
	FavList       []FavoriteItem
	Clipboard     *Clipboard
	Progress      ProgressState
	DeleteTarget  string // Path pending deletion confirmation
	Drives        []DriveItem
}

type Renderer struct {
	Theme                *material.Theme
	listState, favState  layout.List
	driveState           layout.List
	backBtn, fwdBtn      widget.Clickable
	homeBtn              widget.Clickable
	bgClick              widget.Clickable
	focused              bool
	pathEditor           widget.Editor
	pathClick            widget.Clickable
	isEditing            bool
	searchEditor         widget.Editor
	menuVisible          bool
	menuPos              image.Point
	menuPath             string
	menuIsDir, menuIsFav bool
	openBtn, copyBtn     widget.Clickable
	cutBtn, pasteBtn     widget.Clickable
	deleteBtn            widget.Clickable
	favBtn               widget.Clickable
	fileMenuBtn          widget.Clickable
	fileMenuOpen         bool
	newWindowBtn         widget.Clickable
	settingsBtn          widget.Clickable
	settingsOpen         bool
	settingsCloseBtn     widget.Clickable
	searchEngine         widget.Enum
	mousePos             image.Point
	mouseTag             struct{}

	// Delete confirmation
	deleteConfirmOpen    bool
	deleteConfirmYes     widget.Clickable
	deleteConfirmNo      widget.Clickable

	// Column sorting
	headerBtns    [4]widget.Clickable
	SortColumn    SortColumn
	SortAscending bool

	// Settings
	ShowDotfiles      bool
	showDotfilesCheck widget.Bool
}

var (
	colWhite     = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	colBlack     = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	colGray      = color.NRGBA{R: 100, G: 100, B: 100, A: 255}
	colLightGray = color.NRGBA{R: 200, G: 200, B: 200, A: 255}
	colDirBlue   = color.NRGBA{R: 0, G: 0, B: 128, A: 255}
	colSelected  = color.NRGBA{R: 200, G: 220, B: 255, A: 255}
	colSidebar   = color.NRGBA{R: 245, G: 245, B: 245, A: 255}
	colDisabled  = color.NRGBA{R: 150, G: 150, B: 150, A: 255}
	colProgress  = color.NRGBA{R: 66, G: 133, B: 244, A: 255}
	colDanger    = color.NRGBA{R: 220, G: 53, B: 69, A: 255}
	colHomeBtnBg = color.NRGBA{R: 76, G: 175, B: 80, A: 255}  // Green
	colDriveIcon = color.NRGBA{R: 96, G: 125, B: 139, A: 255} // Blue-gray
)

func NewRenderer() *Renderer {
	r := &Renderer{Theme: material.NewTheme(), SortAscending: true}
	r.listState.Axis = layout.Vertical
	r.favState.Axis = layout.Vertical
	r.driveState.Axis = layout.Vertical
	r.pathEditor.SingleLine, r.pathEditor.Submit = true, true
	r.searchEditor.SingleLine, r.searchEditor.Submit = true, true
	r.searchEngine.Value = "default"
	return r
}

func (r *Renderer) SetShowDotfilesCheck(v bool) { r.showDotfilesCheck.Value = v }

func (r *Renderer) detectRightClick(gtx layout.Context, tag event.Tag) bool {
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Press | pointer.Release})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok && e.Kind == pointer.Press && e.Buttons.Contain(pointer.ButtonSecondary) {
			return true
		}
	}
	return false
}

func (r *Renderer) processGlobalInput(gtx layout.Context, state *State) UIEvent {
	event.Op(gtx.Ops, &r.mouseTag)
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &r.mouseTag, Kinds: pointer.Move})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok {
			r.mousePos = image.Pt(int(e.Position.X), int(e.Position.Y))
		}
	}

	for {
		e, ok := gtx.Event(key.Filter{Focus: true, Name: ""})
		if !ok {
			break
		}
		if r.isEditing || r.settingsOpen || r.deleteConfirmOpen {
			continue
		}
		k, ok := e.(key.Event)
		if !ok || k.State != key.Press {
			continue
		}
		switch k.Name {
		case "Up":
			if idx := state.SelectedIndex - 1; idx >= 0 {
				r.listState.ScrollTo(idx)
				return UIEvent{Action: ActionSelect, NewIndex: idx}
			} else if state.SelectedIndex == -1 && len(state.Entries) > 0 {
				idx := len(state.Entries) - 1
				r.listState.ScrollTo(idx)
				return UIEvent{Action: ActionSelect, NewIndex: idx}
			}
		case "Down":
			if idx := state.SelectedIndex + 1; idx < len(state.Entries) {
				r.listState.ScrollTo(idx)
				return UIEvent{Action: ActionSelect, NewIndex: idx}
			} else if state.SelectedIndex == -1 && len(state.Entries) > 0 {
				return UIEvent{Action: ActionSelect, NewIndex: 0}
			}
		case "Left":
			if k.Modifiers.Contain(key.ModAlt) && state.CanBack {
				return UIEvent{Action: ActionBack}
			}
		case "Right":
			if k.Modifiers.Contain(key.ModAlt) && state.CanForward {
				return UIEvent{Action: ActionForward}
			}
		case "Return", "Enter":
			if idx := state.SelectedIndex; idx >= 0 && idx < len(state.Entries) {
				item := state.Entries[idx]
				if item.IsDir {
					return UIEvent{Action: ActionNavigate, Path: item.Path}
				}
				return UIEvent{Action: ActionOpen, Path: item.Path}
			}
		case "C":
			if k.Modifiers.Contain(key.ModCtrl) && state.SelectedIndex >= 0 {
				return UIEvent{Action: ActionCopy, Path: state.Entries[state.SelectedIndex].Path}
			}
		case "X":
			if k.Modifiers.Contain(key.ModCtrl) && state.SelectedIndex >= 0 {
				return UIEvent{Action: ActionCut, Path: state.Entries[state.SelectedIndex].Path}
			}
		case "V":
			if k.Modifiers.Contain(key.ModCtrl) && state.Clipboard != nil {
				return UIEvent{Action: ActionPaste}
			}
		case "⌫", "⌦", "Delete":
			if state.SelectedIndex >= 0 {
				r.deleteConfirmOpen = true
				state.DeleteTarget = state.Entries[state.SelectedIndex].Path
			}
		}
	}
	return UIEvent{}
}

func (r *Renderer) renderColumns(gtx layout.Context) (layout.Dimensions, UIEvent) {
	type colDef struct {
		label string
		col   SortColumn
		flex  float32
		align text.Alignment
	}
	cols := []colDef{
		{"Name", SortByName, 0.5, text.Start},
		{"Date Modified", SortByDate, 0.25, text.Start},
		{"Type", SortByType, 0.15, text.Start},
		{"Size", SortBySize, 0.10, text.End},
	}

	var evt UIEvent
	children := make([]layout.FlexChild, len(cols))

	for i, c := range cols {
		i, c := i, c
		if r.headerBtns[i].Clicked(gtx) {
			if r.SortColumn == c.col {
				r.SortAscending = !r.SortAscending
			} else {
				r.SortColumn, r.SortAscending = c.col, true
			}
			evt = UIEvent{Action: ActionSort, SortColumn: r.SortColumn, SortAscending: r.SortAscending}
		}

		children[i] = layout.Flexed(c.flex, func(gtx layout.Context) layout.Dimensions {
			label := c.label
			if r.SortColumn == c.col {
				if r.SortAscending {
					label += " ▲"
				} else {
					label += " ▼"
				}
			}
			textColor, weight := colGray, font.Normal
			if r.SortColumn == c.col {
				textColor, weight = colBlack, font.Medium
			}
			return material.Clickable(gtx, &r.headerBtns[i], func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(r.Theme, label)
				lbl.Color, lbl.Font.Weight, lbl.Alignment = textColor, weight, c.align
				return lbl.Layout(gtx)
			})
		})
	}

	return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx, children...), evt
}

func (r *Renderer) clickableRow(gtx layout.Context, clk *widget.Clickable, rightTag *int, selected bool, content layout.Widget) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, clk, func(gtx layout.Context) layout.Dimensions {
				return layout.Stack{}.Layout(gtx,
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						if selected {
							paint.FillShape(gtx.Ops, colSelected, clip.Rect{Max: gtx.Constraints.Min}.Op())
						}
						return layout.Dimensions{}
					}),
					layout.Stacked(content),
				)
			})
		}),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
			event.Op(gtx.Ops, rightTag)
			defer pointer.PassOp{}.Push(gtx.Ops).Pop()
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
	)
}

func (r *Renderer) renderRow(gtx layout.Context, item *UIEntry, selected bool) layout.Dimensions {
	return r.clickableRow(gtx, &item.Clickable, &item.RightClickTag, selected, func(gtx layout.Context) layout.Dimensions {
		name, typeStr, sizeStr := item.Name, "File", formatSize(item.Size)
		dateStr := item.ModTime.Format("01/02/06 03:04 PM")
		textColor, weight := colBlack, font.Normal

		if item.IsDir {
			name, typeStr, sizeStr = item.Name+"/", "File Folder", ""
			textColor, weight = colDirBlue, font.Bold
		} else if ext := filepath.Ext(item.Name); len(ext) > 1 {
			typeStr = strings.ToUpper(ext[1:]) + " File"
		}

		return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(0.5, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body1(r.Theme, name)
						lbl.Color, lbl.Font.Weight, lbl.MaxLines = textColor, weight, 1
						return lbl.Layout(gtx)
					}),
					layout.Flexed(0.25, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body2(r.Theme, dateStr)
						lbl.Color, lbl.MaxLines = colGray, 1
						return lbl.Layout(gtx)
					}),
					layout.Flexed(0.15, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body2(r.Theme, typeStr)
						lbl.Color, lbl.MaxLines = colGray, 1
						return lbl.Layout(gtx)
					}),
					layout.Flexed(0.10, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body2(r.Theme, sizeStr)
						lbl.Color, lbl.Alignment, lbl.MaxLines = colGray, text.End, 1
						return lbl.Layout(gtx)
					}),
				)
			})
	})
}

func (r *Renderer) renderFavoriteRow(gtx layout.Context, fav *FavoriteItem) layout.Dimensions {
	return r.clickableRow(gtx, &fav.Clickable, &fav.RightClickTag, false, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body1(r.Theme, fav.Name)
						lbl.Color, lbl.MaxLines = colDirBlue, 1
						return lbl.Layout(gtx)
					}),
				)
			})
	})
}

func (r *Renderer) renderDriveRow(gtx layout.Context, drive *DriveItem) layout.Dimensions {
	return material.Clickable(gtx, &drive.Clickable, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body1(r.Theme, "💾")
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body2(r.Theme, drive.Name)
						lbl.Color, lbl.MaxLines = colDriveIcon, 1
						return lbl.Layout(gtx)
					}),
				)
			})
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