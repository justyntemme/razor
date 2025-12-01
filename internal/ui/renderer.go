package ui

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	// Image format decoders
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
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

	// Additional image format support
	"github.com/jdeng/goheif"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"github.com/justyntemme/razor/internal/config"
	"github.com/justyntemme/razor/internal/debug"
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
	ActionToggleSelect   // Toggle selection for multi-select mode
	ActionRangeSelect    // Select range from current selection to clicked item
	ActionClearSelection // Clear all selections
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
	// Tab actions
	ActionNewTab
	ActionCloseTab
	ActionSwitchTab
	ActionOpenInNewTab
	ActionNextTab
	ActionPrevTab
	// Additional actions
	ActionSelectAll
	ActionRefresh
	ActionFocusSearch
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
	Active             bool   // Whether dialog is visible
	SourcePath         string // Path of file being copied
	DestPath           string // Path where file would be placed
	SourceSize         int64  // Size of source file
	DestSize           int64  // Size of existing destination file
	SourceTime         time.Time
	DestTime           time.Time
	IsDir              bool // Whether the conflict is for a directory
	ApplyToAll         bool // Checkbox state for "Apply to All"
	RemainingConflicts int  // Number of remaining conflicts (including current)
}

// BrowserTab represents a single browser tab with its own navigation state
type BrowserTab struct {
	ID           string // Unique identifier
	Title        string // Display title (usually directory name)
	Path         string // Current path
	closeBtn     widget.Clickable
	tabBtn       widget.Clickable
}

// BreadcrumbSegment represents a clickable segment in the path breadcrumb
type BreadcrumbSegment struct {
	Name       string // Display name (directory name or "...")
	Path       string // Full path to this segment (empty for "..." placeholder)
	IsEllipsis bool   // True if this is the "..." placeholder
}

// parseBreadcrumbSegments converts a path into clickable segments
// It collapses the middle when there are too many segments
func parseBreadcrumbSegments(fullPath string, maxSegments int) []BreadcrumbSegment {
	if fullPath == "" {
		return nil
	}

	// Normalize path separators
	sep := string(filepath.Separator)

	// Handle root specially
	var segments []BreadcrumbSegment
	var parts []string

	// Split the path
	cleanPath := filepath.Clean(fullPath)

	// Handle Unix root or Windows drive
	if filepath.IsAbs(cleanPath) {
		vol := filepath.VolumeName(cleanPath)
		if vol != "" {
			// Windows: C:\
			segments = append(segments, BreadcrumbSegment{
				Name: vol + sep,
				Path: vol + sep,
			})
			cleanPath = cleanPath[len(vol):]
		} else if strings.HasPrefix(cleanPath, sep) {
			// Unix root
			segments = append(segments, BreadcrumbSegment{
				Name: sep,
				Path: sep,
			})
			cleanPath = cleanPath[1:]
		}
	}

	// Split remaining path into parts
	if cleanPath != "" && cleanPath != sep {
		parts = strings.Split(cleanPath, sep)
	}

	// Filter empty parts
	filtered := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			filtered = append(filtered, p)
		}
	}
	parts = filtered

	// Build full paths for each part
	currentPath := ""
	if len(segments) > 0 {
		currentPath = segments[0].Path
	}

	allParts := make([]BreadcrumbSegment, 0, len(parts))
	for _, part := range parts {
		currentPath = filepath.Join(currentPath, part)
		allParts = append(allParts, BreadcrumbSegment{
			Name: part,
			Path: currentPath,
		})
	}

	// If total segments fit, return all
	totalSegments := len(segments) + len(allParts)
	if totalSegments <= maxSegments {
		return append(segments, allParts...)
	}

	// Need to collapse middle - keep first segment(s), ellipsis, and last 2
	// segments = root (if any)
	// Keep: root + first dir + "..." + last 2 dirs

	keepStart := 1 // Keep at least first directory after root
	keepEnd := 2   // Keep last 2 directories

	if len(allParts) <= keepStart+keepEnd {
		// Not enough to collapse, just return all
		return append(segments, allParts...)
	}

	// Build collapsed version
	result := segments
	result = append(result, allParts[:keepStart]...)
	result = append(result, BreadcrumbSegment{
		Name:       "...",
		Path:       "", // No path for ellipsis
		IsEllipsis: true,
	})
	result = append(result, allParts[len(allParts)-keepEnd:]...)

	return result
}

// ResizeHandle is a reusable component for resizing containers
// It can be placed on any edge of a container to allow drag-to-resize
type ResizeHandle struct {
	dragging   bool    // Whether currently being dragged
	lastX      float32 // Last X position during drag
	lastY      float32 // Last Y position during drag
	Horizontal bool    // True for horizontal resize (left/right edge), false for vertical
	MinSize    int     // Minimum size constraint
	MaxSize    int     // Maximum size constraint (0 = no limit)
	Inverted   bool    // True if dragging left/up should increase size
}

// ResizeHandleStyle defines the visual appearance of the resize handle
type ResizeHandleStyle struct {
	Width      unit.Dp     // Width/height of the handle area
	Color      color.NRGBA // Color of the visible handle indicator
	HoverColor color.NRGBA // Color when hovered
}

// DefaultResizeHandleStyle returns the default style for resize handles
func DefaultResizeHandleStyle() ResizeHandleStyle {
	return ResizeHandleStyle{
		Width:      unit.Dp(6),
		Color:      color.NRGBA{R: 200, G: 200, B: 200, A: 255},
		HoverColor: color.NRGBA{R: 150, G: 150, B: 150, A: 255},
	}
}

// Layout renders the resize handle and processes drag events.
// Returns the new size value (or current value if not changed).
// The caller should use this returned value to update their container size.
func (h *ResizeHandle) Layout(gtx layout.Context, style ResizeHandleStyle, currentSize int) (layout.Dimensions, int) {
	newSize := currentSize

	// Determine handle dimensions based on orientation
	var handleWidth, handleHeight int
	if h.Horizontal {
		handleWidth = gtx.Dp(style.Width)
		handleHeight = gtx.Constraints.Max.Y
	} else {
		handleWidth = gtx.Constraints.Max.X
		handleHeight = gtx.Dp(style.Width)
	}

	// Create the handle area
	handleRect := image.Rect(0, 0, handleWidth, handleHeight)

	// Process pointer events for dragging
	// Use incremental deltas to avoid jitter from coordinate space changes
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: h,
			Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel,
		})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok {
			switch e.Kind {
			case pointer.Press:
				if e.Buttons.Contain(pointer.ButtonPrimary) {
					h.dragging = true
					h.lastX = e.Position.X
					h.lastY = e.Position.Y
				}
			case pointer.Drag:
				if h.dragging {
					// Calculate incremental delta since last event
					var delta int
					if h.Horizontal {
						delta = int(e.Position.X - h.lastX)
						h.lastX = e.Position.X
					} else {
						delta = int(e.Position.Y - h.lastY)
						h.lastY = e.Position.Y
					}
					if h.Inverted {
						delta = -delta
					}
					newSize = currentSize + delta

					// Apply constraints
					if h.MinSize > 0 && newSize < h.MinSize {
						newSize = h.MinSize
					}
					if h.MaxSize > 0 && newSize > h.MaxSize {
						newSize = h.MaxSize
					}
				}
			case pointer.Release, pointer.Cancel:
				h.dragging = false
			}
		}
	}

	// Register for pointer events
	defer clip.Rect(handleRect).Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, h)

	// Set cursor to resize cursor when hovering
	if h.Horizontal {
		pointer.CursorColResize.Add(gtx.Ops)
	} else {
		pointer.CursorRowResize.Add(gtx.Ops)
	}

	// Draw the handle indicator (a thin line in the center)
	handleColor := style.Color
	if h.dragging {
		handleColor = style.HoverColor
	}

	// Draw a subtle line indicator
	var indicatorRect image.Rectangle
	if h.Horizontal {
		// Vertical line in the center
		lineWidth := 2
		centerX := handleWidth / 2
		indicatorRect = image.Rect(centerX-lineWidth/2, handleHeight/4, centerX+lineWidth/2, handleHeight*3/4)
	} else {
		// Horizontal line in the center
		lineHeight := 2
		centerY := handleHeight / 2
		indicatorRect = image.Rect(handleWidth/4, centerY-lineHeight/2, handleWidth*3/4, centerY+lineHeight/2)
	}

	paint.FillShape(gtx.Ops, handleColor, clip.Rect(indicatorRect).Op())

	return layout.Dimensions{Size: image.Pt(handleWidth, handleHeight)}, newSize
}

type SortColumn int

const (
	SortByName SortColumn = iota
	SortByDate
	SortByType
	SortBySize
)

type Clipboard struct {
	Paths []string // Multiple paths for multi-select copy/cut
	Op    ClipOp
}

type UIEvent struct {
	Action             UIAction
	Path               string
	Paths              []string // Multiple paths for multi-select operations
	OldPath            string   // Original path for rename operations
	NewIndex           int
	OldIndex           int    // Previous index (for multi-select enter)
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
	TabIndex           int    // Tab index for tab operations
}

type UIEntry struct {
	Name, Path    string
	IsDir         bool
	Size          int64
	ModTime       time.Time
	Clickable     widget.Clickable
	Checkbox      widget.Bool // For multi-select mode
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
	CurrentPath     string
	Entries         []UIEntry
	SelectedIndex   int              // Primary selection (for keyboard nav)
	SelectedIndices map[int]bool     // Multi-select: set of selected indices
	CanBack         bool
	CanForward      bool
	Favorites       map[string]bool
	FavList         []FavoriteItem
	Clipboard       *Clipboard
	Progress        ProgressState
	DeleteTargets   []string // Paths to delete (supports multi-select)
	Drives          []DriveItem
	IsSearchResult  bool
	SearchQuery     string
	Conflict        ConflictState // File conflict dialog state
}

type Renderer struct {
	Theme               *material.Theme
	listState, favState layout.List
	driveState          layout.List
	sidebarScroll       layout.List      // Scrollable container for entire sidebar
	sidebarClick        widget.Clickable // For dismissing menus when clicking sidebar empty space
	sidebarTabs         *TabBar     // Tab bar for Favorites/Drives switching
	sidebarLayout       string      // "tabbed" | "stacked" | "favorites_only" | "drives_only"
	backBtn, fwdBtn     widget.Clickable
	homeBtn             widget.Clickable
	bgClick             widget.Clickable
	focused             bool
	pathEditor          widget.Editor
	pathClick           widget.Clickable
	isEditing           bool
	// Breadcrumb navigation
	breadcrumbSegments    []BreadcrumbSegment
	breadcrumbBtns        []widget.Clickable // One button per visible segment
	breadcrumbLastClicks  []time.Time        // For double-click detection on segments
	searchEditor        widget.Editor
	searchClearBtn      widget.Clickable
	searchBoxClick      widget.Clickable // For dismissing menus when clicking search area
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
	openInNewTabBtn     widget.Clickable
	fileMenuBtn         widget.Clickable
	fileMenuOpen        bool
	newWindowBtn        widget.Clickable
	newTabBtn           widget.Clickable
	newFileBtn          widget.Clickable
	newFolderBtn        widget.Clickable

	// Browser tabs
	browserTabs         []BrowserTab
	activeTabIndex      int
	tabsEnabled         bool
	settingsBtn         widget.Clickable
	settingsOpen        bool
	settingsCloseBtn    widget.Clickable
	searchEngine        widget.Enum
	mousePos            image.Point
	mouseTag            struct{}
	bgRightClickTag     struct{}    // Tag for detecting right-clicks on empty space
	fileListOffset      image.Point // Offset of file list area from window origin
	fileListSize        image.Point // Size of file list area (for hit-testing background clicks)

	// Multi-select mode
	multiSelectMode    bool          // When true, show checkboxes for multi-select
	lastClickModifiers key.Modifiers // Modifiers held during last pointer click

	// Double-click detection (no delay - selection happens immediately)
	lastClickIndex int       // Index of last clicked item (-1 if none)
	lastClickTime  time.Time // Time of the last click

	// Background click pending (to detect if click was on row or empty space)
	bgRightClickPending bool
	bgRightClickPos     image.Point
	bgLeftClickPending  bool // Left-click on background - dismiss menus

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
	previewContent    string         // Content loaded for preview (for text)
	previewError      string         // Error message if load failed
	previewIsJSON     bool           // Whether content is JSON (for formatting)
	previewIsImage    bool           // Whether previewing an image
	previewImage      paint.ImageOp  // Image data for image preview
	previewImageSize  image.Point    // Original image dimensions
	previewScroll       layout.List    // Scrollable list for preview content
	previewExtensions   []string       // Extensions that trigger text preview
	previewImageExts    []string       // Extensions that trigger image preview
	previewMaxSize      int64          // Max file size to preview
	previewWidthPct     int            // Width percentage for preview pane (initial)
	previewWidth        int            // Current preview pane width in pixels (after resize)
	previewResizeHandle ResizeHandle   // Resize handle for preview pane

	// Markdown preview state
	previewIsMarkdown     bool             // Whether previewing a markdown file
	previewMarkdownRender bool             // True = render markdown, False = show raw
	previewMarkdownBlocks []MarkdownBlock  // Parsed markdown blocks
	previewMdToggleBtn    widget.Clickable // Toggle button for raw/rendered

	// Recent files state
	recentFilesBtn    widget.Clickable // Button to show recent files
	openLocationBtn   widget.Clickable // Context menu item to open file location
	isRecentView      bool             // True when viewing recent files

	// Preview pane close button
	previewCloseBtn   widget.Clickable

	// Configurable hotkeys
	hotkeys *config.HotkeyMatcher
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
	// UI Polish colors
	colShadow          = color.NRGBA{R: 0, G: 0, B: 0, A: 40}        // Subtle shadow
	colBackdrop        = color.NRGBA{R: 0, G: 0, B: 0, A: 150}       // Modal backdrop (consistent)
	colCodeBlockBg     = color.NRGBA{R: 245, G: 245, B: 245, A: 255} // Code block background
	colCodeBlockBorder = color.NRGBA{R: 220, G: 220, B: 220, A: 255} // Code block border
	colBlockquoteBg    = color.NRGBA{R: 248, G: 248, B: 248, A: 255} // Blockquote background
	colBlockquoteLine  = color.NRGBA{R: 180, G: 180, B: 180, A: 255} // Blockquote left border
	colPrimaryBtn      = color.NRGBA{R: 66, G: 133, B: 244, A: 255}  // Primary button (blue)
	colPrimaryBtnText  = color.NRGBA{R: 255, G: 255, B: 255, A: 255} // Primary button text
	colDangerBtn       = color.NRGBA{R: 220, G: 53, B: 69, A: 255}   // Danger button (red)
	colDangerBtnText   = color.NRGBA{R: 255, G: 255, B: 255, A: 255} // Danger button text
)

func NewRenderer() *Renderer {
	r := &Renderer{Theme: material.NewTheme(), SortAscending: true, DefaultDepth: 2, renameIndex: -1, lastClickIndex: -1}
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
	r.previewWidth = 0 // Will be initialized from percentage on first layout

	// Configure preview resize handle (left edge of preview pane)
	r.previewResizeHandle = ResizeHandle{
		Horizontal: true,
		MinSize:    150, // Minimum 150px width
		MaxSize:    800, // Maximum 800px width
		Inverted:   true, // Dragging left increases size (since handle is on left edge)
	}

	// Initialize default hotkeys (can be overridden via SetHotkeys)
	r.hotkeys = config.NewHotkeyMatcher(config.DefaultHotkeys())

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

// SetHotkeys configures the keyboard shortcuts from config
func (r *Renderer) SetHotkeys(cfg config.HotkeysConfig) {
	r.hotkeys = config.NewHotkeyMatcher(cfg)
	debug.Log(debug.HOTKEY, "Hotkeys configured: NewTab=%s, CloseTab=%s, FocusSearch=%s, Back=%s",
		r.hotkeys.NewTab.String(), r.hotkeys.CloseTab.String(),
		r.hotkeys.FocusSearch.String(), r.hotkeys.Back.String())
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
func (r *Renderer) SetPreviewConfig(textExtensions, imageExtensions []string, maxSize int64, widthPct int, markdownRendered bool) {
	r.previewExtensions = textExtensions
	r.previewImageExts = imageExtensions
	r.previewMaxSize = maxSize
	r.previewWidthPct = widthPct
	r.previewMarkdownRender = markdownRendered
}

// EnableTabs enables the browser tab bar
func (r *Renderer) EnableTabs(enabled bool) {
	r.tabsEnabled = enabled
	if enabled && len(r.browserTabs) == 0 {
		// Initialize with one tab
		r.browserTabs = []BrowserTab{{
			ID:    "tab-0",
			Title: "Home",
			Path:  "",
		}}
		r.activeTabIndex = 0
	}
}

// AddTab adds a new browser tab and returns its index
func (r *Renderer) AddTab(id, title, path string) int {
	r.browserTabs = append(r.browserTabs, BrowserTab{
		ID:    id,
		Title: title,
		Path:  path,
	})
	return len(r.browserTabs) - 1
}

// CloseTab removes a tab by index, returns the new active index
func (r *Renderer) CloseTab(index int) int {
	if index < 0 || index >= len(r.browserTabs) || len(r.browserTabs) <= 1 {
		return r.activeTabIndex
	}
	r.browserTabs = append(r.browserTabs[:index], r.browserTabs[index+1:]...)
	if r.activeTabIndex >= len(r.browserTabs) {
		r.activeTabIndex = len(r.browserTabs) - 1
	} else if r.activeTabIndex > index {
		r.activeTabIndex--
	}
	return r.activeTabIndex
}

// SetActiveTab sets the active tab index
func (r *Renderer) SetActiveTab(index int) {
	if index >= 0 && index < len(r.browserTabs) {
		r.activeTabIndex = index
	}
}

// UpdateTabTitle updates the title of a tab
func (r *Renderer) UpdateTabTitle(index int, title string) {
	if index >= 0 && index < len(r.browserTabs) {
		r.browserTabs[index].Title = title
	}
}

// UpdateTabPath updates the path of a tab
func (r *Renderer) UpdateTabPath(index int, path string) {
	if index >= 0 && index < len(r.browserTabs) {
		r.browserTabs[index].Path = path
	}
}

// GetActiveTabIndex returns the current active tab index
func (r *Renderer) GetActiveTabIndex() int {
	return r.activeTabIndex
}

// GetTabCount returns the number of tabs
func (r *Renderer) GetTabCount() int {
	return len(r.browserTabs)
}

// TabsEnabled returns whether tabs are enabled
func (r *Renderer) TabsEnabled() bool {
	return r.tabsEnabled
}

// ShowPreview loads and displays the preview pane for the given file
func (r *Renderer) ShowPreview(path string) error {
	debug.Log(debug.UI, "ShowPreview called for: %s", path)

	// Check file extension
	ext := strings.ToLower(filepath.Ext(path))
	debug.Log(debug.UI, "ShowPreview: ext=%s, textExts=%v, imageExts=%v", ext, r.previewExtensions, r.previewImageExts)

	// Check if it's a text file
	isText := false
	for _, e := range r.previewExtensions {
		if strings.ToLower(e) == ext {
			isText = true
			break
		}
	}

	// Check if it's an image file
	isImage := false
	for _, e := range r.previewImageExts {
		if strings.ToLower(e) == ext {
			isImage = true
			break
		}
	}

	debug.Log(debug.UI, "ShowPreview: isText=%v, isImage=%v", isText, isImage)

	if !isText && !isImage {
		debug.Log(debug.UI, "ShowPreview: hiding preview (not text or image)")
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
		r.previewIsImage = false
		return err
	}
	if info.IsDir() {
		r.HidePreview()
		return nil
	}
	// For text files, check file size limit (images are scaled so no limit needed)
	if !isImage && r.previewMaxSize > 0 && info.Size() > r.previewMaxSize {
		r.previewError = fmt.Sprintf("File too large (%s)", formatSize(info.Size()))
		r.previewVisible = true
		r.previewPath = path
		r.previewContent = ""
		r.previewIsImage = false
		return nil
	}

	r.previewPath = path
	r.previewError = ""

	if isImage {
		// Load image (will be scaled to fit preview pane)
		return r.loadImagePreview(path)
	}

	// Load text content
	return r.loadTextPreview(path, ext)
}

// loadImagePreview loads an image file for preview
func (r *Renderer) loadImagePreview(path string) error {
	debug.Log(debug.UI, "loadImagePreview: loading %s", path)

	file, err := os.Open(path)
	if err != nil {
		debug.Log(debug.UI, "loadImagePreview: cannot open file: %v", err)
		r.previewError = fmt.Sprintf("Cannot open file: %v", err)
		r.previewVisible = true
		r.previewIsImage = false
		return err
	}
	defer file.Close()

	var img image.Image

	// Check if it's a HEIC/HEIF file (goheif doesn't register with image.Decode)
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".heic" || ext == ".heif" {
		debug.Log(debug.UI, "loadImagePreview: decoding HEIC/HEIF")
		img, err = goheif.Decode(file)
		if err != nil {
			debug.Log(debug.UI, "loadImagePreview: HEIC decode error: %v", err)
			r.previewError = fmt.Sprintf("Cannot decode HEIC: %v", err)
			r.previewVisible = true
			r.previewIsImage = false
			return err
		}
	} else {
		debug.Log(debug.UI, "loadImagePreview: decoding standard image format")
		img, _, err = image.Decode(file)
		if err != nil {
			debug.Log(debug.UI, "loadImagePreview: decode error: %v", err)
			r.previewError = fmt.Sprintf("Cannot decode image: %v", err)
			r.previewVisible = true
			r.previewIsImage = false
			return err
		}
	}

	debug.Log(debug.UI, "loadImagePreview: decoded successfully, size=%v", img.Bounds().Size())
	r.previewImage = paint.NewImageOp(img)
	r.previewImageSize = img.Bounds().Size()
	r.previewIsImage = true
	r.previewIsJSON = false
	r.previewContent = ""
	r.previewVisible = true
	debug.Log(debug.UI, "loadImagePreview: previewVisible=%v, previewIsImage=%v, previewImageSize=%v",
		r.previewVisible, r.previewIsImage, r.previewImageSize)
	return nil
}

// loadTextPreview loads a text file for preview
func (r *Renderer) loadTextPreview(path, ext string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		r.previewError = fmt.Sprintf("Cannot read file: %v", err)
		r.previewVisible = true
		r.previewIsImage = false
		r.previewContent = ""
		return err
	}

	r.previewIsImage = false
	r.previewIsJSON = ext == ".json"
	r.previewIsMarkdown = ext == ".md" || ext == ".markdown"

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

	// Parse markdown if this is a markdown file
	if r.previewIsMarkdown {
		r.previewMarkdownBlocks = ParseMarkdown(string(data))
		// previewMarkdownRender is already set from config via SetPreviewConfig
	} else {
		r.previewMarkdownBlocks = nil
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
	r.previewIsImage = false
	r.previewImage = paint.ImageOp{}
	r.previewImageSize = image.Point{}
	r.previewIsMarkdown = false
	r.previewMarkdownBlocks = nil
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

func (r *Renderer) StartRename(index int, path, name string, isDir bool) {
	r.renameIndex = index
	r.renamePath = path
	r.renameEditor.SetText(name)

	// For files, select only the name part (not the extension) so users can
	// easily type a new name while keeping the extension
	if !isDir {
		ext := filepath.Ext(name)
		nameWithoutExt := len(name) - len(ext)
		if nameWithoutExt > 0 {
			r.renameEditor.SetCaret(0, nameWithoutExt)
		} else {
			r.renameEditor.SetCaret(0, len(name))
		}
	} else {
		// For directories, select all text
		r.renameEditor.SetCaret(0, len(name))
	}
}

func (r *Renderer) CancelRename() {
	r.renameIndex = -1
	r.renamePath = ""
}

// ResetMultiSelect exits multi-select mode. Call this when navigating to a new directory.
func (r *Renderer) ResetMultiSelect() {
	r.multiSelectMode = false
	r.lastClickIndex = -1
	r.lastClickTime = time.Time{}
}

// IsMultiSelectMode returns whether multi-select mode is active
func (r *Renderer) IsMultiSelectMode() bool {
	return r.multiSelectMode
}

// SetMultiSelectMode enables or disables multi-select mode
func (r *Renderer) SetMultiSelectMode(enabled bool) {
	r.multiSelectMode = enabled
}

// FocusSearch sets a flag to focus the search editor on next layout
func (r *Renderer) FocusSearch() {
	r.searchEditorFocused = true
}

// collectSelectedPaths returns all selected file paths.
// If in multi-select mode, returns all paths from SelectedIndices.
// Otherwise, returns the single selected item or the menu target path.
func (r *Renderer) collectSelectedPaths(state *State) []string {
	// If we have multi-select items, use those
	if state.SelectedIndices != nil && len(state.SelectedIndices) > 0 {
		paths := make([]string, 0, len(state.SelectedIndices))
		for idx := range state.SelectedIndices {
			if idx >= 0 && idx < len(state.Entries) {
				paths = append(paths, state.Entries[idx].Path)
			}
		}
		return paths
	}
	// Otherwise, use the single selected item
	if state.SelectedIndex >= 0 && state.SelectedIndex < len(state.Entries) {
		return []string{state.Entries[state.SelectedIndex].Path}
	}
	// Fallback to menu path (right-click target)
	if r.menuPath != "" {
		return []string{r.menuPath}
	}
	return nil
}

// onLeftClick should be called at the start of any left-click handler.
// It dismisses context menus and performs other common click cleanup.
func (r *Renderer) onLeftClick() {
	r.menuVisible = false
	r.fileMenuOpen = false
	r.isEditing = false // Exit path edit mode when clicking elsewhere
}

// ============================================================================
// REUSABLE UI COMPONENTS
// ============================================================================

// ButtonStyle defines the visual style for a button
type ButtonStyle int

const (
	ButtonPrimary   ButtonStyle = iota // Blue background, white text
	ButtonDanger                       // Red background, white text
	ButtonSecondary                    // Gray background, black text
	ButtonDisabled                     // Light gray background, muted text
)

// styledButton renders a button with consistent styling based on the style type
func (r *Renderer) styledButton(gtx layout.Context, btn *widget.Clickable, label string, style ButtonStyle) layout.Dimensions {
	b := material.Button(r.Theme, btn, label)
	switch style {
	case ButtonPrimary:
		b.Background = colPrimaryBtn
		b.Color = colPrimaryBtnText
	case ButtonDanger:
		b.Background = colDangerBtn
		b.Color = colDangerBtnText
	case ButtonSecondary:
		b.Background = colLightGray
		b.Color = colBlack
	case ButtonDisabled:
		b.Background = colLightGray
		b.Color = colDisabled
	}
	return b.Layout(gtx)
}

// dialogButtonRow renders a standard button row for dialogs (Cancel on left, action on right)
func (r *Renderer) dialogButtonRow(gtx layout.Context, cancelBtn, actionBtn *widget.Clickable, cancelLabel, actionLabel string, actionStyle ButtonStyle) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceStart}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.styledButton(gtx, cancelBtn, cancelLabel, ButtonSecondary)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.styledButton(gtx, actionBtn, actionLabel, actionStyle)
		}),
	)
}

// modalBackdrop renders a modal dialog with backdrop, centered content using menuShell
// The dismissBtn is optional - if provided, clicking the backdrop will trigger it
func (r *Renderer) modalBackdrop(gtx layout.Context, width unit.Dp, dismissBtn *widget.Clickable, content layout.Widget) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, colBackdrop, clip.Rect{Max: gtx.Constraints.Max}.Op())
			if dismissBtn != nil {
				return material.Clickable(gtx, dismissBtn, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: gtx.Constraints.Max}
				})
			}
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return r.menuShell(gtx, width, content)
			})
		}),
	)
}

// modalContent renders standard modal content with title, body, and button row
func (r *Renderer) modalContent(gtx layout.Context, title string, titleColor color.NRGBA, body layout.Widget, buttons layout.Widget) layout.Dimensions {
	return layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				h6 := material.H6(r.Theme, title)
				h6.Color = titleColor
				return h6.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(body),
			layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
			layout.Rigid(buttons),
		)
	})
}

// invisibleClickable renders content inside an invisible clickable area.
// Unlike material.Clickable, this does not add any visual hover or press effects.
// Use this for areas that need to detect clicks to dismiss menus without visual feedback.
func invisibleClickable(gtx layout.Context, click *widget.Clickable, w layout.Widget) layout.Dimensions {
	return click.Layout(gtx, w)
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

func (r *Renderer) processGlobalInput(gtx layout.Context, state *State, keyTag event.Tag) UIEvent {
	// Keyboard input handling using configurable hotkeys
	// We register explicit filters for each hotkey to ensure they're captured
	// even when other widgets might consume generic key events

	if r.hotkeys == nil {
		return UIEvent{}
	}

	// Skip if modal dialogs are open
	if r.isEditing || r.settingsOpen || r.deleteConfirmOpen || r.createDialogOpen {
		return UIEvent{}
	}

	// Build filters for all configured hotkeys
	filters := r.buildHotkeyFilters(keyTag)

	// Process events matching our hotkey filters
	for {
		e, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		k, ok := e.(key.Event)
		if !ok || k.State != key.Press {
			continue
		}

		// Debug log the key press
		debug.Log(debug.HOTKEY, "Key pressed: name=%s mods=%v", k.Name, k.Modifiers)

		// Check configurable hotkeys
		if r.hotkeys != nil {
			// File operations
			if r.hotkeys.Copy.Matches(k) && state.SelectedIndex >= 0 {
				paths := r.collectSelectedPaths(state)
				return UIEvent{Action: ActionCopy, Paths: paths}
			}
			if r.hotkeys.Cut.Matches(k) && state.SelectedIndex >= 0 {
				paths := r.collectSelectedPaths(state)
				return UIEvent{Action: ActionCut, Paths: paths}
			}
			if r.hotkeys.Paste.Matches(k) && state.Clipboard != nil {
				return UIEvent{Action: ActionPaste}
			}
			if r.hotkeys.Delete.Matches(k) {
				if state.SelectedIndex >= 0 || (state.SelectedIndices != nil && len(state.SelectedIndices) > 0) {
					r.deleteConfirmOpen = true
					state.DeleteTargets = r.collectSelectedPaths(state)
					continue
				}
			}
			if r.hotkeys.Rename.Matches(k) && state.SelectedIndex >= 0 {
				entry := state.Entries[state.SelectedIndex]
				r.StartRename(state.SelectedIndex, entry.Path, entry.Name, entry.IsDir)
				continue
			}
			if r.hotkeys.NewFolder.Matches(k) {
				r.ShowCreateDialog(true)
				continue
			}
			if r.hotkeys.NewFile.Matches(k) {
				r.ShowCreateDialog(false)
				continue
			}
			if r.hotkeys.SelectAll.Matches(k) && len(state.Entries) > 0 {
				return UIEvent{Action: ActionSelectAll}
			}

			// Navigation
			if r.hotkeys.Back.Matches(k) && state.CanBack {
				return UIEvent{Action: ActionBack}
			}
			if r.hotkeys.Forward.Matches(k) && state.CanForward {
				return UIEvent{Action: ActionForward}
			}
			if r.hotkeys.Up.Matches(k) {
				return UIEvent{Action: ActionBack}
			}
			if r.hotkeys.Home.Matches(k) {
				return UIEvent{Action: ActionHome}
			}
			if r.hotkeys.Refresh.Matches(k) {
				return UIEvent{Action: ActionRefresh}
			}

			// UI
			if r.hotkeys.FocusSearch.Matches(k) {
				return UIEvent{Action: ActionFocusSearch}
			}
			if r.hotkeys.TogglePreview.Matches(k) {
				if r.previewVisible {
					r.HidePreview()
				} else if state.SelectedIndex >= 0 && state.SelectedIndex < len(state.Entries) {
					r.ShowPreview(state.Entries[state.SelectedIndex].Path)
				}
				continue
			}
			if r.hotkeys.ToggleHidden.Matches(k) {
				return UIEvent{Action: ActionToggleDotfiles}
			}
			if r.hotkeys.Escape.Matches(k) {
				if r.previewVisible {
					r.HidePreview()
				}
				if r.menuVisible {
					r.menuVisible = false
				}
				continue
			}

			// Tabs
			if r.hotkeys.NewTab.Matches(k) {
				return UIEvent{Action: ActionNewTab}
			}
			if r.hotkeys.CloseTab.Matches(k) {
				return UIEvent{Action: ActionCloseTab}
			}
			if r.hotkeys.NextTab.Matches(k) {
				return UIEvent{Action: ActionNextTab}
			}
			if r.hotkeys.PrevTab.Matches(k) {
				return UIEvent{Action: ActionPrevTab}
			}
		}

		// Arrow keys and Enter are not configurable (fundamental navigation)
		switch k.Name {
		case key.NameUpArrow:
			if idx := state.SelectedIndex - 1; idx >= 0 {
				r.listState.ScrollTo(idx)
				return UIEvent{Action: ActionSelect, NewIndex: idx}
			} else if state.SelectedIndex == -1 && len(state.Entries) > 0 {
				idx := len(state.Entries) - 1
				r.listState.ScrollTo(idx)
				return UIEvent{Action: ActionSelect, NewIndex: idx}
			}
		case key.NameDownArrow:
			if idx := state.SelectedIndex + 1; idx < len(state.Entries) {
				r.listState.ScrollTo(idx)
				return UIEvent{Action: ActionSelect, NewIndex: idx}
			} else if state.SelectedIndex == -1 && len(state.Entries) > 0 {
				return UIEvent{Action: ActionSelect, NewIndex: 0}
			}
		case key.NameLeftArrow:
			// Left arrow: go back (up one directory level)
			if state.CanBack {
				return UIEvent{Action: ActionBack}
			}
		case key.NameRightArrow:
			// Right arrow: enter selection (navigate into folder or open file)
			if idx := state.SelectedIndex; idx >= 0 && idx < len(state.Entries) {
				item := state.Entries[idx]
				if item.IsDir {
					return UIEvent{Action: ActionNavigate, Path: item.Path}
				}
				return UIEvent{Action: ActionOpen, Path: item.Path}
			}
		case key.NameReturn, key.NameEnter:
			if idx := state.SelectedIndex; idx >= 0 && idx < len(state.Entries) {
				item := state.Entries[idx]
				if item.IsDir {
					return UIEvent{Action: ActionNavigate, Path: item.Path}
				}
				return UIEvent{Action: ActionOpen, Path: item.Path}
			}
		}
	}
	return UIEvent{}
}

// buildHotkeyFilters creates key.Filter slice for all configured hotkeys
func (r *Renderer) buildHotkeyFilters(keyTag event.Tag) []event.Filter {
	if r.hotkeys == nil {
		return nil
	}

	// Collect all non-empty hotkey filters
	hotkeys := []config.Hotkey{
		r.hotkeys.Copy, r.hotkeys.Cut, r.hotkeys.Paste, r.hotkeys.Delete,
		r.hotkeys.Rename, r.hotkeys.NewFile, r.hotkeys.NewFolder, r.hotkeys.SelectAll,
		r.hotkeys.Back, r.hotkeys.Forward, r.hotkeys.Up, r.hotkeys.Home, r.hotkeys.Refresh,
		r.hotkeys.FocusSearch, r.hotkeys.TogglePreview, r.hotkeys.ToggleHidden, r.hotkeys.Escape,
		r.hotkeys.NewTab, r.hotkeys.CloseTab, r.hotkeys.NextTab, r.hotkeys.PrevTab,
	}

	// Also include arrow keys and Enter for navigation
	arrowFilters := []key.Filter{
		{Focus: keyTag, Name: key.NameUpArrow},
		{Focus: keyTag, Name: key.NameDownArrow},
		{Focus: keyTag, Name: key.NameLeftArrow},
		{Focus: keyTag, Name: key.NameRightArrow},
		{Focus: keyTag, Name: key.NameReturn},
		{Focus: keyTag, Name: key.NameEnter},
	}

	filters := make([]event.Filter, 0, len(hotkeys)+len(arrowFilters))

	for _, hk := range hotkeys {
		if !hk.IsEmpty() {
			filters = append(filters, hk.Filter(keyTag))
		}
	}

	for _, f := range arrowFilters {
		filters = append(filters, f)
	}

	return filters
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
			r.onLeftClick()
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
func (r *Renderer) renderRow(gtx layout.Context, item *UIEntry, index int, selected bool, isRenaming bool, isChecked bool, showCheckbox bool) (layout.Dimensions, bool, bool, bool, image.Point, *UIEvent, bool) {
	// Check for left-click BEFORE layout (Gio pattern)
	leftClicked := item.Clickable.Clicked(gtx)

	// Check for shift modifier during click by tracking last click modifiers
	shiftHeld := r.lastClickModifiers.Contain(key.ModShift)

	// Check for checkbox toggle (only if checkboxes are visible)
	checkboxToggled := false
	if showCheckbox && item.Checkbox.Update(gtx) {
		checkboxToggled = true
	}

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

	// Determine if row should show as selected
	// In multi-select mode, only use isChecked (from SelectedIndices)
	// In single-select mode, use the primary selection
	showSelected := isChecked

	// Layout the clickable row (but not if renaming - we handle clicks differently)
	var dims layout.Dimensions
	cornerRadius := gtx.Dp(4)
	if isRenaming {
		// For renaming, render content first to get proper size, then draw background
		// Use a macro to record the content, measure it, then draw background + content
		macro := op.Record(gtx.Ops)
		contentDims := r.renderRowContent(gtx, item, true, false, false)
		call := macro.Stop()

		// Draw selection background with rounded corners
		rr := clip.RRect{
			Rect: image.Rect(0, 0, contentDims.Size.X, contentDims.Size.Y),
			NE:   cornerRadius, NW: cornerRadius, SE: cornerRadius, SW: cornerRadius,
		}
		paint.FillShape(gtx.Ops, colSelected, rr.Op(gtx.Ops))
		// Replay the content on top
		call.Add(gtx.Ops)

		dims = contentDims
	} else {
		dims = material.Clickable(gtx, &item.Clickable, func(gtx layout.Context) layout.Dimensions {
			// Register right-click handler on this row's area
			defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
			event.Op(gtx.Ops, &item.RightClickTag)

			// Draw selection background with rounded corners
			if showSelected {
				rr := clip.RRect{
					Rect: image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Constraints.Max.Y),
					NE:   cornerRadius, NW: cornerRadius, SE: cornerRadius, SW: cornerRadius,
				}
				paint.FillShape(gtx.Ops, colSelected, rr.Op(gtx.Ops))
			}

			return r.renderRowContent(gtx, item, false, showCheckbox, isChecked)
		})
	}

	return dims, leftClicked, rightClicked, shiftHeld, clickPos, renameEvent, checkboxToggled
}

// renderRowContent renders the content of a row (shared between normal and rename mode)
func (r *Renderer) renderRowContent(gtx layout.Context, item *UIEntry, isRenaming bool, showCheckbox bool, isChecked bool) layout.Dimensions {
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

	// Sync checkbox state with selection state
	item.Checkbox.Value = isChecked

	return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			var children []layout.FlexChild

			// Add checkbox if in multi-select mode
			if showCheckbox {
				children = append(children,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						cb := material.CheckBox(r.Theme, &item.Checkbox, "")
						cb.Size = unit.Dp(14) // Smaller checkbox
						if isChecked {
							cb.Color = colAccent
							cb.IconColor = colAccent
						} else {
							cb.Color = colGray
							cb.IconColor = colGray
						}
						return cb.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					// Divider between checkbox and filename
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						size := image.Pt(gtx.Dp(unit.Dp(1)), gtx.Dp(unit.Dp(16)))
						paint.FillShape(gtx.Ops, color.NRGBA{R: 200, G: 200, B: 200, A: 255},
							clip.Rect{Max: size}.Op())
						return layout.Dimensions{Size: size}
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				)
			}

			// Name column (adjusted flex weight when checkbox is present)
			nameWeight := float32(0.5)
			if showCheckbox {
				nameWeight = 0.45
			}
			children = append(children,
				layout.Flexed(nameWeight, func(gtx layout.Context) layout.Dimensions {
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

			return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween, Alignment: layout.Middle}.Layout(gtx, children...)
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
