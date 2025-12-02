//go:build darwin

package config

import "os/exec"

// TerminalInfo represents an available terminal application
type TerminalInfo struct {
	ID      string // Identifier used in config
	Name    string // Display name
	Cmd     string // Command to check if installed
	Default bool   // True if this is the platform default
}

// DetectTerminals returns a list of installed terminal applications on macOS
func DetectTerminals() []TerminalInfo {
	// Popular macOS terminals in order of preference
	candidates := []TerminalInfo{
		{ID: "terminal", Name: "Terminal.app", Cmd: "", Default: true}, // Always available on macOS
		{ID: "iterm2", Name: "iTerm2", Cmd: ""},
		{ID: "wezterm", Name: "WezTerm", Cmd: "wezterm"},
		{ID: "kitty", Name: "Kitty", Cmd: "kitty"},
		{ID: "alacritty", Name: "Alacritty", Cmd: "alacritty"},
		{ID: "hyper", Name: "Hyper", Cmd: ""},
	}

	var installed []TerminalInfo

	for _, term := range candidates {
		if term.Default {
			// Terminal.app is always available
			installed = append(installed, term)
			continue
		}

		// Check if command exists in PATH
		if term.Cmd != "" {
			if _, err := exec.LookPath(term.Cmd); err == nil {
				installed = append(installed, term)
				continue
			}
		}

		// Check if app exists in /Applications
		appPaths := []string{
			"/Applications/" + term.Name + ".app",
			"/Applications/" + term.ID + ".app",
		}
		for _, path := range appPaths {
			if _, err := exec.Command("test", "-d", path).Output(); err == nil {
				installed = append(installed, term)
				break
			}
		}
	}

	return installed
}

// DefaultTerminalID returns the ID of the default terminal for macOS
func DefaultTerminalID() string {
	return "terminal"
}
