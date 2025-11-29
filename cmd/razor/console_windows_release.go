//go:build windows && !debug

package main

import "syscall"

func manageConsole() {
	// Hide console window in release builds
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	freeConsole := kernel32.NewProc("FreeConsole")
	freeConsole.Call()
}
