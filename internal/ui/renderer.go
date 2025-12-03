package ui

import (
	"image"
	"image/color"
	"path/filepath"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/justyntemme/razor/internal/config"
)

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
	openTerminalBtn     widget.Clickable
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
	settingsListState   widget.List
	hotkeysBtn          widget.Clickable
	hotkeysOpen         bool
	hotkeysCloseBtn     widget.Clickable
	hotkeysListState    widget.List
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

	// Quick-jump letter navigation (press a-z to jump to files starting with that letter)
	lastJumpKey   rune      // Last letter pressed for quick-jump
	lastJumpTime  time.Time // Time of last jump key press
	lastJumpIndex int       // Index we jumped to (for cycling)

	// Tree view (inline directory expansion)
	expandedDirs map[string]bool // Tracks which directories are expanded by path
	treeIndent   int             // Indentation width in dp per level (default 20)

	// Background click pending (to detect if click was on row or empty space)
	bgRightClickPending bool
	bgRightClickPos     image.Point
	bgLeftClickPending  bool // Left-click on background - dismiss menus

	// Drag and drop state
	dragSourcePath      string               // Path of item being dragged (primary)
	dragSourcePaths     []string             // All paths being dragged (for multi-select)
	dropTargetPath      string               // Path of directory currently being hovered as drop target
	dragHoverCandidates []dragHoverCandidate // Candidates for drop target hover (rebuilt each frame)
	listAreaOffset      int                  // Y offset of the list area within the file list (after header)
	dragCurrentX        int                  // Current X position of drag cursor in window coordinates
	dragCurrentY        int                  // Current Y position of drag cursor in list coordinates
	dragWindowY         int                  // Current Y position of drag cursor in window coordinates
	sidebarDropTarget   string               // Path of sidebar item being hovered as drop target

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

	// Column sorting and resizing
	headerBtns          [4]widget.Clickable
	SortColumn          SortColumn
	SortAscending       bool
	columnWidths        [4]int  // Column widths in pixels
	columnWidthsInited  bool
	colDragActive       int     // Which divider is being dragged (-1 = none)
	colDragID           pointer.ID
	colDragTag          [3]bool // Tags for pointer event registration

	// Settings
	ShowDotfiles      bool
	showDotfilesCheck widget.Bool
	
	// Search engine settings
	SearchEngines     []SearchEngineInfo // Available engines (detected on startup)
	SelectedEngine    string             // Currently selected engine name

	// Terminal settings
	Terminals         []TerminalInfo     // Available terminals (detected on startup)
	SelectedTerminal  string             // Currently selected terminal ID
	terminalEnum      widget.Enum        // Radio button group for terminal selection

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
	searchFocusRequested  bool                // One-shot flag to request focus on search editor

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

	// Trash state
	trashBtn            widget.Clickable // Button to show trash
	isTrashView         bool             // True when viewing trash
	emptyTrashBtn       widget.Clickable // Button to empty trash (in context menu)
	permanentDeleteBtn  widget.Clickable // Button to permanently delete (bypass trash)

	// Preview pane close button
	previewCloseBtn   widget.Clickable

	// Configurable hotkeys
	hotkeys *config.HotkeyMatcher
}

func NewRenderer() *Renderer {
	r := &Renderer{Theme: material.NewTheme(), SortAscending: true, DefaultDepth: 2, renameIndex: -1, lastClickIndex: -1}
	r.listState.Axis = layout.Vertical
	r.expandedDirs = make(map[string]bool)
	r.treeIndent = 20 // 20dp per indentation level
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

// IsExpanded returns whether a directory path is expanded in tree view
func (r *Renderer) IsExpanded(path string) bool {
	return r.expandedDirs[path]
}

// SetExpanded sets the expansion state of a directory path
func (r *Renderer) SetExpanded(path string, expanded bool) {
	if expanded {
		r.expandedDirs[path] = true
	} else {
		delete(r.expandedDirs, path)
	}
}

// GetExpandedDirs returns a copy of all expanded directory paths
func (r *Renderer) GetExpandedDirs() []string {
	paths := make([]string, 0, len(r.expandedDirs))
	for path := range r.expandedDirs {
		paths = append(paths, path)
	}
	return paths
}

// ClearExpanded clears all expanded directories (e.g., when navigating to new folder)
func (r *Renderer) ClearExpanded() {
	r.expandedDirs = make(map[string]bool)
}

// FocusSearch sets a flag to focus the search editor on next layout
func (r *Renderer) FocusSearch() {
	r.searchFocusRequested = true
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

// layoutHorizontalSeparator renders a horizontal line separator that fills the available width
func (r *Renderer) layoutHorizontalSeparator(gtx layout.Context, c color.NRGBA) layout.Dimensions {
	height := gtx.Dp(unit.Dp(1))
	paint.FillShape(gtx.Ops, c, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, height)}.Op())
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
}

// layoutHorizontalSeparatorMin renders a horizontal line separator using min width (for menus)
func (r *Renderer) layoutHorizontalSeparatorMin(gtx layout.Context, c color.NRGBA) layout.Dimensions {
	height := gtx.Dp(unit.Dp(1))
	paint.FillShape(gtx.Ops, c, clip.Rect{Max: image.Pt(gtx.Constraints.Min.X, height)}.Op())
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, height)}
}

// layoutInsetSeparator renders a horizontal separator with vertical insets
func (r *Renderer) layoutInsetSeparator(gtx layout.Context, c color.NRGBA, topInset, bottomInset unit.Dp) layout.Dimensions {
	return layout.Inset{Top: topInset, Bottom: bottomInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return r.layoutHorizontalSeparator(gtx, c)
	})
}

// layoutMenuSeparator renders a standard menu separator with 4dp vertical insets
func (r *Renderer) layoutMenuSeparator(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return r.layoutHorizontalSeparatorMin(gtx, colLightGray)
	})
}

// drawXIcon draws an X close icon at the current position
func (r *Renderer) drawXIcon(gtx layout.Context, size int, iconColor color.NRGBA) layout.Dimensions {
	centerX := float32(size) / 2
	centerY := float32(size) / 2
	armLen := float32(size) / 4

	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(f32.Pt(centerX-armLen, centerY-armLen))
	p.LineTo(f32.Pt(centerX+armLen, centerY+armLen))
	p.MoveTo(f32.Pt(centerX+armLen, centerY-armLen))
	p.LineTo(f32.Pt(centerX-armLen, centerY+armLen))
	paint.FillShape(gtx.Ops, iconColor, clip.Stroke{Path: p.End(), Width: 1.5}.Op())

	return layout.Dimensions{Size: image.Pt(size, size)}
}

// modalBackdrop renders a modal dialog with backdrop, centered content using menuShell
// The dismissBtn is optional - if provided, clicking the backdrop will trigger it
func (r *Renderer) modalBackdrop(gtx layout.Context, width unit.Dp, dismissBtn *widget.Clickable, content layout.Widget) layout.Dimensions {
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		// Backdrop fills the entire screen
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, colBackdrop, clip.Rect{Max: gtx.Constraints.Max}.Op())
			if dismissBtn != nil {
				return material.Clickable(gtx, dismissBtn, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: gtx.Constraints.Max}
				})
			}
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		// Modal content centered on screen
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return r.menuShell(gtx, width, content)
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

// modalContentWithClose renders modal content with title, close button (X with divider), body, and optional button row
func (r *Renderer) modalContentWithClose(gtx layout.Context, title string, titleColor color.NRGBA, closeBtn *widget.Clickable, body layout.Widget, buttons layout.Widget) layout.Dimensions {
	return layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			// Header row with title and close button
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					// Title (takes remaining space)
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						h6 := material.H6(r.Theme, title)
						h6.Color = titleColor
						return h6.Layout(gtx)
					}),
					// Divider line
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						height := gtx.Dp(20)
						return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							paint.FillShape(gtx.Ops, colLightGray, clip.Rect{Max: image.Pt(1, height)}.Op())
							return layout.Dimensions{Size: image.Pt(1, height)}
						})
					}),
					// Close button (X)
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						closeSize := gtx.Dp(24)
						return material.Clickable(gtx, closeBtn, func(gtx layout.Context) layout.Dimensions {
							// Draw X
							centerX := float32(closeSize) / 2
							centerY := float32(closeSize) / 2
							armLen := float32(closeSize) / 4

							xColor := colGray
							var p clip.Path
							p.Begin(gtx.Ops)
							p.MoveTo(f32.Pt(centerX-armLen, centerY-armLen))
							p.LineTo(f32.Pt(centerX+armLen, centerY+armLen))
							p.MoveTo(f32.Pt(centerX+armLen, centerY-armLen))
							p.LineTo(f32.Pt(centerX-armLen, centerY+armLen))
							paint.FillShape(gtx.Ops, xColor, clip.Stroke{Path: p.End(), Width: 1.5}.Op())

							return layout.Dimensions{Size: image.Pt(closeSize, closeSize)}
						})
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(body),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if buttons != nil {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
						layout.Rigid(buttons),
					)
				}
				return layout.Dimensions{}
			}),
		)
	})
}

