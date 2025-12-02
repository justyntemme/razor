package ui

import (
	"fmt"
	"image"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"github.com/justyntemme/razor/internal/debug"
	"github.com/justyntemme/razor/internal/trash"
)

// Settings and hotkeys modal dialogs

func (r *Renderer) layoutSettingsModal(gtx layout.Context, eventOut *UIEvent) layout.Dimensions {
	if !r.settingsOpen {
		return layout.Dimensions{}
	}
	if r.settingsCloseBtn.Clicked(gtx) {
		r.onLeftClick()
		r.settingsOpen = false
	}
	if r.showDotfilesCheck.Update(gtx) {
		debug.Log(debug.UI, "showDotfilesCheck toggled! New value: %v", r.showDotfilesCheck.Value)
		r.ShowDotfiles = r.showDotfilesCheck.Value
		*eventOut = UIEvent{Action: ActionToggleDotfiles, ShowDotfiles: r.ShowDotfiles}
	}

	// Check for dark mode toggle
	if r.darkModeCheck.Update(gtx) {
		r.DarkMode = r.darkModeCheck.Value
		r.applyTheme()
		*eventOut = UIEvent{Action: ActionChangeTheme, DarkMode: r.DarkMode}
	}

	// Check for search engine selection changes
	if r.searchEngine.Update(gtx) {
		selected := r.searchEngine.Value
		// Only allow selecting available engines
		for _, eng := range r.SearchEngines {
			if eng.ID == selected && eng.Available {
				r.SelectedEngine = selected
				*eventOut = UIEvent{Action: ActionChangeSearchEngine, SearchEngine: selected}
				break
			}
		}
		// Reset to previous if unavailable was clicked
		if r.SelectedEngine != selected {
			r.searchEngine.Value = r.SelectedEngine
		}
	}

	// Check for depth changes
	if r.depthDecBtn.Clicked(gtx) && r.DefaultDepth > 1 {
		r.onLeftClick()
		r.DefaultDepth--
		*eventOut = UIEvent{Action: ActionChangeDefaultDepth, DefaultDepth: r.DefaultDepth}
	}
	if r.depthIncBtn.Clicked(gtx) && r.DefaultDepth < 20 {
		r.onLeftClick()
		r.DefaultDepth++
		*eventOut = UIEvent{Action: ActionChangeDefaultDepth, DefaultDepth: r.DefaultDepth}
	}

	return r.modalBackdrop(gtx, 400, &r.settingsCloseBtn, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = gtx.Dp(420)
		return r.modalContentWithClose(gtx, "Settings", colBlack, &r.settingsCloseBtn,
			// Body content - scrollable
			func(gtx layout.Context) layout.Dimensions {
				r.settingsListState.Axis = layout.Vertical
				return material.List(r.Theme, &r.settingsListState).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						// Section: Appearance
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Caption(r.Theme, "APPEARANCE")
							lbl.Color = colGray
							lbl.Font.Weight = font.Bold
							return lbl.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							cb := material.CheckBox(r.Theme, &r.showDotfilesCheck, "Show dotfiles")
							cb.Color = colBlack
							return cb.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							cb := material.CheckBox(r.Theme, &r.darkModeCheck, "Dark mode")
							cb.Color = colBlack
							return cb.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
						// Divider
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return r.layoutHorizontalSeparator(gtx, colLightGray)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
						// Section: Search
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Caption(r.Theme, "SEARCH")
							lbl.Color = colGray
							lbl.Font.Weight = font.Bold
							return lbl.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Body2(r.Theme, "Default Recursive Depth")
							lbl.Color = colBlack
							return lbl.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									btn := material.Button(r.Theme, &r.depthDecBtn, "-")
									btn.Inset = layout.UniformInset(unit.Dp(8))
									if r.DefaultDepth <= 1 {
										btn.Background = colLightGray
										btn.Color = colDisabled
									}
									return btn.Layout(gtx)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									lbl := material.H6(r.Theme, fmt.Sprintf("%d", r.DefaultDepth))
									lbl.Color = colBlack
									return lbl.Layout(gtx)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									btn := material.Button(r.Theme, &r.depthIncBtn, "+")
									btn.Inset = layout.UniformInset(unit.Dp(8))
									if r.DefaultDepth >= 20 {
										btn.Background = colLightGray
										btn.Color = colDisabled
									}
									return btn.Layout(gtx)
								}),
							)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Caption(r.Theme, "Used when recursive: has no value")
							lbl.Color = colGray
							return lbl.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Body2(r.Theme, "Content Search Engine")
							lbl.Color = colBlack
							return lbl.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
						// Render each search engine as a radio button
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return r.layoutSearchEngineOptions(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
						// Divider
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return r.layoutHorizontalSeparator(gtx, colLightGray)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
						// Section: Terminal
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Caption(r.Theme, "TERMINAL")
							lbl.Color = colGray
							lbl.Font.Weight = font.Bold
							return lbl.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Body2(r.Theme, "Default Terminal Application")
							lbl.Color = colBlack
							return lbl.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
						// Render each terminal as a radio button
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return r.layoutTerminalOptions(gtx, eventOut)
						}),
						// Bottom padding for scroll
						layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
					)
				})
			},
			nil, // No button row for settings
		)
	})
}

func (r *Renderer) layoutHotkeysModal(gtx layout.Context) layout.Dimensions {
	if !r.hotkeysOpen {
		return layout.Dimensions{}
	}
	if r.hotkeysCloseBtn.Clicked(gtx) {
		r.onLeftClick()
		r.hotkeysOpen = false
	}

	// Define hotkey categories and their shortcuts
	type hotkeyEntry struct {
		action  string
		binding string
	}

	type hotkeySection struct {
		title   string
		entries []hotkeyEntry
	}

	sections := []hotkeySection{
		{
			title: "FILE OPERATIONS",
			entries: []hotkeyEntry{
				{"Copy", r.hotkeys.Copy.String()},
				{"Cut", r.hotkeys.Cut.String()},
				{"Paste", r.hotkeys.Paste.String()},
				{trash.VerbPhrase(), r.hotkeys.Delete.String()},
				{"Delete Permanently", r.hotkeys.PermanentDelete.String()},
				{"Rename", r.hotkeys.Rename.String()},
				{"New File", r.hotkeys.NewFile.String()},
				{"New Folder", r.hotkeys.NewFolder.String()},
				{"Select All", r.hotkeys.SelectAll.String()},
			},
		},
		{
			title: "NAVIGATION",
			entries: []hotkeyEntry{
				{"Back", r.hotkeys.Back.String()},
				{"Forward", r.hotkeys.Forward.String()},
				{"Go Up", r.hotkeys.Up.String()},
				{"Go Home", r.hotkeys.Home.String()},
				{"Refresh", r.hotkeys.Refresh.String()},
			},
		},
		{
			title: "USER INTERFACE",
			entries: []hotkeyEntry{
				{"Search", r.hotkeys.FocusSearch.String()},
				{"Toggle Preview", r.hotkeys.TogglePreview.String()},
				{"Toggle Hidden Files", r.hotkeys.ToggleHidden.String()},
				{"Cancel/Close", r.hotkeys.Escape.String()},
			},
		},
		{
			title: "TABS",
			entries: []hotkeyEntry{
				{"New Tab (Current)", r.hotkeys.NewTab.String()},
				{"New Tab (Home)", r.hotkeys.NewTabHome.String()},
				{"Close Tab", r.hotkeys.CloseTab.String()},
				{"Next Tab", r.hotkeys.NextTab.String()},
				{"Previous Tab", r.hotkeys.PrevTab.String()},
				{"Tab 1", r.hotkeys.Tab1.String()},
				{"Tab 2", r.hotkeys.Tab2.String()},
				{"Tab 3", r.hotkeys.Tab3.String()},
				{"Tab 4", r.hotkeys.Tab4.String()},
				{"Tab 5", r.hotkeys.Tab5.String()},
				{"Tab 6", r.hotkeys.Tab6.String()},
			},
		},
	}

	// Helper to render a hotkey row
	renderRow := func(gtx layout.Context, entry hotkeyEntry) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(r.Theme, entry.action)
				lbl.Color = colBlack
				return lbl.Layout(gtx)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(r.Theme, entry.binding)
					lbl.Color = colGray
					return lbl.Layout(gtx)
				})
			}),
		)
	}

	// Helper to render a section
	renderSection := func(gtx layout.Context, section hotkeySection) layout.Dimensions {
		var children []layout.FlexChild
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Caption(r.Theme, section.title)
				lbl.Color = colGray
				lbl.Font.Weight = font.Bold
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		)
		for _, entry := range section.entries {
			entry := entry
			if entry.binding != "" {
				children = append(children,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return renderRow(gtx, entry)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
				)
			}
		}
		children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}

	// Determine modal width based on screen size
	// Use 2 columns if screen is wide enough (>800dp), otherwise single column
	screenWidth := gtx.Constraints.Max.X
	minColumnWidth := gtx.Dp(300)
	columnGap := gtx.Dp(32)
	useMultiColumn := screenWidth > gtx.Dp(800)

	modalWidth := unit.Dp(380)
	if useMultiColumn {
		modalWidth = unit.Dp(700)
	}

	// Calculate max modal height (80% of screen height)
	maxModalHeight := gtx.Constraints.Max.Y * 80 / 100
	if maxModalHeight < gtx.Dp(300) {
		maxModalHeight = gtx.Dp(300)
	}

	return r.modalBackdrop(gtx, modalWidth, &r.hotkeysCloseBtn, func(gtx layout.Context) layout.Dimensions {
		// Constrain modal height
		gtx.Constraints.Max.Y = maxModalHeight

		return layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				// Header row with title and close button
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						// Title (takes remaining space)
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							h6 := material.H6(r.Theme, "Keyboard Shortcuts")
							h6.Color = colBlack
							return h6.Layout(gtx)
						}),
						// Divider line
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							height := gtx.Dp(20)
							return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								paint.FillShape(gtx.Ops, colLightGray, clip.Rect{Max: image.Pt(1, height)}.Op())
								return layout.Dimensions{Size: image.Pt(1, height)}
							})
						}),
						// Close button (X)
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							closeSize := gtx.Dp(24)
							return material.Clickable(gtx, &r.hotkeysCloseBtn, func(gtx layout.Context) layout.Dimensions {
								return r.drawXIcon(gtx, closeSize, colGray)
							})
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

				// Scrollable content area
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					r.hotkeysListState.Axis = layout.Vertical
					return material.List(r.Theme, &r.hotkeysListState).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
						if useMultiColumn {
							// Two-column layout
							return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
								// Left column
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.X = minColumnWidth
									return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return renderSection(gtx, sections[0]) // File Operations
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return renderSection(gtx, sections[1]) // Navigation
										}),
									)
								}),
								// Gap between columns
								layout.Rigid(layout.Spacer{Width: unit.Dp(float32(columnGap) / gtx.Metric.PxPerDp)}.Layout),
								// Right column
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.X = minColumnWidth
									return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return renderSection(gtx, sections[2]) // UI
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return renderSection(gtx, sections[3]) // Tabs
										}),
									)
								}),
							)
						}
						// Single column layout
						var children []layout.FlexChild
						for _, section := range sections {
							section := section
							children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return renderSection(gtx, section)
							}))
						}
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
					})
				}),

				// Footer (fixed)
				layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.layoutHorizontalSeparator(gtx, colLightGray)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Caption(r.Theme, "Edit shortcuts in ~/.config/razor/config.json")
					lbl.Color = colGray
					return lbl.Layout(gtx)
				}),
			)
		})
	})
}

func (r *Renderer) layoutSearchEngineOptions(gtx layout.Context) layout.Dimensions {
	var children []layout.FlexChild

	for _, eng := range r.SearchEngines {
		eng := eng // capture loop variable
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.layoutSearchEngineOption(gtx, eng)
		}))
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (r *Renderer) layoutSearchEngineOption(gtx layout.Context, eng SearchEngineInfo) layout.Dimensions {
	// Radio button shows just the name
	rb := material.RadioButton(r.Theme, &r.searchEngine, eng.ID, eng.Name)

	if !eng.Available {
		// Grey out unavailable engines
		rb.Color = colDisabled
		rb.IconColor = colDisabled
	} else if r.SelectedEngine == eng.ID {
		rb.Color = colAccent
	} else {
		rb.Color = colBlack
	}

	return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return rb.Layout(gtx)
			}),
			// Show version on separate line if available
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if eng.Version == "" || !eng.Available {
					return layout.Dimensions{}
				}
				return layout.Inset{Left: unit.Dp(32)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Caption(r.Theme, "v"+eng.Version)
					lbl.Color = colGray
					return lbl.Layout(gtx)
				})
			}),
		)
	})
}

func (r *Renderer) layoutTerminalOptions(gtx layout.Context, eventOut *UIEvent) layout.Dimensions {
	// Check for terminal selection changes
	if r.terminalEnum.Update(gtx) {
		selected := r.terminalEnum.Value
		r.SelectedTerminal = selected
		*eventOut = UIEvent{Action: ActionChangeTerminal, TerminalApp: selected}
	}

	var children []layout.FlexChild

	for _, term := range r.Terminals {
		term := term // capture loop variable
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.layoutTerminalOption(gtx, term)
		}))
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (r *Renderer) layoutTerminalOption(gtx layout.Context, term TerminalInfo) layout.Dimensions {
	// Build display label
	label := term.Name
	if term.Default {
		label += " (default)"
	}

	rb := material.RadioButton(r.Theme, &r.terminalEnum, term.ID, label)

	if r.SelectedTerminal == term.ID {
		rb.Color = colAccent
	} else {
		rb.Color = colBlack
	}

	return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return rb.Layout(gtx)
	})
}
