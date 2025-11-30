package ui

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type UIAction int

const (
	ActionNone UIAction = iota
	ActionNavigate
	ActionBack
	ActionForward
	ActionHome
	ActionNewWindow
	ActionSelect
	ActionSearch
	ActionOpen
	ActionOpenWith
	ActionOpenWithApp
	ActionAddFavorite
	ActionRemoveFavorite
	ActionSort
	ActionToggleDotfiles
	ActionCopy
	ActionCut
	ActionPaste
	ActionDelete
	ActionConfirmDelete
	ActionCancelDelete
	ActionCreateFile
	ActionCreateFolder
	ActionClearSearch
	ActionRename
	// Conflict resolution actions
	ActionConflictReplace
	ActionConflictKeepBoth
	ActionConflictSkip
	ActionConflictStop
	// Settings actions
	ActionChangeSearchEngine
	ActionChangeDefaultDepth
	ActionChangeTheme
	// Search history
	ActionRequestSearchHistory
	ActionSelectSearchHistory
	// Recent files
	ActionShowRecentFiles
	ActionOpenFileLocation
)

type ClipOp int

const (
	ClipCopy ClipOp = iota
	ClipCut
)

// SearchEngineInfo contains information about a search engine
type SearchEngineInfo struct {
	Name      string // Display name (e.g., "ripgrep", "ugrep", "Built-in")
	ID        string // Internal ID (e.g., "ripgrep", "ugrep", "builtin")
	Command   string // Command path (empty for builtin)
	Available bool   // Whether it's installed
	Version   string // Version string if available
}

// ConflictResolution represents the user's choice for handling file conflicts
type ConflictResolution int

const (
	ConflictAsk ConflictResolution = iota // Ask for each conflict
	ConflictReplaceAll                    // Replace all conflicting files
	ConflictKeepBothAll                   // Keep both for all conflicts
	ConflictSkipAll                       // Skip all conflicting files
)

// ConflictState holds state for the file conflict dialog
type ConflictState struct {
	Active      bool   // Whether dialog is visible
	SourcePath  string // Path of file being copied
	DestPath    string // Path where file would be placed
	SourceSize  int64  // Size of source file
	DestSize    int64  // Size of existing destination file
	SourceTime  time.Time
	DestTime    time.Time
	IsDir       bool   // Whether the conflict is for a directory
	ApplyToAll  bool   // Checkbox state for "Apply to All"
}

type SortColumn int

const (
	SortByName SortColumn = iota
	SortByDate
	SortByType
	SortBySize
)

type Clipboard struct {
	Path string
	Op   ClipOp
}

type UIEvent struct {
	Action             UIAction
	Path               string
	OldPath            string // Original path for rename operations
	NewIndex           int
	SortColumn         SortColumn
	SortAscending      bool
	ShowDotfiles       bool
	ClipOp             ClipOp
	FileName           string
	AppPath            string
	SearchEngine       string // Selected search engine ID
	DefaultDepth       int    // Default recursive search depth
	DarkMode           bool   // Theme dark mode setting
	SearchHistoryQuery string // For fetching/selecting search history
	SearchSubmitted    bool   // True if search was submitted via Enter, not typed
}

type UIEntry struct {
	Name, Path    string
	IsDir         bool
	Size          int64
	ModTime       time.Time
	Clickable     widget.Clickable
	RightClickTag int
	LastClick     time.Time
}

type FavoriteItem struct {
	Name, Path    string
	Clickable     widget.Clickable
	RightClickTag int
}

type ProgressState struct {
	Active  bool
	Label   string
	Current int64 // Updated atomically via sync/atomic in orchestrator
	Total   int64
}

type DriveItem struct {
	Name, Path string
	Clickable  widget.Clickable
}

// SearchHistoryItem represents a search history entry for UI display
type SearchHistoryItem struct {
	Query string
	Score float64
}

// DetectedDirective represents a parsed search directive for visual display
type DetectedDirective struct {
	Type  string // "contents", "ext", "size", "modified", "filename"
	Value string // The value after the colon
	Full  string // Full directive string e.g. "contents:foo"
}

// parseDirectivesForDisplay extracts directives from search text for visual feedback
func parseDirectivesForDisplay(text string) ([]DetectedDirective, string) {
	var directives []DetectedDirective
	var remaining []string
	
	// Known directive prefixes
	knownDirectives := []string{"contents:", "ext:", "size:", "modified:", "filename:", "recursive:", "depth:"}
	
	parts := strings.Fields(text)
	for _, part := range parts {
		found := false
		for _, prefix := range knownDirectives {
			if strings.HasPrefix(strings.ToLower(part), prefix) {
				dirType := strings.TrimSuffix(prefix, ":")
				value := part[len(prefix):]
				// For recursive, empty value is OK (defaults to depth 10)
				if value != "" || dirType == "recursive" || dirType == "depth" {
					directives = append(directives, DetectedDirective{
						Type:  dirType,
						Value: value,
						Full:  part,
					})
					found = true
					break
				}
			}
		}
		if !found {
			remaining = append(remaining, part)
		}
	}
	
	return directives, strings.Join(remaining, " ")
}

type State struct {
	CurrentPath    string
	Entries        []UIEntry
	SelectedIndex  int
	CanBack        bool
	CanForward     bool
	Favorites      map[string]bool
	FavList        []FavoriteItem
	Clipboard      *Clipboard
	Progress       ProgressState
	DeleteTarget   string
	Drives         []DriveItem
	IsSearchResult bool
	SearchQuery    string
	Conflict       ConflictState // File conflict dialog state
}

type Renderer struct {
	Theme               *material.Theme
	listState, favState layout.List
	driveState          layout.List
	sidebarScroll       layout.List // Scrollable container for entire sidebar
	sidebarTabs         *TabBar     // Tab bar for Favorites/Drives switching
	sidebarLayout       string      // "tabbed" | "stacked" | "favorites_only" | "drives_only"
	backBtn, fwdBtn     widget.Clickable
	homeBtn             widget.Clickable
	bgClick             widget.Clickable
	focused             bool
	pathEditor          widget.Editor
	pathClick           widget.Clickable
	isEditing           bool
	searchEditor        widget.Editor
	searchClearBtn      widget.Clickable
	searchActive        bool
	lastSearchQuery     string
	detectedDirectives     []DetectedDirective // Parsed directives for visual display
	directiveRestored      bool                // Track if we've restored directory after detecting directive
	lastParsedSearchText   string              // Cache key for directive parsing
	menuVisible         bool
	menuPos             image.Point
	menuPath            string
	menuIsDir, menuIsFav bool
	menuIsBackground    bool // True when menu is shown on empty space (not on a file/folder)
	openBtn, copyBtn    widget.Clickable
	cutBtn, pasteBtn    widget.Clickable
	deleteBtn           widget.Clickable
	renameBtn           widget.Clickable
	favBtn              widget.Clickable
	openWithBtn         widget.Clickable
	fileMenuBtn         widget.Clickable
	fileMenuOpen        bool
	newWindowBtn        widget.Clickable
	newFileBtn          widget.Clickable
	newFolderBtn        widget.Clickable
	settingsBtn         widget.Clickable
	settingsOpen        bool
	settingsCloseBtn    widget.Clickable
	searchEngine        widget.Enum
	mousePos            image.Point
	mouseTag            struct{}
	bgRightClickTag     struct{} // Tag for detecting right-clicks on empty space
	fileListOffset      image.Point // Offset of file list area from window origin

	// Delete confirmation
	deleteConfirmOpen bool
	deleteConfirmYes  widget.Clickable
	deleteConfirmNo   widget.Clickable

	// Inline rename
	renameIndex    int           // Index of item being renamed (-1 if none)
	renameEditor   widget.Editor // Editor for inline rename
	renamePath     string        // Path of item being renamed

	// Create file/folder dialog
	createDialogOpen   bool
	createDialogIsDir  bool
	createDialogEditor widget.Editor
	createDialogOK     widget.Clickable
	createDialogCancel widget.Clickable

	// Conflict resolution dialog
	conflictReplaceBtn  widget.Clickable
	conflictKeepBothBtn widget.Clickable
	conflictSkipBtn     widget.Clickable
	conflictStopBtn     widget.Clickable
	conflictApplyToAll  widget.Bool

	// Column sorting
	headerBtns    [4]widget.Clickable
	SortColumn    SortColumn
	SortAscending bool

	// Settings
	ShowDotfiles      bool
	showDotfilesCheck widget.Bool
	
	// Search engine settings
	SearchEngines     []SearchEngineInfo // Available engines (detected on startup)
	SelectedEngine    string             // Currently selected engine name
	
	// Default recursive depth setting
	DefaultDepth      int               // Current default depth value
	depthDecBtn       widget.Clickable  // Decrease depth button
	depthIncBtn       widget.Clickable  // Increase depth button
	
	// Animation state for indeterminate progress
	progressAnimStart time.Time

	// Theme settings
	DarkMode      bool
	darkModeCheck widget.Bool

	// Config error banner
	ConfigError string // Non-empty if config.json failed to parse

	// Search history dropdown
	searchHistoryVisible  bool
	searchHistoryItems    []SearchHistoryItem
	searchHistoryBtns     [3]widget.Clickable // Up to 3 items
	searchBoxClicked      bool                // Track if search box was clicked
	lastHistoryQuery      string              // Track last query we fetched history for
	searchBoxPos          image.Point         // Position of search box for overlay dropdown
	searchEditorFocused   bool                // Track if search editor has focus

	// Preview pane state
	previewVisible    bool           // Whether preview pane is shown
	previewPath       string         // Path of file being previewed
	previewContent    string         // Content loaded for preview
	previewError      string         // Error message if load failed
	previewIsJSON     bool           // Whether content is JSON (for formatting)
	previewScroll     layout.List    // Scrollable list for preview content
	previewExtensions []string       // Extensions that trigger preview
	previewMaxSize    int64          // Max file size to preview
	previewWidthPct   int            // Width percentage for preview pane

	// Recent files state
	recentFilesBtn    widget.Clickable // Button to show recent files
	openLocationBtn   widget.Clickable // Context menu item to open file location
	isRecentView      bool             // True when viewing recent files

	// Preview pane close button
	previewCloseBtn   widget.Clickable
}

var (
	colWhite     = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	colBlack     = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	colGray      = color.NRGBA{R: 100, G: 100, B: 100, A: 255}
	colLightGray = color.NRGBA{R: 200, G: 200, B: 200, A: 255}
	colDirBlue   = color.NRGBA{R: 0, G: 0, B: 128, A: 255}
	colSelected  = color.NRGBA{R: 200, G: 220, B: 255, A: 255}
	colSidebar   = color.NRGBA{R: 245, G: 245, B: 245, A: 255}
	colDisabled  = color.NRGBA{R: 150, G: 150, B: 150, A: 255}
	colProgress  = color.NRGBA{R: 66, G: 133, B: 244, A: 255}
	colDanger    = color.NRGBA{R: 220, G: 53, B: 69, A: 255}
	colHomeBtnBg = color.NRGBA{R: 76, G: 175, B: 80, A: 255}
	colDriveIcon = color.NRGBA{R: 96, G: 125, B: 139, A: 255}
	colSuccess   = color.NRGBA{R: 40, G: 167, B: 69, A: 255}
	colAccent    = color.NRGBA{R: 66, G: 133, B: 244, A: 255}
	colDirective = color.NRGBA{R: 103, G: 58, B: 183, A: 255}  // Purple for directives
	colDirectiveBg = color.NRGBA{R: 237, G: 231, B: 246, A: 255} // Light purple background
	// Config error banner colors
	colErrorBannerBg   = color.NRGBA{R: 220, G: 53, B: 69, A: 255}   // Red background
	colErrorBannerText = color.NRGBA{R: 139, G: 69, B: 0, A: 255}    // Dark orange text (readable on red)
)

func NewRenderer() *Renderer {
	r := &Renderer{Theme: material.NewTheme(), SortAscending: true, DefaultDepth: 2, renameIndex: -1}
	r.listState.Axis = layout.Vertical
	r.favState.Axis = layout.Vertical
	r.driveState.Axis = layout.Vertical
	r.sidebarScroll.Axis = layout.Vertical
	r.previewScroll.Axis = layout.Vertical

	// Initialize sidebar tabs (default to manila, can be changed via SetSidebarTabStyle)
	r.sidebarTabs = NewTabBar(
		Tab{Label: "Favorites", ID: "favorites"},
		Tab{Label: "Drives", ID: "drives"},
	)
	r.sidebarTabs.Style = TabStyleManila
	r.sidebarTabs.Distribute = false
	r.sidebarLayout = "stacked" // Default layout
	r.pathEditor.SingleLine, r.pathEditor.Submit = true, true
	r.searchEditor.SingleLine, r.searchEditor.Submit = true, true
	r.createDialogEditor.SingleLine, r.createDialogEditor.Submit = true, true
	r.renameEditor.SingleLine, r.renameEditor.Submit = true, true
	r.searchEngine.Value = "builtin"
	r.SelectedEngine = "builtin"

	// Default preview config (will be overridden by SetPreviewConfig)
	r.previewExtensions = []string{".txt", ".json", ".csv", ".md", ".log"}
	r.previewMaxSize = 1024 * 1024 // 1MB
	r.previewWidthPct = 33

	return r
}

func (r *Renderer) SetShowDotfilesCheck(v bool) { r.showDotfilesCheck.Value = v }

func (r *Renderer) SetSearchEngine(engineID string) {
	r.SelectedEngine = engineID
	r.searchEngine.Value = engineID
}

func (r *Renderer) SetDefaultDepth(depth int) {
	r.DefaultDepth = depth
}

func (r *Renderer) SetDarkMode(dark bool) {
	r.DarkMode = dark
	r.darkModeCheck.Value = dark
	r.applyTheme()
}

// SetConfigError sets the config error message to display in the banner
func (r *Renderer) SetConfigError(err string) {
	r.ConfigError = err
}

// SetSearchHistory updates the search history dropdown items
func (r *Renderer) SetSearchHistory(items []SearchHistoryItem) {
	r.searchHistoryItems = items
}

// ShowSearchHistory shows the search history dropdown
func (r *Renderer) ShowSearchHistory() {
	r.searchHistoryVisible = true
}

// HideSearchHistory hides the search history dropdown
func (r *Renderer) HideSearchHistory() {
	r.searchHistoryVisible = false
}

// SetSidebarTabStyle sets the sidebar tab style based on config
// Valid values: "manila", "underline", "pill"
func (r *Renderer) SetSidebarTabStyle(style string) {
	switch style {
	case "underline":
		r.sidebarTabs.Style = TabStyleUnderline
		r.sidebarTabs.Distribute = true
	case "pill":
		r.sidebarTabs.Style = TabStylePill
		r.sidebarTabs.Distribute = true
	case "manila":
		fallthrough
	default:
		r.sidebarTabs.Style = TabStyleManila
		r.sidebarTabs.Distribute = false
	}
}

// SetSidebarLayout sets the sidebar layout mode
// Valid values: "tabbed", "stacked", "favorites_only", "drives_only"
func (r *Renderer) SetSidebarLayout(layout string) {
	switch layout {
	case "tabbed", "favorites_only", "drives_only":
		r.sidebarLayout = layout
	default:
		r.sidebarLayout = "stacked"
	}
}

// SetPreviewConfig sets the preview pane configuration
func (r *Renderer) SetPreviewConfig(extensions []string, maxSize int64, widthPct int) {
	r.previewExtensions = extensions
	r.previewMaxSize = maxSize
	r.previewWidthPct = widthPct
}

// ShowPreview loads and displays the preview pane for the given file
func (r *Renderer) ShowPreview(path string) error {
	// Check file extension
	ext := strings.ToLower(filepath.Ext(path))
	supported := false
	for _, e := range r.previewExtensions {
		if strings.ToLower(e) == ext {
			supported = true
			break
		}
	}
	if !supported {
		r.HidePreview()
		return nil
	}

	// Check file size
	info, err := os.Stat(path)
	if err != nil {
		r.previewError = fmt.Sprintf("Cannot access file: %v", err)
		r.previewVisible = true
		r.previewPath = path
		r.previewContent = ""
		return err
	}
	if info.IsDir() {
		r.HidePreview()
		return nil
	}
	if r.previewMaxSize > 0 && info.Size() > r.previewMaxSize {
		r.previewError = fmt.Sprintf("File too large (%s)", formatSize(info.Size()))
		r.previewVisible = true
		r.previewPath = path
		r.previewContent = ""
		return nil
	}

	// Read file content
	data, err := os.ReadFile(path)
	if err != nil {
		r.previewError = fmt.Sprintf("Cannot read file: %v", err)
		r.previewVisible = true
		r.previewPath = path
		r.previewContent = ""
		return err
	}

	r.previewPath = path
	r.previewError = ""
	r.previewIsJSON = ext == ".json"

	// Format JSON with indentation
	if r.previewIsJSON {
		var jsonData interface{}
		if err := json.Unmarshal(data, &jsonData); err == nil {
			if formatted, err := json.MarshalIndent(jsonData, "", "  "); err == nil {
				r.previewContent = string(formatted)
			} else {
				r.previewContent = string(data)
			}
		} else {
			r.previewContent = string(data)
			r.previewError = "Invalid JSON: " + err.Error()
		}
	} else {
		r.previewContent = string(data)
	}

	r.previewVisible = true
	return nil
}

// HidePreview hides the preview pane
func (r *Renderer) HidePreview() {
	r.previewVisible = false
	r.previewPath = ""
	r.previewContent = ""
	r.previewError = ""
}

// IsPreviewVisible returns whether the preview pane is currently shown
func (r *Renderer) IsPreviewVisible() bool {
	return r.previewVisible
}

// SetRecentView sets whether we're viewing recent files
func (r *Renderer) SetRecentView(isRecent bool) {
	r.isRecentView = isRecent
}

// IsRecentView returns whether we're viewing recent files
func (r *Renderer) IsRecentView() bool {
	return r.isRecentView
}

func (r *Renderer) applyTheme() {
	if r.DarkMode {
		// Dark theme colors
		r.Theme.Palette.Bg = color.NRGBA{R: 30, G: 30, B: 30, A: 255}
		r.Theme.Palette.Fg = color.NRGBA{R: 220, G: 220, B: 220, A: 255}
		r.Theme.Palette.ContrastBg = color.NRGBA{R: 50, G: 50, B: 50, A: 255}
		r.Theme.Palette.ContrastFg = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	} else {
		// Light theme colors (default)
		r.Theme.Palette.Bg = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		r.Theme.Palette.Fg = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
		r.Theme.Palette.ContrastBg = color.NRGBA{R: 66, G: 133, B: 244, A: 255}
		r.Theme.Palette.ContrastFg = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	}
}

func (r *Renderer) ShowCreateDialog(isDir bool) {
	r.createDialogOpen = true
	r.createDialogIsDir = isDir
	r.createDialogEditor.SetText("")
}

func (r *Renderer) StartRename(index int, path, name string) {
	r.renameIndex = index
	r.renamePath = path
	r.renameEditor.SetText(name)
	// Select all text for easy replacement
	r.renameEditor.SetCaret(0, len(name))
}

func (r *Renderer) CancelRename() {
	r.renameIndex = -1
	r.renamePath = ""
}

func (r *Renderer) detectRightClick(gtx layout.Context, tag event.Tag) bool {
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Press | pointer.Release})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok && e.Kind == pointer.Press && e.Buttons.Contain(pointer.ButtonSecondary) {
			// Don't override mousePos here - use the global position tracked in processGlobalInput
			// The position from this event is relative to the local clip area, not window coordinates
			return true
		}
	}
	return false
}

func (r *Renderer) processGlobalInput(gtx layout.Context, state *State) UIEvent {
	// Keyboard input handling

	for {
		e, ok := gtx.Event(key.Filter{Focus: true, Name: ""})
		if !ok {
			break
		}
		if r.isEditing || r.settingsOpen || r.deleteConfirmOpen || r.createDialogOpen {
			continue
		}
		k, ok := e.(key.Event)
		if !ok || k.State != key.Press {
			continue
		}
		switch k.Name {
		case "Up":
			if idx := state.SelectedIndex - 1; idx >= 0 {
				r.listState.ScrollTo(idx)
				return UIEvent{Action: ActionSelect, NewIndex: idx}
			} else if state.SelectedIndex == -1 && len(state.Entries) > 0 {
				idx := len(state.Entries) - 1
				r.listState.ScrollTo(idx)
				return UIEvent{Action: ActionSelect, NewIndex: idx}
			}
		case "Down":
			if idx := state.SelectedIndex + 1; idx < len(state.Entries) {
				r.listState.ScrollTo(idx)
				return UIEvent{Action: ActionSelect, NewIndex: idx}
			} else if state.SelectedIndex == -1 && len(state.Entries) > 0 {
				return UIEvent{Action: ActionSelect, NewIndex: 0}
			}
		case "Left":
			if k.Modifiers.Contain(key.ModAlt) && state.CanBack {
				return UIEvent{Action: ActionBack}
			}
		case "Right":
			if k.Modifiers.Contain(key.ModAlt) && state.CanForward {
				return UIEvent{Action: ActionForward}
			}
		case "Return", "Enter":
			if idx := state.SelectedIndex; idx >= 0 && idx < len(state.Entries) {
				item := state.Entries[idx]
				if item.IsDir {
					return UIEvent{Action: ActionNavigate, Path: item.Path}
				}
				return UIEvent{Action: ActionOpen, Path: item.Path}
			}
		case "C":
			if k.Modifiers.Contain(key.ModCtrl) && state.SelectedIndex >= 0 {
				return UIEvent{Action: ActionCopy, Path: state.Entries[state.SelectedIndex].Path}
			}
		case "X":
			if k.Modifiers.Contain(key.ModCtrl) && state.SelectedIndex >= 0 {
				return UIEvent{Action: ActionCut, Path: state.Entries[state.SelectedIndex].Path}
			}
		case "V":
			if k.Modifiers.Contain(key.ModCtrl) && state.Clipboard != nil {
				return UIEvent{Action: ActionPaste}
			}
		case "N":
			if k.Modifiers.Contain(key.ModCtrl) {
				if k.Modifiers.Contain(key.ModShift) {
					r.ShowCreateDialog(true)
				} else {
					r.ShowCreateDialog(false)
				}
			}
		case "⌫", "⌦", "Delete":
			if state.SelectedIndex >= 0 {
				r.deleteConfirmOpen = true
				state.DeleteTarget = state.Entries[state.SelectedIndex].Path
			}
		case key.NameEscape:
			// Dismiss preview pane on Escape
			if r.previewVisible {
				r.HidePreview()
			}
		}
	}
	return UIEvent{}
}

func (r *Renderer) renderColumns(gtx layout.Context) (layout.Dimensions, UIEvent) {
	type colDef struct {
		label string
		col   SortColumn
		flex  float32
		align text.Alignment
	}
	cols := []colDef{
		{"Name", SortByName, 0.5, text.Start},
		{"Date Modified", SortByDate, 0.25, text.Start},
		{"Type", SortByType, 0.15, text.Start},
		{"Size", SortBySize, 0.10, text.End},
	}

	var evt UIEvent
	children := make([]layout.FlexChild, len(cols))

	for i, c := range cols {
		i, c := i, c
		if r.headerBtns[i].Clicked(gtx) {
			if r.SortColumn == c.col {
				r.SortAscending = !r.SortAscending
			} else {
				r.SortColumn, r.SortAscending = c.col, true
			}
			evt = UIEvent{Action: ActionSort, SortColumn: r.SortColumn, SortAscending: r.SortAscending}
		}

		children[i] = layout.Flexed(c.flex, func(gtx layout.Context) layout.Dimensions {
			label := c.label
			if r.SortColumn == c.col {
				if r.SortAscending {
					label += " ▲"
				} else {
					label += " ▼"
				}
			}
			textColor, weight := colGray, font.Normal
			if r.SortColumn == c.col {
				textColor, weight = colBlack, font.Medium
			}
			return material.Clickable(gtx, &r.headerBtns[i], func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(r.Theme, label)
				lbl.Color, lbl.Font.Weight, lbl.Alignment = textColor, weight, c.align
				return lbl.Layout(gtx)
			})
		})
	}

	return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx, children...), evt
}

// renderRow renders a file/folder row. Returns dimensions, left-clicked, right-clicked, click position, and rename event.
func (r *Renderer) renderRow(gtx layout.Context, item *UIEntry, index int, selected bool, isRenaming bool) (layout.Dimensions, bool, bool, image.Point, *UIEvent) {
	// Check for left-click BEFORE layout (Gio pattern)
	leftClicked := item.Clickable.Clicked(gtx)
	
	// Check for right-click and capture position
	rightClicked := false
	var clickPos image.Point
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &item.RightClickTag, Kinds: pointer.Press})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok && e.Buttons.Contain(pointer.ButtonSecondary) {
			rightClicked = true
			clickPos = image.Pt(int(e.Position.X), int(e.Position.Y))
		}
	}
	
	// Check for rename submission or cancellation
	var renameEvent *UIEvent
	if isRenaming {
		// Handle Enter to submit
		for {
			ev, ok := r.renameEditor.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.SubmitEvent); ok {
				newName := r.renameEditor.Text()
				if newName != "" && newName != item.Name {
					renameEvent = &UIEvent{
						Action:  ActionRename,
						Path:    filepath.Join(filepath.Dir(r.renamePath), newName),
						OldPath: r.renamePath,
					}
				}
				r.CancelRename()
			}
		}
		
		// Handle Escape to cancel - check focused key events
		for {
			ev, ok := gtx.Event(key.Filter{Focus: true, Name: key.NameEscape})
			if !ok {
				break
			}
			if e, ok := ev.(key.Event); ok && e.State == key.Press {
				r.CancelRename()
			}
		}
	}
	
	// Layout the clickable row (but not if renaming - we handle clicks differently)
	var dims layout.Dimensions
	if isRenaming {
		// For renaming, render content first to get proper size, then draw background
		// Use a macro to record the content, measure it, then draw background + content
		macro := op.Record(gtx.Ops)
		contentDims := r.renderRowContent(gtx, item, true)
		call := macro.Stop()
		
		// Draw selection background sized to content
		paint.FillShape(gtx.Ops, colSelected, clip.Rect{Max: contentDims.Size}.Op())
		// Replay the content on top
		call.Add(gtx.Ops)
		
		dims = contentDims
	} else {
		dims = material.Clickable(gtx, &item.Clickable, func(gtx layout.Context) layout.Dimensions {
			// Register right-click handler on this row's area
			defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
			event.Op(gtx.Ops, &item.RightClickTag)
			
			// Draw selection background
			if selected {
				paint.FillShape(gtx.Ops, colSelected, clip.Rect{Max: gtx.Constraints.Max}.Op())
			}

			return r.renderRowContent(gtx, item, false)
		})
	}
	
	return dims, leftClicked, rightClicked, clickPos, renameEvent
}

// renderRowContent renders the content of a row (shared between normal and rename mode)
func (r *Renderer) renderRowContent(gtx layout.Context, item *UIEntry, isRenaming bool) layout.Dimensions {
	name, typeStr, sizeStr := item.Name, "File", formatSize(item.Size)
	dateStr := item.ModTime.Format("01/02/06 03:04 PM")
	textColor, weight := colBlack, font.Normal

	if item.IsDir {
		if !isRenaming {
			name = item.Name + "/"
		}
		typeStr, sizeStr = "File Folder", ""
		textColor, weight = colDirBlue, font.Bold
	} else if ext := filepath.Ext(item.Name); len(ext) > 1 {
		typeStr = strings.ToUpper(ext[1:]) + " File"
	}

	return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(0.5, func(gtx layout.Context) layout.Dimensions {
					if isRenaming {
						// Show editor for renaming
						gtx.Execute(key.FocusCmd{Tag: &r.renameEditor})
						ed := material.Editor(r.Theme, &r.renameEditor, "")
						ed.TextSize = unit.Sp(14)
						ed.Color = textColor
						ed.Font.Weight = weight
						return widget.Border{Color: colAccent, Width: unit.Dp(1)}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, ed.Layout)
							})
					}
					lbl := material.Body1(r.Theme, name)
					lbl.Color, lbl.Font.Weight, lbl.MaxLines = textColor, weight, 1
					return lbl.Layout(gtx)
				}),
				layout.Flexed(0.25, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(r.Theme, dateStr)
					lbl.Color, lbl.MaxLines = colGray, 1
					return lbl.Layout(gtx)
				}),
				layout.Flexed(0.15, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(r.Theme, typeStr)
					lbl.Color, lbl.MaxLines = colGray, 1
					return lbl.Layout(gtx)
				}),
				layout.Flexed(0.10, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(r.Theme, sizeStr)
					lbl.Color, lbl.Alignment, lbl.MaxLines = colGray, text.End, 1
					return lbl.Layout(gtx)
				}),
			)
		})
}

// renderFavoriteRow renders a favorite item. Returns dimensions, left-clicked, right-clicked, and click position.
func (r *Renderer) renderFavoriteRow(gtx layout.Context, fav *FavoriteItem) (layout.Dimensions, bool, bool, image.Point) {
	// Check for left-click BEFORE layout
	leftClicked := fav.Clickable.Clicked(gtx)
	
	// Check for right-click and capture position
	rightClicked := false
	var clickPos image.Point
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &fav.RightClickTag, Kinds: pointer.Press})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok && e.Buttons.Contain(pointer.ButtonSecondary) {
			rightClicked = true
			clickPos = image.Pt(int(e.Position.X), int(e.Position.Y))
		}
	}
	
	dims := material.Clickable(gtx, &fav.Clickable, func(gtx layout.Context) layout.Dimensions {
		// Register right-click handler
		defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
		event.Op(gtx.Ops, &fav.RightClickTag)
		
		return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body1(r.Theme, fav.Name)
						lbl.Color, lbl.MaxLines = colDirBlue, 1
						return lbl.Layout(gtx)
					}),
				)
			})
	})
	
	return dims, leftClicked, rightClicked, clickPos
}

func (r *Renderer) renderDriveRow(gtx layout.Context, drive *DriveItem) (layout.Dimensions, bool) {
	// Check for click BEFORE layout
	clicked := drive.Clickable.Clicked(gtx)

	dims := material.Clickable(gtx, &drive.Clickable, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(r.Theme, drive.Name)
				lbl.Color, lbl.MaxLines = colDriveIcon, 1
				return lbl.Layout(gtx)
			})
	})

	return dims, clicked
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// truncatePathMiddle truncates a path to maxLen characters, showing start.../end
// The end portion (current directory) is prioritized to always be visible
func truncatePathMiddle(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}

	// Find the last path separator to get the current directory
	lastSep := strings.LastIndex(path, string(filepath.Separator))
	if lastSep <= 0 || lastSep >= len(path)-1 {
		// No separator or at start/end, just truncate end
		return path[:maxLen-3] + "..."
	}

	// Get the end part (current directory with separator)
	endPart := path[lastSep:]

	// Calculate space for start
	// Reserve: 3 for "...", endPart length
	startLen := maxLen - 3 - len(endPart)

	if startLen < 5 {
		// Not enough room for meaningful start, just show end
		if len(endPart) > maxLen-3 {
			return "..." + endPart[len(endPart)-(maxLen-3):]
		}
		return "..." + endPart
	}

	return path[:startLen] + "..." + endPart
}
