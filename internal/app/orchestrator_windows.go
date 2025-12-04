//go:build windows

package app

import (
	"gioui.org/app"

	"github.com/justyntemme/razor/internal/debug"
	"github.com/justyntemme/razor/internal/platform"
)

// handlePlatformEvent handles Windows-specific view events
func (o *Orchestrator) handlePlatformEvent(e any) bool {
	switch evt := e.(type) {
	case app.Win32ViewEvent:
		// Windows: Set up external drag-and-drop when we get the HWND
		debug.Log(debug.APP, "Win32ViewEvent received: Valid=%v HWND=%d", evt.Valid(), evt.HWND)
		if evt.Valid() {
			debug.Log(debug.APP, "Win32ViewEvent: calling SetupExternalDrop")
			platform.SetupExternalDrop(evt.HWND)
		}
		return true
	}
	return false
}
