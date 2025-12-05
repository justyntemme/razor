//go:build windows

package app

import (
	"fmt"
	"os"

	"gioui.org/app"

	"github.com/justyntemme/razor/internal/debug"
	"github.com/justyntemme/razor/internal/platform"
)

// handlePlatformEvent handles Windows-specific view events
func (o *Orchestrator) handlePlatformEvent(e any) bool {
	// Debug: log all event types to file
	f, _ := os.OpenFile(`C:\razor_events.txt`, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if f != nil {
		fmt.Fprintf(f, "Event type: %T\n", e)
		f.Close()
	}

	switch evt := e.(type) {
	case app.Win32ViewEvent:
		// Windows: Set up external drag-and-drop when we get the HWND
		debug.Log(debug.APP, "Win32ViewEvent received: Valid=%v HWND=%d", evt.Valid(), evt.HWND)

		// Also write to file for debugging
		f, _ := os.OpenFile(`C:\razor_events.txt`, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if f != nil {
			fmt.Fprintf(f, ">>> Win32ViewEvent: Valid=%v HWND=%d\n", evt.Valid(), evt.HWND)
			f.Close()
		}

		if evt.Valid() {
			debug.Log(debug.APP, "Win32ViewEvent: calling SetupExternalDrop")
			platform.SetupExternalDrop(evt.HWND)
		}
		return true
	}
	return false
}
