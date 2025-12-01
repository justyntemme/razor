//go:build !darwin

package config

// DefaultHotkeys returns the default keyboard shortcuts for Windows/Linux
// Uses Alt for navigation shortcuts (standard convention)
func DefaultHotkeys() HotkeysConfig {
	return HotkeysConfig{
		// File operations
		Copy:      "Ctrl+C",
		Cut:       "Ctrl+X",
		Paste:     "Ctrl+V",
		Delete:    "Delete",
		Rename:    "F2",
		NewFile:   "Ctrl+N",
		NewFolder: "Ctrl+Shift+N",
		SelectAll: "Ctrl+A",

		// Navigation - uses Alt on Windows/Linux
		Back:    "Alt+Left",
		Forward: "Alt+Right",
		Up:      "Alt+Up",
		Home:    "Alt+Home",
		Refresh: "F5",

		// UI
		FocusSearch:   "Ctrl+F",
		TogglePreview: "Ctrl+P",
		ToggleHidden:  "Ctrl+H",
		Escape:        "Escape",

		// Tabs
		NewTab:   "Ctrl+T",
		CloseTab: "Ctrl+W",
		NextTab:  "Ctrl+Tab",
		PrevTab:  "Ctrl+Shift+Tab",
	}
}
