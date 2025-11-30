package app

import (
	"io"
	iofs "io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gioui.org/app"
	"gioui.org/op"

	"github.com/charlievieth/fastwalk"
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
	conflictResolution ui.ConflictResolution          // Current resolution mode
	conflictResponse   chan ui.ConflictResolution    // Channel for dialog response
	conflictAbort      bool                          // True if user clicked Stop

	// Search engine settings
	searchEngines     []search.EngineInfo  // Detected search engines
	selectedEngine    search.SearchEngine  // Currently selected engine
	selectedEngineCmd string               // Command for the selected engine
	defaultDepth      int                  // Default recursive search depth
}

func NewOrchestrator() *Orchestrator {
	home, _ := os.UserHomeDir()
	
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
		ui:               ui.NewRenderer(),
		state:            ui.State{SelectedIndex: -1, Favorites: make(map[string]bool)},
		historyIndex:     -1,
		sortAsc:          true,
		homePath:         home,
		conflictResponse: make(chan ui.ConflictResolution, 1),
		searchEngines:    engines,
		selectedEngine:   search.EngineBuiltin,
		defaultDepth:     2, // Default recursive depth
	}
	
	// Set up UI with detected engines
	o.ui.SearchEngines = uiEngines
	o.ui.SetSearchEngine("builtin")
	o.ui.SetDefaultDepth(2)
	
	return o
}

// expandPath expands and normalizes a path string, handling:
// - ~ for home directory
// - Relative paths (../, ./)
// - Absolute paths
// - Windows drive letters (C:, D:, etc.)
// - Root path (/)
func (o *Orchestrator) expandPath(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return o.state.CurrentPath
	}

	// Handle home directory expansion
	if strings.HasPrefix(input, "~") {
		if input == "~" {
			return o.homePath
		}
		if strings.HasPrefix(input, "~/") || strings.HasPrefix(input, "~\\") {
			input = filepath.Join(o.homePath, input[2:])
			return filepath.Clean(input)
		}
	}

	// Check if it's an absolute path
	if o.isAbsolutePath(input) {
		return filepath.Clean(input)
	}

	// Handle relative paths - join with current directory
	return filepath.Clean(filepath.Join(o.state.CurrentPath, input))
}

// isAbsolutePath checks if a path is absolute, handling both Unix and Windows paths
func (o *Orchestrator) isAbsolutePath(path string) bool {
	if len(path) == 0 {
		return false
	}

	// Unix absolute path
	if path[0] == '/' {
		return true
	}

	// Windows absolute path checks
	if runtime.GOOS == "windows" {
		// Drive letter paths: C:\, D:\, C:/, etc.
		if len(path) >= 2 && isLetter(path[0]) && path[1] == ':' {
			return true
		}
		// UNC paths: \\server\share
		if len(path) >= 2 && path[0] == '\\' && path[1] == '\\' {
			return true
		}
	}

	return false
}

// isLetter checks if a byte is an ASCII letter
func isLetter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

// validatePath checks if a path exists and returns info about it
func (o *Orchestrator) validatePath(path string) (exists bool, isDir bool) {
	info, err := os.Stat(path)
	if err != nil {
		return false, false
	}
	return true, info.IsDir()
}

func (o *Orchestrator) Run(startPath string) error {
	if debug.Enabled {
		log.Println("Starting Razor in DEBUG mode")
		debug.Log(debug.APP, "Debug categories enabled: %v", debug.ListEnabled())
	}

	configDir, _ := os.UserConfigDir()
	dbPath := filepath.Join(configDir, "razor", "razor.db")
	debug.Log(debug.APP, "Opening database: %s", dbPath)
	if err := o.store.Open(dbPath); err != nil {
		log.Printf("Failed to open DB: %v", err)
	}
	defer o.store.Close()

	go o.fs.Start()
	go o.store.Start()
	go o.processEvents()

	o.store.RequestChan <- store.Request{Op: store.FetchFavorites}
	o.store.RequestChan <- store.Request{Op: store.FetchSettings}

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
		// Cancel any active rename
		o.ui.CancelRename()
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
		} else {
			// Path doesn't exist - log error
			log.Printf("Path does not exist: %s (expanded from: %s)", expandedPath, evt.Path)
		}
	case ui.ActionBack:
		o.ui.CancelRename()
		o.goBack()
	case ui.ActionForward:
		o.ui.CancelRename()
		o.goForward()
	case ui.ActionHome:
		o.ui.CancelRename()
		o.navigate(o.homePath)
	case ui.ActionNewWindow:
		o.openNewWindow()
	case ui.ActionSelect:
		o.state.SelectedIndex = evt.NewIndex
		o.window.Invalidate()
	case ui.ActionSearch:
		o.doSearch(evt.Path)
	case ui.ActionOpen:
		if err := platformOpen(evt.Path); err != nil {
			log.Printf("Error opening file: %v", err)
		}
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
		o.store.RequestChan <- store.Request{Op: store.AddFavorite, Path: evt.Path}
	case ui.ActionRemoveFavorite:
		o.store.RequestChan <- store.Request{Op: store.RemoveFavorite, Path: evt.Path}
	case ui.ActionSort:
		o.sortColumn, o.sortAsc = evt.SortColumn, evt.SortAscending
		o.applyFilterAndSort()
		o.window.Invalidate()
	case ui.ActionToggleDotfiles:
		o.showDotfiles = evt.ShowDotfiles
		val := "false"
		if o.showDotfiles {
			val = "true"
		}
		o.store.RequestChan <- store.Request{Op: store.SaveSetting, Key: "show_dotfiles", Value: val}
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
		// Save setting to database
		o.store.RequestChan <- store.Request{Op: store.SaveSetting, Key: "search_engine", Value: evt.SearchEngine}
	case ui.ActionChangeDefaultDepth:
		o.defaultDepth = evt.DefaultDepth
		o.ui.DefaultDepth = evt.DefaultDepth
		// Save setting to database
		o.store.RequestChan <- store.Request{Op: store.SaveSetting, Key: "default_depth", Value: strconv.Itoa(evt.DefaultDepth)}
		debug.Log(debug.APP, "Settings: default_depth=%d", evt.DefaultDepth)
	case ui.ActionChangeTheme:
		val := "false"
		if evt.DarkMode {
			val = "true"
		}
		o.store.RequestChan <- store.Request{Op: store.SaveSetting, Key: "dark_mode", Value: val}
		debug.Log(debug.APP, "Settings: dark_mode=%v", evt.DarkMode)
	}
}

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

func (o *Orchestrator) openNewWindow() {
	exe, err := os.Executable()
	if err != nil {
		log.Printf("Error getting executable path: %v", err)
		return
	}
	cmd := exec.Command(exe, "-path", o.state.CurrentPath)
	cmd.Start()
}

func (o *Orchestrator) navigate(path string) {
	if o.historyIndex >= 0 && o.historyIndex < len(o.history)-1 {
		o.history = o.history[:o.historyIndex+1]
	}
	o.history = append(o.history, path)
	o.historyIndex = len(o.history) - 1

	// Limit history size to prevent unbounded memory growth
	if len(o.history) > maxHistorySize {
		// Remove oldest entries
		excess := len(o.history) - maxHistorySize
		o.history = o.history[excess:]
		o.historyIndex -= excess
		if o.historyIndex < 0 {
			o.historyIndex = 0
		}
	}

	o.requestDir(path)
}

func (o *Orchestrator) goBack() {
	parent := filepath.Dir(o.state.CurrentPath)
	if parent == o.state.CurrentPath {
		return
	}
	if o.historyIndex > 0 && o.history[o.historyIndex-1] == parent {
		o.historyIndex--
	} else {
		o.history = append(o.history[:o.historyIndex], append([]string{parent}, o.history[o.historyIndex:]...)...)
	}
	o.requestDir(parent)
}

func (o *Orchestrator) goForward() {
	if o.historyIndex < len(o.history)-1 {
		o.historyIndex++
		o.requestDir(o.history[o.historyIndex])
	}
}

func (o *Orchestrator) requestDir(path string) {
	o.state.SelectedIndex = -1
	// Increment generation to invalidate any pending search results (atomic)
	gen := o.searchGen.Add(1)
	o.fs.RequestChan <- fs.Request{Op: fs.FetchDir, Path: path, Gen: gen}
}

func (o *Orchestrator) doSearch(query string) {
	debug.Log(debug.SEARCH, "doSearch: query=%q", query)
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
	case store.FetchFavorites:
		o.state.Favorites = make(map[string]bool)
		o.state.FavList = make([]ui.FavoriteItem, len(resp.Favorites))
		for i, path := range resp.Favorites {
			o.state.Favorites[path] = true
			o.state.FavList[i] = ui.FavoriteItem{Path: path, Name: filepath.Base(path)}
		}
	case store.FetchSettings:
		if val, ok := resp.Settings["show_dotfiles"]; ok {
			o.showDotfiles = val == "true"
			o.ui.ShowDotfiles = o.showDotfiles
			o.ui.SetShowDotfilesCheck(o.showDotfiles)
			if len(o.rawEntries) > 0 {
				o.applyFilterAndSort()
			}
		}
		// Load search engine setting
		if val, ok := resp.Settings["search_engine"]; ok {
			o.handleSearchEngineChange(val)
		}
		// Load default depth setting
		if val, ok := resp.Settings["default_depth"]; ok {
			if depth, err := strconv.Atoi(val); err == nil && depth >= 1 && depth <= 20 {
				o.defaultDepth = depth
				o.ui.SetDefaultDepth(depth)
				debug.Log(debug.APP, "Settings: loaded default_depth=%d", depth)
			}
		}
		// Load dark mode setting
		if val, ok := resp.Settings["dark_mode"]; ok {
			darkMode := val == "true"
			o.ui.SetDarkMode(darkMode)
			debug.Log(debug.APP, "Settings: loaded dark_mode=%v", darkMode)
		}
	}
	o.window.Invalidate()
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

// --- File Operations ---

func (o *Orchestrator) setProgress(active bool, label string, current, total int64) {
	o.progressMu.Lock()
	o.state.Progress = ui.ProgressState{Active: active, Label: label, Current: current, Total: total}
	o.progressMu.Unlock()
	o.window.Invalidate()
}

func (o *Orchestrator) doCreateFile(name string) {
	if name == "" {
		return
	}

	path := filepath.Join(o.state.CurrentPath, name)

	// Check if file already exists
	if _, err := os.Stat(path); err == nil {
		log.Printf("File already exists: %s", path)
		return
	}

	// Create the file
	file, err := os.Create(path)
	if err != nil {
		log.Printf("Error creating file: %v", err)
		return
	}
	file.Close()

	// Refresh the directory
	o.requestDir(o.state.CurrentPath)
}

func (o *Orchestrator) doCreateFolder(name string) {
	if name == "" {
		return
	}

	path := filepath.Join(o.state.CurrentPath, name)

	// Check if folder already exists
	if _, err := os.Stat(path); err == nil {
		log.Printf("Folder already exists: %s", path)
		return
	}

	// Create the folder
	if err := os.Mkdir(path, 0755); err != nil {
		log.Printf("Error creating folder: %v", err)
		return
	}

	// Refresh the directory
	o.requestDir(o.state.CurrentPath)
}

func (o *Orchestrator) doRename(oldPath, newPath string) {
	if oldPath == "" || newPath == "" || oldPath == newPath {
		return
	}

	// Check if new path already exists
	if _, err := os.Stat(newPath); err == nil {
		log.Printf("Cannot rename: destination already exists: %s", newPath)
		return
	}

	// Perform the rename
	if err := os.Rename(oldPath, newPath); err != nil {
		log.Printf("Error renaming %s to %s: %v", oldPath, newPath, err)
		return
	}

	log.Printf("Renamed %s to %s", oldPath, newPath)

	// Refresh the directory
	o.requestDir(o.state.CurrentPath)
}

// handleConflictResolution is called when user responds to conflict dialog
func (o *Orchestrator) handleConflictResolution(resolution ui.ConflictResolution) {
	// If "Apply to All" was checked, update the resolution mode
	if o.state.Conflict.ApplyToAll {
		o.conflictResolution = resolution
	}
	// Send response to waiting paste operation
	select {
	case o.conflictResponse <- resolution:
	default:
	}
}

func (o *Orchestrator) doPaste() {
	clip := o.state.Clipboard
	if clip == nil {
		return
	}

	// Reset conflict resolution state
	o.conflictResolution = ui.ConflictAsk
	o.conflictAbort = false

	src := clip.Path
	dstDir := o.state.CurrentPath
	dstName := filepath.Base(src)
	dst := filepath.Join(dstDir, dstName)

	srcInfo, err := os.Stat(src)
	if err != nil {
		log.Printf("Paste error: %v", err)
		return
	}

	// Check for conflict
	dstInfo, err := os.Stat(dst)
	if err == nil {
		// Destination exists - need to resolve conflict
		resolution := o.resolveConflict(src, dst, srcInfo, dstInfo)
		
		switch resolution {
		case ui.ConflictReplaceAll:
			// Replace - delete destination first
			if dstInfo.IsDir() {
				os.RemoveAll(dst)
			} else {
				os.Remove(dst)
			}
		case ui.ConflictKeepBothAll:
			// Keep both - rename destination
			ext := filepath.Ext(dstName)
			base := strings.TrimSuffix(dstName, ext)
			for i := 1; ; i++ {
				dst = filepath.Join(dstDir, base+"_copy"+strconv.Itoa(i)+ext)
				if _, err := os.Stat(dst); os.IsNotExist(err) {
					break
				}
			}
		case ui.ConflictSkipAll:
			// Skip this file
			o.requestDir(o.state.CurrentPath)
			return
		case ui.ConflictAsk:
			// User clicked Stop or dialog was aborted
			o.requestDir(o.state.CurrentPath)
			return
		}
	}

	label := "Copying"
	if clip.Op == ui.ClipCut {
		label = "Moving"
	}

	if srcInfo.IsDir() {
		o.setProgress(true, label+" folder...", 0, 0)
		err = o.copyDir(src, dst, clip.Op == ui.ClipCut)
	} else {
		o.setProgress(true, label+" "+filepath.Base(src), 0, srcInfo.Size())
		err = o.copyFile(src, dst, clip.Op == ui.ClipCut)
	}

	o.setProgress(false, "", 0, 0)

	if err != nil {
		log.Printf("Paste error: %v", err)
	} else if clip.Op == ui.ClipCut {
		o.state.Clipboard = nil
	}

	o.requestDir(o.state.CurrentPath)
}

// resolveConflict shows the conflict dialog and waits for user response
func (o *Orchestrator) resolveConflict(src, dst string, srcInfo, dstInfo os.FileInfo) ui.ConflictResolution {
	// If we have a remembered resolution from "Apply to All", use it
	if o.conflictResolution != ui.ConflictAsk {
		return o.conflictResolution
	}
	
	// If abort was requested, return immediately
	if o.conflictAbort {
		return ui.ConflictAsk
	}

	// Set up the conflict state and show dialog
	o.state.Conflict = ui.ConflictState{
		Active:     true,
		SourcePath: src,
		DestPath:   dst,
		SourceSize: srcInfo.Size(),
		DestSize:   dstInfo.Size(),
		SourceTime: srcInfo.ModTime(),
		DestTime:   dstInfo.ModTime(),
		IsDir:      srcInfo.IsDir(),
		ApplyToAll: false,
	}
	o.window.Invalidate()

	// Wait for user response
	resolution := <-o.conflictResponse
	return resolution
}

func (o *Orchestrator) copyFile(src, dst string, move bool) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// Progress-tracking writer (uses atomic add to avoid races with UI thread)
	pw := &progressWriter{
		w: dstFile,
		onWrite: func(n int64) {
			atomic.AddInt64(&o.state.Progress.Current, n)
			o.window.Invalidate()
		},
	}

	if _, err := io.Copy(pw, srcFile); err != nil {
		return err
	}

	if err := os.Chmod(dst, info.Mode()); err != nil {
		return err
	}

	if move {
		return os.Remove(src)
	}
	return nil
}

func (o *Orchestrator) copyDir(src, dst string, move bool) error {
	// Single-pass copy using fastwalk: count total size while building file list
	var totalSize atomic.Int64
	type copyItem struct {
		srcPath string
		dstPath string
		isDir   bool
		mode    iofs.FileMode
	}
	var items []copyItem
	var itemsMu sync.Mutex

	conf := &fastwalk.Config{Follow: true}
	srcLen := len(src)

	err := fastwalk.Walk(conf, src, func(fullPath string, d iofs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // Skip errors, continue walking
		}

		// Get relative path from source root
		relPath := fullPath[srcLen:]
		if len(relPath) > 0 && (relPath[0] == '/' || relPath[0] == '\\') {
			relPath = relPath[1:]
		}
		if relPath == "" {
			return nil // Skip source root itself
		}

		dstPath := filepath.Join(dst, relPath)
		info, err := fastwalk.StatDirEntry(fullPath, d)
		if err != nil {
			return nil // Skip files we can't stat
		}

		if info.IsDir() {
			itemsMu.Lock()
			items = append(items, copyItem{srcPath: fullPath, dstPath: dstPath, isDir: true, mode: info.Mode()})
			itemsMu.Unlock()
		} else {
			totalSize.Add(info.Size())
			itemsMu.Lock()
			items = append(items, copyItem{srcPath: fullPath, dstPath: dstPath, isDir: false, mode: info.Mode()})
			itemsMu.Unlock()
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Set up progress with counted total
	o.progressMu.Lock()
	o.state.Progress.Total = totalSize.Load()
	o.state.Progress.Current = 0
	o.progressMu.Unlock()

	// Create destination root
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	// Process items: directories first (sorted by path length to ensure parents exist)
	// then files
	sort.Slice(items, func(i, j int) bool {
		// Directories before files
		if items[i].isDir != items[j].isDir {
			return items[i].isDir
		}
		// Shorter paths first (parents before children)
		return len(items[i].dstPath) < len(items[j].dstPath)
	})

	for _, item := range items {
		if item.isDir {
			if err := os.MkdirAll(item.dstPath, item.mode); err != nil {
				return err
			}
		} else {
			if err := o.copyFileWithProgress(item.srcPath, item.dstPath); err != nil {
				return err
			}
		}
	}

	if move {
		return os.RemoveAll(src)
	}
	return nil
}

func (o *Orchestrator) copyFileWithProgress(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// Progress-tracking writer (uses atomic add to avoid races with UI thread)
	pw := &progressWriter{
		w: dstFile,
		onWrite: func(n int64) {
			atomic.AddInt64(&o.state.Progress.Current, n)
			o.window.Invalidate()
		},
	}

	if _, err := io.Copy(pw, srcFile); err != nil {
		return err
	}

	return os.Chmod(dst, info.Mode())
}

func (o *Orchestrator) doDelete(path string) {
	info, err := os.Stat(path)
	if err != nil {
		log.Printf("Delete error: %v", err)
		return
	}

	o.setProgress(true, "Deleting "+filepath.Base(path), 0, 0)

	if info.IsDir() {
		err = os.RemoveAll(path)
	} else {
		err = os.Remove(path)
	}

	o.setProgress(false, "", 0, 0)

	if err != nil {
		log.Printf("Delete error: %v", err)
	}

	o.requestDir(o.state.CurrentPath)
}

// progressWriter wraps an io.Writer and calls onWrite after each write
type progressWriter struct {
	w       io.Writer
	onWrite func(int64)
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.w.Write(p)
	if n > 0 && pw.onWrite != nil {
		pw.onWrite(int64(n))
	}
	return n, err
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
