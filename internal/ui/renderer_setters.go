package ui

import (
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
}
