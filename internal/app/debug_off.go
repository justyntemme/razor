//go:build !debug

package app

import "github.com/justyntemme/razor/internal/debug"

// debugEnabled is false when not built with -tags debug
var debugEnabled = debug.Enabled
