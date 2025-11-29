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
)

type Request struct {
	Op   EventType
	Path string
}

type Response struct {
	Op        EventType
	Favorites []string // List of paths
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

	// Schema
	query := `
	CREATE TABLE IF NOT EXISTS favorites (
		path TEXT PRIMARY KEY,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err := db.Exec(query); err != nil {
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

func (d *DB) Close() {
	if d.conn != nil {
		d.conn.Close()
	}
}
