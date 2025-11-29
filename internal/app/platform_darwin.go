//go:build darwin

package app

import "os/exec"

// platformOpen opens the file using the macOS 'open' command.
func platformOpen(path string) error {
	return exec.Command("open", path).Start()
}
