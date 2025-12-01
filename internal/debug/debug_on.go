//go:build debug

// Package debug provides a centralized, categorized debug logging system.
// Build with -tags debug to enable logging.
package debug

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

// Enabled indicates whether debug logging is active
const Enabled = true

// Category represents a debug logging category
type Category string

const (
	// Core categories
	APP    Category = "APP"    // Application orchestration, navigation, state
	FS     Category = "FS"     // Filesystem operations (fetch, walk)
	SEARCH Category = "SEARCH" // Search engine, query parsing, matching
	STORE  Category = "STORE"  // Database operations, settings, favorites
	UI     Category = "UI"     // UI events, layout, rendering
	HOTKEY Category = "HOTKEY" // Keyboard shortcut handling and matching

	// Detailed subcategories (use sparingly - can be verbose)
	FS_ENTRY  Category = "FS_ENTRY"  // Individual entry processing (very verbose)
	FS_WALK   Category = "FS_WALK"   // Directory walking during search
	UI_EVENT  Category = "UI_EVENT"  // UI event handling
	UI_LAYOUT Category = "UI_LAYOUT" // Layout calculations (extremely verbose)
)

var (
	// enabledCategories controls which categories are active
	// By default, all main categories are enabled
	enabledCategories = map[Category]bool{
		APP:    true,
		FS:     true,
		SEARCH: true,
		STORE:  true,
		UI:     true,
		HOTKEY: true,
		// Verbose categories disabled by default
		FS_ENTRY:  false,
		FS_WALK:   false,
		UI_EVENT:  false,
		UI_LAYOUT: false,
	}
	categoryMu sync.RWMutex

	// Output destination
	logger = log.New(os.Stderr, "", log.Ltime|log.Lmicroseconds)
)

func init() {
	// Check environment variable for category overrides
	// Format: RAZOR_DEBUG=APP,FS,SEARCH or RAZOR_DEBUG=all or RAZOR_DEBUG=none
	if env := os.Getenv("RAZOR_DEBUG"); env != "" {
		categoryMu.Lock()
		defer categoryMu.Unlock()

		env = strings.ToUpper(env)
		switch env {
		case "ALL":
			for cat := range enabledCategories {
				enabledCategories[cat] = true
			}
		case "NONE":
			for cat := range enabledCategories {
				enabledCategories[cat] = false
			}
		default:
			// Disable all first, then enable specified
			for cat := range enabledCategories {
				enabledCategories[cat] = false
			}
			for _, cat := range strings.Split(env, ",") {
				cat = strings.TrimSpace(cat)
				enabledCategories[Category(cat)] = true
			}
		}
	}
}

// Log logs a debug message for the specified category
func Log(cat Category, format string, args ...interface{}) {
	categoryMu.RLock()
	enabled := enabledCategories[cat]
	categoryMu.RUnlock()

	if !enabled {
		return
	}

	msg := fmt.Sprintf(format, args...)
	logger.Printf("[%s] %s", cat, msg)
}

// Enable enables a debug category
func Enable(cat Category) {
	categoryMu.Lock()
	enabledCategories[cat] = true
	categoryMu.Unlock()
}

// Disable disables a debug category
func Disable(cat Category) {
	categoryMu.Lock()
	enabledCategories[cat] = false
	categoryMu.Unlock()
}

// IsEnabled returns whether a category is enabled
func IsEnabled(cat Category) bool {
	categoryMu.RLock()
	defer categoryMu.RUnlock()
	return enabledCategories[cat]
}

// EnableAll enables all debug categories including verbose ones
func EnableAll() {
	categoryMu.Lock()
	for cat := range enabledCategories {
		enabledCategories[cat] = true
	}
	categoryMu.Unlock()
}

// DisableAll disables all debug categories
func DisableAll() {
	categoryMu.Lock()
	for cat := range enabledCategories {
		enabledCategories[cat] = false
	}
	categoryMu.Unlock()
}

// SetCategories sets the enabled state for multiple categories
func SetCategories(cats map[Category]bool) {
	categoryMu.Lock()
	for cat, enabled := range cats {
		enabledCategories[cat] = enabled
	}
	categoryMu.Unlock()
}

// ListEnabled returns a slice of currently enabled categories
func ListEnabled() []Category {
	categoryMu.RLock()
	defer categoryMu.RUnlock()

	var enabled []Category
	for cat, on := range enabledCategories {
		if on {
			enabled = append(enabled, cat)
		}
	}
	return enabled
}
