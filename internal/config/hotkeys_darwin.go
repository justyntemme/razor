//go:build darwin

package config

// DefaultHotkeys returns the default keyboard shortcuts for macOS
// Uses Cmd instead of Alt for navigation shortcuts (macOS convention)
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

		// Navigation - uses Cmd on macOS
		Back:    "Cmd+Left",
		Forward: "Cmd+Right",
		Up:      "Cmd+Up",
		Home:    "Cmd+Shift+H",
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
