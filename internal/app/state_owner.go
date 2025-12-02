package app

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gioui.org/app"
	"github.com/justyntemme/razor/internal/debug"
	"github.com/justyntemme/razor/internal/ui"
)

// StateOwner is the single source of truth for all entry state.
// It consolidates what was previously spread across rawEntries, dirEntries,
// state.Entries, tab.DirEntries, and tab.RawEntries.
//
// All mutations go through StateOwner methods which hold the mutex.
// UI reads via GetSnapshot() which returns an immutable view.
type StateOwner struct {
	mu sync.RWMutex

	// THE source of truth - what UI displays
	entries []ui.UIEntry

	// Unfiltered entries for current path (needed for dotfiles toggle)
	rawEntries []ui.UIEntry

	// Current view metadata
	currentPath  string
	expandedDirs map[string]bool

	// Display settings
	showDotfiles bool
	sortColumn   ui.SortColumn
	sortAsc      bool

	// Selection state
	selectedIndex   int
	selectedIndices map[int]bool

	// Navigation
	canBack    bool
	canForward bool

	// Tab state (metadata only, NO entry copies)
	tabs        map[string]*TabMeta
	activeTabID string

	// Window reference for invalidation
	window *app.Window
}

// TabMeta stores only metadata for tabs - no entry copies
type TabMeta struct {
	Path         string
	ExpandedDirs map[string]bool
	ScrollPos    int
	SelectedIdx  int
	History      []string
	HistoryIndex int
}

// Snapshot is an immutable view of state for the UI to render
type Snapshot struct {
	Entries         []ui.UIEntry
	CurrentPath     string
	SelectedIndex   int
	SelectedIndices map[int]bool
	CanBack         bool
	CanForward      bool
	ExpandedDirs    map[string]bool
}

// NewStateOwner creates a new state owner
func NewStateOwner(window *app.Window, showDotfiles bool) *StateOwner {
	return &StateOwner{
		entries:         make([]ui.UIEntry, 0),
		rawEntries:      make([]ui.UIEntry, 0),
		expandedDirs:    make(map[string]bool),
		selectedIndices: make(map[int]bool),
		tabs:            make(map[string]*TabMeta),
		showDotfiles:    showDotfiles,
		sortColumn:      ui.SortByName,
		sortAsc:         true,
		selectedIndex:   -1,
		window:          window,
	}
}

// GetSnapshot returns an immutable snapshot for UI rendering
// Uses RLock for concurrent read access
func (s *StateOwner) GetSnapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return copies so UI can't mutate our state
	entries := make([]ui.UIEntry, len(s.entries))
	copy(entries, s.entries)

	selectedIndices := make(map[int]bool, len(s.selectedIndices))
	for k, v := range s.selectedIndices {
		selectedIndices[k] = v
	}

	expandedDirs := make(map[string]bool, len(s.expandedDirs))
	for k, v := range s.expandedDirs {
		expandedDirs[k] = v
	}

	return Snapshot{
		Entries:         entries,
		CurrentPath:     s.currentPath,
		SelectedIndex:   s.selectedIndex,
		SelectedIndices: selectedIndices,
		CanBack:         s.canBack,
		CanForward:      s.canForward,
		ExpandedDirs:    expandedDirs,
	}
}

// GetEntries returns the current entries (for direct access when needed)
func (s *StateOwner) GetEntries() []ui.UIEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.entries
}

// GetCurrentPath returns the current directory path
func (s *StateOwner) GetCurrentPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentPath
}

// SetEntries sets entries from an external source (e.g., filesystem response)
func (s *StateOwner) SetEntries(path string, entries []ui.UIEntry, canBack, canForward bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.currentPath = path
	s.rawEntries = entries
	s.expandedDirs = make(map[string]bool) // Clear expansions on navigation
	s.canBack = canBack
	s.canForward = canForward

	s.rebuildLocked()
}

// SetEntriesKeepExpanded sets entries but preserves expansion state (for refresh)
func (s *StateOwner) SetEntriesKeepExpanded(entries []ui.UIEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.rawEntries = entries
	s.rebuildLocked()
}

// ExpandDir expands a directory inline
func (s *StateOwner) ExpandDir(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	debug.Log(debug.APP, "StateOwner.ExpandDir: %s", path)

	// Mark as expanded
	s.expandedDirs[path] = true

	// Find parent in entries
	parentIdx := -1
	parentDepth := 0
	for i, entry := range s.entries {
		if entry.Path == path {
			parentIdx = i
			parentDepth = entry.Depth
			break
		}
	}

	if parentIdx < 0 {
		debug.Log(debug.APP, "StateOwner.ExpandDir: parent not found in entries")
		return
	}

	// Read children from filesystem
	children := s.readDirLocked(path)
	children = s.filterLocked(children)
	s.sortLocked(children)

	// Set depth and parent path on children
	for i := range children {
		children[i].Depth = parentDepth + 1
		children[i].ParentPath = path
	}

	debug.Log(debug.APP, "StateOwner.ExpandDir: inserting %d children", len(children))

	// Mark parent as expanded
	s.entries[parentIdx].IsExpanded = true

	// Insert children after parent
	newEntries := make([]ui.UIEntry, 0, len(s.entries)+len(children))
	newEntries = append(newEntries, s.entries[:parentIdx+1]...)
	newEntries = append(newEntries, children...)
	newEntries = append(newEntries, s.entries[parentIdx+1:]...)
	s.entries = newEntries

	s.invalidate()
}

// CollapseDir collapses an expanded directory
func (s *StateOwner) CollapseDir(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	debug.Log(debug.APP, "StateOwner.CollapseDir: %s", path)

	// Mark as collapsed
	delete(s.expandedDirs, path)

	// Find parent in entries
	parentIdx := -1
	parentDepth := 0
	for i, entry := range s.entries {
		if entry.Path == path {
			parentIdx = i
			parentDepth = entry.Depth
			break
		}
	}

	if parentIdx < 0 {
		debug.Log(debug.APP, "StateOwner.CollapseDir: parent not found")
		return
	}

	// Mark parent as collapsed
	s.entries[parentIdx].IsExpanded = false

	// Find range of children to remove
	removeStart := parentIdx + 1
	removeEnd := removeStart
	for i := removeStart; i < len(s.entries); i++ {
		if s.entries[i].Depth <= parentDepth {
			break
		}
		// Also collapse any nested expanded dirs
		if s.entries[i].IsDir && s.entries[i].IsExpanded {
			delete(s.expandedDirs, s.entries[i].Path)
		}
		removeEnd = i + 1
	}

	// Remove children
	if removeEnd > removeStart {
		newEntries := make([]ui.UIEntry, 0, len(s.entries)-(removeEnd-removeStart))
		newEntries = append(newEntries, s.entries[:removeStart]...)
		newEntries = append(newEntries, s.entries[removeEnd:]...)
		s.entries = newEntries
	}

	s.invalidate()
}

// ToggleDotfiles toggles dotfile visibility and rebuilds
func (s *StateOwner) ToggleDotfiles(show bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.showDotfiles = show
	s.rebuildLocked()
	s.invalidate()
}

// SetSort changes sort settings and rebuilds
func (s *StateOwner) SetSort(column ui.SortColumn, ascending bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sortColumn = column
	s.sortAsc = ascending
	s.rebuildLocked()
	s.invalidate()
}

// SetSelection sets the selected index
func (s *StateOwner) SetSelection(index int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.selectedIndex = index
	s.invalidate()
}

// SetSelectedIndices sets multi-selection
func (s *StateOwner) SetSelectedIndices(indices map[int]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.selectedIndices = indices
	s.invalidate()
}

// ClearSelection clears all selection
func (s *StateOwner) ClearSelection() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.selectedIndex = -1
	s.selectedIndices = make(map[int]bool)
	s.invalidate()
}

// SaveTab saves current state to a tab
func (s *StateOwner) SaveTab(tabID string, history []string, historyIndex int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tabs[tabID] = &TabMeta{
		Path:         s.currentPath,
		ExpandedDirs: copyStringBoolMap(s.expandedDirs),
		SelectedIdx:  s.selectedIndex,
		History:      history,
		HistoryIndex: historyIndex,
	}
}

// LoadTab loads state from a tab
func (s *StateOwner) LoadTab(tabID string) *TabMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if tab, ok := s.tabs[tabID]; ok {
		return tab
	}
	return nil
}

// RestoreTab restores expanded dirs from a tab
func (s *StateOwner) RestoreTab(tabID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if tab, ok := s.tabs[tabID]; ok {
		s.expandedDirs = copyStringBoolMap(tab.ExpandedDirs)
		s.selectedIndex = tab.SelectedIdx
	}
	s.activeTabID = tabID
}

// DeleteTab removes a tab's state
func (s *StateOwner) DeleteTab(tabID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.tabs, tabID)
}

// IsExpanded checks if a directory is expanded
func (s *StateOwner) IsExpanded(path string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.expandedDirs[path]
}

// GetExpandedDirs returns list of expanded directory paths
func (s *StateOwner) GetExpandedDirs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]string, 0, len(s.expandedDirs))
	for path := range s.expandedDirs {
		result = append(result, path)
	}
	return result
}

// ClearExpanded clears all expanded directories
func (s *StateOwner) ClearExpanded() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expandedDirs = make(map[string]bool)
}

// RemoveEntry removes an entry from the current view without resetting expansion state
// If the entry is an expanded directory, also removes its children
func (s *StateOwner) RemoveEntry(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find the entry
	entryIdx := -1
	entryDepth := 0
	for i, entry := range s.entries {
		if entry.Path == path {
			entryIdx = i
			entryDepth = entry.Depth
			break
		}
	}

	if entryIdx < 0 {
		return
	}

	// Also remove from rawEntries
	for i, entry := range s.rawEntries {
		if entry.Path == path {
			s.rawEntries = append(s.rawEntries[:i], s.rawEntries[i+1:]...)
			break
		}
	}

	// If this was an expanded directory, remove it from expandedDirs
	if s.expandedDirs[path] {
		delete(s.expandedDirs, path)
	}

	// Find the range to remove (entry + any children if expanded)
	removeStart := entryIdx
	removeEnd := entryIdx + 1

	// Check if this entry has children (was expanded)
	if s.entries[entryIdx].IsDir && s.entries[entryIdx].IsExpanded {
		for i := entryIdx + 1; i < len(s.entries); i++ {
			if s.entries[i].Depth <= entryDepth {
				break
			}
			// Also remove any nested expanded dirs from the map
			if s.entries[i].IsDir && s.expandedDirs[s.entries[i].Path] {
				delete(s.expandedDirs, s.entries[i].Path)
			}
			removeEnd = i + 1
		}
	}

	// Remove the entries
	newEntries := make([]ui.UIEntry, 0, len(s.entries)-(removeEnd-removeStart))
	newEntries = append(newEntries, s.entries[:removeStart]...)
	newEntries = append(newEntries, s.entries[removeEnd:]...)
	s.entries = newEntries

	s.invalidate()
}

// RemoveEntries removes multiple entries from the current view
func (s *StateOwner) RemoveEntries(paths []string) {
	// Remove in reverse order to avoid index shifting issues
	// But since we're using paths, we can just call RemoveEntry for each
	for _, path := range paths {
		s.RemoveEntry(path)
	}
}

// RefreshExpandedDir refreshes children of an expanded directory
func (s *StateOwner) RefreshExpandedDir(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.expandedDirs[path] {
		return
	}

	// Find parent
	parentIdx := -1
	parentDepth := 0
	for i, entry := range s.entries {
		if entry.Path == path {
			parentIdx = i
			parentDepth = entry.Depth
			break
		}
	}

	if parentIdx < 0 {
		return
	}

	// Read fresh children
	children := s.readDirLocked(path)
	children = s.filterLocked(children)
	s.sortLocked(children)

	for i := range children {
		children[i].Depth = parentDepth + 1
		children[i].ParentPath = path
		children[i].IsExpanded = s.expandedDirs[children[i].Path]
	}

	// Find and remove old children
	removeStart := parentIdx + 1
	removeEnd := removeStart
	for i := removeStart; i < len(s.entries); i++ {
		if s.entries[i].Depth <= parentDepth {
			break
		}
		if s.entries[i].ParentPath == path {
			removeEnd = i + 1
		}
	}

	// Replace with new children
	newEntries := make([]ui.UIEntry, 0, len(s.entries)-(removeEnd-removeStart)+len(children))
	newEntries = append(newEntries, s.entries[:removeStart]...)
	newEntries = append(newEntries, children...)
	if removeEnd < len(s.entries) {
		newEntries = append(newEntries, s.entries[removeEnd:]...)
	}
	s.entries = newEntries

	s.invalidate()
}

// --- Internal methods (must be called with lock held) ---

func (s *StateOwner) rebuildLocked() {
	// Start from raw entries, apply filter and sort
	s.entries = s.filterLocked(s.rawEntries)
	s.sortLocked(s.entries)

	// Re-apply expansions
	s.applyExpansionsLocked()
}

func (s *StateOwner) applyExpansionsLocked() {
	// For each expanded dir, insert its children
	// Need to process in order to maintain tree structure
	for i := 0; i < len(s.entries); i++ {
		entry := &s.entries[i]
		if entry.IsDir && s.expandedDirs[entry.Path] {
			entry.IsExpanded = true

			children := s.readDirLocked(entry.Path)
			children = s.filterLocked(children)
			s.sortLocked(children)

			for j := range children {
				children[j].Depth = entry.Depth + 1
				children[j].ParentPath = entry.Path
				children[j].IsExpanded = s.expandedDirs[children[j].Path]
			}

			// Insert children
			newEntries := make([]ui.UIEntry, 0, len(s.entries)+len(children))
			newEntries = append(newEntries, s.entries[:i+1]...)
			newEntries = append(newEntries, children...)
			newEntries = append(newEntries, s.entries[i+1:]...)
			s.entries = newEntries

			// Skip past children we just inserted
			i += len(children)
		}
	}
}

func (s *StateOwner) readDirLocked(path string) []ui.UIEntry {
	dirEntries, err := os.ReadDir(path)
	if err != nil {
		debug.Log(debug.APP, "StateOwner.readDirLocked error: %v", err)
		return nil
	}

	entries := make([]ui.UIEntry, 0, len(dirEntries))
	for _, e := range dirEntries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		entries = append(entries, ui.UIEntry{
			Name:    e.Name(),
			Path:    filepath.Join(path, e.Name()),
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}
	return entries
}

func (s *StateOwner) filterLocked(entries []ui.UIEntry) []ui.UIEntry {
	if s.showDotfiles {
		result := make([]ui.UIEntry, len(entries))
		copy(result, entries)
		return result
	}

	result := make([]ui.UIEntry, 0, len(entries))
	for _, e := range entries {
		if !strings.HasPrefix(e.Name, ".") {
			result = append(result, e)
		}
	}
	return result
}

func (s *StateOwner) sortLocked(entries []ui.UIEntry) {
	sort.Slice(entries, func(i, j int) bool {
		// Directories first
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}

		var less bool
		switch s.sortColumn {
		case ui.SortByDate:
			less = entries[i].ModTime.Before(entries[j].ModTime)
		case ui.SortBySize:
			less = entries[i].Size < entries[j].Size
		case ui.SortByType:
			extI := strings.ToLower(filepath.Ext(entries[i].Name))
			extJ := strings.ToLower(filepath.Ext(entries[j].Name))
			if extI == extJ {
				less = strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
			} else {
				less = extI < extJ
			}
		default: // SortByName
			less = strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		}

		if !s.sortAsc {
			return !less
		}
		return less
	})
}

func (s *StateOwner) invalidate() {
	if s.window != nil {
		s.window.Invalidate()
	}
}

// --- Helper functions ---

func copyStringBoolMap(m map[string]bool) map[string]bool {
	result := make(map[string]bool, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
