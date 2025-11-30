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

const maxHistorySize = 100 // Limit history to prevent unbounded memory growth

type Orchestrator struct {
	window         *app.Window
	fs             *fs.System
	store          *store.DB
	config         *config.Manager // Config manager for settings
	ui             *ui.Renderer
	stateMu        sync.RWMutex // Protects state from concurrent read/write
	state          ui.State
	history        []string
	historyIndex   int
	sortColumn     ui.SortColumn
	sortAsc        bool
	showDotfiles   bool
	rawEntries     []ui.UIEntry // Current display entries (filtered/sorted)
	dirEntries     []ui.UIEntry // Original directory entries (never overwritten by search)
	progressMu     sync.Mutex
	homePath       string
	searchGen      atomic.Int64 // Tracks search generation to ignore stale results (atomic for perf)

	// Progress throttling - avoid flooding UI with updates
	lastProgressUpdate time.Time
	progressThrottleMu sync.Mutex

	// Conflict resolution state
	conflictResolution ui.ConflictResolution       // Current resolution mode
	conflictResponse   chan ui.ConflictResolution  // Channel for dialog response
	conflictAbort      bool                        // True if user clicked Stop

	// Search engine settings
	searchEngines     []search.EngineInfo // Detected search engines
	selectedEngine    search.SearchEngine // Currently selected engine
	selectedEngineCmd string              // Command for the selected engine
	defaultDepth      int                 // Default recursive search depth
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

	o := &Orchestrator{
		window:           new(app.Window),
		fs:               fs.NewSystem(),
		store:            store.NewDB(),
		config:           cfgMgr,
		ui:               ui.NewRenderer(),
		state:            ui.State{SelectedIndex: -1, Favorites: make(map[string]bool)},
		historyIndex:     -1,
		sortAsc:          cfg.UI.FileList.SortAscending,
		showDotfiles:     cfg.UI.FileList.ShowDotfiles,
		homePath:         home,
		conflictResponse: make(chan ui.ConflictResolution, 1),
		searchEngines:    engines,
		selectedEngine:   search.EngineBuiltin,
		defaultDepth:     cfg.Search.DefaultDepth,
	}

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
	o.ui.SetPreviewConfig(cfg.Preview.TextExtensions, cfg.Preview.MaxFileSize, cfg.Preview.WidthPercent)

	// Set search engine from config
	o.handleSearchEngineChange(cfg.Search.Engine)

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

	for _, fav := range favorites {
		// Skip groups for now (flat list only)
		if fav.Type == "group" {
			// TODO: Handle favorite groups in the future
			continue
		}
		// Expand ~ in path
		path := fav.Path
		if len(path) > 0 && path[0] == '~' {
			path = filepath.Join(o.homePath, path[1:])
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
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".config", "razor", "razor.db")
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
		startPath = o.homePath
		if startPath == "" {
			startPath, _ = os.Getwd()
		}
	}
	o.navigate(startPath)

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

func (o *Orchestrator) handleUIEvent(evt ui.UIEvent) {
	switch evt.Action {
	case ui.ActionNavigate:
		// Cancel any active rename and hide preview
		o.ui.CancelRename()
		o.ui.HidePreview()
		o.ui.SetRecentView(false) // Exit recent view
		// Expand the path before navigating
		expandedPath := o.expandPath(evt.Path)
		// Validate the path exists
		if exists, isDir := o.validatePath(expandedPath); exists && isDir {
			o.navigate(expandedPath)
		} else if exists && !isDir {
			// It's a file, open it instead
			if err := platformOpen(expandedPath); err != nil {
				log.Printf("Error opening file: %v", err)
			}
			// Track in recent files
			o.store.RequestChan <- store.Request{Op: store.AddRecentFile, Path: expandedPath}
		} else {
			// Path doesn't exist - log error
			log.Printf("Path does not exist: %s (expanded from: %s)", expandedPath, evt.Path)
		}
	case ui.ActionBack:
		o.ui.CancelRename()
		o.ui.HidePreview()
		o.ui.SetRecentView(false)
		o.goBack()
	case ui.ActionForward:
		o.ui.CancelRename()
		o.ui.HidePreview()
		o.ui.SetRecentView(false)
		o.goForward()
	case ui.ActionHome:
		o.ui.CancelRename()
		o.ui.HidePreview()
		o.ui.SetRecentView(false)
		o.navigate(o.homePath)
	case ui.ActionNewWindow:
		o.openNewWindow()
	case ui.ActionSelect:
		o.state.SelectedIndex = evt.NewIndex
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
	case ui.ActionSearch:
		o.doSearch(evt.Path, evt.SearchSubmitted)
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
		o.state.Clipboard = &ui.Clipboard{Path: evt.Path, Op: ui.ClipCopy}
		o.window.Invalidate()
	case ui.ActionCut:
		o.state.Clipboard = &ui.Clipboard{Path: evt.Path, Op: ui.ClipCut}
		o.window.Invalidate()
	case ui.ActionPaste:
		if o.state.Clipboard != nil {
			go o.doPaste()
		}
	case ui.ActionConfirmDelete:
		go o.doDelete(evt.Path)
	case ui.ActionCreateFile:
		go o.doCreateFile(evt.FileName)
	case ui.ActionCreateFolder:
		go o.doCreateFolder(evt.FileName)
	case ui.ActionRename:
		go o.doRename(evt.OldPath, evt.Path)
	case ui.ActionClearSearch:
		debug.Log(debug.APP, "ClearSearch: cancelling search")
		// Send cancel request to stop any ongoing search
		o.fs.RequestChan <- fs.Request{Op: fs.CancelSearch}
		// Clear all search state and restore original directory contents
		o.state.IsSearchResult = false
		o.state.SearchQuery = ""
		// Clear progress bar
		o.setProgress(false, "", 0, 0)
		// Increment generation to invalidate any pending search results (atomic)
		newGen := o.searchGen.Add(1)
		debug.Log(debug.APP, "ClearSearch: generation=%d", newGen)
		// Restore from cached directory entries (no disk access needed)
		if len(o.dirEntries) > 0 {
			o.rawEntries = o.dirEntries
			o.applyFilterAndSort()
			o.window.Invalidate()
		} else {
			// Fallback: re-fetch if dirEntries is empty
			o.requestDir(o.state.CurrentPath)
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
		o.handleSearchEngineChange(evt.SearchEngine)
		// Save setting to config.json
		o.config.SetSearchEngine(evt.SearchEngine)
	case ui.ActionChangeDefaultDepth:
		o.defaultDepth = evt.DefaultDepth
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
		// Navigate to the directory containing the file
		o.openFileLocation(evt.Path)
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
	if now.Sub(o.lastProgressUpdate) < 100*time.Millisecond {
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
	} else {
		// Search result: only update rawEntries, keep dirEntries intact
		o.rawEntries = entries
		debug.Log(debug.APP, "FSResponse: SearchDir complete, %d results", len(entries))
		// IsSearchResult and SearchQuery were already set in doSearch
	}

	o.applyFilterAndSort()

	parent := filepath.Dir(o.state.CurrentPath)
	o.state.CanBack = parent != o.state.CurrentPath
	o.state.CanForward = o.historyIndex < len(o.history)-1

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
	o.navigate(dir)

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
