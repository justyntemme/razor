package store

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

type EventType int

const (
	FetchFavorites EventType = iota
	AddFavorite
	RemoveFavorite
	FetchSettings
	SaveSetting
)

type Request struct {
	Op    EventType
	Path  string
	Key   string
	Value string
}

type Response struct {
	Op        EventType
	Favorites []string          // List of paths
	Settings  map[string]string // Key-value settings
	Err       error
}

type DB struct {
	conn         *sql.DB
	RequestChan  chan Request
	ResponseChan chan Response
}

func NewDB() *DB {
	return &DB{
		RequestChan:  make(chan Request, 10),
		ResponseChan: make(chan Response, 10),
	}
}

// Open initializes the database connection and schema
func (d *DB) Open(dbPath string) error {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}

	// Performance Tuning
	// WAL mode allows simultaneous readers and writers
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return err
	}
	// Synchronous NORMAL is safe against app crashes, faster than FULL
	if _, err := db.Exec("PRAGMA synchronous=NORMAL;"); err != nil {
		return err
	}

	// Schema - Favorites table
	query := `
	CREATE TABLE IF NOT EXISTS favorites (
		path TEXT PRIMARY KEY,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err := db.Exec(query); err != nil {
		return err
	}

	// Schema - Settings table
	settingsQuery := `
	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	`
	if _, err := db.Exec(settingsQuery); err != nil {
		return err
	}

	d.conn = db
	return nil
}

func (d *DB) Start() {
	for req := range d.RequestChan {
		switch req.Op {
		case FetchFavorites:
			d.handleFetch()
		case AddFavorite:
			d.handleAdd(req.Path)
		case RemoveFavorite:
			d.handleRemove(req.Path)
		case FetchSettings:
			d.handleFetchSettings()
		case SaveSetting:
			d.handleSaveSetting(req.Key, req.Value)
		}
	}
}

func (d *DB) handleFetch() {
	rows, err := d.conn.Query("SELECT path FROM favorites ORDER BY created_at ASC")
	if err != nil {
		d.ResponseChan <- Response{Op: FetchFavorites, Err: err}
		return
	}
	defer rows.Close()

	var favs []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err == nil {
			favs = append(favs, path)
		}
	}

	d.ResponseChan <- Response{Op: FetchFavorites, Favorites: favs}
}

func (d *DB) handleAdd(path string) {
	// Use INSERT OR IGNORE to handle duplicates gracefully
	_, err := d.conn.Exec("INSERT OR IGNORE INTO favorites (path) VALUES (?)", path)
	if err != nil {
		log.Printf("Store Error: %v", err)
	}
	// Always trigger a fetch after modification to sync UI
	d.handleFetch()
}

func (d *DB) handleRemove(path string) {
	_, err := d.conn.Exec("DELETE FROM favorites WHERE path = ?", path)
	if err != nil {
		log.Printf("Store Error: %v", err)
	}
	d.handleFetch()
}

func (d *DB) handleFetchSettings() {
	rows, err := d.conn.Query("SELECT key, value FROM settings")
	if err != nil {
		d.ResponseChan <- Response{Op: FetchSettings, Err: err}
		return
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err == nil {
			settings[key] = value
		}
	}

	d.ResponseChan <- Response{Op: FetchSettings, Settings: settings}
}

func (d *DB) handleSaveSetting(key, value string) {
	// Use INSERT OR REPLACE to upsert the setting
	_, err := d.conn.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)", key, value)
	if err != nil {
		log.Printf("Store Error saving setting: %v", err)
	}
	// Trigger a fetch to sync settings
	d.handleFetchSettings()
}

func (d *DB) Close() {
	if d.conn != nil {
		d.conn.Close()
	}
}