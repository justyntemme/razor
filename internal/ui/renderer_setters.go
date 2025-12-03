package ui

import (
	"path/filepath"
	"strings"

	"github.com/justyntemme/razor/internal/config"
	"github.com/justyntemme/razor/internal/debug"
)

// Configuration setters - methods to update renderer state from orchestrator

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

// SetTerminals sets the available terminal applications
func (r *Renderer) SetTerminals(terminals []TerminalInfo) {
	r.Terminals = terminals
}

// SetSelectedTerminal sets the currently selected terminal
func (r *Renderer) SetSelectedTerminal(terminalID string) {
	r.SelectedTerminal = terminalID
	r.terminalEnum.Value = terminalID
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
	r.previewOrgmodeRender = markdownRendered // Use same default for orgmode
}

// SetTrashView sets whether the trash view is active
func (r *Renderer) SetTrashView(active bool) {
	r.isTrashView = active
	if active {
		r.isRecentView = false // Can't be in both views
	}
}

// IsTrashView returns whether the trash view is currently active
func (r *Renderer) IsTrashView() bool {
	return r.isTrashView
}

// trackVisibleImage checks if a file path is an image and adds it to the visible list.
// This is called during list layout for each visible item.
func (r *Renderer) trackVisibleImage(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	for _, imgExt := range r.previewImageExts {
		if ext == strings.ToLower(imgExt) {
			r.visibleImagePaths = append(r.visibleImagePaths, path)
			return
		}
	}
}

// RequestVisibleThumbnails queues visible image files for thumbnail loading.
// This should be called after the file list has been rendered.
func (r *Renderer) RequestVisibleThumbnails() {
	for _, path := range r.visibleImagePaths {
		r.thumbnailCache.RequestLoad(path)
	}
}

// ClearThumbnailCache clears the thumbnail cache.
// Call this when navigating to a new directory.
func (r *Renderer) ClearThumbnailCache() {
	r.thumbnailCache.Clear()
	r.thumbnailLoadDelay = 3 // Wait 3 frames before loading thumbnails
}

// OnDirectoryLoaded should be called when a directory listing completes.
// It resets the thumbnail load delay counter.
func (r *Renderer) OnDirectoryLoaded() {
	r.thumbnailLoadDelay = 3 // Wait 3 frames for UI to settle
}

// SetViewMode sets the file list view mode
func (r *Renderer) SetViewMode(mode ViewMode) {
	r.viewMode = mode
}

// GetViewMode returns the current view mode
func (r *Renderer) GetViewMode() ViewMode {
	return r.viewMode
}

// ToggleViewMode switches between list and grid view
func (r *Renderer) ToggleViewMode() ViewMode {
	if r.viewMode == ViewModeList {
		r.viewMode = ViewModeGrid
	} else {
		r.viewMode = ViewModeList
	}
	return r.viewMode
}
