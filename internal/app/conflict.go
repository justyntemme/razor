package app

import (
	"os"

	"github.com/justyntemme/razor/internal/ui"
)

// handleConflictResolution is called when user responds to conflict dialog
func (o *Orchestrator) handleConflictResolution(resolution ui.ConflictResolution) {
	// If "Apply to All" was checked, update the resolution mode
	if o.state.Conflict.ApplyToAll {
		o.conflictResolution = resolution
	}
	// Send response to waiting paste operation
	select {
	case o.conflictResponse <- resolution:
	default:
	}
}

// resolveConflict shows the conflict dialog and waits for user response
// remainingConflicts is the number of remaining files that may have conflicts (including current)
func (o *Orchestrator) resolveConflict(src, dst string, srcInfo, dstInfo os.FileInfo, remainingConflicts int) ui.ConflictResolution {
	// If we have a remembered resolution from "Apply to All", use it
	if o.conflictResolution != ui.ConflictAsk {
		return o.conflictResolution
	}

	// If abort was requested, return immediately
	if o.conflictAbort {
		return ui.ConflictAsk
	}

	// Set up the conflict state and show dialog
	o.state.Conflict = ui.ConflictState{
		Active:             true,
		SourcePath:         src,
		DestPath:           dst,
		SourceSize:         srcInfo.Size(),
		DestSize:           dstInfo.Size(),
		SourceTime:         srcInfo.ModTime(),
		DestTime:           dstInfo.ModTime(),
		IsDir:              srcInfo.IsDir(),
		ApplyToAll:         false,
		RemainingConflicts: remainingConflicts,
	}
	o.window.Invalidate()

	// Wait for user response
	resolution := <-o.conflictResponse
	return resolution
}
