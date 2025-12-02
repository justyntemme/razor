//go:build darwin

package config

// DefaultHotkeys returns the default keyboard shortcuts for macOS
// Uses Cmd for most shortcuts (macOS convention)
// Delete key on Mac is backspace (NameDeleteBackward)
func DefaultHotkeys() HotkeysConfig {
	return HotkeysConfig{
		// File operations - use Cmd on macOS (standard macOS convention)
		Copy:      "Cmd+C",
		Cut:       "Cmd+X",
		Paste:     "Cmd+V",
		Delete:    "Backspace", // Mac Delete key is backspace
		Rename:    "F2",
		NewFile:   "Cmd+N",
		NewFolder: "Cmd+Shift+N",
		SelectAll: "Cmd+A",

		// Navigation - uses Cmd on macOS
		Back:    "Cmd+Left",
		Forward: "Cmd+Right",
		Up:      "Cmd+Up",
		Home:    "Ctrl+Shift+H",
		Refresh: "Cmd+R",

		// UI
		FocusSearch:   "Cmd+F",
		TogglePreview: "Cmd+P",
		ToggleHidden:  "Cmd+Shift+>", // macOS convention (Shift+. = >)
		Escape:        "Escape",

		// Tabs - use Cmd on macOS (standard macOS convention)
		NewTab:     "Cmd+T",
		NewTabHome: "Cmd+Shift+T",
		CloseTab:   "Cmd+W",
		NextTab:    "Cmd+Shift+L", // Vim: right
		PrevTab:    "Cmd+Shift+H", // Vim: left

		// Direct tab switching (Cmd+1-6, standard macOS)
		Tab1: "Cmd+1",
		Tab2: "Cmd+2",
		Tab3: "Cmd+3",
		Tab4: "Cmd+4",
		Tab5: "Cmd+5",
		Tab6: "Cmd+6",
	}
}
