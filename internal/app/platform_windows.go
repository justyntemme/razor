//go:build windows

package app

import "os/exec"

// platformOpen opens the file using the Windows 'start' command.
func platformOpen(path string) error {
	// 'cmd /c start "" "path"' is the standard way to launch files in Windows
	return exec.Command("cmd", "/c", "start", "", path).Start()
}
