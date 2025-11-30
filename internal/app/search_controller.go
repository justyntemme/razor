package app

import (
	"strings"

	"github.com/justyntemme/razor/internal/debug"
	"github.com/justyntemme/razor/internal/fs"
	"github.com/justyntemme/razor/internal/search"
	"github.com/justyntemme/razor/internal/store"
)

// Search directive prefixes for advanced query syntax
var searchDirectives = []string{
	"contents:",
	"ext:",
	"size:",
	"modified:",
	"filename:",
	"recursive:",
	"depth:",
}

// Directives that don't require a value after the prefix
var valueOptionalDirectives = map[string]bool{
	"recursive:": true,
	"depth:":     true,
}

// DoSearch performs a search with the given query.
// submitted indicates if the user pressed Enter (true) or if this is search-as-you-type (false).
// restoreDirectory is a callback to restore directory entries when search is cleared (returns true if successful).
// setProgress is a callback to update the progress bar.
func (s *SearchController) DoSearch(query string, submitted bool, restoreDirectory func() bool, setProgress func(bool, string, int64, int64)) {
	debug.Log(debug.SEARCH, "DoSearch: query=%q submitted=%v", query, submitted)

	s.state.Mu.Lock()
	s.state.State.SelectedIndex = -1
	s.state.Mu.Unlock()

	// Empty query clears the search and restores directory
	if query == "" {
		debug.Log(debug.SEARCH, "DoSearch: empty query, restoring directory")
		// Cancel any ongoing search
		s.deps.FS.RequestChan <- fs.Request{Op: fs.CancelSearch}

		s.state.Mu.Lock()
		s.state.State.IsSearchResult = false
		s.state.State.SearchQuery = ""
		currentPath := s.state.State.CurrentPath
		s.state.Mu.Unlock()

		// Clear progress bar
		setProgress(false, "", 0, 0)

		// Increment generation to invalidate any pending search results
		s.state.SearchGen.Add(1)

		// Restore from cached directory entries via callback
		if restoreDirectory() {
			s.deps.Window.Invalidate()
		} else {
			// Fallback: re-fetch if no cached entries
			s.requestDir(currentPath)
		}
		return
	}

	// Check if query contains directive prefix but no value (incomplete)
	if isIncompleteDirective(query) {
		debug.Log(debug.SEARCH, "DoSearch: incomplete directive, waiting: %q", query)
		return
	}

	// Track search state
	s.state.Mu.Lock()
	s.state.State.SearchQuery = query
	s.state.State.IsSearchResult = true
	currentPath := s.state.State.CurrentPath
	s.state.Mu.Unlock()

	// Check if this is a directive search (slow operation)
	isDirectiveSearch := hasAnyDirective(query)

	debug.Log(debug.SEARCH, "DoSearch: isDirective=%v hasContents=%v", isDirectiveSearch, hasCompleteDirective(query, "contents:"))

	if isDirectiveSearch {
		label := "Searching..."
		if hasCompleteDirective(query, "contents:") {
			label = "Searching file contents..."
		}
		setProgress(true, label, 0, 0)
	}

	// Increment generation for this search
	gen := s.state.SearchGen.Add(1)

	// Save search to history ONLY when submitted via Enter (not search-as-you-type)
	if submitted && len(query) >= 2 {
		s.deps.Store.RequestChan <- store.Request{
			Op:    store.AddSearchHistory,
			Query: query,
		}
		debug.Log(debug.SEARCH, "DoSearch: saved to history: %q", query)
	}

	debug.Log(debug.SEARCH, "DoSearch: sending request path=%q gen=%d engine=%d depth=%d",
		currentPath, gen, s.SelectedEngine, s.DefaultDepth)

	s.deps.FS.RequestChan <- fs.Request{
		Op:           fs.SearchDir,
		Path:         currentPath,
		Query:        query,
		Gen:          gen,
		SearchEngine: int(s.SelectedEngine),
		EngineCmd:    s.SelectedEngineCmd,
		DefaultDepth: s.DefaultDepth,
	}
}

// ChangeEngine updates the search engine setting.
func (s *SearchController) ChangeEngine(engineID string) {
	engine := search.GetEngineByName(engineID)
	engineCmd := search.GetEngineCommand(engine, s.Engines)

	// For non-builtin engines, verify it's actually available
	if engine != search.EngineBuiltin && engineCmd == "" {
		debug.Log(debug.SEARCH, "Engine %s not available, staying with current", engineID)
		return
	}

	s.SelectedEngine = engine
	s.SelectedEngineCmd = engineCmd
	s.deps.UI.SetSearchEngine(engineID)
	debug.Log(debug.SEARCH, "Changed engine to: %s (cmd: %s)", engineID, s.SelectedEngineCmd)
}

// CancelSearch cancels any ongoing search and clears search state.
func (s *SearchController) CancelSearch(setProgress func(bool, string, int64, int64)) {
	debug.Log(debug.APP, "CancelSearch: cancelling search")
	s.deps.FS.RequestChan <- fs.Request{Op: fs.CancelSearch}

	s.state.Mu.Lock()
	s.state.State.IsSearchResult = false
	s.state.State.SearchQuery = ""
	s.state.Mu.Unlock()

	setProgress(false, "", 0, 0)
	s.state.SearchGen.Add(1)
}

// requestDir sends a request to fetch directory contents (internal helper).
func (s *SearchController) requestDir(path string) {
	gen := s.state.SearchGen.Add(1)
	s.deps.FS.RequestChan <- fs.Request{Op: fs.FetchDir, Path: path, Gen: gen}
}

// hasAnyDirective checks if query has any complete directive.
func hasAnyDirective(query string) bool {
	for _, prefix := range searchDirectives {
		if hasCompleteDirective(query, prefix) {
			return true
		}
	}
	return false
}

// hasCompleteDirective checks if query has a directive with an actual value.
func hasCompleteDirective(query, prefix string) bool {
	lowerQuery := strings.ToLower(query)
	idx := strings.Index(lowerQuery, prefix)
	if idx < 0 {
		return false
	}
	// For value-optional directives, presence alone is enough
	if valueOptionalDirectives[prefix] {
		return true
	}
	// Check there's something after the prefix
	afterPrefix := query[idx+len(prefix):]
	parts := strings.Fields(afterPrefix)
	if len(parts) == 0 {
		return false
	}
	return len(parts[0]) > 0
}

// isIncompleteDirective checks if query ends with a directive prefix but no value.
func isIncompleteDirective(query string) bool {
	lowerQuery := strings.ToLower(strings.TrimSpace(query))

	for _, prefix := range searchDirectives {
		if strings.HasSuffix(lowerQuery, prefix) {
			if valueOptionalDirectives[prefix] {
				return false
			}
			return true
		}
		if strings.Contains(lowerQuery, prefix) {
			idx := strings.Index(lowerQuery, prefix)
			afterPrefix := strings.TrimSpace(lowerQuery[idx+len(prefix):])
			if afterPrefix == "" && !valueOptionalDirectives[prefix] {
				return true
			}
		}
	}
	return false
}
