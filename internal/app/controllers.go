package app

import (
	"sync"
	"sync/atomic"

	"gioui.org/app"

	"github.com/justyntemme/razor/internal/fs"
	"github.com/justyntemme/razor/internal/search"
	"github.com/justyntemme/razor/internal/store"
	"github.com/justyntemme/razor/internal/ui"
)

// SharedDeps holds references to shared dependencies that multiple controllers need.
// This prevents duplication of state and ensures a single source of truth.
// All controllers receive a pointer to this struct rather than copying fields.
type SharedDeps struct {
	Window   *app.Window
	FS       *fs.System
	Store    *store.DB
	UI       *ui.Renderer
	HomePath string
}

// SharedState holds mutable state that multiple controllers may need to read/write.
// Access must be synchronized using the provided mutex.
type SharedState struct {
	Mu sync.RWMutex

	// UI State (passed to renderer)
	State *ui.State

	// Search generation counter (atomic for lock-free access)
	SearchGen *atomic.Int64
}

// SearchController handles all search-related operations.
// It encapsulates search engine management, query execution, and result handling.
type SearchController struct {
	deps  *SharedDeps
	state *SharedState

	// Search-specific state
	Engines       []search.EngineInfo // Available search engines
	SelectedEngine    search.SearchEngine // Currently selected engine
	SelectedEngineCmd string              // Command for the selected engine
	DefaultDepth      int                 // Default recursive search depth
}

// NewSearchController creates a search controller with the given dependencies.
func NewSearchController(deps *SharedDeps, state *SharedState, engines []search.EngineInfo) *SearchController {
	return &SearchController{
		deps:           deps,
		state:          state,
		Engines:        engines,
		SelectedEngine: search.EngineBuiltin,
		DefaultDepth:   2,
	}
}

// NavigationController handles path navigation, history, and directory requests.
type NavigationController struct {
	deps  *SharedDeps
	state *SharedState

	// Navigation-specific state
	History      []string
	HistoryIndex int
}

// NewNavigationController creates a navigation controller with the given dependencies.
func NewNavigationController(deps *SharedDeps, state *SharedState) *NavigationController {
	return &NavigationController{
		deps:         deps,
		state:        state,
		History:      make([]string, 0, maxHistorySize),
		HistoryIndex: -1,
	}
}

