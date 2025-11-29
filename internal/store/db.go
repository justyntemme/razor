package store

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
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
	Op         EventType
	Path       string
	Key, Value string
}

type Response struct {
	Op        EventType
	Favorites []string
	Settings  map[string]string
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

func (d *DB) Open(dbPath string) error {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}

	for _, pragma := range []string{"PRAGMA journal_mode=WAL;", "PRAGMA synchronous=NORMAL;"} {
		if _, err := db.Exec(pragma); err != nil {
			return err
		}
	}

	schema := `
		CREATE TABLE IF NOT EXISTS favorites (path TEXT PRIMARY KEY, created_at DATETIME DEFAULT CURRENT_TIMESTAMP);
		CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
	`
	if _, err := db.Exec(schema); err != nil {
		return err
	}

	d.conn = db
	return nil
}

func (d *DB) Start() {
	for req := range d.RequestChan {
		switch req.Op {
		case FetchFavorites:
			d.fetchFavorites()
		case AddFavorite:
			d.execAndFetch("INSERT OR IGNORE INTO favorites (path) VALUES (?)", req.Path)
		case RemoveFavorite:
			d.execAndFetch("DELETE FROM favorites WHERE path = ?", req.Path)
		case FetchSettings:
			d.fetchSettings()
		case SaveSetting:
			d.saveSetting(req.Key, req.Value)
		}
	}
}

func (d *DB) fetchFavorites() {
	rows, err := d.conn.Query("SELECT path FROM favorites ORDER BY created_at ASC")
	if err != nil {
		d.ResponseChan <- Response{Op: FetchFavorites, Err: err}
		return
	}
	defer rows.Close()

	var favs []string
	for rows.Next() {
		var path string
		if rows.Scan(&path) == nil {
			favs = append(favs, path)
		}
	}
	d.ResponseChan <- Response{Op: FetchFavorites, Favorites: favs}
}

func (d *DB) execAndFetch(query, path string) {
	if _, err := d.conn.Exec(query, path); err != nil {
		log.Printf("Store Error: %v", err)
	}
	d.fetchFavorites()
}

func (d *DB) fetchSettings() {
	rows, err := d.conn.Query("SELECT key, value FROM settings")
	if err != nil {
		d.ResponseChan <- Response{Op: FetchSettings, Err: err}
		return
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) == nil {
			settings[k] = v
		}
	}
	d.ResponseChan <- Response{Op: FetchSettings, Settings: settings}
}

func (d *DB) saveSetting(key, value string) {
	if _, err := d.conn.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)", key, value); err != nil {
		log.Printf("Store Error: %v", err)
	}
	d.fetchSettings()
}

func (d *DB) Close() {
	if d.conn != nil {
		d.conn.Close()
	}
}
