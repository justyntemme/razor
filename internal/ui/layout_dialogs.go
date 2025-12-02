package ui

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/justyntemme/razor/internal/trash"
)

// Confirmation and input dialogs

func (r *Renderer) layoutDeleteConfirm(gtx layout.Context, state *State, eventOut *UIEvent) layout.Dimensions {
	if !r.deleteConfirmOpen {
		return layout.Dimensions{}
	}

	if r.deleteConfirmYes.Clicked(gtx) {
		r.onLeftClick()
		r.deleteConfirmOpen = false
		*eventOut = UIEvent{Action: ActionConfirmDelete, Paths: state.DeleteTargets}
		state.DeleteTargets = nil
	}
	if r.deleteConfirmNo.Clicked(gtx) {
		r.onLeftClick()
		r.deleteConfirmOpen = false
		state.DeleteTargets = nil
	}

	// Determine if using trash or permanent delete
	useTrash := trash.IsAvailable()

	// Build message based on number of items and trash availability
	var message, subMessage, title, buttonText string
	if useTrash {
		title = trash.VerbPhrase()
		buttonText = trash.VerbPhrase()
		subMessage = fmt.Sprintf("Items can be restored from the %s.", trash.DisplayName())
		if len(state.DeleteTargets) == 1 {
			message = fmt.Sprintf("Move \"%s\" to the %s?", filepath.Base(state.DeleteTargets[0]), trash.DisplayName())
		} else {
			message = fmt.Sprintf("Move %d items to the %s?", len(state.DeleteTargets), trash.DisplayName())
		}
	} else {
		title = "Confirm Delete"
		buttonText = "Delete"
		subMessage = "This action cannot be undone."
		if len(state.DeleteTargets) == 1 {
			message = fmt.Sprintf("Are you sure you want to delete \"%s\"?", filepath.Base(state.DeleteTargets[0]))
		} else {
			message = fmt.Sprintf("Are you sure you want to delete %d items?", len(state.DeleteTargets))
		}
	}

	return r.modalBackdrop(gtx, 350, nil, func(gtx layout.Context) layout.Dimensions {
		return r.modalContent(gtx, title, colDanger,
			// Body content
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body1(r.Theme, message)
						lbl.Color = colBlack
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body2(r.Theme, subMessage)
						lbl.Color = colGray
						return lbl.Layout(gtx)
					}),
				)
			},
			// Button row
			func(gtx layout.Context) layout.Dimensions {
				return r.dialogButtonRow(gtx, &r.deleteConfirmNo, &r.deleteConfirmYes, "Cancel", buttonText, ButtonDanger)
			},
		)
	})
}

func (r *Renderer) layoutCreateDialog(gtx layout.Context, state *State, eventOut *UIEvent) layout.Dimensions {
	if !r.createDialogOpen {
		return layout.Dimensions{}
	}

	for {
		evt, ok := r.createDialogEditor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := evt.(widget.SubmitEvent); ok {
			name := strings.TrimSpace(r.createDialogEditor.Text())
			if name != "" {
				r.createDialogOpen = false
				if r.createDialogIsDir {
					*eventOut = UIEvent{Action: ActionCreateFolder, FileName: name}
				} else {
					*eventOut = UIEvent{Action: ActionCreateFile, FileName: name}
				}
			}
		}
	}

	if r.createDialogOK.Clicked(gtx) {
		r.onLeftClick()
		name := strings.TrimSpace(r.createDialogEditor.Text())
		if name != "" {
			r.createDialogOpen = false
			if r.createDialogIsDir {
				*eventOut = UIEvent{Action: ActionCreateFolder, FileName: name}
			} else {
				*eventOut = UIEvent{Action: ActionCreateFile, FileName: name}
			}
		}
	}
	if r.createDialogCancel.Clicked(gtx) {
		r.onLeftClick()
		r.createDialogOpen = false
	}

	title := "Create New File"
	placeholder := "filename.txt"
	if r.createDialogIsDir {
		title = "Create New Folder"
		placeholder = "folder name"
	}

	return r.modalBackdrop(gtx, 350, &r.createDialogCancel, func(gtx layout.Context) layout.Dimensions {
		return r.modalContent(gtx, title, colBlack,
			// Body content
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body2(r.Theme, "Enter name:")
						lbl.Color = colGray
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return widget.Border{Color: colLightGray, Width: unit.Dp(1), CornerRadius: unit.Dp(4)}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
									func(gtx layout.Context) layout.Dimensions {
										ed := material.Editor(r.Theme, &r.createDialogEditor, placeholder)
										return ed.Layout(gtx)
									})
							})
					}),
				)
			},
			// Button row
			func(gtx layout.Context) layout.Dimensions {
				return r.dialogButtonRow(gtx, &r.createDialogCancel, &r.createDialogOK, "Cancel", "Create", ButtonPrimary)
			},
		)
	})
}

func (r *Renderer) layoutConflictDialog(gtx layout.Context, state *State, eventOut *UIEvent) layout.Dimensions {
	if !state.Conflict.Active {
		return layout.Dimensions{}
	}

	// Handle button clicks
	applyToAll := r.conflictApplyToAll.Value

	if r.conflictReplaceBtn.Clicked(gtx) {
		r.onLeftClick()
		state.Conflict.Active = false
		state.Conflict.ApplyToAll = applyToAll
		*eventOut = UIEvent{Action: ActionConflictReplace}
	}
	if r.conflictKeepBothBtn.Clicked(gtx) {
		r.onLeftClick()
		state.Conflict.Active = false
		state.Conflict.ApplyToAll = applyToAll
		*eventOut = UIEvent{Action: ActionConflictKeepBoth}
	}
	if r.conflictSkipBtn.Clicked(gtx) {
		r.onLeftClick()
		state.Conflict.Active = false
		state.Conflict.ApplyToAll = applyToAll
		*eventOut = UIEvent{Action: ActionConflictSkip}
	}
	if r.conflictStopBtn.Clicked(gtx) {
		r.onLeftClick()
		state.Conflict.Active = false
		*eventOut = UIEvent{Action: ActionConflictStop}
	}

	// Colors for the dialog
	colWarning := color.NRGBA{R: 255, G: 152, B: 0, A: 255}

	itemType := "File"
	if state.Conflict.IsDir {
		itemType = "Folder"
	}

	return r.modalBackdrop(gtx, 450, nil, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				// Title
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					h6 := material.H6(r.Theme, itemType+" Already Exists")
					h6.Color = colWarning
					return h6.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

				// Conflict description
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					name := filepath.Base(state.Conflict.DestPath)
					lbl := material.Body1(r.Theme, fmt.Sprintf("A file named \"%s\" already exists in this location.", name))
					lbl.Color = colBlack
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

				// Source file info
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(r.Theme, "Source:")
					lbl.Font.Weight = font.Bold
					lbl.Color = colGray
					return lbl.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					sizeStr := formatSizeForDialog(state.Conflict.SourceSize)
					timeStr := state.Conflict.SourceTime.Format("Jan 2, 2006 3:04 PM")
					lbl := material.Body2(r.Theme, fmt.Sprintf("  %s • %s", sizeStr, timeStr))
					lbl.Color = colBlack
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

				// Destination file info
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(r.Theme, "Destination:")
					lbl.Font.Weight = font.Bold
					lbl.Color = colGray
					return lbl.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					sizeStr := formatSizeForDialog(state.Conflict.DestSize)
					timeStr := state.Conflict.DestTime.Format("Jan 2, 2006 3:04 PM")
					lbl := material.Body2(r.Theme, fmt.Sprintf("  %s • %s", sizeStr, timeStr))
					lbl.Color = colBlack
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

				// Apply to all checkbox (only show when multiple conflicts)
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if state.Conflict.RemainingConflicts > 1 {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								cb := material.CheckBox(r.Theme, &r.conflictApplyToAll, fmt.Sprintf("Apply to all %d conflicts", state.Conflict.RemainingConflicts))
								cb.Color = colBlack
								return cb.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
						)
					}
					return layout.Spacer{Height: unit.Dp(4)}.Layout(gtx)
				}),

				// Buttons row 1: Replace and Keep Both
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return r.styledButton(gtx, &r.conflictReplaceBtn, "Replace", ButtonDanger)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							// Keep Both uses success color - custom styling
							btn := material.Button(r.Theme, &r.conflictKeepBothBtn, "Keep Both")
							btn.Background = colSuccess
							return btn.Layout(gtx)
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

				// Buttons row 2: Skip and Stop
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return r.styledButton(gtx, &r.conflictSkipBtn, "Skip", ButtonSecondary)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							// Stop uses gray - custom styling
							btn := material.Button(r.Theme, &r.conflictStopBtn, "Stop")
							btn.Background = colGray
							return btn.Layout(gtx)
						}),
					)
				}),
			)
		})
	})
}

func formatSizeForDialog(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d bytes", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
