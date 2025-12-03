//go:build !darwin || ios

package platform

// DropHandler is called when files are dropped from an external source
type DropHandler func(paths []string)

// SetDropHandler sets the callback for external file drops (no-op on non-macOS)
func SetDropHandler(handler DropHandler) {
	// Not implemented on this platform
}

// SetupExternalDrop configures external file drop handling (no-op on non-macOS)
func SetupExternalDrop(viewPtr uintptr) {
	// Not implemented on this platform
}
