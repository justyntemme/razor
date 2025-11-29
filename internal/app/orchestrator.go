package app

import (
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

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
	sortColumn   ui.SortColumn
	sortAsc      bool
	showDotfiles bool
	rawEntries   []ui.UIEntry
	progressMu   sync.Mutex
	homePath     string
}

func NewOrchestrator() *Orchestrator {
	home, _ := os.UserHomeDir()
	return &Orchestrator{
		window:       new(app.Window),
		fs:           fs.NewSystem(),
		store:        store.NewDB(),
		ui:           ui.NewRenderer(),
		state:        ui.State{SelectedIndex: -1, Favorites: make(map[string]bool)},
		historyIndex: -1,
		sortAsc:      true,
		homePath:     home,
	}
}

func (o *Orchestrator) Run(startPath string) error {
	if debugEnabled {
		log.Println("Starting Razor in DEBUG mode")
	}

	configDir, _ := os.UserConfigDir()
	if err := o.store.Open(filepath.Join(configDir, "razor", "razor.db")); err != nil {
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
			evt := o.ui.Layout(gtx, &o.state)

			if evt.Action != ui.ActionNone {
				debugLog("Action: %d, Path: %s, Index: %d", evt.Action, evt.Path, evt.NewIndex)
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
		o.navigate(evt.Path)
	case ui.ActionBack:
		o.goBack()
	case ui.ActionForward:
		o.goForward()
	case ui.ActionHome:
		o.navigate(o.homePath)
	case ui.ActionNewWindow:
		o.openNewWindow()
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
	var entries []ui.UIEntry
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

func (o *Orchestrator) doPaste() {
	clip := o.state.Clipboard
	if clip == nil {
		return
	}

	src := clip.Path
	dstDir := o.state.CurrentPath
	dstName := filepath.Base(src)
	dst := filepath.Join(dstDir, dstName)

	// Avoid overwriting - append suffix if exists
	if _, err := os.Stat(dst); err == nil {
		ext := filepath.Ext(dstName)
		base := strings.TrimSuffix(dstName, ext)
		for i := 1; ; i++ {
			dst = filepath.Join(dstDir, base+"_copy"+itoa(i)+ext)
			if _, err := os.Stat(dst); os.IsNotExist(err) {
				break
			}
		}
	}

	info, err := os.Stat(src)
	if err != nil {
		log.Printf("Paste error: %v", err)
		return
	}

	label := "Copying"
	if clip.Op == ui.ClipCut {
		label = "Moving"
	}

	if info.IsDir() {
		o.setProgress(true, label+" folder...", 0, 0)
		err = o.copyDir(src, dst, clip.Op == ui.ClipCut)
	} else {
		o.setProgress(true, label+" "+filepath.Base(src), 0, info.Size())
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

	// Progress-tracking writer
	pw := &progressWriter{
		w: dstFile,
		onWrite: func(n int64) {
			o.progressMu.Lock()
			o.state.Progress.Current += n
			o.progressMu.Unlock()
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
	// Calculate total size first
	var totalSize int64
	filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})

	o.progressMu.Lock()
	o.state.Progress.Total = totalSize
	o.state.Progress.Current = 0
	o.progressMu.Unlock()

	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := o.copyDir(srcPath, dstPath, false); err != nil {
				return err
			}
		} else {
			if err := o.copyFileWithProgress(srcPath, dstPath); err != nil {
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

	pw := &progressWriter{
		w: dstFile,
		onWrite: func(n int64) {
			o.progressMu.Lock()
			o.state.Progress.Current += n
			o.progressMu.Unlock()
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

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
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