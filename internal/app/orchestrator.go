package app

import (
	"log"
	"os"

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
}

func NewOrchestrator() *Orchestrator {
	return &Orchestrator{
		window: new(app.Window),
		fs:     fs.NewSystem(),
		ui:     ui.NewRenderer(),
		state:  ui.State{CurrentPath: "Initializing..."},
	}
}

func (o *Orchestrator) Run() error {
	go o.fs.Start()
	go o.processFSEvents()

	// Initial directory fetch (Current Working Directory)
	cwd, _ := os.Getwd()
	o.fs.RequestChan <- fs.Request{Op: fs.FetchDir, Path: cwd}

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

func (o *Orchestrator) processFSEvents() {
	for resp := range o.fs.ResponseChan {
		if resp.Err != nil {
			log.Printf("FS Error: %v", resp.Err)
			continue
		}

		// Map FS entries to UI entries
		// This separation prevents UI from depending on FS types
		uiEntries := make([]ui.UIEntry, len(resp.Entries))
		for i, e := range resp.Entries {
			uiEntries[i] = ui.UIEntry{
				Name:  e.Name,
				IsDir: e.IsDir,
			}
		}

		o.state.CurrentPath = resp.Path
		o.state.Entries = uiEntries

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