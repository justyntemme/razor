package app

import (
	"strings"

	"github.com/justyntemme/razor/internal/debug"
	"github.com/justyntemme/razor/internal/fs"
	"github.com/justyntemme/razor/internal/search"
	"github.com/justyntemme/razor/internal/store"
)

// doSearch performs a search with the given query
// submitted indicates if the user pressed Enter (true) or if this is search-as-you-type (false)
func (o *Orchestrator) doSearch(query string, submitted bool) {
	debug.Log(debug.SEARCH, "doSearch: query=%q submitted=%v", query, submitted)
	o.state.SelectedIndex = -1

	// Empty query clears the search and restores directory
	if query == "" {
		debug.Log(debug.SEARCH, "doSearch: empty query, restoring directory")
		// Cancel any ongoing search
		o.fs.RequestChan <- fs.Request{Op: fs.CancelSearch}
		o.state.IsSearchResult = false
		o.state.SearchQuery = ""
		// Clear progress bar
		o.setProgress(false, "", 0, 0)
		// Increment generation to invalidate any pending search results (atomic)
		o.searchGen.Add(1)
		// Restore from cached directory entries
		if len(o.dirEntries) > 0 {
			o.rawEntries = o.dirEntries
			o.applyFilterAndSort()
			o.window.Invalidate()
		} else {
			o.requestDir(o.state.CurrentPath)
		}
		return
	}

	// Check if query contains directive prefix but no value (e.g., "contents:" alone)
	// These are incomplete and should not trigger a search
	if isIncompleteDirective(query) {
		debug.Log(debug.SEARCH, "doSearch: incomplete directive, waiting: %q", query)
		// Don't search, just wait for user to complete the directive
		// But also don't clear the current results
		return
	}

	// Track search state
	o.state.SearchQuery = query
	o.state.IsSearchResult = true

	// Check if this is a directive search (slow operation)
	isDirectiveSearch := hasCompleteDirective(query, "contents:") ||
		hasCompleteDirective(query, "ext:") ||
		hasCompleteDirective(query, "size:") ||
		hasCompleteDirective(query, "modified:") ||
		hasCompleteDirective(query, "filename:") ||
		hasCompleteDirective(query, "recursive:") ||
		hasCompleteDirective(query, "depth:")

	debug.Log(debug.SEARCH, "doSearch: isDirective=%v hasContents=%v", isDirectiveSearch, hasCompleteDirective(query, "contents:"))

	if isDirectiveSearch {
		// Show progress for directive searches
		label := "Searching..."
		if hasCompleteDirective(query, "contents:") {
			label = "Searching file contents..."
		}
		o.setProgress(true, label, 0, 0)
	}

	// Increment generation for this search (atomic)
	gen := o.searchGen.Add(1)

	// Save search to history ONLY when submitted via Enter (not search-as-you-type)
	// Also only save meaningful queries (2+ chars)
	if submitted && len(query) >= 2 {
		o.store.RequestChan <- store.Request{
			Op:    store.AddSearchHistory,
			Query: query,
		}
		debug.Log(debug.SEARCH, "doSearch: saved to history: %q", query)
	}

	debug.Log(debug.SEARCH, "doSearch: sending request path=%q gen=%d engine=%d depth=%d",
		o.state.CurrentPath, gen, o.selectedEngine, o.defaultDepth)
	o.fs.RequestChan <- fs.Request{
		Op:           fs.SearchDir,
		Path:         o.state.CurrentPath,
		Query:        query,
		Gen:          gen,
		SearchEngine: int(o.selectedEngine),
		EngineCmd:    o.selectedEngineCmd,
		DefaultDepth: o.defaultDepth,
	}
}

// hasCompleteDirective checks if query has a directive with an actual value
// Note: recursive: and depth: are allowed to have empty values (defaults to 10)
func hasCompleteDirective(query, prefix string) bool {
	lowerQuery := strings.ToLower(query)
	idx := strings.Index(lowerQuery, prefix)
	if idx < 0 {
		return false
	}
	// For recursive: and depth:, presence alone is enough
	if prefix == "recursive:" || prefix == "depth:" {
		return true
	}
	// Check there's something after the prefix
	afterPrefix := query[idx+len(prefix):]
	// Get the value (up to next space or end)
	parts := strings.Fields(afterPrefix)
	if len(parts) == 0 {
		return false
	}
	return len(parts[0]) > 0
}

// isIncompleteDirective checks if query ends with a directive prefix but no value
func isIncompleteDirective(query string) bool {
	lowerQuery := strings.ToLower(strings.TrimSpace(query))
	prefixes := []string{"contents:", "ext:", "size:", "modified:", "filename:", "recursive:", "depth:"}

	for _, prefix := range prefixes {
		// Check if query ends with just the prefix (no value)
		if strings.HasSuffix(lowerQuery, prefix) {
			// For recursive:, empty value is allowed (defaults to depth 10)
			if prefix == "recursive:" || prefix == "depth:" {
				return false
			}
			return true
		}
		// Check if query contains prefix with no value (prefix at end of a word boundary)
		if strings.Contains(lowerQuery, prefix) {
			idx := strings.Index(lowerQuery, prefix)
			afterPrefix := strings.TrimSpace(lowerQuery[idx+len(prefix):])
			// If nothing after the prefix, or next char is another directive prefix, incomplete
			// (except for recursive: which can be empty)
			if afterPrefix == "" && prefix != "recursive:" && prefix != "depth:" {
				return true
			}
		}
	}
	return false
}

// handleSearchEngineChange updates the search engine setting
func (o *Orchestrator) handleSearchEngineChange(engineID string) {
	// Check if the engine is available before selecting it
	engine := search.GetEngineByName(engineID)
	engineCmd := search.GetEngineCommand(engine, o.searchEngines)

	// For non-builtin engines, verify it's actually available
	if engine != search.EngineBuiltin && engineCmd == "" {
		debug.Log(debug.SEARCH, "Engine %s not available, staying with current", engineID)
		return
	}

	o.selectedEngine = engine
	o.selectedEngineCmd = engineCmd
	o.ui.SetSearchEngine(engineID)
	debug.Log(debug.SEARCH, "Changed engine to: %s (cmd: %s)", engineID, o.selectedEngineCmd)
}
