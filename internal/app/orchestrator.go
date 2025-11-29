package app

import (
	"log"
	"os"
	"path/filepath"

	"gioui.org/app"
	"gioui.org/op"

	"github.com/justyntemme/razor/internal/fs"
	"github.com/justyntemme/razor/internal/ui"
)

type Orchestrator struct {
	window *app.Window
	fs     *fs.System
	ui     *ui.Renderer
	state  ui.State

	history      []string
	historyIndex int
	debug        bool
}

func NewOrchestrator(debug bool) *Orchestrator {
	renderer := ui.NewRenderer()
	renderer.Debug = debug

	return &Orchestrator{
		window:       new(app.Window),
		fs:           fs.NewSystem(),
		ui:           renderer,
		state:        ui.State{CurrentPath: "Initializing...", SelectedIndex: -1},
		history:      make([]string, 0),
		historyIndex: -1,
		debug:        debug,
	}
}

func (o *Orchestrator) Run() error {
	if o.debug {
		log.Println("Starting Razor in DEBUG mode")
	}

	go o.fs.Start()
	go o.processFSEvents()

	cwd, _ := os.Getwd()
	o.navigate(cwd)

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
	// Live Search Logic: 
	// If the user clears the search box, revert to the standard directory listing (FetchDir).
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

func (o *Orchestrator) processFSEvents() {
	for resp := range o.fs.ResponseChan {
		if resp.Err != nil {
			log.Printf("FS Error: %v", resp.Err)
			continue
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
		o.state.Entries = uiEntries
		
		parent := filepath.Dir(resp.Path)
		o.state.CanBack = parent != resp.Path
		o.state.CanForward = o.historyIndex < len(o.history)-1

		if o.state.SelectedIndex >= len(uiEntries) {
			o.state.SelectedIndex = -1
		}

		o.window.Invalidate()
	}
}

func Main(debug bool) {
	go func() {
		orchestrator := NewOrchestrator(debug)
		if err := orchestrator.Run(); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}