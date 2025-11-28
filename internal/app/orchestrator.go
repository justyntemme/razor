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

	// History Management
	history      []string
	historyIndex int
}

func NewOrchestrator() *Orchestrator {
	return &Orchestrator{
		window:       new(app.Window),
		fs:           fs.NewSystem(),
		ui:           ui.NewRenderer(),
		// Default SelectedIndex to -1 (No Selection)
		state:        ui.State{CurrentPath: "Initializing...", SelectedIndex: -1},
		history:      make([]string, 0),
		historyIndex: -1, // Empty state
	}
}

func (o *Orchestrator) Run() error {
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
			}

			e.Frame(gtx.Ops)
		}
	}
}

func (o *Orchestrator) navigate(path string) {
	// 1. Truncate history at current index (remove forward history)
	if o.historyIndex >= 0 && o.historyIndex < len(o.history)-1 {
		o.history = o.history[:o.historyIndex+1]
	}

	// 2. Append new path
	o.history = append(o.history, path)
	
	// 3. Update index
	o.historyIndex = len(o.history) - 1

	o.requestDir(path)
}

func (o *Orchestrator) goBack() {
	current := o.state.CurrentPath
	parent := filepath.Dir(current)
	
	// Prevent going up past root
	if parent == current {
		return
	}

	// Optimization: If the PREVIOUS item in history is already the parent,
	// just use standard history traversal.
	if o.historyIndex > 0 && o.history[o.historyIndex-1] == parent {
		o.historyIndex--
		o.requestDir(parent)
		return
	}

	// "Insert Parent" Logic:
	// Insert the Parent at the CURRENT index to allow "Forward" to return here.
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
	log.Printf("Navigating to: %s", path)
	// Reset selection to -1 (None) when changing folders
	o.state.SelectedIndex = -1
	o.fs.RequestChan <- fs.Request{Op: fs.FetchDir, Path: path}
}

func (o *Orchestrator) processFSEvents() {
	for resp := range o.fs.ResponseChan {
		if resp.Err != nil {
			log.Printf("FS Error: %v", resp.Err)
			continue
		}

		uiEntries := make([]ui.UIEntry, len(resp.Entries))
		for i, e := range resp.Entries {
			uiEntries[i] = ui.UIEntry{
				Name:  e.Name,
				Path:  e.Path,
				IsDir: e.IsDir,
			}
		}

		o.state.CurrentPath = resp.Path
		o.state.Entries = uiEntries
		
		parent := filepath.Dir(resp.Path)
		o.state.CanBack = parent != resp.Path
		o.state.CanForward = o.historyIndex < len(o.history)-1

		// Reset selection if out of bounds (or keep at -1)
		if o.state.SelectedIndex >= len(uiEntries) {
			o.state.SelectedIndex = -1
		}

		o.window.Invalidate()
	}
}

func Main() {
	go func() {
		orchestrator := NewOrchestrator()
		if err := orchestrator.Run(); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}