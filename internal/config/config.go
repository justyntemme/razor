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

// Config holds all user-configurable settings loaded from config.json
type Config struct {
	UI        UIConfig        `json:"ui"`
	Search    SearchConfig    `json:"search"`
	Behavior  BehaviorConfig  `json:"behavior"`
	Tabs      TabsConfig      `json:"tabs"`
	Panels    PanelsConfig    `json:"panels"`
	Preview   PreviewConfig   `json:"preview"`
	Favorites []FavoriteEntry `json:"favorites"`
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
	NewTabLocation     string `json:"newTabLocation"`     // "current" | "home" | "custom"
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
	Enabled        bool     `json:"enabled"`
	Position       string   `json:"position"`       // "right" | "bottom"
	WidthPercent   int      `json:"widthPercent"`   // Percentage of screen width (e.g., 33 for 1/3)
	TextExtensions []string `json:"textExtensions"` // Extensions to show text preview for
	MaxFileSize    int64    `json:"maxFileSize"`    // Max file size in bytes to preview (0 = no limit)
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
			NewTabLocation:     "current",
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
			Enabled:        true,
			Position:       "right",
			WidthPercent:   33, // 1/3 of screen width
			TextExtensions: []string{".txt", ".json", ".csv", ".md", ".log", ".xml", ".yaml", ".yml", ".toml", ".ini", ".conf", ".cfg"},
			MaxFileSize:    1024 * 1024, // 1MB default limit
		},
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
	if err := os.MkdirAll(configDir, 0o755); err != nil {
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
	return os.WriteFile(m.path, data, 0o644)
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
	m.config.UI.Theme = theme
	m.mu.Unlock()
	m.Save()
}

// SetShowDotfiles updates the show dotfiles setting
func (m *Manager) SetShowDotfiles(show bool) {
	m.mu.Lock()
	m.config.UI.FileList.ShowDotfiles = show
	m.mu.Unlock()
	m.Save()
}

// SetSearchEngine updates the search engine setting
func (m *Manager) SetSearchEngine(engine string) {
	m.mu.Lock()
	m.config.Search.Engine = engine
	m.mu.Unlock()
	m.Save()
}

// SetDefaultDepth updates the default search depth
func (m *Manager) SetDefaultDepth(depth int) {
	m.mu.Lock()
	m.config.Search.DefaultDepth = depth
	m.mu.Unlock()
	m.Save()
}

// SetSidebarLayout updates the sidebar layout
func (m *Manager) SetSidebarLayout(layout string) {
	m.mu.Lock()
	m.config.UI.Sidebar.Layout = layout
	m.mu.Unlock()
	m.Save()
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
	m.config.Favorites = append(m.config.Favorites, FavoriteEntry{
		Name: name,
		Path: path,
		Icon: "folder",
	})
	m.mu.Unlock()
	m.Save()
}

// RemoveFavorite removes a favorite by path
func (m *Manager) RemoveFavorite(path string) {
	m.mu.Lock()
	for i, fav := range m.config.Favorites {
		if fav.Path == path {
			m.config.Favorites = append(m.config.Favorites[:i], m.config.Favorites[i+1:]...)
			break
		}
	}
	m.mu.Unlock()
	m.Save()
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
		if err := os.WriteFile(backupPath, data, 0o644); err != nil {
			return "", fmt.Errorf("failed to write backup: %w", err)
		}
	}

	// Ensure config directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return backupPath, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write fresh default config
	defaultCfg := DefaultConfig()
	data, err := json.MarshalIndent(defaultCfg, "", "  ")
	if err != nil {
		return backupPath, fmt.Errorf("failed to marshal default config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return backupPath, fmt.Errorf("failed to write config: %w", err)
	}

	return backupPath, nil
}
