//go:build !darwin

package config

// DefaultHotkeys returns the default keyboard shortcuts for Windows/Linux
// Uses Alt for navigation shortcuts (standard convention)
func DefaultHotkeys() HotkeysConfig {
	return HotkeysConfig{
		// File operations
		Copy:            "Ctrl+C",
		Cut:             "Ctrl+X",
		Paste:           "Ctrl+V",
		Delete:          "Delete",
		PermanentDelete: "Shift+Delete", // Bypass trash (standard Windows/Linux)
		Rename:          "F2",
		NewFile:         "Ctrl+N",
		NewFolder:       "Ctrl+Shift+N",
		SelectAll:       "Ctrl+A",

		// Navigation - uses Alt on Windows/Linux
		Back:    "Alt+Left",
		Forward: "Alt+Right",
		Up:      "Alt+Up",
		Home:    "Alt+Home",
		Refresh: "F5",

		// UI
		FocusSearch:    "Ctrl+F",
		TogglePreview:  "Ctrl+P",
		ToggleHidden:   "Ctrl+Shift+H", // Toggle hidden files
		ToggleViewMode: "Ctrl+Shift+G", // Toggle between list and grid view
		Escape:         "Escape",

		// Tabs - vim bindings: Ctrl+H (left) and Ctrl+L (right)
		NewTab:     "Ctrl+T",           // New tab in current directory
		NewTabHome: "Ctrl+Shift+T",     // New tab in home directory
		CloseTab:   "Ctrl+W",
		NextTab:    "Ctrl+L",           // Vim: right
		PrevTab:    "Ctrl+H",           // Vim: left

		// Direct tab switching (Ctrl+Shift+1-6)
		Tab1: "Ctrl+Shift+1",
		Tab2: "Ctrl+Shift+2",
		Tab3: "Ctrl+Shift+3",
		Tab4: "Ctrl+Shift+4",
		Tab5: "Ctrl+Shift+5",
		Tab6: "Ctrl+Shift+6",
	}
}
