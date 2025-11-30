//go:build debug

package app

import "github.com/justyntemme/razor/internal/debug"

// debugEnabled is true when built with -tags debug
// Use debug.Enabled from the debug package for checking if debug is enabled
var debugEnabled = debug.Enabled
