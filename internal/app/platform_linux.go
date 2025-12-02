//go:build linux

package app

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// platformOpen opens the file using 'xdg-open' (default application).
func platformOpen(path string) error {
	return exec.Command("xdg-open", path).Start()
}

// platformOpenWith shows the system "Open With" dialog or opens with a specific app.
// On Linux, we use xdg-open by default. For a custom dialog, you'd need zenity or similar.
// If appPath is empty, this tries to show the system open-with dialog.
func platformOpenWith(filePath string, appPath string) error {
	if appPath != "" {
		// Open with specific application
		return exec.Command(appPath, filePath).Start()
	}

	// Try different desktop-specific "open with" commands
	// KDE Plasma
	if _, err := exec.LookPath("kde-open5"); err == nil {
		return exec.Command("kde-open5", "--openwith", filePath).Start()
	}
	if _, err := exec.LookPath("kde-open"); err == nil {
		return exec.Command("kde-open", "--openwith", filePath).Start()
	}

	// GNOME - use gio open which may show app chooser for unknown types
	if _, err := exec.LookPath("gio"); err == nil {
		return exec.Command("gio", "open", filePath).Start()
	}

	// Fallback to xdg-open
	return exec.Command("xdg-open", filePath).Start()
}

// platformGetApplications returns a list of applications that can open the given file type.
// This queries the system's mime database.
func platformGetApplications(filePath string) ([]AppInfo, error) {
	var apps []AppInfo

	// Get the mime type of the file
	mimeOut, err := exec.Command("xdg-mime", "query", "filetype", filePath).Output()
	if err != nil {
		return apps, err
	}
	mimeType := strings.TrimSpace(string(mimeOut))

	// Get default application
	defaultApp, err := exec.Command("xdg-mime", "query", "default", mimeType).Output()
	if err == nil && len(defaultApp) > 0 {
		appName := strings.TrimSpace(string(defaultApp))
		appName = strings.TrimSuffix(appName, ".desktop")
		apps = append(apps, AppInfo{
			Name: appName,
			Path: string(defaultApp),
		})
	}

	// Try to get more apps from mimeapps.list or gio
	moreApps, _ := exec.Command("gio", "mime", mimeType).Output()
	if len(moreApps) > 0 {
		lines := strings.Split(string(moreApps), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasSuffix(line, ".desktop") {
				name := strings.TrimSuffix(filepath.Base(line), ".desktop")
				apps = append(apps, AppInfo{
					Name: name,
					Path: line,
				})
			}
		}
	}

	return apps, nil
}

// AppInfo represents an application that can open files
type AppInfo struct {
	Name string // Display name
	Path string // Executable path or desktop file
}

// platformOpenTerminal opens a terminal emulator in the specified directory.
// If terminalApp is empty, tries common terminal emulators in order of preference.
func platformOpenTerminal(dir, terminalApp string) error {
	// Terminal commands and their arguments for working directory
	terminalArgs := map[string][]string{
		"gnome-terminal":     {"--working-directory=" + dir},
		"konsole":            {"--workdir", dir},
		"xfce4-terminal":     {"--working-directory=" + dir},
		"mate-terminal":      {"--working-directory=" + dir},
		"tilix":              {"--working-directory=" + dir},
		"terminator":         {"--working-directory=" + dir},
		"alacritty":          {"--working-directory", dir},
		"kitty":              {"--directory", dir},
		"wezterm":            {"start", "--cwd", dir},
		"x-terminal-emulator": {"--working-directory=" + dir},
		"xterm":              {"-e", "cd '" + dir + "' && $SHELL -l"},
	}

	// If a specific terminal is configured, use it
	if terminalApp != "" {
		if args, ok := terminalArgs[terminalApp]; ok {
			return exec.Command(terminalApp, args...).Start()
		}
		// Unknown terminal, try with common args
		return exec.Command(terminalApp, "--working-directory="+dir).Start()
	}

	// Auto-detect: try terminals in order of preference
	order := []string{
		"gnome-terminal", "konsole", "xfce4-terminal", "mate-terminal",
		"tilix", "terminator", "alacritty", "kitty", "wezterm",
		"x-terminal-emulator", "xterm",
	}

	for _, term := range order {
		if _, err := exec.LookPath(term); err == nil {
			return exec.Command(term, terminalArgs[term]...).Start()
		}
	}

	// If no terminal found, try xdg-open (unlikely to work well)
	return exec.Command("xdg-open", dir).Start()
}
