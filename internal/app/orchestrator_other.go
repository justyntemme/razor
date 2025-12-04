//go:build !darwin && !windows

package app

// handlePlatformEvent handles platform-specific view events (no-op on unsupported platforms)
func (o *Orchestrator) handlePlatformEvent(e any) bool {
	return false
}
