package app

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"gioui.org/app"
	"gioui.org/op"

	"github.com/justyntemme/razor/internal/config"
	"github.com/justyntemme/razor/internal/debug"
	"github.com/justyntemme/razor/internal/fs"
	"github.com/justyntemme/razor/internal/search"
	"github.com/justyntemme/razor/internal/store"
	"github.com/justyntemme/razor/internal/trash"
	"github.com/justyntemme/razor/internal/ui"
)

const (
	maxHistorySize       = 100                      // Limit history to prevent unbounded memory growth
	progressThrottleTime = 100 * time.Millisecond  // Minimum interval between progress updates
)

// Orchestrator is the central coordinator that wires together all components.
// It owns shared state and dependencies, delegating domain-specific logic to controllers.
type Orchestrator struct {
	// Core dependencies
	window  *app.Window
	fs      *fs.System
	store   *store.DB
	config  *config.Manager
	ui      *ui.Renderer
	watcher *DirectoryWatcher // Directory change watcher

	// Shared state (protected by stateMu)
	stateMu    sync.RWMutex
	state      ui.State
	stateOwner *StateOwner   // Single source of truth for entries
	searchGen  atomic.Int64  // Search generation counter

	// Controllers (own their domain-specific state, share deps/state via pointers)
	searchCtrl *SearchController
	navCtrl    *NavigationController

	// Sorting state
	sortColumn   ui.SortColumn
	sortAsc      bool
	showDotfiles bool

	// Tab state
	tabs           []TabState
	activeTabIndex int
	tabCounter     int // For generating unique IDs

	// Progress tracking
	progressMu         sync.Mutex
	lastProgressUpdate time.Time
	progressThrottleMu sync.Mutex

	// Conflict resolution state
	conflictResolution ui.ConflictResolution
	conflictResponse   chan ui.ConflictResolution
	conflictAbort      bool

	// Shared dependencies for controllers (set during init)
	sharedDeps  *SharedDeps
	sharedState *SharedState
}

func NewOrchestrator() *Orchestrator {
	home, _ := os.UserHomeDir()

	// Initialize config manager and load config
	cfgMgr := config.NewManager()
	if err := cfgMgr.Load(); err != nil {
		log.Printf("Warning: Failed to load config: %v", err)
	}
	cfg := cfgMgr.Get()

	// Detect available search engines
	engines := search.DetectEngines()

	// Convert to UI format
	uiEngines := make([]ui.SearchEngineInfo, len(engines))
	for i, e := range engines {
		uiEngines[i] = ui.SearchEngineInfo{
			Name:      e.Name,
			ID:        e.Engine.String(),
			Command:   e.Command,
			Available: e.Available,
			Version:   e.Version,
		}
	}

	// Create window first so StateOwner can reference it
	window := new(app.Window)

	// Create core orchestrator
	o := &Orchestrator{
		window:           window,
		fs:               fs.NewSystem(),
		store:            store.NewDB(),
		config:           cfgMgr,
		ui:               ui.NewRenderer(),
		state:            ui.State{SelectedIndex: -1, Favorites: make(map[string]bool)},
		stateOwner:       NewStateOwner(window, cfg.UI.FileList.ShowDotfiles),
		sortAsc:          cfg.UI.FileList.SortAscending,
		showDotfiles:     cfg.UI.FileList.ShowDotfiles,
		conflictResponse: make(chan ui.ConflictResolution, 1),
	}

	// Create shared dependencies and state for controllers
	o.sharedDeps = &SharedDeps{
		Window:   o.window,
		FS:       o.fs,
		Store:    o.store,
		UI:       o.ui,
		HomePath: home,
	}

	o.sharedState = &SharedState{
		State:     &o.state,
		SearchGen: &o.searchGen,
	}

	// Create controllers with shared dependencies
	o.searchCtrl = NewSearchController(o.sharedDeps, o.sharedState, engines)
	o.searchCtrl.DefaultDepth = cfg.Search.DefaultDepth

	o.navCtrl = NewNavigationController(o.sharedDeps, o.sharedState)

	// Set up UI with detected engines
	o.ui.SearchEngines = uiEngines

	// Detect and set up available terminals
	terminals := config.DetectTerminals()
	var uiTerminals []ui.TerminalInfo
	for _, t := range terminals {
		uiTerminals = append(uiTerminals, ui.TerminalInfo{
			ID:      t.ID,
			Name:    t.Name,
			Default: t.Default,
		})
	}
	o.ui.SetTerminals(uiTerminals)

	// Set configured terminal (or default if not set)
	configuredTerminal := cfg.Terminal.App
	if configuredTerminal == "" {
		configuredTerminal = config.DefaultTerminalID()
	}
	o.ui.SetSelectedTerminal(configuredTerminal)

	// Apply config to UI
	o.ui.ShowDotfiles = cfg.UI.FileList.ShowDotfiles
	o.ui.SetShowDotfilesCheck(cfg.UI.FileList.ShowDotfiles)
	o.ui.SetDefaultDepth(cfg.Search.DefaultDepth)
	o.ui.SetDarkMode(cfg.UI.Theme == "dark")
	o.ui.SetSidebarLayout(cfg.UI.Sidebar.Layout)
	o.ui.SetSidebarTabStyle(cfg.UI.Sidebar.TabStyle)

	// Set preview pane config
	o.ui.SetPreviewConfig(cfg.Preview.TextExtensions, cfg.Preview.ImageExtensions, cfg.Preview.MaxFileSize, cfg.Preview.WidthPercent, cfg.Preview.MarkdownRendered)

	// Set hotkeys from config
	o.ui.SetHotkeys(cfg.Hotkeys)

	// Set search engine from config
	o.searchCtrl.ChangeEngine(cfg.Search.Engine)

	// Set config error banner if config failed to parse
	if parseErr := cfgMgr.ParseError(); parseErr != nil {
		o.ui.SetConfigError(parseErr.Error())
	}

	// Load favorites from config into state
	o.loadFavoritesFromConfig()

	return o
}

// loadFavoritesFromConfig loads favorites from config.json into the state
func (o *Orchestrator) loadFavoritesFromConfig() {
	favorites := o.config.GetFavorites()
	o.state.Favorites = make(map[string]bool)
	// +1 for Trash entry at the end
	o.state.FavList = make([]ui.FavoriteItem, 0, len(favorites)+1)

	homePath := o.sharedDeps.HomePath
	for _, fav := range favorites {
		// Skip groups for now (flat list only)
		if fav.Type == "group" {
			// TODO: Handle favorite groups in the future
			continue
		}
		// Expand ~ in path
		path := fav.Path
		if len(path) > 0 && path[0] == '~' {
			path = filepath.Join(homePath, path[1:])
		}
		o.state.Favorites[path] = true
		o.state.FavList = append(o.state.FavList, ui.FavoriteItem{
			Path: path,
			Name: fav.Name,
			Type: ui.FavoriteTypeNormal,
		})
	}

	// Always add Trash at the end if available
	if trash.IsAvailable() {
		o.state.FavList = append(o.state.FavList, ui.FavoriteItem{
			Path: trash.GetPath(),
			Name: trash.DisplayName(),
			Type: ui.FavoriteTypeTrash,
		})
	}
}

func (o *Orchestrator) Run(startPath string) error {
	if debug.Enabled {
		log.Println("Starting Razor in DEBUG mode")
		debug.Log(debug.APP, "Debug categories enabled: %v", debug.ListEnabled())
	}

	// Database is still used for search history
	// Use ~/.config/razor/ on all platforms for consistency
	dbPath := filepath.Join(o.sharedDeps.HomePath, ".config", "razor", "razor.db")
	debug.Log(debug.APP, "Opening database: %s", dbPath)
	if err := o.store.Open(dbPath); err != nil {
		log.Printf("Failed to open DB: %v", err)
	}
	defer o.store.Close()

	// Initialize directory watcher for live updates
	watcher, err := NewDirectoryWatcher(200) // 200ms debounce
	if err != nil {
		log.Printf("Warning: Failed to create directory watcher: %v", err)
	} else {
		o.watcher = watcher
		defer o.watcher.Close()
	}

	go o.fs.Start()
	go o.store.Start()
	go o.processEvents()

	// Favorites are now loaded from config.json in NewOrchestrator
	// Settings are also loaded from config.json

	// Load drives
	o.refreshDrives()

	if startPath == "" {
		startPath = o.sharedDeps.HomePath
		if startPath == "" {
			startPath, _ = os.Getwd()
		}
	}

	// Initialize tabs with the starting path
	o.initializeTabs(startPath)

	o.navCtrl.Navigate(startPath)

	var ops op.Ops
	for {
		switch e := o.window.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			// Lock state for reading during UI layout
			o.stateMu.RLock()
			evt := o.ui.Layout(gtx, &o.state)
			o.stateMu.RUnlock()

			if evt.Action != ui.ActionNone {
				debug.Log(debug.UI_EVENT, "Action: %d Path: %s Index: %d", evt.Action, evt.Path, evt.NewIndex)
			}

			o.handleUIEvent(evt)
			e.Frame(gtx.Ops)
		}
	}
}

func (o *Orchestrator) refreshDrives() {
	// Load drives asynchronously to avoid blocking UI on slow/disconnected drives
	go func() {
		drives := fs.ListDrives()
		driveItems := make([]ui.DriveItem, len(drives))
		for i, d := range drives {
			driveItems[i] = ui.DriveItem{Name: d.Name, Path: d.Path}
		}
		o.stateMu.Lock()
		o.state.Drives = driveItems
		o.stateMu.Unlock()
		o.window.Invalidate()
	}()
}

// resetUIState cancels any active rename, hides preview, exits recent/trash view,
// and clears multi-select mode. Called before navigation operations to ensure clean state.
func (o *Orchestrator) resetUIState() {
	o.ui.CancelRename()
	o.ui.HidePreview()
	o.ui.SetRecentView(false)
	o.ui.SetTrashView(false)
	o.ui.ResetMultiSelect()
	o.state.SelectedIndices = nil
	o.state.SelectedIndex = -1
}

// restoreDirectory restores the cached directory entries after a search is cleared.
// Returns true if entries were restored, false if a re-fetch is needed.
func (o *Orchestrator) restoreDirectory() bool {
	// StateOwner manages raw entries internally - request a refresh
	snapshot := o.stateOwner.GetSnapshot()
	if len(snapshot.Entries) > 0 {
		o.stateMu.Lock()
		o.state.Entries = snapshot.Entries
		o.stateMu.Unlock()
		return true
	}
	return false
}

func (o *Orchestrator) handleUIEvent(evt ui.UIEvent) {
	switch evt.Action {
	case ui.ActionNavigate:
		o.resetUIState()
		expandedPath := o.navCtrl.ExpandPath(evt.Path)
		if exists, isDir := o.navCtrl.ValidatePath(expandedPath); exists && isDir {
			o.navCtrl.Navigate(expandedPath)
		} else if exists && !isDir {
			// It's a file, open it instead
			if err := platformOpen(expandedPath); err != nil {
				log.Printf("Error opening file: %v", err)
			}
			o.store.RequestChan <- store.Request{Op: store.AddRecentFile, Path: expandedPath}
		} else {
			log.Printf("Path does not exist: %s (expanded from: %s)", expandedPath, evt.Path)
		}
	case ui.ActionBack:
		o.resetUIState()
		o.navCtrl.GoBack(nil)
	case ui.ActionForward:
		o.resetUIState()
		o.navCtrl.GoForward()
	case ui.ActionHome:
		o.resetUIState()
		o.navCtrl.GoHome()
	case ui.ActionNewWindow:
		o.openNewWindow()
	case ui.ActionSelect:
		o.state.SelectedIndex = evt.NewIndex
		// Clear multi-select when doing single select
		o.state.SelectedIndices = nil
		// Show preview for selected file if it's a previewable type
		if evt.NewIndex >= 0 && evt.NewIndex < len(o.state.Entries) {
			entry := &o.state.Entries[evt.NewIndex]
			if !entry.IsDir {
				o.ui.ShowPreview(entry.Path)
			} else {
				o.ui.HidePreview()
			}
		}
		o.window.Invalidate()
	case ui.ActionToggleSelect:
		// Initialize SelectedIndices map if needed
		if o.state.SelectedIndices == nil {
			o.state.SelectedIndices = make(map[int]bool)
		}
		// If OldIndex is set, we're entering multi-select mode - add the old selection first
		if evt.OldIndex >= 0 && evt.OldIndex != evt.NewIndex {
			o.state.SelectedIndices[evt.OldIndex] = true
		}
		// Toggle the new index
		wasSelected := o.state.SelectedIndices[evt.NewIndex]
		if wasSelected {
			delete(o.state.SelectedIndices, evt.NewIndex)
		} else {
			o.state.SelectedIndices[evt.NewIndex] = true
		}

		// Update primary selection based on toggle result
		if wasSelected {
			// Item was unchecked - pick another selected item as primary, or -1 if none
			o.state.SelectedIndex = -1
			for idx := range o.state.SelectedIndices {
				o.state.SelectedIndex = idx
				break
			}
		} else {
			// Item was checked - make it the primary selection
			o.state.SelectedIndex = evt.NewIndex
		}

		// If no items left selected, clear multi-select state
		if len(o.state.SelectedIndices) == 0 {
			o.state.SelectedIndices = nil
			o.ui.ResetMultiSelect()
		}

		// Hide preview in multi-select mode (multiple items selected)
		if len(o.state.SelectedIndices) > 1 {
			o.ui.HidePreview()
		} else if len(o.state.SelectedIndices) == 1 && o.state.SelectedIndex >= 0 {
			// Single item selected - show preview if it's a file
			if o.state.SelectedIndex < len(o.state.Entries) {
				entry := &o.state.Entries[o.state.SelectedIndex]
				if !entry.IsDir {
					o.ui.ShowPreview(entry.Path)
				}
			}
		}
		o.window.Invalidate()
	case ui.ActionRangeSelect:
		// Select all items from OldIndex to NewIndex (inclusive)
		if o.state.SelectedIndices == nil {
			o.state.SelectedIndices = make(map[int]bool)
		}
		start, end := evt.OldIndex, evt.NewIndex
		if start > end {
			start, end = end, start
		}
		for i := start; i <= end; i++ {
			o.state.SelectedIndices[i] = true
		}
		o.state.SelectedIndex = evt.NewIndex
		o.ui.HidePreview() // Hide preview in multi-select mode
		o.window.Invalidate()
	case ui.ActionClearSelection:
		o.state.SelectedIndex = -1
		o.state.SelectedIndices = nil
		o.ui.HidePreview()
		o.window.Invalidate()
	case ui.ActionSearch:
		o.searchCtrl.DoSearch(evt.Path, evt.SearchSubmitted, o.restoreDirectory, o.setProgress)
	case ui.ActionOpen:
		if err := platformOpen(evt.Path); err != nil {
			log.Printf("Error opening file: %v", err)
		}
		// Track in recent files
		o.store.RequestChan <- store.Request{Op: store.AddRecentFile, Path: evt.Path}
	case ui.ActionOpenWith:
		// Show the system "Open With" dialog
		if err := platformOpenWith(evt.Path, ""); err != nil {
			log.Printf("Error showing Open With dialog: %v", err)
		}
	case ui.ActionOpenWithApp:
		// Open with a specific application
		if err := platformOpenWith(evt.Path, evt.AppPath); err != nil {
			log.Printf("Error opening with app: %v", err)
		}
	case ui.ActionOpenTerminal:
		// Open terminal in the specified directory using configured terminal app
		terminalApp := o.config.GetTerminalApp()
		if err := platformOpenTerminal(evt.Path, terminalApp); err != nil {
			log.Printf("Error opening terminal: %v", err)
		}
	case ui.ActionChangeTerminal:
		// Update terminal app in config
		o.config.SetTerminalApp(evt.TerminalApp)
	case ui.ActionAddFavorite:
		// Add favorite to config.json
		name := filepath.Base(evt.Path)
		o.config.AddFavorite(name, evt.Path)
		o.loadFavoritesFromConfig()
		o.window.Invalidate()
	case ui.ActionRemoveFavorite:
		// Remove favorite from config.json
		o.config.RemoveFavorite(evt.Path)
		o.loadFavoritesFromConfig()
		o.window.Invalidate()
	case ui.ActionSort:
		o.sortColumn, o.sortAsc = evt.SortColumn, evt.SortAscending
		o.stateOwner.SetSort(evt.SortColumn, evt.SortAscending)
		o.applyFilterAndSort()
		o.window.Invalidate()
	case ui.ActionToggleDotfiles:
		debug.Log(debug.APP, "ActionToggleDotfiles received! ShowDotfiles=%v", evt.ShowDotfiles)
		o.showDotfiles = evt.ShowDotfiles
		o.config.SetShowDotfiles(o.showDotfiles)
		o.stateOwner.ToggleDotfiles(evt.ShowDotfiles)
		o.applyFilterAndSort()
		o.window.Invalidate()
	case ui.ActionCopy:
		o.state.Clipboard = &ui.Clipboard{Paths: evt.Paths, Op: ui.ClipCopy}
		// Clear selections after copying
		o.state.SelectedIndex = -1
		o.state.SelectedIndices = nil
		o.ui.ResetMultiSelect()
		o.window.Invalidate()
	case ui.ActionCut:
		o.state.Clipboard = &ui.Clipboard{Paths: evt.Paths, Op: ui.ClipCut}
		// Clear selections after cutting
		o.state.SelectedIndex = -1
		o.state.SelectedIndices = nil
		o.ui.ResetMultiSelect()
		o.window.Invalidate()
	case ui.ActionPaste:
		if o.state.Clipboard != nil {
			go o.doPaste()
		}
	case ui.ActionConfirmDelete:
		// Support deleting multiple files
		if len(evt.Paths) > 0 {
			go o.doDeleteMultiple(evt.Paths)
		} else if evt.Path != "" {
			go o.doDelete(evt.Path)
		}
	case ui.ActionCreateFile:
		go o.doCreateFile(evt.FileName)
	case ui.ActionCreateFolder:
		go o.doCreateFolder(evt.FileName)
	case ui.ActionRename:
		go o.doRename(evt.OldPath, evt.Path)
	case ui.ActionClearSearch:
		debug.Log(debug.APP, "ClearSearch: cancelling search")
		o.searchCtrl.CancelSearch(o.setProgress)
		// Restore from StateOwner (no disk access needed if cached)
		if !o.restoreDirectory() {
			// Fallback: re-fetch if entries are empty
			o.navCtrl.RequestDir(o.state.CurrentPath)
		}
		o.window.Invalidate()
	case ui.ActionConflictReplace:
		o.handleConflictResolution(ui.ConflictReplaceAll)
	case ui.ActionConflictKeepBoth:
		o.handleConflictResolution(ui.ConflictKeepBothAll)
	case ui.ActionConflictSkip:
		o.handleConflictResolution(ui.ConflictSkipAll)
	case ui.ActionConflictStop:
		o.handleConflictResolution(ui.ConflictAsk) // Stop uses Ask to signal abort
		o.conflictAbort = true
	case ui.ActionChangeSearchEngine:
		o.searchCtrl.ChangeEngine(evt.SearchEngine)
		// Save setting to config.json
		o.config.SetSearchEngine(evt.SearchEngine)
	case ui.ActionChangeDefaultDepth:
		o.searchCtrl.DefaultDepth = evt.DefaultDepth
		o.ui.DefaultDepth = evt.DefaultDepth
		// Save setting to config.json
		o.config.SetDefaultDepth(evt.DefaultDepth)
		debug.Log(debug.APP, "Settings: default_depth=%d", evt.DefaultDepth)
	case ui.ActionChangeTheme:
		theme := "light"
		if evt.DarkMode {
			theme = "dark"
		}
		o.config.SetTheme(theme)
		debug.Log(debug.APP, "Settings: theme=%s", theme)
	case ui.ActionRequestSearchHistory:
		// Request search history from database
		o.store.RequestChan <- store.Request{
			Op:    store.FetchSearchHistory,
			Query: evt.SearchHistoryQuery,
			Limit: 3,
		}
	case ui.ActionShowRecentFiles:
		// Switch to recent files view
		o.showRecentFiles()
	case ui.ActionShowTrash:
		// Switch to trash view
		o.showTrash()
	case ui.ActionEmptyTrash:
		// Empty the trash
		go o.emptyTrash()
	case ui.ActionPermanentDelete:
		// Permanent delete (Shift+Delete)
		if len(evt.Paths) > 0 {
			go o.doPermanentDeleteMultiple(evt.Paths)
		} else if evt.Path != "" {
			go o.doPermanentDelete(evt.Path)
		}
	case ui.ActionOpenFileLocation:
		// Navigate to the directory containing the file (with file selection)
		o.openFileLocation(evt.Path)
	case ui.ActionNewTab:
		o.createNewTabInCurrentDir()
	case ui.ActionNewTabHome:
		o.createNewTabInHome()
	case ui.ActionCloseTab:
		o.closeTab(evt.TabIndex)
	case ui.ActionSwitchTab:
		o.switchToTab(evt.TabIndex)
	case ui.ActionOpenInNewTab:
		o.openPathInNewTab(evt.Path)
	case ui.ActionNextTab:
		o.nextTab()
	case ui.ActionPrevTab:
		o.prevTab()
	case ui.ActionSwitchToTab:
		o.switchToTab(evt.TabIndex)
	case ui.ActionSelectAll:
		o.selectAll()
	case ui.ActionRefresh:
		o.refreshCurrentDir()
	case ui.ActionFocusSearch:
		o.ui.FocusSearch()
	case ui.ActionJumpToLetter:
		// Jump navigation handled in renderer, just update selection state
		if evt.NewIndex >= 0 && evt.NewIndex < len(o.state.Entries) {
			o.stateMu.Lock()
			o.state.SelectedIndex = evt.NewIndex
			o.stateMu.Unlock()
		}
	case ui.ActionExpandDir:
		debug.Log(debug.APP, "Received ActionExpandDir for path: %s", evt.Path)
		o.expandDirectory(evt.Path)
	case ui.ActionCollapseDir:
		debug.Log(debug.APP, "Received ActionCollapseDir for path: %s", evt.Path)
		o.collapseDirectory(evt.Path)
	}
}

func (o *Orchestrator) openNewWindow() {
	exe, err := os.Executable()
	if err != nil {
		log.Printf("Error getting executable path: %v", err)
		return
	}
	cmd := exec.Command(exe, "-path", o.state.CurrentPath)
	cmd.Start()
}

func (o *Orchestrator) processEvents() {
	// Get watcher notification channel (may be nil if watcher failed to init)
	var watcherChan <-chan string
	if o.watcher != nil {
		watcherChan = o.watcher.Notify()
		debug.Log(debug.APP, "Directory watcher initialized, channel ready")
	} else {
		debug.Log(debug.APP, "Directory watcher not available")
	}

	for {
		// Use separate select to handle nil watcherChan gracefully
		if watcherChan != nil {
			select {
			case resp := <-o.fs.ResponseChan:
				o.handleFSResponse(resp)
			case progress := <-o.fs.ProgressChan:
				o.handleProgress(progress)
			case resp := <-o.store.ResponseChan:
				o.handleStoreResponse(resp)
			case changedDir := <-watcherChan:
				o.handleDirectoryChange(changedDir)
			}
		} else {
			select {
			case resp := <-o.fs.ResponseChan:
				o.handleFSResponse(resp)
			case progress := <-o.fs.ProgressChan:
				o.handleProgress(progress)
			case resp := <-o.store.ResponseChan:
				o.handleStoreResponse(resp)
			}
		}
	}
}

// handleDirectoryChange refreshes the display if the changed directory is currently visible
func (o *Orchestrator) handleDirectoryChange(changedDir string) {
	o.stateMu.RLock()
	currentPath := o.state.CurrentPath
	isSearchResult := o.state.IsSearchResult
	o.stateMu.RUnlock()

	// Don't auto-refresh during search results
	if isSearchResult {
		return
	}

	// Check if the changed directory is the one we're currently viewing
	if changedDir == currentPath {
		debug.Log(debug.APP, "Directory changed, refreshing: %s", changedDir)
		o.refreshCurrentDir()
		// Don't return - also update other tabs viewing this directory
	} else if o.ui.IsExpanded(changedDir) {
		// Check if it's an expanded directory in the current view
		debug.Log(debug.APP, "Expanded directory changed, refreshing subtree: %s", changedDir)
		o.refreshExpandedDir(changedDir)
		// Don't return - also update other tabs with this expanded directory
	}

	// Update ALL other tabs that are viewing the changed directory
	for i, tab := range o.tabs {
		if i == o.activeTabIndex {
			continue
		}

		// Check if this tab's current directory changed
		if tab.CurrentPath == changedDir {
			debug.Log(debug.APP, "Tab %d directory changed: %s (refreshing cached entries)", i, changedDir)
			o.refreshTabEntries(i)
			// Don't continue - also check expanded dirs
		}

		// Check if an expanded directory within this tab changed
		if tab.ExpandedDirs != nil && tab.ExpandedDirs[changedDir] {
			debug.Log(debug.APP, "Tab %d expanded directory changed: %s (refreshing cached entries)", i, changedDir)
			o.refreshTabExpandedDir(i, changedDir)
		}
	}
}

func (o *Orchestrator) handleProgress(p fs.Progress) {
	// Check if this is for the current search (atomic load)
	currentGen := o.searchGen.Load()

	if p.Gen != currentGen {
		// Stale progress, ignore
		return
	}

	// Throttle progress updates to avoid flooding UI (100ms minimum interval)
	o.progressThrottleMu.Lock()
	now := time.Now()
	if now.Sub(o.lastProgressUpdate) < progressThrottleTime {
		o.progressThrottleMu.Unlock()
		return
	}
	o.lastProgressUpdate = now
	o.progressThrottleMu.Unlock()

	o.setProgress(true, p.Label, p.Current, p.Total)
	o.window.Invalidate()
}

func (o *Orchestrator) handleFSResponse(resp fs.Response) {
	debug.Log(debug.APP, "FSResponse: op=%d path=%q entries=%d gen=%d cancelled=%v err=%v",
		resp.Op, resp.Path, len(resp.Entries), resp.Gen, resp.Cancelled, resp.Err)

	// Clear any progress indicator
	o.setProgress(false, "", 0, 0)

	// If cancelled, just ignore the response
	if resp.Cancelled {
		debug.Log(debug.APP, "FSResponse: cancelled, ignoring")
		return
	}

	// Check if this is a stale response (a newer request has been made) - atomic load
	currentGen := o.searchGen.Load()

	if resp.Gen < currentGen {
		// Stale response, ignore it
		debug.Log(debug.APP, "FSResponse: STALE (gen %d < current %d), ignoring", resp.Gen, currentGen)
		return
	}

	if resp.Err != nil {
		log.Printf("FS Error: %v", resp.Err)
		return
	}

	// Convert response entries to UI entries
	entries := make([]ui.UIEntry, len(resp.Entries))
	for i, e := range resp.Entries {
		entries[i] = ui.UIEntry{
			Name: e.Name, Path: e.Path, IsDir: e.IsDir, Size: e.Size, ModTime: e.ModTime,
		}
	}

	// Track whether this is a search result or regular directory fetch
	canBack := o.navCtrl.HistoryIndex > 0
	canForward := o.navCtrl.HistoryIndex < len(o.navCtrl.History)-1

	if resp.Op == fs.FetchDir {
		// Directory fetch: use StateOwner as single source of truth
		o.stateOwner.SetEntries(resp.Path, entries, canBack, canForward)
		debug.Log(debug.APP, "FSResponse: FetchDir complete, %d entries", len(entries))

		// Clear expanded directories when navigating to a new folder
		o.ui.ClearExpanded()
		o.stateOwner.ClearExpanded()

		// Update current tab title and path
		if o.activeTabIndex >= 0 && o.activeTabIndex < len(o.tabs) {
			title := filepath.Base(resp.Path)
			if title == "" || title == "/" || title == "." {
				title = resp.Path
			}
			o.ui.UpdateTabTitle(o.activeTabIndex, title)
			o.ui.UpdateTabPath(o.activeTabIndex, resp.Path)
		}

		// Update directory watcher - watch the new directory
		// We don't unwatch old directories since other tabs might still be viewing them
		// Cleanup happens when tabs are closed
		if o.watcher != nil {
			if err := o.watcher.Watch(resp.Path); err != nil {
				debug.Log(debug.APP, "Failed to watch directory %s: %v", resp.Path, err)
			} else {
				debug.Log(debug.APP, "Successfully watching directory: %s", resp.Path)
			}
		}
	} else {
		// Search result: update entries but keep expanded state
		o.stateOwner.SetEntriesKeepExpanded(entries)
		debug.Log(debug.APP, "FSResponse: SearchDir complete, %d results", len(entries))
		// IsSearchResult and SearchQuery were already set in doSearch
	}

	// Sync StateOwner snapshot to o.state for UI rendering
	snapshot := o.stateOwner.GetSnapshot()
	o.stateMu.Lock()
	o.state.Entries = snapshot.Entries
	o.state.CurrentPath = snapshot.CurrentPath
	o.state.CanBack = snapshot.CanBack
	o.state.CanForward = snapshot.CanForward
	if resp.Op == fs.FetchDir {
		o.state.IsSearchResult = false
		o.state.SearchQuery = ""
	}
	if o.state.SelectedIndex >= len(o.state.Entries) {
		o.state.SelectedIndex = -1
	}
	o.stateMu.Unlock()

	o.window.Invalidate()
}

func (o *Orchestrator) handleStoreResponse(resp store.Response) {
	if resp.Err != nil {
		log.Printf("Store Error: %v", resp.Err)
		return
	}

	switch resp.Op {
	case store.FetchSearchHistory:
		// Update UI with search history results
		items := make([]ui.SearchHistoryItem, len(resp.SearchHistory))
		for i, entry := range resp.SearchHistory {
			items[i] = ui.SearchHistoryItem{
				Query: entry.Query,
				Score: entry.Score,
			}
		}
		o.ui.SetSearchHistory(items)
		o.window.Invalidate()
	case store.FetchRecentFiles:
		// Convert recent files to UI entries
		o.handleRecentFilesResponse(resp.RecentFiles)
	}
}

func (o *Orchestrator) applyFilterAndSort() {
	// StateOwner handles filtering and sorting internally
	// Just sync snapshot to state for UI rendering
	snapshot := o.stateOwner.GetSnapshot()
	o.state.Entries = snapshot.Entries
}

func (o *Orchestrator) setProgress(active bool, label string, current, total int64) {
	o.progressMu.Lock()
	o.state.Progress = ui.ProgressState{Active: active, Label: label, Current: current, Total: total}
	o.progressMu.Unlock()
	o.window.Invalidate()
}

// handleRecentFilesResponse processes recent files from the database
func (o *Orchestrator) handleRecentFilesResponse(recentFiles []store.RecentFileEntry) {
	debug.Log(debug.APP, "handleRecentFilesResponse: %d entries", len(recentFiles))

	// Convert to UI entries, filtering out files that no longer exist
	entries := make([]ui.UIEntry, 0, len(recentFiles))
	for _, rf := range recentFiles {
		// Check if file still exists
		info, err := os.Stat(rf.Path)
		if err != nil {
			// File no longer exists, skip it
			continue
		}

		entries = append(entries, ui.UIEntry{
			Name:    rf.Name,
			Path:    rf.Path,
			IsDir:   info.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}

	// Use StateOwner as single source of truth
	o.stateOwner.SetEntries("Recent Files", entries, true, false)

	// Sync to state for UI
	snapshot := o.stateOwner.GetSnapshot()
	o.stateMu.Lock()
	o.state.CurrentPath = snapshot.CurrentPath
	o.state.Entries = snapshot.Entries
	o.state.SelectedIndex = -1
	o.state.IsSearchResult = false
	o.state.SearchQuery = ""
	o.state.CanBack = snapshot.CanBack
	o.state.CanForward = snapshot.CanForward
	o.stateMu.Unlock()

	o.window.Invalidate()
}

// showRecentFiles switches to the virtual "Recent Files" view
func (o *Orchestrator) showRecentFiles() {
	debug.Log(debug.APP, "Showing recent files")
	o.ui.SetRecentView(true)
	o.ui.HidePreview()

	// Request recent files from database
	o.store.RequestChan <- store.Request{
		Op:    store.FetchRecentFiles,
		Limit: 50,
	}
}

// showTrash switches to the virtual "Trash" view
func (o *Orchestrator) showTrash() {
	debug.Log(debug.APP, "Showing trash")
	o.ui.SetTrashView(true)
	o.ui.HidePreview()

	// Load trash contents
	items, err := trash.List()
	if err != nil {
		log.Printf("Error loading trash: %v", err)
		items = nil
	}

	// Convert trash items to UI entries
	entries := make([]ui.UIEntry, len(items))
	for i, item := range items {
		entries[i] = ui.UIEntry{
			Name:    item.Name,
			Path:    item.TrashPath, // Use trash path for operations
			IsDir:   item.IsDir,
			Size:    item.Size,
			ModTime: item.DeletedAt,
		}
	}

	// Use StateOwner as single source of truth
	o.stateOwner.SetEntries(trash.DisplayName(), entries, true, false)

	// Sync to state for UI
	snapshot := o.stateOwner.GetSnapshot()
	o.stateMu.Lock()
	o.state.CurrentPath = snapshot.CurrentPath
	o.state.Entries = snapshot.Entries
	o.state.SelectedIndex = -1
	o.state.SelectedIndices = nil
	o.state.IsSearchResult = false
	o.state.CanBack = snapshot.CanBack
	o.state.CanForward = snapshot.CanForward
	o.stateMu.Unlock()
	o.ui.ResetMultiSelect()

	o.window.Invalidate()
}

// emptyTrash permanently deletes all items in the trash
func (o *Orchestrator) emptyTrash() {
	o.setProgress(true, "Emptying "+trash.DisplayName()+"...", 0, 0)
	err := trash.Empty()
	o.setProgress(false, "", 0, 0)

	if err != nil {
		log.Printf("Error emptying trash: %v", err)
	}

	// Refresh trash view if currently showing
	if o.ui.IsTrashView() {
		o.showTrash()
	}
}

// openFileLocation navigates to the directory containing the file
func (o *Orchestrator) openFileLocation(path string) {
	debug.Log(debug.APP, "Open file location: %s", path)
	dir := filepath.Dir(path)
	o.ui.SetRecentView(false)
	o.navCtrl.Navigate(dir)

	// Optionally select the file after navigation
	// We'll do this asynchronously after the directory loads
	go func() {
		// Give time for directory to load
		time.Sleep(100 * time.Millisecond)
		o.stateMu.Lock()
		for i, entry := range o.state.Entries {
			if entry.Path == path {
				o.state.SelectedIndex = i
				o.stateMu.Unlock()
				o.window.Invalidate()
				return
			}
		}
		o.stateMu.Unlock()
	}()
}

// selectAll toggles selection of all entries in the current view
// If all are selected, deselects all. Otherwise, selects all.
func (o *Orchestrator) selectAll() {
	o.stateMu.Lock()
	defer o.stateMu.Unlock()

	if len(o.state.Entries) == 0 {
		return
	}

	// Check if all entries are already selected
	allSelected := len(o.state.SelectedIndices) == len(o.state.Entries)
	if allSelected {
		for i := range o.state.SelectedIndices {
			if !o.state.SelectedIndices[i] {
				allSelected = false
				break
			}
		}
	}

	if allSelected {
		// Deselect all
		o.state.SelectedIndices = nil
		o.state.SelectedIndex = -1
		o.ui.SetMultiSelectMode(false)
	} else {
		// Select all
		o.state.SelectedIndices = make(map[int]bool)
		for i := range o.state.Entries {
			o.state.SelectedIndices[i] = true
		}
		o.state.SelectedIndex = 0 // Primary selection at first item
		o.ui.SetMultiSelectMode(true)
	}
	o.window.Invalidate()
}

// expandDirectory expands a directory inline in the tree view
func (o *Orchestrator) expandDirectory(path string) {
	debug.Log(debug.APP, "expandDirectory called for: %s", path)

	// Mark as expanded in the UI
	o.ui.SetExpanded(path, true)

	// Use StateOwner to expand directory
	o.stateOwner.ExpandDir(path)

	// Sync to state for UI
	snapshot := o.stateOwner.GetSnapshot()
	o.stateMu.Lock()
	o.state.Entries = snapshot.Entries
	o.stateMu.Unlock()

	// Watch the expanded directory for changes
	if o.watcher != nil {
		o.watcher.Watch(path)
	}

	o.window.Invalidate()
}

// collapseDirectory collapses an expanded directory in the tree view
func (o *Orchestrator) collapseDirectory(path string) {
	debug.Log(debug.APP, "Collapsing directory: %s", path)

	// Mark as collapsed in the UI
	o.ui.SetExpanded(path, false)

	// Use StateOwner to collapse directory
	o.stateOwner.CollapseDir(path)

	// Sync to state for UI
	snapshot := o.stateOwner.GetSnapshot()
	o.stateMu.Lock()
	o.state.Entries = snapshot.Entries
	o.stateMu.Unlock()

	// Unwatch the collapsed directory
	if o.watcher != nil {
		o.watcher.Unwatch(path)
	}

	o.window.Invalidate()
}

func Main(startPath string) {
	go func() {
		o := NewOrchestrator()
		if err := o.Run(startPath); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}
