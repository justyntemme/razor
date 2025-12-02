package config

import (
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/key"
)

// Hotkey represents a parsed keyboard shortcut
type Hotkey struct {
	Key       key.Name
	Modifiers key.Modifiers
}

// ParseHotkey parses a hotkey string like "Ctrl+Shift+N" into a Hotkey struct
func ParseHotkey(s string) Hotkey {
	if s == "" {
		return Hotkey{}
	}

	var mods key.Modifiers
	var rawKeyPart string

	parts := strings.Split(s, "+")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch strings.ToLower(part) {
		case "ctrl", "control":
			mods |= key.ModCtrl
		case "shift":
			mods |= key.ModShift
		case "alt", "option":
			mods |= key.ModAlt
		case "cmd", "command":
			mods |= key.ModCommand // Use ModCommand for macOS Cmd key
		case "super", "meta", "win", "windows":
			mods |= key.ModSuper // Use ModSuper for Windows logo key
		default:
			// This is the key name
			rawKeyPart = part
		}
	}

	// Convert the key part to key.Name
	keyName := parseKeyName(rawKeyPart)

	// If Shift is held and this is a number key, convert to the shifted character
	// because Gio reports the shifted character (e.g., Shift+1 = "!")
	// Note: Punctuation should be specified directly (e.g., "Cmd+Shift+>" not "Cmd+Shift+.")
	if mods.Contain(key.ModShift) {
		if shifted, ok := shiftedNumbers[string(keyName)]; ok {
			keyName = key.Name(shifted)
		}
	}

	return Hotkey{Key: keyName, Modifiers: mods}
}

// shiftedNumbers maps number keys to their shifted equivalents (US keyboard layout)
// This is needed because Gio reports the shifted character, not the physical key
// Note: Punctuation should be specified directly as the shifted character (e.g., ">" not ".")
var shiftedNumbers = map[string]string{
	"1": "!", "2": "@", "3": "#", "4": "$", "5": "%",
	"6": "^", "7": "&", "8": "*", "9": "(", "0": ")",
}

// unshiftedNumbers is the reverse mapping for display purposes
var unshiftedNumbers = map[string]string{
	"!": "1", "@": "2", "#": "3", "$": "4", "%": "5",
	"^": "6", "&": "7", "*": "8", "(": "9", ")": "0",
}

// parseKeyName converts a key string to Gio's key.Name
func parseKeyName(s string) key.Name {
	// Handle single letters (case insensitive for parsing, but key.Name uses uppercase)
	if len(s) == 1 {
		return key.Name(strings.ToUpper(s))
	}

	// Handle special keys
	switch strings.ToLower(s) {
	// Function keys
	case "f1":
		return key.NameF1
	case "f2":
		return key.NameF2
	case "f3":
		return key.NameF3
	case "f4":
		return key.NameF4
	case "f5":
		return key.NameF5
	case "f6":
		return key.NameF6
	case "f7":
		return key.NameF7
	case "f8":
		return key.NameF8
	case "f9":
		return key.NameF9
	case "f10":
		return key.NameF10
	case "f11":
		return key.NameF11
	case "f12":
		return key.NameF12

	// Navigation keys
	case "up", "uparrow":
		return key.NameUpArrow
	case "down", "downarrow":
		return key.NameDownArrow
	case "left", "leftarrow":
		return key.NameLeftArrow
	case "right", "rightarrow":
		return key.NameRightArrow
	case "home":
		return key.NameHome
	case "end":
		return key.NameEnd
	case "pageup", "pgup":
		return key.NamePageUp
	case "pagedown", "pgdn", "pgdown":
		return key.NamePageDown

	// Editing keys
	case "enter", "return":
		return key.NameReturn
	case "tab":
		return key.NameTab
	case "space", "spacebar":
		return key.NameSpace
	case "backspace", "back":
		return key.NameDeleteBackward
	case "delete", "del":
		return key.NameDeleteForward
	case "escape", "esc":
		return key.NameEscape
	case "insert", "ins":
		return key.Name("Insert")

	default:
		// Return as-is for unknown keys (supports custom key names)
		return key.Name(s)
	}
}

// Matches checks if a key event matches this hotkey
// Uses exact matching for modifiers to distinguish between similar hotkeys
// (e.g., Ctrl+H vs Ctrl+Shift+H)
func (h Hotkey) Matches(k key.Event) bool {
	if h.Key == "" {
		return false
	}
	return k.Name == h.Key && k.Modifiers == h.Modifiers
}

// MatchesLoose checks if a key event matches this hotkey, allowing extra modifiers
// This is useful when Shift might be held for capitalization
func (h Hotkey) MatchesLoose(k key.Event) bool {
	if h.Key == "" {
		return false
	}
	// Check that all required modifiers are present
	return k.Name == h.Key && (k.Modifiers&h.Modifiers) == h.Modifiers
}

// IsEmpty returns true if the hotkey is not configured
func (h Hotkey) IsEmpty() bool {
	return h.Key == ""
}

// String returns a human-readable representation of the hotkey
func (h Hotkey) String() string {
	if h.Key == "" {
		return ""
	}

	var parts []string
	if h.Modifiers.Contain(key.ModCtrl) {
		parts = append(parts, "Ctrl")
	}
	if h.Modifiers.Contain(key.ModCommand) {
		parts = append(parts, "Cmd")
	}
	if h.Modifiers.Contain(key.ModShift) {
		parts = append(parts, "Shift")
	}
	if h.Modifiers.Contain(key.ModAlt) {
		parts = append(parts, "Alt")
	}
	if h.Modifiers.Contain(key.ModSuper) {
		parts = append(parts, "Super")
	}

	// For display, convert shifted number symbols back to their original keys
	keyStr := string(h.Key)
	if h.Modifiers.Contain(key.ModShift) {
		if original, ok := unshiftedNumbers[keyStr]; ok {
			keyStr = original
		}
	}
	parts = append(parts, keyStr)
	return strings.Join(parts, "+")
}

// Filter returns a key.Filter that matches this hotkey
func (h Hotkey) Filter(focus event.Tag) key.Filter {
	return key.Filter{
		Focus:    focus,
		Name:     h.Key,
		Required: h.Modifiers,
	}
}

// HotkeyMatcher provides efficient hotkey matching from config
type HotkeyMatcher struct {
	// File operations
	Copy            Hotkey
	Cut             Hotkey
	Paste           Hotkey
	Delete          Hotkey
	PermanentDelete Hotkey
	Rename          Hotkey
	NewFile         Hotkey
	NewFolder       Hotkey
	SelectAll       Hotkey

	// Navigation
	Back    Hotkey
	Forward Hotkey
	Up      Hotkey
	Home    Hotkey
	Refresh Hotkey

	// UI
	FocusSearch   Hotkey
	TogglePreview Hotkey
	ToggleHidden  Hotkey
	Escape        Hotkey

	// Tabs
	NewTab     Hotkey
	NewTabHome Hotkey
	CloseTab   Hotkey
	NextTab    Hotkey
	PrevTab    Hotkey

	// Tab switching (direct)
	Tab1 Hotkey
	Tab2 Hotkey
	Tab3 Hotkey
	Tab4 Hotkey
	Tab5 Hotkey
	Tab6 Hotkey
}

// NewHotkeyMatcher creates a matcher from config
func NewHotkeyMatcher(cfg HotkeysConfig) *HotkeyMatcher {
	return &HotkeyMatcher{
		// File operations
		Copy:            ParseHotkey(cfg.Copy),
		Cut:             ParseHotkey(cfg.Cut),
		Paste:           ParseHotkey(cfg.Paste),
		Delete:          ParseHotkey(cfg.Delete),
		PermanentDelete: ParseHotkey(cfg.PermanentDelete),
		Rename:          ParseHotkey(cfg.Rename),
		NewFile:         ParseHotkey(cfg.NewFile),
		NewFolder:       ParseHotkey(cfg.NewFolder),
		SelectAll:       ParseHotkey(cfg.SelectAll),

		// Navigation
		Back:    ParseHotkey(cfg.Back),
		Forward: ParseHotkey(cfg.Forward),
		Up:      ParseHotkey(cfg.Up),
		Home:    ParseHotkey(cfg.Home),
		Refresh: ParseHotkey(cfg.Refresh),

		// UI
		FocusSearch:   ParseHotkey(cfg.FocusSearch),
		TogglePreview: ParseHotkey(cfg.TogglePreview),
		ToggleHidden:  ParseHotkey(cfg.ToggleHidden),
		Escape:        ParseHotkey(cfg.Escape),

		// Tabs
		NewTab:     ParseHotkey(cfg.NewTab),
		NewTabHome: ParseHotkey(cfg.NewTabHome),
		CloseTab:   ParseHotkey(cfg.CloseTab),
		NextTab:    ParseHotkey(cfg.NextTab),
		PrevTab:    ParseHotkey(cfg.PrevTab),

		// Tab switching (direct)
		Tab1: ParseHotkey(cfg.Tab1),
		Tab2: ParseHotkey(cfg.Tab2),
		Tab3: ParseHotkey(cfg.Tab3),
		Tab4: ParseHotkey(cfg.Tab4),
		Tab5: ParseHotkey(cfg.Tab5),
		Tab6: ParseHotkey(cfg.Tab6),
	}
}
