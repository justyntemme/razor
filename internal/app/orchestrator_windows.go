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
		debug.Log(debug.APP, "Win32ViewEvent received: Valid=%v HWND=0x%x", evt.Valid(), evt.HWND)
		if evt.Valid() {
			platform.SetupExternalDrop(evt.HWND)
		}
		return true
	}
	return false
}
