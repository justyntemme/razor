package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/justyntemme/razor/internal/debug"
)

// TabState holds the navigation state for a single tab
// NOTE: Entry data is NOT stored here - it lives in StateOwner
// Tabs only store metadata needed to restore state when switching
type TabState struct {
	ID           string          // Unique identifier
	CurrentPath  string          // Current directory path
	History      []string        // Navigation history
	HistoryIndex int             // Current position in history
	SelectedIdx  int             // Selected item index
	ExpandedDirs map[string]bool // Expanded directories in tree view
}

// createNewTabInCurrentDir creates a new tab in the current directory
func (o *Orchestrator) createNewTabInCurrentDir() {
	o.createTabAtPath(o.state.CurrentPath)
}

// createNewTabInHome creates a new tab in the home directory
func (o *Orchestrator) createNewTabInHome() {
	o.createTabAtPath(o.sharedDeps.HomePath)
}

// createTabAtPath creates a new tab at the specified path
func (o *Orchestrator) createTabAtPath(path string) {
	// Save current tab state before creating new tab
	o.saveCurrentTabState()

	o.tabCounter++
	id := fmt.Sprintf("tab-%d", o.tabCounter)

	// Create new tab state
	tab := TabState{
		ID:           id,
		CurrentPath:  path,
		History:      []string{path},
		HistoryIndex: 0,
		SelectedIdx:  -1,
		ExpandedDirs: make(map[string]bool),
	}

	o.tabs = append(o.tabs, tab)

	// Enable tabs now that we have more than one
	o.ui.EnableTabs(true)

	// Add to UI
	title := filepath.Base(path)
	if title == "" || title == "/" || title == "." {
		title = path
	}
	newIdx := o.ui.AddTab(id, title, path)
	o.ui.SetActiveTab(newIdx)
	o.activeTabIndex = newIdx

	debug.Log(debug.APP, "Created new tab %s at %s (index %d)", id, path, newIdx)

	o.navCtrl.Navigate(path)
	o.window.Invalidate()
}

// openPathInNewTab creates a new tab and navigates to the specified path
func (o *Orchestrator) openPathInNewTab(path string) {
	// Validate path is a directory
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		debug.Log(debug.APP, "openPathInNewTab: invalid directory path %s", path)
		return
	}

	// Save current tab state before creating new tab
	o.saveCurrentTabState()

	o.tabCounter++
	id := fmt.Sprintf("tab-%d", o.tabCounter)

	// Create new tab state
	tab := TabState{
		ID:           id,
		CurrentPath:  path,
		History:      []string{path},
		HistoryIndex: 0,
		SelectedIdx:  -1,
		ExpandedDirs: make(map[string]bool),
	}

	o.tabs = append(o.tabs, tab)

	// Enable tabs now that we have more than one
	o.ui.EnableTabs(true)

	// Add to UI
	title := filepath.Base(path)
	if title == "" || title == "/" || title == "." {
		title = path
	}
	newIdx := o.ui.AddTab(id, title, path)
	o.ui.SetActiveTab(newIdx)
	o.activeTabIndex = newIdx

	debug.Log(debug.APP, "Opened path in new tab %s at %s (index %d)", id, path, newIdx)

	// Navigate to the path
	o.navCtrl.Navigate(path)

	o.window.Invalidate()
}

// closeTab closes the tab at the given index
func (o *Orchestrator) closeTab(index int) {
	if index < 0 || index >= len(o.tabs) {
		return
	}

	// Don't close the last tab
	if len(o.tabs) <= 1 {
		debug.Log(debug.APP, "Cannot close last tab")
		return
	}

	debug.Log(debug.APP, "Closing tab %d", index)

	// Get the paths of the closing tab for watcher cleanup
	closingTab := o.tabs[index]
	closingPath := closingTab.CurrentPath
	closingExpandedDirs := closingTab.ExpandedDirs

	// Remove from our state
	o.tabs = append(o.tabs[:index], o.tabs[index+1:]...)

	// Tell UI to close it and get the new active index
	newActiveIdx := o.ui.CloseTab(index)

	// Hide tab bar if we're back to single tab
	if len(o.tabs) == 1 {
		o.ui.EnableTabs(false)
	}

	// Switch to the new active tab
	if newActiveIdx >= 0 && newActiveIdx < len(o.tabs) {
		o.activeTabIndex = newActiveIdx
		o.loadTabState(newActiveIdx)
	}

	// Unwatch the closed tab's directories if no other tab is viewing them
	if o.watcher != nil {
		// Collect all paths that are still being watched by other tabs
		stillWatched := make(map[string]bool)
		for _, tab := range o.tabs {
			stillWatched[tab.CurrentPath] = true
			for path := range tab.ExpandedDirs {
				stillWatched[path] = true
			}
		}

		// Unwatch the main path if not used by other tabs
		if closingPath != "" && !stillWatched[closingPath] {
			o.watcher.Unwatch(closingPath)
			debug.Log(debug.APP, "Unwatched directory (tab closed): %s", closingPath)
		}

		// Unwatch expanded directories if not used by other tabs
		for path := range closingExpandedDirs {
			if !stillWatched[path] {
				o.watcher.Unwatch(path)
				debug.Log(debug.APP, "Unwatched expanded directory (tab closed): %s", path)
			}
		}
	}

	o.window.Invalidate()
}

// switchToTab switches to the tab at the given index
func (o *Orchestrator) switchToTab(index int) {
	if index < 0 || index >= len(o.tabs) || index == o.activeTabIndex {
		return
	}

	debug.Log(debug.APP, "Switching from tab %d to tab %d", o.activeTabIndex, index)

	// Save current tab state
	o.saveCurrentTabState()

	// Switch to new tab
	o.activeTabIndex = index
	o.ui.SetActiveTab(index)

	// Load new tab state
	o.loadTabState(index)

	o.window.Invalidate()
}

// cycleTab switches to an adjacent tab with wrapping.
// delta should be +1 for next or -1 for previous.
func (o *Orchestrator) cycleTab(delta int) {
	if len(o.tabs) <= 1 {
		return
	}
	newIdx := (o.activeTabIndex + delta + len(o.tabs)) % len(o.tabs)
	o.switchToTab(newIdx)
}

// nextTab switches to the next tab (wraps around)
func (o *Orchestrator) nextTab() {
	o.cycleTab(1)
}

// prevTab switches to the previous tab (wraps around)
func (o *Orchestrator) prevTab() {
	o.cycleTab(-1)
}

// saveCurrentTabState saves the current orchestrator state to the active tab
// NOTE: Entry data is managed by StateOwner, not stored in tabs
func (o *Orchestrator) saveCurrentTabState() {
	if o.activeTabIndex < 0 || o.activeTabIndex >= len(o.tabs) {
		return
	}

	tab := &o.tabs[o.activeTabIndex]
	tab.CurrentPath = o.state.CurrentPath
	tab.History = make([]string, len(o.navCtrl.History))
	copy(tab.History, o.navCtrl.History)
	tab.HistoryIndex = o.navCtrl.HistoryIndex
	tab.SelectedIdx = o.state.SelectedIndex

	// Save expanded directories state from StateOwner
	tab.ExpandedDirs = make(map[string]bool)
	for _, path := range o.stateOwner.GetExpandedDirs() {
		tab.ExpandedDirs[path] = true
	}
}

// loadTabState loads state from the given tab into the orchestrator
// Entry data will be re-fetched from filesystem via StateOwner
func (o *Orchestrator) loadTabState(index int) {
	if index < 0 || index >= len(o.tabs) {
		return
	}

	tab := &o.tabs[index]

	// Restore navigation history
	o.navCtrl.History = make([]string, len(tab.History))
	copy(o.navCtrl.History, tab.History)
	o.navCtrl.HistoryIndex = tab.HistoryIndex

	// Clear and restore expanded directories in StateOwner (before fetching dir)
	o.stateOwner.ClearExpanded()
	// Also clear and restore expanded dirs in UI renderer
	o.ui.ClearExpanded()
	if tab.ExpandedDirs != nil {
		for path := range tab.ExpandedDirs {
			o.ui.SetExpanded(path, true)
		}
	}

	// Restore state
	o.stateMu.Lock()
	o.state.CurrentPath = tab.CurrentPath
	o.state.SelectedIndex = tab.SelectedIdx
	o.state.CanBack = o.navCtrl.HistoryIndex > 0
	o.state.CanForward = o.navCtrl.HistoryIndex < len(o.navCtrl.History)-1
	o.stateMu.Unlock()

	// Re-fetch directory contents - StateOwner will apply expansions
	// This ensures we always have fresh data
	o.navCtrl.RequestDir(tab.CurrentPath)

	// Watch the tab's current directory and expanded directories
	if o.watcher != nil {
		o.watcher.Watch(tab.CurrentPath)
		if tab.ExpandedDirs != nil {
			for path := range tab.ExpandedDirs {
				o.watcher.Watch(path)
			}
		}
	}

	// Update tab title in UI
	title := filepath.Base(tab.CurrentPath)
	if title == "" || title == "/" || title == "." {
		title = tab.CurrentPath
	}
	o.ui.UpdateTabTitle(index, title)
}

// initializeTabs sets up the initial tab (hidden until a second tab is created)
func (o *Orchestrator) initializeTabs(startPath string) {
	o.tabCounter = 1
	id := "tab-1"

	title := filepath.Base(startPath)
	if title == "" || title == "/" || title == "." {
		title = startPath
	}

	// Create initial tab state (but keep tab bar hidden)
	// NOTE: No entry copies - StateOwner manages all entry data
	o.tabs = []TabState{{
		ID:           id,
		CurrentPath:  startPath,
		History:      []string{startPath},
		HistoryIndex: 0,
		SelectedIdx:  -1,
		ExpandedDirs: make(map[string]bool),
	}}
	o.activeTabIndex = 0

	// Add initial tab to UI but keep tabs disabled (hidden) until second tab is created
	o.ui.EnableTabs(false)
	o.ui.AddTab(id, title, startPath)
	o.ui.SetActiveTab(0)
}
