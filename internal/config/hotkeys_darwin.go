//go:build darwin

package config

// DefaultHotkeys returns the default keyboard shortcuts for macOS
// Uses Cmd instead of Alt for navigation shortcuts (macOS convention)
// Delete key on Mac is backspace (NameDeleteBackward)
func DefaultHotkeys() HotkeysConfig {
	return HotkeysConfig{
		// File operations
		Copy:      "Ctrl+C",
		Cut:       "Ctrl+X",
		Paste:     "Ctrl+V",
		Delete:    "Backspace", // Mac Delete key is backspace
		Rename:    "F2",
		NewFile:   "Ctrl+N",
		NewFolder: "Ctrl+Shift+N",
		SelectAll: "Ctrl+A",

		// Navigation - uses Cmd on macOS (only difference from Windows/Linux)
		Back:    "Cmd+Left",
		Forward: "Cmd+Right",
		Up:      "Cmd+Up",
		Home:    "Cmd+Shift+H",
		Refresh: "F5",

		// UI
		FocusSearch:   "Ctrl+F",
		TogglePreview: "Ctrl+P",
		ToggleHidden:  "Cmd+Shift+>",  // macOS convention (Shift+. = >)
		Escape:        "Escape",

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
