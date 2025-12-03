package ui

import (
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

type UIAction int

// FileDragMIME is the MIME type for internal drag-and-drop file transfers
const FileDragMIME = "application/x-razor-file-paths"

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
	ActionMove // Drag-and-drop move: Paths=sources, Path=destination
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
	ActionChangeTerminal
	// Search history
	ActionRequestSearchHistory
	ActionSelectSearchHistory
	// Recent files
	ActionShowRecentFiles
	ActionOpenFileLocation
	// Tab actions
	ActionNewTab      // New tab in current directory
	ActionNewTabHome  // New tab in home directory
	ActionCloseTab
	ActionSwitchTab
	ActionOpenInNewTab
	ActionNextTab
	ActionPrevTab
	ActionSwitchToTab // Switch to specific tab by index (uses TabIndex)
	// Additional actions
	ActionSelectAll
	ActionRefresh
	ActionFocusSearch
	ActionJumpToLetter // Jump to file starting with letter (uses NewIndex for target)
	// Trash actions
	ActionShowTrash       // Show trash view
	ActionEmptyTrash      // Empty all trash
	ActionPermanentDelete // Delete permanently (Shift+Delete)
	// Tree view actions
	ActionExpandDir   // Expand a directory inline (uses Path)
	ActionCollapseDir // Collapse an expanded directory (uses Path)
	// Terminal action
	ActionOpenTerminal // Open terminal in directory (uses Path)
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

// TerminalInfo contains information about a terminal application
type TerminalInfo struct {
	ID      string // Internal ID (e.g., "terminal", "iterm2", "wezterm")
	Name    string // Display name (e.g., "Terminal.app", "iTerm2")
	Default bool   // Whether this is the platform default
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
	TerminalApp        string // Selected terminal application ID
}

type UIEntry struct {
	Name, Path string
	IsDir      bool
	Size       int64
	ModTime    time.Time
	Touch      Touchable   // Combined click, right-click, and drag handling
	DropTag    struct{}    // Unique tag for drop target registration (address is unique per entry)
	Checkbox   widget.Bool // For multi-select mode
	LastClick  time.Time
	// Tree view fields
	Depth      int              // Indentation level (0 = root level)
	IsExpanded bool             // Whether this directory is expanded inline
	ExpandBtn  widget.Clickable // Clickable for chevron expand/collapse button
	ParentPath string           // Path of parent directory (empty for root level)
}

// FavoriteItemType indicates the type of favorite entry
type FavoriteItemType int

const (
	FavoriteTypeNormal FavoriteItemType = iota // Regular user favorite
	FavoriteTypeTrash                          // System trash entry
)

type FavoriteItem struct {
	Name, Path    string
	Type          FavoriteItemType // Type of favorite (normal, trash, etc.)
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
