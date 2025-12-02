package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/justyntemme/razor/internal/debug"
	"github.com/justyntemme/razor/internal/ui"
)

// TabState holds the navigation state for a single tab
type TabState struct {
	ID           string       // Unique identifier
	CurrentPath  string       // Current directory path
	History      []string     // Navigation history
	HistoryIndex int          // Current position in history
	DirEntries   []ui.UIEntry // Cached directory entries
	RawEntries   []ui.UIEntry // Raw entries (before filter)
	SelectedIdx  int          // Selected item index
}

// createNewTab creates a new tab based on config settings
func (o *Orchestrator) createNewTab() {
	// Save current tab state before creating new tab
	o.saveCurrentTabState()

	o.tabCounter++
	id := fmt.Sprintf("tab-%d", o.tabCounter)

	// Determine new tab path based on config
	tabsCfg := o.config.GetTabsConfig()
	newTabPath := o.state.CurrentPath // default to current

	switch tabsCfg.NewTabLocation {
	case "home":
		newTabPath = o.sharedDeps.HomePath
	case "recent":
		// For "recent", we'll navigate to recent files view after tab creation
		newTabPath = o.sharedDeps.HomePath // Start at home, then switch to recent
	case "current":
		newTabPath = o.state.CurrentPath
	default:
		// Treat as custom path
		if tabsCfg.NewTabLocation != "" {
			// Check if it's a valid path
			if info, err := os.Stat(tabsCfg.NewTabLocation); err == nil && info.IsDir() {
				newTabPath = tabsCfg.NewTabLocation
			}
		}
	}

	// Create new tab state
	tab := TabState{
		ID:           id,
		CurrentPath:  newTabPath,
		History:      []string{newTabPath},
		HistoryIndex: 0,
		DirEntries:   nil, // Will be populated when we navigate
		RawEntries:   nil,
		SelectedIdx:  -1,
	}

	o.tabs = append(o.tabs, tab)

	// Enable tabs now that we have more than one
	o.ui.EnableTabs(true)

	// Add to UI
	title := filepath.Base(newTabPath)
	if title == "" || title == "/" || title == "." {
		title = newTabPath
	}
	newIdx := o.ui.AddTab(id, title, newTabPath)
	o.ui.SetActiveTab(newIdx)
	o.activeTabIndex = newIdx

	debug.Log(debug.APP, "Created new tab %s at %s (index %d)", id, newTabPath, newIdx)

	// Navigate to the new path (or show recent files)
	if tabsCfg.NewTabLocation == "recent" {
		o.showRecentFiles()
	} else {
		o.navCtrl.Navigate(newTabPath)
	}

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
		DirEntries:   nil,
		RawEntries:   nil,
		SelectedIdx:  -1,
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

	// Get the path of the closing tab for watcher cleanup
	closingPath := o.tabs[index].CurrentPath

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

	// Unwatch the closed tab's directory if no other tab is viewing it
	if o.watcher != nil && closingPath != "" {
		stillWatching := false
		for _, tab := range o.tabs {
			if tab.CurrentPath == closingPath {
				stillWatching = true
				break
			}
		}
		if !stillWatching {
			o.watcher.Unwatch(closingPath)
			debug.Log(debug.APP, "Unwatched directory (tab closed): %s", closingPath)
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

// nextTab switches to the next tab (wraps around)
func (o *Orchestrator) nextTab() {
	if len(o.tabs) <= 1 {
		return
	}
	nextIdx := (o.activeTabIndex + 1) % len(o.tabs)
	o.switchToTab(nextIdx)
}

// prevTab switches to the previous tab (wraps around)
func (o *Orchestrator) prevTab() {
	if len(o.tabs) <= 1 {
		return
	}
	prevIdx := o.activeTabIndex - 1
	if prevIdx < 0 {
		prevIdx = len(o.tabs) - 1
	}
	o.switchToTab(prevIdx)
}

// saveCurrentTabState saves the current orchestrator state to the active tab
func (o *Orchestrator) saveCurrentTabState() {
	if o.activeTabIndex < 0 || o.activeTabIndex >= len(o.tabs) {
		return
	}

	tab := &o.tabs[o.activeTabIndex]
	tab.CurrentPath = o.state.CurrentPath
	tab.History = make([]string, len(o.navCtrl.History))
	copy(tab.History, o.navCtrl.History)
	tab.HistoryIndex = o.navCtrl.HistoryIndex
	tab.DirEntries = make([]ui.UIEntry, len(o.dirEntries))
	copy(tab.DirEntries, o.dirEntries)
	tab.RawEntries = make([]ui.UIEntry, len(o.rawEntries))
	copy(tab.RawEntries, o.rawEntries)
	tab.SelectedIdx = o.state.SelectedIndex
}

// loadTabState loads state from the given tab into the orchestrator
func (o *Orchestrator) loadTabState(index int) {
	if index < 0 || index >= len(o.tabs) {
		return
	}

	tab := &o.tabs[index]

	// Restore navigation history
	o.navCtrl.History = make([]string, len(tab.History))
	copy(o.navCtrl.History, tab.History)
	o.navCtrl.HistoryIndex = tab.HistoryIndex

	// Restore entries
	o.dirEntries = make([]ui.UIEntry, len(tab.DirEntries))
	copy(o.dirEntries, tab.DirEntries)
	o.rawEntries = make([]ui.UIEntry, len(tab.RawEntries))
	copy(o.rawEntries, tab.RawEntries)

	// Restore state
	o.stateMu.Lock()
	o.state.CurrentPath = tab.CurrentPath
	o.state.SelectedIndex = tab.SelectedIdx
	o.applyFilterAndSort()
	o.state.CanBack = o.navCtrl.HistoryIndex > 0
	o.state.CanForward = o.navCtrl.HistoryIndex < len(o.navCtrl.History)-1
	o.stateMu.Unlock()

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
	o.tabs = []TabState{{
		ID:           id,
		CurrentPath:  startPath,
		History:      []string{startPath},
		HistoryIndex: 0,
		DirEntries:   nil,
		RawEntries:   nil,
		SelectedIdx:  -1,
	}}
	o.activeTabIndex = 0

	// Add initial tab to UI but keep tabs disabled (hidden) until second tab is created
	o.ui.EnableTabs(false)
	o.ui.AddTab(id, title, startPath)
	o.ui.SetActiveTab(0)
}
