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
	window       *app.Window
	fs           *fs.System
	store        *store.DB
	ui           *ui.Renderer
	state        ui.State
	history      []string
	historyIndex int
	debug        bool
	sortColumn   ui.SortColumn
	sortAsc      bool
	showDotfiles bool
	rawEntries   []ui.UIEntry
}

func NewOrchestrator(debug bool) *Orchestrator {
	r := ui.NewRenderer()
	r.Debug = debug
	return &Orchestrator{
		window:       new(app.Window),
		fs:           fs.NewSystem(),
		store:        store.NewDB(),
		ui:           r,
		state:        ui.State{SelectedIndex: -1, Favorites: make(map[string]bool)},
		historyIndex: -1,
		debug:        debug,
		sortAsc:      true,
	}
}

func (o *Orchestrator) Run(startPath string) error {
	if o.debug {
		log.Println("Starting Razor in DEBUG mode")
	}

	// Init DB
	configDir, _ := os.UserConfigDir()
	if err := o.store.Open(filepath.Join(configDir, "razor", "razor.db")); err != nil {
		log.Printf("Failed to open DB: %v", err)
	}
	defer o.store.Close()

	// Start workers
	go o.fs.Start()
	go o.store.Start()
	go o.processEvents()

	// Initial fetch
	o.store.RequestChan <- store.Request{Op: store.FetchFavorites}
	o.store.RequestChan <- store.Request{Op: store.FetchSettings}

	// Initial path
	if startPath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			startPath = home
		} else {
			startPath, _ = os.Getwd()
		}
	}
	o.navigate(startPath)

	// Event loop
	var ops op.Ops
	for {
		switch e := o.window.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			evt := o.ui.Layout(gtx, &o.state)

			if o.debug && evt.Action != ui.ActionNone {
				log.Printf("[DEBUG] Action: %d, Path: %s, Index: %d", evt.Action, evt.Path, evt.NewIndex)
			}

			o.handleUIEvent(evt)
			e.Frame(gtx.Ops)
		}
	}
}

func (o *Orchestrator) handleUIEvent(evt ui.UIEvent) {
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
		if err := platformOpen(evt.Path); err != nil {
			log.Printf("Error opening file: %v", err)
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
	}
}

func (o *Orchestrator) navigate(path string) {
	if o.historyIndex >= 0 && o.historyIndex < len(o.history)-1 {
		o.history = o.history[:o.historyIndex+1]
	}
	o.history = append(o.history, path)
	o.historyIndex = len(o.history) - 1
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
	o.fs.RequestChan <- fs.Request{Op: fs.FetchDir, Path: path}
}

func (o *Orchestrator) search(query string) {
	o.state.SelectedIndex = -1
	if query == "" {
		o.requestDir(o.state.CurrentPath)
		return
	}
	o.fs.RequestChan <- fs.Request{Op: fs.SearchDir, Path: o.state.CurrentPath, Query: query}
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

	o.rawEntries = make([]ui.UIEntry, len(resp.Entries))
	for i, e := range resp.Entries {
		o.rawEntries[i] = ui.UIEntry{
			Name: e.Name, Path: e.Path, IsDir: e.IsDir, Size: e.Size, ModTime: e.ModTime,
		}
	}

	o.state.CurrentPath = resp.Path
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
	}
	o.window.Invalidate()
}

func (o *Orchestrator) applyFilterAndSort() {
	// Filter
	var entries []ui.UIEntry
	for _, e := range o.rawEntries {
		if !o.showDotfiles && strings.HasPrefix(e.Name, ".") {
			continue
		}
		entries = append(entries, e)
	}

	// Sort with comparison function
	cmp := o.getComparator()
	sort.SliceStable(entries, func(i, j int) bool {
		// Directories first
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
	default: // SortByName
		return func(a, b ui.UIEntry) bool { return strings.ToLower(a.Name) < strings.ToLower(b.Name) }
	}
}

func Main(debug bool, startPath string) {
	go func() {
		o := NewOrchestrator(debug)
		if err := o.Run(startPath); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}