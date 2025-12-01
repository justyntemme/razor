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
	var keyName key.Name

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
		case "cmd", "command", "super", "meta":
			mods |= key.ModSuper
		default:
			// This is the key name
			keyName = parseKeyName(part)
		}
	}

	return Hotkey{Key: keyName, Modifiers: mods}
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
	if h.Modifiers.Contain(key.ModShift) {
		parts = append(parts, "Shift")
	}
	if h.Modifiers.Contain(key.ModAlt) {
		parts = append(parts, "Alt")
	}
	if h.Modifiers.Contain(key.ModSuper) {
		parts = append(parts, "Cmd")
	}
	parts = append(parts, string(h.Key))
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
	Copy      Hotkey
	Cut       Hotkey
	Paste     Hotkey
	Delete    Hotkey
	Rename    Hotkey
	NewFile   Hotkey
	NewFolder Hotkey
	SelectAll Hotkey

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
	NewTab   Hotkey
	CloseTab Hotkey
	NextTab  Hotkey
	PrevTab  Hotkey
}

// NewHotkeyMatcher creates a matcher from config
func NewHotkeyMatcher(cfg HotkeysConfig) *HotkeyMatcher {
	return &HotkeyMatcher{
		// File operations
		Copy:      ParseHotkey(cfg.Copy),
		Cut:       ParseHotkey(cfg.Cut),
		Paste:     ParseHotkey(cfg.Paste),
		Delete:    ParseHotkey(cfg.Delete),
		Rename:    ParseHotkey(cfg.Rename),
		NewFile:   ParseHotkey(cfg.NewFile),
		NewFolder: ParseHotkey(cfg.NewFolder),
		SelectAll: ParseHotkey(cfg.SelectAll),

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
		NewTab:   ParseHotkey(cfg.NewTab),
		CloseTab: ParseHotkey(cfg.CloseTab),
		NextTab:  ParseHotkey(cfg.NextTab),
		PrevTab:  ParseHotkey(cfg.PrevTab),
	}
}
