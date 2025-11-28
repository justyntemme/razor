package app

import (
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/op"
	
	// Updated import paths
	"github.com/justyntemme/razor/internal/fs"
	"github.com/justyntemme/razor/internal/ui"
)

// Orchestrator manages the application lifecycle and state.
type Orchestrator struct {
	window *app.Window
	fs     *fs.System
	ui     *ui.Renderer
	
	// Application State
	state ui.State
}

// NewOrchestrator wires up the dependencies.
func NewOrchestrator() *Orchestrator {
	return &Orchestrator{
		window: new(app.Window),
		fs:     fs.NewSystem(),
		ui:     ui.NewRenderer(),
		state:  ui.State{CurrentPath: "Initializing..."},
	}
}

// Run is the main entry point for the logic.
func (o *Orchestrator) Run() error {
	// 1. Start the File System worker in the background.
	go o.fs.Start()

	// 2. Start a goroutine to listen for FS responses.
	go o.processFSEvents()

	// 3. Trigger an initial load
	o.fs.RequestChan <- fs.Request{Op: fs.FetchDir, Path: "."}

	// 4. Run the main UI loop.
	var ops op.Ops
	for {
		switch e := o.window.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			o.ui.Layout(gtx, o.state)
			e.Frame(gtx.Ops)
		}
	}
}

// processFSEvents listens to the FS system and updates state.
func (o *Orchestrator) processFSEvents() {
	for resp := range o.fs.ResponseChan {
		// Update State safely
		o.state.CurrentPath = resp.Path
		o.state.Files = resp.Items

		log.Printf("App: Received data for %s", resp.Path)

		// Wake up the UI thread!
		o.window.Invalidate()
	}
}

// Main is called by cmd/razor.
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