package ui

import (
	"time"
	"unicode"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/justyntemme/razor/internal/config"
	"github.com/justyntemme/razor/internal/debug"
)

// Keyboard and mouse input handling

// invisibleClickable renders content inside an invisible clickable area.
// Unlike material.Clickable, this does not add any visual hover or press effects.
// Use this for areas that need to detect clicks to dismiss menus without visual feedback.
func invisibleClickable(gtx layout.Context, click *widget.Clickable, w layout.Widget) layout.Dimensions {
	return click.Layout(gtx, w)
}

func (r *Renderer) detectRightClick(gtx layout.Context, tag event.Tag) bool {
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Press | pointer.Release})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok && e.Kind == pointer.Press && e.Buttons.Contain(pointer.ButtonSecondary) {
			// Don't override mousePos here - use the global position tracked in processGlobalInput
			// The position from this event is relative to the local clip area, not window coordinates
			return true
		}
	}
	return false
}

func (r *Renderer) processGlobalInput(gtx layout.Context, state *State, keyTag event.Tag) UIEvent {
	// Keyboard input handling using configurable hotkeys
	// We register explicit filters for each hotkey to ensure they're captured
	// even when other widgets might consume generic key events

	if r.hotkeys == nil {
		return UIEvent{}
	}

	// Skip if modal dialogs are open
	if r.isEditing || r.settingsOpen || r.deleteConfirmOpen || r.createDialogOpen {
		return UIEvent{}
	}

	// Build filters for all configured hotkeys
	filters := r.buildHotkeyFilters(keyTag)

	// Process events matching our hotkey filters
	for {
		e, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		k, ok := e.(key.Event)
		if !ok || k.State != key.Press {
			continue
		}

		// Debug log the key press with detailed modifier info
		debug.Log(debug.HOTKEY, "Key pressed: name=%q mods=0x%x (Copy: key=%q mods=0x%x)",
			k.Name, k.Modifiers, r.hotkeys.Copy.Key, r.hotkeys.Copy.Modifiers)

		// Check configurable hotkeys
		if r.hotkeys != nil {
			// File operations
			if r.hotkeys.Copy.Matches(k) && state.SelectedIndex >= 0 {
				paths := r.collectSelectedPaths(state)
				return UIEvent{Action: ActionCopy, Paths: paths}
			}
			if r.hotkeys.Cut.Matches(k) && state.SelectedIndex >= 0 {
				paths := r.collectSelectedPaths(state)
				return UIEvent{Action: ActionCut, Paths: paths}
			}
			if r.hotkeys.Paste.Matches(k) && state.Clipboard != nil {
				return UIEvent{Action: ActionPaste}
			}
			if r.hotkeys.Delete.Matches(k) {
				if state.SelectedIndex >= 0 || (state.SelectedIndices != nil && len(state.SelectedIndices) > 0) {
					r.deleteConfirmOpen = true
					state.DeleteTargets = r.collectSelectedPaths(state)
					continue
				}
			}
			if r.hotkeys.Rename.Matches(k) && state.SelectedIndex >= 0 {
				entry := state.Entries[state.SelectedIndex]
				r.StartRename(state.SelectedIndex, entry.Path, entry.Name, entry.IsDir)
				continue
			}
			if r.hotkeys.NewFolder.Matches(k) {
				r.ShowCreateDialog(true)
				continue
			}
			if r.hotkeys.NewFile.Matches(k) {
				r.ShowCreateDialog(false)
				continue
			}
			if r.hotkeys.SelectAll.Matches(k) && len(state.Entries) > 0 {
				return UIEvent{Action: ActionSelectAll}
			}

			// Navigation
			if r.hotkeys.Back.Matches(k) && state.CanBack {
				return UIEvent{Action: ActionBack}
			}
			if r.hotkeys.Forward.Matches(k) && state.CanForward {
				return UIEvent{Action: ActionForward}
			}
			if r.hotkeys.Up.Matches(k) {
				return UIEvent{Action: ActionBack}
			}
			if r.hotkeys.Home.Matches(k) {
				return UIEvent{Action: ActionHome}
			}
			if r.hotkeys.Refresh.Matches(k) {
				return UIEvent{Action: ActionRefresh}
			}

			// UI
			if r.hotkeys.FocusSearch.Matches(k) {
				return UIEvent{Action: ActionFocusSearch}
			}
			if r.hotkeys.TogglePreview.Matches(k) {
				if r.previewVisible {
					r.HidePreview()
				} else if state.SelectedIndex >= 0 && state.SelectedIndex < len(state.Entries) {
					r.ShowPreview(state.Entries[state.SelectedIndex].Path)
				}
				continue
			}
			if r.hotkeys.ToggleHidden.Matches(k) {
				return UIEvent{Action: ActionToggleDotfiles}
			}
			if r.hotkeys.Escape.Matches(k) {
				if r.previewVisible {
					r.HidePreview()
				}
				if r.menuVisible {
					r.menuVisible = false
				}
				continue
			}

			// Tabs
			if r.hotkeys.NewTab.Matches(k) {
				debug.Log(debug.HOTKEY, "NewTab hotkey matched")
				return UIEvent{Action: ActionNewTab}
			}
			if r.hotkeys.NewTabHome.Matches(k) {
				debug.Log(debug.HOTKEY, "NewTabHome hotkey matched")
				return UIEvent{Action: ActionNewTabHome}
			}
			if r.hotkeys.CloseTab.Matches(k) {
				debug.Log(debug.HOTKEY, "CloseTab hotkey matched")
				return UIEvent{Action: ActionCloseTab}
			}
			if r.hotkeys.NextTab.Matches(k) {
				debug.Log(debug.HOTKEY, "NextTab hotkey matched: %s", r.hotkeys.NextTab.String())
				return UIEvent{Action: ActionNextTab}
			}
			if r.hotkeys.PrevTab.Matches(k) {
				debug.Log(debug.HOTKEY, "PrevTab hotkey matched: %s", r.hotkeys.PrevTab.String())
				return UIEvent{Action: ActionPrevTab}
			}

			// Direct tab switching (Ctrl+Shift+1-6)
			if r.hotkeys.Tab1.Matches(k) {
				return UIEvent{Action: ActionSwitchToTab, TabIndex: 0}
			}
			if r.hotkeys.Tab2.Matches(k) {
				return UIEvent{Action: ActionSwitchToTab, TabIndex: 1}
			}
			if r.hotkeys.Tab3.Matches(k) {
				return UIEvent{Action: ActionSwitchToTab, TabIndex: 2}
			}
			if r.hotkeys.Tab4.Matches(k) {
				return UIEvent{Action: ActionSwitchToTab, TabIndex: 3}
			}
			if r.hotkeys.Tab5.Matches(k) {
				return UIEvent{Action: ActionSwitchToTab, TabIndex: 4}
			}
			if r.hotkeys.Tab6.Matches(k) {
				return UIEvent{Action: ActionSwitchToTab, TabIndex: 5}
			}
		}

		// Arrow keys and Enter are not configurable (fundamental navigation)
		switch k.Name {
		case key.NameUpArrow:
			if idx := state.SelectedIndex - 1; idx >= 0 {
				r.listState.ScrollTo(idx)
				return UIEvent{Action: ActionSelect, NewIndex: idx}
			} else if state.SelectedIndex == -1 && len(state.Entries) > 0 {
				idx := len(state.Entries) - 1
				r.listState.ScrollTo(idx)
				return UIEvent{Action: ActionSelect, NewIndex: idx}
			}
		case key.NameDownArrow:
			if idx := state.SelectedIndex + 1; idx < len(state.Entries) {
				r.listState.ScrollTo(idx)
				return UIEvent{Action: ActionSelect, NewIndex: idx}
			} else if state.SelectedIndex == -1 && len(state.Entries) > 0 {
				return UIEvent{Action: ActionSelect, NewIndex: 0}
			}
		case key.NameLeftArrow:
			// Left arrow: go back (up one directory level)
			if state.CanBack {
				return UIEvent{Action: ActionBack}
			}
		case key.NameRightArrow:
			// Right arrow: enter selection (navigate into folder or open file)
			if idx := state.SelectedIndex; idx >= 0 && idx < len(state.Entries) {
				item := state.Entries[idx]
				if item.IsDir {
					return UIEvent{Action: ActionNavigate, Path: item.Path}
				}
				return UIEvent{Action: ActionOpen, Path: item.Path}
			}
		case key.NameReturn, key.NameEnter:
			if idx := state.SelectedIndex; idx >= 0 && idx < len(state.Entries) {
				item := state.Entries[idx]
				if item.IsDir {
					return UIEvent{Action: ActionNavigate, Path: item.Path}
				}
				return UIEvent{Action: ActionOpen, Path: item.Path}
			}
		default:
			// Quick-jump: single letter A-Z with no modifiers
			if k.Modifiers == 0 && len(k.Name) == 1 {
				letter := rune(k.Name[0])
				if letter >= 'A' && letter <= 'Z' {
					if jumpIdx := r.findNextMatchingEntry(state, letter); jumpIdx >= 0 {
						r.listState.ScrollTo(jumpIdx)
						return UIEvent{Action: ActionJumpToLetter, NewIndex: jumpIdx}
					}
				}
			}
		}
	}
	return UIEvent{}
}

// findNextMatchingEntry finds the next entry starting with the given letter
// If the same letter was pressed recently, it cycles to the next match
func (r *Renderer) findNextMatchingEntry(state *State, letter rune) int {
	if len(state.Entries) == 0 {
		return -1
	}

	letterLower := unicode.ToLower(letter)
	now := time.Now()

	// Determine starting position for search
	startIdx := 0
	if r.lastJumpKey == letter && now.Sub(r.lastJumpTime) < 1*time.Second {
		// Same letter pressed recently - cycle to next match
		startIdx = r.lastJumpIndex + 1
		if startIdx >= len(state.Entries) {
			startIdx = 0 // Wrap around
		}
	} else if state.SelectedIndex >= 0 {
		// Different letter or timeout - start from after current selection
		startIdx = state.SelectedIndex
	}

	// Search from startIdx to end, then wrap around
	for i := 0; i < len(state.Entries); i++ {
		idx := (startIdx + i) % len(state.Entries)
		entry := state.Entries[idx]
		if len(entry.Name) > 0 {
			firstChar := unicode.ToLower(rune(entry.Name[0]))
			if firstChar == letterLower {
				// Found a match
				r.lastJumpKey = letter
				r.lastJumpTime = now
				r.lastJumpIndex = idx
				return idx
			}
		}
	}

	return -1 // No match found
}

// buildHotkeyFilters creates key.Filter slice for all configured hotkeys
func (r *Renderer) buildHotkeyFilters(keyTag event.Tag) []event.Filter {
	if r.hotkeys == nil {
		return nil
	}

	// Create a filter for each hotkey
	hotkeys := []config.Hotkey{
		r.hotkeys.Copy, r.hotkeys.Cut, r.hotkeys.Paste, r.hotkeys.Delete,
		r.hotkeys.Rename, r.hotkeys.NewFile, r.hotkeys.NewFolder, r.hotkeys.SelectAll,
		r.hotkeys.Back, r.hotkeys.Forward, r.hotkeys.Up, r.hotkeys.Home, r.hotkeys.Refresh,
		r.hotkeys.FocusSearch, r.hotkeys.TogglePreview, r.hotkeys.ToggleHidden, r.hotkeys.Escape,
		r.hotkeys.NewTab, r.hotkeys.CloseTab, r.hotkeys.NextTab, r.hotkeys.PrevTab,
		r.hotkeys.Tab1, r.hotkeys.Tab2, r.hotkeys.Tab3, r.hotkeys.Tab4, r.hotkeys.Tab5, r.hotkeys.Tab6,
	}

	// Use a map to deduplicate filters with same key+modifiers
	type filterKey struct {
		name key.Name
		mods key.Modifiers
	}
	seen := make(map[filterKey]bool)

	filters := make([]event.Filter, 0, len(hotkeys)+6)

	for _, hk := range hotkeys {
		if !hk.IsEmpty() {
			fk := filterKey{hk.Key, hk.Modifiers}
			if !seen[fk] {
				seen[fk] = true
				filters = append(filters, hk.Filter(keyTag))
			}
		}
	}

	// Add arrow keys and Enter for navigation (no modifiers required)
	arrowKeys := []key.Name{
		key.NameUpArrow, key.NameDownArrow, key.NameLeftArrow, key.NameRightArrow,
		key.NameReturn, key.NameEnter,
	}
	for _, k := range arrowKeys {
		fk := filterKey{k, 0}
		if !seen[fk] {
			seen[fk] = true
			filters = append(filters, key.Filter{
				Focus: keyTag,
				Name:  k,
			})
		}
	}

	// Add letter keys A-Z for quick-jump navigation (no modifiers)
	for c := 'A'; c <= 'Z'; c++ {
		letterKey := key.Name(string(c))
		fk := filterKey{letterKey, 0}
		if !seen[fk] {
			seen[fk] = true
			filters = append(filters, key.Filter{
				Focus: keyTag,
				Name:  letterKey,
			})
		}
	}

	return filters
}
