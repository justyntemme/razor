package app

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
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
	window *app.Window
	fs     *fs.System
	store  *store.DB
	config *config.Manager
	ui     *ui.Renderer

	// Shared state (protected by stateMu)
	stateMu    sync.RWMutex
	state      ui.State
	rawEntries []ui.UIEntry // Current display entries (filtered/sorted)
	dirEntries []ui.UIEntry // Original directory entries (cache)
	searchGen  atomic.Int64 // Search generation counter

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

	// Create core orchestrator
	o := &Orchestrator{
		window:           new(app.Window),
		fs:               fs.NewSystem(),
		store:            store.NewDB(),
		config:           cfgMgr,
		ui:               ui.NewRenderer(),
		state:            ui.State{SelectedIndex: -1, Favorites: make(map[string]bool)},
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
	o.state.FavList = make([]ui.FavoriteItem, 0, len(favorites))

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
	drives := fs.ListDrives()
	o.state.Drives = make([]ui.DriveItem, len(drives))
	for i, d := range drives {
		o.state.Drives[i] = ui.DriveItem{Name: d.Name, Path: d.Path}
	}
}

// resetUIState cancels any active rename, hides preview, exits recent view,
// and clears multi-select mode. Called before navigation operations to ensure clean state.
func (o *Orchestrator) resetUIState() {
	o.ui.CancelRename()
	o.ui.HidePreview()
	o.ui.SetRecentView(false)
	o.ui.ResetMultiSelect()
	o.state.SelectedIndices = nil
	o.state.SelectedIndex = -1
}

// restoreDirectory restores the cached directory entries after a search is cleared.
// Returns true if entries were restored, false if a re-fetch is needed.
func (o *Orchestrator) restoreDirectory() bool {
	if len(o.dirEntries) > 0 {
		o.rawEntries = o.dirEntries
		o.applyFilterAndSort()
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
		o.applyFilterAndSort()
		o.window.Invalidate()
	case ui.ActionToggleDotfiles:
		o.showDotfiles = evt.ShowDotfiles
		o.config.SetShowDotfiles(o.showDotfiles)
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
		// Restore from cached directory entries (no disk access needed)
		if len(o.dirEntries) > 0 {
			o.rawEntries = o.dirEntries
			o.applyFilterAndSort()
			o.window.Invalidate()
		} else {
			// Fallback: re-fetch if dirEntries is empty
			o.navCtrl.RequestDir(o.state.CurrentPath)
		}
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
	case ui.ActionOpenFileLocation:
		// Navigate to the directory containing the file (with file selection)
		o.openFileLocation(evt.Path)
	case ui.ActionNewTab:
		o.createNewTab()
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
	case ui.ActionSelectAll:
		o.selectAll()
	case ui.ActionRefresh:
		o.refreshCurrentDir()
	case ui.ActionFocusSearch:
		o.ui.FocusSearch()
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
	for {
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

	// Lock state for writing
	o.stateMu.Lock()

	// Track whether this is a search result or regular directory fetch
	if resp.Op == fs.FetchDir {
		// Directory fetch: store as the canonical directory listing
		o.dirEntries = entries
		o.rawEntries = entries
		o.state.IsSearchResult = false
		o.state.SearchQuery = ""
		o.state.CurrentPath = resp.Path
		debug.Log(debug.APP, "FSResponse: FetchDir complete, %d entries", len(entries))

		// Update current tab title and path
		if o.activeTabIndex >= 0 && o.activeTabIndex < len(o.tabs) {
			title := filepath.Base(resp.Path)
			if title == "" || title == "/" || title == "." {
				title = resp.Path
			}
			o.ui.UpdateTabTitle(o.activeTabIndex, title)
			o.ui.UpdateTabPath(o.activeTabIndex, resp.Path)
		}
	} else {
		// Search result: only update rawEntries, keep dirEntries intact
		o.rawEntries = entries
		debug.Log(debug.APP, "FSResponse: SearchDir complete, %d results", len(entries))
		// IsSearchResult and SearchQuery were already set in doSearch
	}

	o.applyFilterAndSort()

	parent := filepath.Dir(o.state.CurrentPath)
	o.state.CanBack = parent != o.state.CurrentPath
	o.state.CanForward = o.navCtrl.HistoryIndex < len(o.navCtrl.History)-1

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
	// Pre-allocate with estimated capacity (most entries won't be dotfiles)
	entries := make([]ui.UIEntry, 0, len(o.rawEntries))
	for _, e := range o.rawEntries {
		if !o.showDotfiles && strings.HasPrefix(e.Name, ".") {
			continue
		}
		entries = append(entries, e)
	}

	cmp := o.getComparator()
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		less := cmp(entries[i], entries[j])
		if !o.sortAsc {
			return !less
		}
		return less
	})

	o.state.Entries = entries
}

func (o *Orchestrator) getComparator() func(a, b ui.UIEntry) bool {
	switch o.sortColumn {
	case ui.SortByDate:
		return func(a, b ui.UIEntry) bool { return a.ModTime.Before(b.ModTime) }
	case ui.SortByType:
		return func(a, b ui.UIEntry) bool {
			extA, extB := strings.ToLower(filepath.Ext(a.Name)), strings.ToLower(filepath.Ext(b.Name))
			if extA == extB {
				return strings.ToLower(a.Name) < strings.ToLower(b.Name)
			}
			return extA < extB
		}
	case ui.SortBySize:
		return func(a, b ui.UIEntry) bool {
			if a.Size == b.Size {
				return strings.ToLower(a.Name) < strings.ToLower(b.Name)
			}
			return a.Size < b.Size
		}
	default:
		return func(a, b ui.UIEntry) bool { return strings.ToLower(a.Name) < strings.ToLower(b.Name) }
	}
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

	// Update state
	o.stateMu.Lock()
	o.state.CurrentPath = "Recent Files"
	o.state.Entries = entries
	o.state.SelectedIndex = -1
	o.state.IsSearchResult = false
	o.state.SearchQuery = ""
	o.state.CanBack = true // Allow going back
	o.rawEntries = entries
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

// selectAll selects all entries in the current view
func (o *Orchestrator) selectAll() {
	o.stateMu.Lock()
	defer o.stateMu.Unlock()

	if len(o.state.Entries) == 0 {
		return
	}

	// Enter multi-select mode and select all
	o.state.SelectedIndices = make(map[int]bool)
	for i := range o.state.Entries {
		o.state.SelectedIndices[i] = true
	}
	o.state.SelectedIndex = 0 // Primary selection at first item
	o.ui.SetMultiSelectMode(true)
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
