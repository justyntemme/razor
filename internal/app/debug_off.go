//go:build !debug

package app

const debugEnabled = false

func debugLog(format string, args ...interface{}) {}