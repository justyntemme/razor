//go:build debug

package app

import "log"

const debugEnabled = true

func debugLog(format string, args ...interface{}) {
	log.Printf("[DEBUG] "+format, args...)
}
