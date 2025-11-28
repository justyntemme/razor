//go:build windows

package main

import (
	"syscall"
)

// manageConsole handles the console window visibility on Windows.
// If debug is false, it detaches (hides) the console window.
func manageConsole(debug bool) {
	if !debug {
		// FreeConsole detaches the calling process from its console.
		// If the app was launched via 'go run', the console window remains but 
		// the app stops writing to it.
		// If built and launched from Explorer, this prevents a persistent console window.
		kernel32 := syscall.NewLazyDLL("kernel32.dll")
		freeConsole := kernel32.NewProc("FreeConsole")
		freeConsole.Call()
	}
}