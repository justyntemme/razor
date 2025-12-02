//go:build windows

package config

import "os/exec"

// TerminalInfo represents an available terminal application
type TerminalInfo struct {
	ID      string // Identifier used in config
	Name    string // Display name
	Cmd     string // Command to check if installed
	Default bool   // True if this is the platform default
}

// DetectTerminals returns a list of installed terminal applications on Windows
func DetectTerminals() []TerminalInfo {
	// Popular Windows terminals in order of preference
	candidates := []TerminalInfo{
		{ID: "wt", Name: "Windows Terminal", Cmd: "wt.exe", Default: false},
		{ID: "pwsh", Name: "PowerShell 7", Cmd: "pwsh.exe", Default: false},
		{ID: "powershell", Name: "Windows PowerShell", Cmd: "powershell.exe", Default: false},
		{ID: "cmd", Name: "Command Prompt", Cmd: "cmd.exe", Default: true}, // Always available
		{ID: "alacritty", Name: "Alacritty", Cmd: "alacritty.exe", Default: false},
		{ID: "wezterm", Name: "WezTerm", Cmd: "wezterm.exe", Default: false},
		{ID: "kitty", Name: "Kitty", Cmd: "kitty.exe", Default: false},
	}

	var installed []TerminalInfo

	for _, term := range candidates {
		// cmd.exe is always available
		if term.ID == "cmd" {
			installed = append(installed, term)
			continue
		}

		if _, err := exec.LookPath(term.Cmd); err == nil {
			installed = append(installed, term)
		}
	}

	// If Windows Terminal is installed, make it the default instead of cmd
	for i := range installed {
		if installed[i].ID == "wt" {
			// Reset cmd default and set wt as default
			for j := range installed {
				installed[j].Default = installed[j].ID == "wt"
			}
			break
		}
	}

	return installed
}

// DefaultTerminalID returns the ID of the default terminal for Windows
func DefaultTerminalID() string {
	// Prefer Windows Terminal if available, otherwise cmd
	if _, err := exec.LookPath("wt.exe"); err == nil {
		return "wt"
	}
	return "cmd"
}
