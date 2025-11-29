package app

import (
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gioui.org/app"
	"gioui.org/op"

	"github.com/justyntemme/razor/internal/fs"
	"github.com/justyntemme/razor/internal/store"
	"github.com/justyntemme/razor/internal/ui"
)

type Orchestrator struct {
	window *app.Window
	fs     *fs.System
	store  *store.DB
	ui     *ui.Renderer
	state  ui.State

	history      []string
	historyIndex int
	debug        bool

	// Sort settings (persisted across navigation)
	sortColumn    ui.SortColumn
	sortAscending bool

	// Display settings
	showDotfiles bool

	// Raw entries before filtering (to reapply filter when setting changes)
	rawEntries []ui.UIEntry
}

func NewOrchestrator(debug bool) *Orchestrator {
	renderer := ui.NewRenderer()
	renderer.Debug = debug

	return &Orchestrator{
		window:        new(app.Window),
		fs:            fs.NewSystem(),
		store:         store.NewDB(),
		ui:            renderer,
		state:         ui.State{CurrentPath: "Initializing...", SelectedIndex: -1, Favorites: make(map[string]bool)},
		history:       make([]string, 0),
		historyIndex:  -1,
		debug:         debug,
		sortColumn:    ui.SortByName,
		sortAscending: true,
		showDotfiles:  false,
	}
}

// Run now accepts startPath to handle the --path flag or default to Home
func (o *Orchestrator) Run(startPath string) error {
	if o.debug {
		log.Println("Starting Razor in DEBUG mode")
	}

	// 1. Initialize DB
	configDir, _ := os.UserConfigDir()
	dbPath := filepath.Join(configDir, "razor", "razor.db")
	if err := o.store.Open(dbPath); err != nil {
		log.Printf("Failed to open DB: %v", err)
	}
	defer o.store.Close()

	// 2. Start Workers
	go o.fs.Start()
	go o.store.Start()

	// 3. Start Event Listener
	go o.processEvents()

	// 4. Initial Data Fetch
	o.store.RequestChan <- store.Request{Op: store.FetchFavorites}
	o.store.RequestChan <- store.Request{Op: store.FetchSettings}

	// 5. Initial Navigation
	initialPath := startPath
	if initialPath == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			initialPath = home
		} else {
			initialPath, _ = os.Getwd()
		}
	}
	o.navigate(initialPath)

	var ops op.Ops
	for {
		switch e := o.window.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			evt := o.ui.Layout(gtx, &o.state)

			if o.debug && evt.Action != ui.ActionNone {
				log.Printf("[DEBUG] UI Action: %d, Path: %s, Index: %d", evt.Action, evt.Path, evt.NewIndex)
			}

			switch evt.Action {
			case ui.ActionNavigate:
				o.navigate(evt.Path)
			case ui.ActionBack:
				o.goBack()
			case ui.ActionForward:
				o.goForward()
			case ui.ActionSelect:
				o.state.SelectedIndex = evt.NewIndex
				o.window.Invalidate()
			case ui.ActionSearch:
				o.search(evt.Path)
			case ui.ActionOpen:
				o.openFile(evt.Path)
			case ui.ActionAddFavorite:
				o.store.RequestChan <- store.Request{Op: store.AddFavorite, Path: evt.Path}
			case ui.ActionRemoveFavorite:
				o.store.RequestChan <- store.Request{Op: store.RemoveFavorite, Path: evt.Path}
			case ui.ActionSort:
				o.sortColumn = evt.SortColumn
				o.sortAscending = evt.SortAscending
				o.applyFilterAndSort()
				o.window.Invalidate()
			case ui.ActionToggleDotfiles:
				o.showDotfiles = evt.ShowDotfiles
				// Persist the setting
				value := "false"
				if o.showDotfiles {
					value = "true"
				}
				o.store.RequestChan <- store.Request{Op: store.SaveSetting, Key: "show_dotfiles", Value: value}
				// Reapply filter with new setting
				o.applyFilterAndSort()
				o.window.Invalidate()
			}

			e.Frame(gtx.Ops)
		}
	}
}

func (o *Orchestrator) navigate(path string) {
	if o.debug {
		log.Printf("[DEBUG] Navigate requested: %s", path)
	}
	if o.historyIndex >= 0 && o.historyIndex < len(o.history)-1 {
		o.history = o.history[:o.historyIndex+1]
	}
	o.history = append(o.history, path)
	o.historyIndex = len(o.history) - 1
	o.requestDir(path)
}

func (o *Orchestrator) goBack() {
	current := o.state.CurrentPath
	parent := filepath.Dir(current)
	if parent == current {
		return
	}
	if o.historyIndex > 0 && o.history[o.historyIndex-1] == parent {
		o.historyIndex--
		o.requestDir(parent)
		return
	}
	o.history = append(o.history[:o.historyIndex], append([]string{parent}, o.history[o.historyIndex:]...)...)
	o.requestDir(parent)
}

func (o *Orchestrator) goForward() {
	if o.historyIndex < len(o.history)-1 {
		o.historyIndex++
		path := o.history[o.historyIndex]
		o.requestDir(path)
	}
}

func (o *Orchestrator) requestDir(path string) {
	o.state.SelectedIndex = -1
	o.fs.RequestChan <- fs.Request{Op: fs.FetchDir, Path: path}
}

func (o *Orchestrator) search(query string) {
	if query == "" {
		if o.debug {
			log.Printf("[DEBUG] Search cleared, reverting to standard view.")
		}
		o.requestDir(o.state.CurrentPath)
		return
	}

	if o.debug {
		log.Printf("[DEBUG] Searching for: %s in %s", query, o.state.CurrentPath)
	}
	o.state.SelectedIndex = -1
	o.fs.RequestChan <- fs.Request{
		Op:    fs.SearchDir,
		Path:  o.state.CurrentPath,
		Query: query,
	}
}

func (o *Orchestrator) openFile(path string) {
	if o.debug {
		log.Printf("[DEBUG] Opening file via platform handler: %s", path)
	}

	// Delegate to the OS-specific implementation in platform_*.go
	if err := platformOpen(path); err != nil {
		log.Printf("Error opening file: %v", err)
	}
}

func (o *Orchestrator) processEvents() {
	for {
		select {
		case resp := <-o.fs.ResponseChan:
			o.handleFSResponse(resp)
		case resp := <-o.store.ResponseChan:
			o.handleStoreResponse(resp)
		}
	}
}

func (o *Orchestrator) handleFSResponse(resp fs.Response) {
	if resp.Err != nil {
		log.Printf("FS Error: %v", resp.Err)
		return
	}

	if o.debug {
		log.Printf("[DEBUG] Loaded %d entries for %s", len(resp.Entries), resp.Path)
	}

	uiEntries := make([]ui.UIEntry, len(resp.Entries))
	for i, e := range resp.Entries {
		uiEntries[i] = ui.UIEntry{
			Name:    e.Name,
			Path:    e.Path,
			IsDir:   e.IsDir,
			Size:    e.Size,
			ModTime: e.ModTime,
		}
	}

	o.state.CurrentPath = resp.Path
	o.rawEntries = uiEntries

	// Apply filtering and sorting
	o.applyFilterAndSort()

	parent := filepath.Dir(resp.Path)
	o.state.CanBack = parent != resp.Path
	o.state.CanForward = o.historyIndex < len(o.history)-1

	if o.state.SelectedIndex >= len(o.state.Entries) {
		o.state.SelectedIndex = -1
	}

	o.window.Invalidate()
}

func (o *Orchestrator) handleStoreResponse(resp store.Response) {
	if resp.Err != nil {
		log.Printf("Store Error: %v", resp.Err)
		return
	}

	switch resp.Op {
	case store.FetchFavorites:
		favMap := make(map[string]bool)
		favList := make([]ui.FavoriteItem, len(resp.Favorites))

		for i, path := range resp.Favorites {
			favMap[path] = true
			favList[i] = ui.FavoriteItem{
				Path: path,
				Name: filepath.Base(path),
			}
		}

		o.state.Favorites = favMap
		o.state.FavList = favList
		o.window.Invalidate()

	case store.FetchSettings:
		if val, ok := resp.Settings["show_dotfiles"]; ok {
			o.showDotfiles = val == "true"
			o.ui.ShowDotfiles = o.showDotfiles
			o.ui.SetShowDotfilesCheck(o.showDotfiles)
			// Reapply filter if we have entries
			if len(o.rawEntries) > 0 {
				o.applyFilterAndSort()
			}
		}
		o.window.Invalidate()
	}
}

// applyFilterAndSort filters dotfiles and sorts entries
func (o *Orchestrator) applyFilterAndSort() {
	// Filter dotfiles if setting is disabled
	var filtered []ui.UIEntry
	for _, entry := range o.rawEntries {
		if !o.showDotfiles && strings.HasPrefix(entry.Name, ".") {
			continue
		}
		filtered = append(filtered, entry)
	}

	o.state.Entries = filtered
	o.sortEntries()
}

// sortEntries sorts the current entries based on sortColumn and sortAscending
func (o *Orchestrator) sortEntries() {
	entries := o.state.Entries

	sort.SliceStable(entries, func(i, j int) bool {
		// Directories always come first
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}

		// Compare based on the selected column
		var less bool
		switch o.sortColumn {
		case ui.SortByName:
			less = strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		case ui.SortByDate:
			less = entries[i].ModTime.Before(entries[j].ModTime)
		case ui.SortByType:
			extI := strings.ToLower(filepath.Ext(entries[i].Name))
			extJ := strings.ToLower(filepath.Ext(entries[j].Name))
			if extI == extJ {
				// If same type, sort by name
				less = strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
			} else {
				less = extI < extJ
			}
		case ui.SortBySize:
			if entries[i].Size == entries[j].Size {
				// If same size, sort by name
				less = strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
			} else {
				less = entries[i].Size < entries[j].Size
			}
		default:
			less = strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		}

		// Reverse if descending
		if !o.sortAscending {
			return !less
		}
		return less
	})

	o.state.Entries = entries
}

// Main now correctly matches the signature expected by cmd/razor
func Main(debug bool, startPath string) {
	go func() {
		orchestrator := NewOrchestrator(debug)
		if err := orchestrator.Run(startPath); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}