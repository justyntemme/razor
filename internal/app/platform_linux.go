//go:build linux

package app

import "os/exec"

// platformOpen opens the file using 'xdg-open'.
func platformOpen(path string) error {
	return exec.Command("xdg-open", path).Start()
}
