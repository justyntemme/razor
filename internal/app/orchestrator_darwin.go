//go:build darwin && !ios

package app

import (
	"gioui.org/app"

	"github.com/justyntemme/razor/internal/debug"
	"github.com/justyntemme/razor/internal/platform"
)

// handlePlatformEvent handles Darwin-specific view events
func (o *Orchestrator) handlePlatformEvent(e any) bool {
	switch evt := e.(type) {
	case app.AppKitViewEvent:
		// macOS: Set up external drag-and-drop when we get the view handle
		debug.Log(debug.APP, "AppKitViewEvent received: Valid=%v View=%d", evt.Valid(), evt.View)
		if evt.Valid() {
			debug.Log(debug.APP, "AppKitViewEvent: calling SetupExternalDrop")
			platform.SetupExternalDrop(evt.View)
		}
		return true
	}
	return false
}
