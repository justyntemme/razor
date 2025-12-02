package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// File permission constants
const (
	dirPermission  = 0o755 // Standard directory permissions
	filePermission = 0o644 // Standard file permissions
)

// Config holds all user-configurable settings loaded from config.json
type Config struct {
	UI        UIConfig        `json:"ui"`
	Search    SearchConfig    `json:"search"`
	Behavior  BehaviorConfig  `json:"behavior"`
	Tabs      TabsConfig      `json:"tabs"`
	Panels    PanelsConfig    `json:"panels"`
	Preview   PreviewConfig   `json:"preview"`
	Hotkeys   HotkeysConfig   `json:"hotkeys"`
	Favorites []FavoriteEntry `json:"favorites"`
}

// HotkeysConfig holds all configurable keyboard shortcuts
// Keys are action names, values are key combinations like "Ctrl+C", "Ctrl+Shift+N", "F2"
// Values can be a single string or an array of strings for multiple bindings
// Supported modifiers: Ctrl, Shift, Alt, Cmd (or Super)
// Supported keys: A-Z, 0-9, F1-F12, Delete, Backspace, Escape, Enter, Tab, Space
//                 Up, Down, Left, Right, Home, End, PageUp, PageDown
type HotkeysConfig struct {
	// File operations
	Copy          string `json:"copy"`
	Cut           string `json:"cut"`
	Paste         string `json:"paste"`
	Delete        string `json:"delete"`
	Rename        string `json:"rename"`
	NewFile       string `json:"newFile"`
	NewFolder     string `json:"newFolder"`
	SelectAll     string `json:"selectAll"`

	// Navigation
	Back          string `json:"back"`
	Forward       string `json:"forward"`
	Up            string `json:"up"`
	Home          string `json:"home"`
	Refresh       string `json:"refresh"`

	// UI
	FocusSearch   string `json:"focusSearch"`
	TogglePreview string `json:"togglePreview"`
	ToggleHidden  string `json:"toggleHidden"`
	Escape        string `json:"escape"`

	// Tabs
	NewTab        string `json:"newTab"`
	CloseTab      string `json:"closeTab"`
	NextTab       string `json:"nextTab"`
	PrevTab       string `json:"prevTab"`

	// Tab switching (1-6, user can add more)
	Tab1          string `json:"tab1"`
	Tab2          string `json:"tab2"`
	Tab3          string `json:"tab3"`
	Tab4          string `json:"tab4"`
	Tab5          string `json:"tab5"`
	Tab6          string `json:"tab6"`
}

// UIConfig holds UI-related settings
type UIConfig struct {
	Theme     string        `json:"theme"` // "light" or "dark"
	Sidebar   SidebarConfig `json:"sidebar"`
	Toolbar   ToolbarConfig `json:"toolbar"`
	FileList  FileListConfig `json:"fileList"`
	StatusBar StatusBarConfig `json:"statusBar"`
}

// SidebarConfig holds sidebar layout settings
type SidebarConfig struct {
	Layout        string `json:"layout"`        // "tabbed" | "stacked" | "favorites_only" | "drives_only"
	TabStyle      string `json:"tabStyle"`      // "manila" | "underline" | "pill" - style for tabbed layout
	Width         int    `json:"width"`         // Sidebar width in dp
	Position      string `json:"position"`      // "left" | "right"
	ShowFavorites bool   `json:"showFavorites"`
	ShowDrives    bool   `json:"showDrives"`
	Collapsible   bool   `json:"collapsible"`
}

// ToolbarConfig holds toolbar settings
type ToolbarConfig struct {
	Visible    bool   `json:"visible"`
	Position   string `json:"position"`   // "top" | "bottom"
	ShowLabels bool   `json:"showLabels"`
}

// FileListConfig holds file list display settings
type FileListConfig struct {
	ShowDotfiles  bool   `json:"showDotfiles"`
	DefaultSort   string `json:"defaultSort"` // "name" | "date" | "type" | "size"
	SortAscending bool   `json:"sortAscending"`
	RowHeight     string `json:"rowHeight"` // "compact" | "normal" | "comfortable"
	ShowIcons     bool   `json:"showIcons"`
}

// StatusBarConfig holds status bar settings
type StatusBarConfig struct {
	Visible           bool `json:"visible"`
	ShowFileCount     bool `json:"showFileCount"`
	ShowSelectionInfo bool `json:"showSelectionInfo"`
}

// SearchConfig holds search-related settings
type SearchConfig struct {
	Engine              string `json:"engine"`              // "builtin" | "ripgrep" | "ugrep"
	DefaultDepth        int    `json:"defaultDepth"`
	RememberLastQuery   bool   `json:"rememberLastQuery"`
}

// BehaviorConfig holds behavior settings
type BehaviorConfig struct {
	ConfirmDelete      bool `json:"confirmDelete"`
	DoubleClickToOpen  bool `json:"doubleClickToOpen"`
	RestoreLastPath    bool `json:"restoreLastPath"`
	SingleClickToSelect bool `json:"singleClickToSelect"`
}

// TabsConfig holds tab-related settings
type TabsConfig struct {
	Enabled            bool   `json:"enabled"`
	NewTabLocation     string `json:"newTabLocation"`     // "current" | "home" | "recent" | path string
	CustomTabPath      string `json:"customTabPath"`      // Custom path when NewTabLocation is a path
	LastTabBehavior    string `json:"lastTabBehavior"`    // "close_app" | "keep_empty" | "reopen_home"
	RestoreTabsOnStart bool   `json:"restoreTabsOnStart"`
}

// PanelsConfig holds panel settings
type PanelsConfig struct {
	Preview  PanelConfig `json:"preview"`
	Terminal PanelConfig `json:"terminal"`
}

// PanelConfig holds individual panel settings
type PanelConfig struct {
	Enabled  bool   `json:"enabled"`
	Position string `json:"position"` // "right" | "bottom" | "left"
	Width    int    `json:"width"`    // For left/right panels
	Height   int    `json:"height"`   // For top/bottom panels
}

// PreviewConfig holds preview pane settings
type PreviewConfig struct {
	Enabled          bool     `json:"enabled"`
	Position         string   `json:"position"`         // "right" | "bottom"
	WidthPercent     int      `json:"widthPercent"`     // Percentage of screen width (e.g., 33 for 1/3)
	TextExtensions   []string `json:"textExtensions"`   // Extensions to show text preview for
	ImageExtensions  []string `json:"imageExtensions"`  // Extensions to show image preview for
	MaxFileSize      int64    `json:"maxFileSize"`      // Max file size in bytes to preview (0 = no limit)
	MarkdownRendered bool     `json:"markdownRendered"` // Default to rendered markdown (true) or raw (false)
}

// FavoriteEntry represents a single favorite or a group of favorites
type FavoriteEntry struct {
	Name     string          `json:"name"`
	Path     string          `json:"path,omitempty"`     // Empty for groups
	Icon     string          `json:"icon,omitempty"`
	Type     string          `json:"type,omitempty"`     // "group" for folder groups
	Expanded bool            `json:"expanded,omitempty"` // For groups
	Items    []FavoriteEntry `json:"items,omitempty"`    // For groups
}

// Manager handles loading, saving, and accessing configuration
type Manager struct {
	mu       sync.RWMutex
	config   *Config
	path     string
	parseErr error // Stores parsing error if config failed to load
}

// NewManager creates a new configuration manager
func NewManager() *Manager {
	return &Manager{
		config: DefaultConfig(),
	}
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		UI: UIConfig{
			Theme: "light",
			Sidebar: SidebarConfig{
				Layout:        "stacked",
				TabStyle:      "manila",  // "manila" | "underline" | "pill"
				Width:         200,
				Position:      "left",
				ShowFavorites: true,
				ShowDrives:    true,
				Collapsible:   true,
			},
			Toolbar: ToolbarConfig{
				Visible:    true,
				Position:   "top",
				ShowLabels: false,
			},
			FileList: FileListConfig{
				ShowDotfiles:  false,
				DefaultSort:   "name",
				SortAscending: true,
				RowHeight:     "normal",
				ShowIcons:     true,
			},
			StatusBar: StatusBarConfig{
				Visible:           true,
				ShowFileCount:     true,
				ShowSelectionInfo: true,
			},
		},
		Search: SearchConfig{
			Engine:            "builtin",
			DefaultDepth:      2,
			RememberLastQuery: false,
		},
		Behavior: BehaviorConfig{
			ConfirmDelete:       true,
			DoubleClickToOpen:   true,
			RestoreLastPath:     true,
			SingleClickToSelect: true,
		},
		Tabs: TabsConfig{
			Enabled:            false,
			NewTabLocation:     "home",
			CustomTabPath:      "",
			LastTabBehavior:    "close_app",
			RestoreTabsOnStart: false,
		},
		Panels: PanelsConfig{
			Preview: PanelConfig{
				Enabled:  false,
				Position: "right",
				Width:    300,
			},
			Terminal: PanelConfig{
				Enabled:  false,
				Position: "bottom",
				Height:   200,
			},
		},
		Preview: PreviewConfig{
			Enabled:          true,
			Position:         "right",
			WidthPercent:     33, // 1/3 of screen width
			TextExtensions:   []string{".txt", ".json", ".csv", ".md", ".org", ".log", ".xml", ".yaml", ".yml", ".toml", ".ini", ".conf", ".cfg"},
			ImageExtensions:  []string{".png", ".jpg", ".jpeg", ".gif", ".bmp", ".webp", ".heic", ".heif"},
			MaxFileSize:      1024 * 1024, // 1MB default limit
			MarkdownRendered: true,
		},
		Hotkeys: DefaultHotkeys(),
		Favorites: []FavoriteEntry{
			{Name: "Home", Path: home, Icon: "home"},
			{Name: "Documents", Path: filepath.Join(home, "Documents"), Icon: "folder"},
			{Name: "Downloads", Path: filepath.Join(home, "Downloads"), Icon: "folder"},
		},
	}
}

// ConfigPath returns the config file path: ~/.config/razor/config.json
// This is consistent across all platforms (Windows, macOS, Linux)
func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "razor", "config.json")
}

// Load reads the configuration from the config file
// If the file doesn't exist, creates it with defaults
// If parsing fails, stores the error and returns defaults
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.path = ConfigPath()
	m.parseErr = nil

	// Ensure config directory exists
	configDir := filepath.Dir(m.path)
	if err := os.MkdirAll(configDir, dirPermission); err != nil {
		log.Printf("Config: failed to create directory %s: %v", configDir, err)
		return err
	}

	// Try to read existing config
	data, err := os.ReadFile(m.path)
	if os.IsNotExist(err) {
		// Create default config file
		log.Printf("Config: creating default config at %s", m.path)
		m.config = DefaultConfig()
		if saveErr := m.saveUnlocked(); saveErr != nil {
			log.Printf("Config: failed to save default config: %v", saveErr)
			return saveErr
		}
		log.Printf("Config: default config created successfully")
		return nil
	}
	if err != nil {
		log.Printf("Config: failed to read %s: %v", m.path, err)
		return err
	}

	// Parse JSON
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		// Store error for UI display, use defaults
		log.Printf("Config: JSON parse error: %v", err)
		m.parseErr = err
		m.config = DefaultConfig()
		return nil // Don't return error - we're using defaults
	}

	log.Printf("Config: loaded from %s", m.path)
	m.config = &cfg
	return nil
}

// saveUnlocked saves config without acquiring lock (caller must hold lock)
func (m *Manager) saveUnlocked() error {
	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, data, filePermission)
}

// Save writes the current configuration to disk
func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveUnlocked()
}

// Get returns a copy of the current configuration
func (m *Manager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.config == nil {
		return *DefaultConfig()
	}
	return *m.config
}

// ParseError returns the parsing error if config failed to load
func (m *Manager) ParseError() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.parseErr
}

// SetTheme updates the theme setting
func (m *Manager) SetTheme(theme string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config.UI.Theme = theme
	m.saveUnlocked()
}

// SetShowDotfiles updates the show dotfiles setting
func (m *Manager) SetShowDotfiles(show bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config.UI.FileList.ShowDotfiles = show
	m.saveUnlocked()
}

// SetSearchEngine updates the search engine setting
func (m *Manager) SetSearchEngine(engine string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config.Search.Engine = engine
	m.saveUnlocked()
}

// SetDefaultDepth updates the default search depth
func (m *Manager) SetDefaultDepth(depth int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config.Search.DefaultDepth = depth
	m.saveUnlocked()
}

// SetSidebarLayout updates the sidebar layout
func (m *Manager) SetSidebarLayout(layout string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config.UI.Sidebar.Layout = layout
	m.saveUnlocked()
}

// GetSidebarTabStyle returns the sidebar tab style
func (m *Manager) GetSidebarTabStyle() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.config.UI.Sidebar.TabStyle == "" {
		return "manila" // Default
	}
	return m.config.UI.Sidebar.TabStyle
}

// AddFavorite adds a new favorite
func (m *Manager) AddFavorite(name, path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config.Favorites = append(m.config.Favorites, FavoriteEntry{
		Name: name,
		Path: path,
		Icon: "folder",
	})
	if err := m.saveUnlocked(); err != nil {
		log.Printf("Error saving config after adding favorite: %v", err)
	}
}

// RemoveFavorite removes a favorite by path
func (m *Manager) RemoveFavorite(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, fav := range m.config.Favorites {
		if fav.Path == path {
			m.config.Favorites = append(m.config.Favorites[:i], m.config.Favorites[i+1:]...)
			break
		}
	}
	if err := m.saveUnlocked(); err != nil {
		log.Printf("Error saving config after removing favorite: %v", err)
	}
}

// GetFavorites returns a flat list of favorite paths (for backwards compatibility)
func (m *Manager) GetFavorites() []FavoriteEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config.Favorites
}

// IsDarkMode returns true if dark mode is enabled
func (m *Manager) IsDarkMode() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config.UI.Theme == "dark"
}

// GetSidebarLayout returns the sidebar layout mode
func (m *Manager) GetSidebarLayout() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config.UI.Sidebar.Layout
}

// GetPreviewConfig returns the preview pane configuration
func (m *Manager) GetPreviewConfig() PreviewConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config.Preview
}

// GetTabsConfig returns the tabs configuration
func (m *Manager) GetTabsConfig() TabsConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config.Tabs
}

// GetHotkeys returns the hotkeys configuration
func (m *Manager) GetHotkeys() HotkeysConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config.Hotkeys
}

// GenerateConfig backs up existing config and creates a fresh default config
// Returns the backup path if a backup was created, or empty string if no existing config
func GenerateConfig() (backupPath string, err error) {
	configPath := ConfigPath()

	// Check if existing config exists
	if _, err := os.Stat(configPath); err == nil {
		// Create backup with timestamp
		timestamp := time.Now().Format("20060102-150405")
		backupPath = filepath.Join(filepath.Dir(configPath), "config.backup."+timestamp+".json")

		// Read existing config
		data, err := os.ReadFile(configPath)
		if err != nil {
			return "", fmt.Errorf("failed to read existing config: %w", err)
		}

		// Write backup
		if err := os.WriteFile(backupPath, data, filePermission); err != nil {
			return "", fmt.Errorf("failed to write backup: %w", err)
		}
	}

	// Ensure config directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, dirPermission); err != nil {
		return backupPath, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write fresh default config
	defaultCfg := DefaultConfig()
	data, err := json.MarshalIndent(defaultCfg, "", "  ")
	if err != nil {
		return backupPath, fmt.Errorf("failed to marshal default config: %w", err)
	}

	if err := os.WriteFile(configPath, data, filePermission); err != nil {
		return backupPath, fmt.Errorf("failed to write config: %w", err)
	}

	return backupPath, nil
}
