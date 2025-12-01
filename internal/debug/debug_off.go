//go:build !debug

// Package debug provides a centralized, categorized debug logging system.
// This is the no-op version for release builds.
package debug

// Enabled indicates whether debug logging is active
const Enabled = false

// Category represents a debug logging category
type Category string

const (
	APP       Category = "APP"
	FS        Category = "FS"
	SEARCH    Category = "SEARCH"
	STORE     Category = "STORE"
	UI        Category = "UI"
	HOTKEY    Category = "HOTKEY"
	FS_ENTRY  Category = "FS_ENTRY"
	FS_WALK   Category = "FS_WALK"
	UI_EVENT  Category = "UI_EVENT"
	UI_LAYOUT Category = "UI_LAYOUT"
)

// Log is a no-op in release builds
func Log(cat Category, format string, args ...interface{}) {}

// Enable is a no-op in release builds
func Enable(cat Category) {}

// Disable is a no-op in release builds
func Disable(cat Category) {}

// IsEnabled always returns false in release builds
func IsEnabled(cat Category) bool { return false }

// EnableAll is a no-op in release builds
func EnableAll() {}

// DisableAll is a no-op in release builds
func DisableAll() {}

// SetCategories is a no-op in release builds
func SetCategories(cats map[Category]bool) {}

// ListEnabled returns nil in release builds
func ListEnabled() []Category { return nil }
