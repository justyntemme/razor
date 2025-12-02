//go:build linux

package config

import "os/exec"

// TerminalInfo represents an available terminal application
type TerminalInfo struct {
	ID      string // Identifier used in config
	Name    string // Display name
	Cmd     string // Command to check if installed
	Default bool   // True if this is the platform default
}

// DetectTerminals returns a list of installed terminal applications on Linux
func DetectTerminals() []TerminalInfo {
	// Popular Linux terminals in order of preference
	candidates := []TerminalInfo{
		{ID: "gnome-terminal", Name: "GNOME Terminal", Cmd: "gnome-terminal", Default: false},
		{ID: "konsole", Name: "Konsole", Cmd: "konsole", Default: false},
		{ID: "xfce4-terminal", Name: "XFCE Terminal", Cmd: "xfce4-terminal", Default: false},
		{ID: "mate-terminal", Name: "MATE Terminal", Cmd: "mate-terminal", Default: false},
		{ID: "tilix", Name: "Tilix", Cmd: "tilix", Default: false},
		{ID: "terminator", Name: "Terminator", Cmd: "terminator", Default: false},
		{ID: "alacritty", Name: "Alacritty", Cmd: "alacritty", Default: false},
		{ID: "kitty", Name: "Kitty", Cmd: "kitty", Default: false},
		{ID: "wezterm", Name: "WezTerm", Cmd: "wezterm", Default: false},
		{ID: "x-terminal-emulator", Name: "Default Terminal", Cmd: "x-terminal-emulator", Default: false},
		{ID: "xterm", Name: "XTerm", Cmd: "xterm", Default: false},
	}

	var installed []TerminalInfo
	foundDefault := false

	for _, term := range candidates {
		if _, err := exec.LookPath(term.Cmd); err == nil {
			// Mark the first found terminal as default
			if !foundDefault {
				term.Default = true
				foundDefault = true
			}
			installed = append(installed, term)
		}
	}

	return installed
}

// DefaultTerminalID returns the ID of the default terminal for Linux
// Returns empty string to use first available
func DefaultTerminalID() string {
	return ""
}
