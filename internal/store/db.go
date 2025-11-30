package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/justyntemme/razor/internal/debug"
	_ "modernc.org/sqlite"
)

type EventType int

const (
	FetchFavorites EventType = iota
	AddFavorite
	RemoveFavorite
	FetchSettings
	SaveSetting
	// Search history operations
	AddSearchHistory
	FetchSearchHistory
)

// SearchHistoryEntry represents a single search history entry
type SearchHistoryEntry struct {
	Query     string
	Timestamp time.Time
	Score     float64 // For fuzzy matching ranking
}

type Request struct {
	Op         EventType
	Path       string
	Key, Value string
	Query      string // For search history
	Limit      int    // For search history limit
}

type Response struct {
	Op            EventType
	Favorites     []string
	Settings      map[string]string
	SearchHistory []SearchHistoryEntry
	Err           error
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
	debug.Log(debug.STORE, "Opening database: %s", dbPath)

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		debug.Log(debug.STORE, "Failed to create db directory: %v", err)
		return err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		debug.Log(debug.STORE, "Failed to open db: %v", err)
		return err
	}

	for _, pragma := range []string{"PRAGMA journal_mode=WAL;", "PRAGMA synchronous=NORMAL;"} {
		if _, err := db.Exec(pragma); err != nil {
			debug.Log(debug.STORE, "Failed to set pragma: %v", err)
			return err
		}
	}

	schema := `
		CREATE TABLE IF NOT EXISTS favorites (path TEXT PRIMARY KEY, created_at DATETIME DEFAULT CURRENT_TIMESTAMP);
		CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS search_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			query TEXT NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_search_history_timestamp ON search_history(timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_search_history_query ON search_history(query);
	`
	if _, err := db.Exec(schema); err != nil {
		debug.Log(debug.STORE, "Failed to create schema: %v", err)
		return err
	}

	d.conn = db
	debug.Log(debug.STORE, "Database opened successfully")
	return nil
}

func (d *DB) Start() {
	debug.Log(debug.STORE, "Store goroutine started")
	for req := range d.RequestChan {
		debug.Log(debug.STORE, "Request: op=%d path=%q key=%q query=%q", req.Op, req.Path, req.Key, req.Query)

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
		case AddSearchHistory:
			d.addSearchHistory(req.Query)
		case FetchSearchHistory:
			d.fetchSearchHistory(req.Query, req.Limit)
		}
	}
}

func (d *DB) fetchFavorites() {
	rows, err := d.conn.Query("SELECT path FROM favorites ORDER BY created_at ASC")
	if err != nil {
		debug.Log(debug.STORE, "fetchFavorites error: %v", err)
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
	debug.Log(debug.STORE, "fetchFavorites: returning %d favorites", len(favs))
	d.ResponseChan <- Response{Op: FetchFavorites, Favorites: favs}
}

func (d *DB) execAndFetch(query, path string) {
	if _, err := d.conn.Exec(query, path); err != nil {
		debug.Log(debug.STORE, "execAndFetch error: %v", err)
	}
	d.fetchFavorites()
}

func (d *DB) fetchSettings() {
	rows, err := d.conn.Query("SELECT key, value FROM settings")
	if err != nil {
		debug.Log(debug.STORE, "fetchSettings error: %v", err)
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
	debug.Log(debug.STORE, "fetchSettings: returning %d settings", len(settings))
	d.ResponseChan <- Response{Op: FetchSettings, Settings: settings}
}

func (d *DB) saveSetting(key, value string) {
	debug.Log(debug.STORE, "saveSetting: %q = %q", key, value)
	if _, err := d.conn.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)", key, value); err != nil {
		debug.Log(debug.STORE, "saveSetting error: %v", err)
	}
	d.fetchSettings()
}

func (d *DB) Close() {
	if d.conn != nil {
		d.conn.Close()
	}
}

// addSearchHistory adds a query to the search history and prunes old entries
func (d *DB) addSearchHistory(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}

	debug.Log(debug.STORE, "addSearchHistory: %q", query)

	// Insert the new query
	if _, err := d.conn.Exec("INSERT INTO search_history (query) VALUES (?)", query); err != nil {
		debug.Log(debug.STORE, "addSearchHistory error: %v", err)
		return
	}

	// Prune to keep only the last 100 entries
	if _, err := d.conn.Exec(`
		DELETE FROM search_history
		WHERE id NOT IN (
			SELECT id FROM search_history
			ORDER BY timestamp DESC
			LIMIT 100
		)
	`); err != nil {
		debug.Log(debug.STORE, "addSearchHistory prune error: %v", err)
	}
}

// fetchSearchHistory retrieves search history, optionally filtered by a fuzzy match pattern
func (d *DB) fetchSearchHistory(pattern string, limit int) {
	if limit <= 0 {
		limit = 3
	}

	pattern = strings.TrimSpace(pattern)
	debug.Log(debug.STORE, "fetchSearchHistory: pattern=%q limit=%d", pattern, limit)

	var entries []SearchHistoryEntry

	if pattern == "" {
		// No pattern - return most recent unique queries
		rows, err := d.conn.Query(`
			SELECT query, MAX(timestamp) as ts
			FROM search_history
			GROUP BY query
			ORDER BY ts DESC
			LIMIT ?
		`, limit)
		if err != nil {
			debug.Log(debug.STORE, "fetchSearchHistory error: %v", err)
			d.ResponseChan <- Response{Op: FetchSearchHistory, Err: err}
			return
		}
		defer rows.Close()

		for rows.Next() {
			var entry SearchHistoryEntry
			var ts string
			if err := rows.Scan(&entry.Query, &ts); err == nil {
				entry.Timestamp, _ = time.Parse("2006-01-02 15:04:05", ts)
				entry.Score = 1.0
				entries = append(entries, entry)
			}
		}
	} else {
		// With pattern - fuzzy match and rank results
		rows, err := d.conn.Query(`
			SELECT DISTINCT query, MAX(timestamp) as ts
			FROM search_history
			GROUP BY query
			ORDER BY ts DESC
		`)
		if err != nil {
			debug.Log(debug.STORE, "fetchSearchHistory error: %v", err)
			d.ResponseChan <- Response{Op: FetchSearchHistory, Err: err}
			return
		}
		defer rows.Close()

		patternLower := strings.ToLower(pattern)
		for rows.Next() {
			var query, ts string
			if err := rows.Scan(&query, &ts); err != nil {
				continue
			}

			// Calculate fuzzy match score
			score := fuzzyScore(strings.ToLower(query), patternLower)
			if score > 0 {
				timestamp, _ := time.Parse("2006-01-02 15:04:05", ts)
				entries = append(entries, SearchHistoryEntry{
					Query:     query,
					Timestamp: timestamp,
					Score:     score,
				})
			}
		}

		// Sort by score descending
		for i := 0; i < len(entries)-1; i++ {
			for j := i + 1; j < len(entries); j++ {
				if entries[j].Score > entries[i].Score {
					entries[i], entries[j] = entries[j], entries[i]
				}
			}
		}

		// Limit results
		if len(entries) > limit {
			entries = entries[:limit]
		}
	}

	debug.Log(debug.STORE, "fetchSearchHistory: returning %d entries", len(entries))
	d.ResponseChan <- Response{Op: FetchSearchHistory, SearchHistory: entries}
}

// fuzzyScore calculates a simple fuzzy matching score
// Returns a score between 0 (no match) and 1 (exact match)
func fuzzyScore(text, pattern string) float64 {
	if pattern == "" {
		return 1.0
	}
	if text == "" {
		return 0
	}

	// Exact match
	if text == pattern {
		return 1.0
	}

	// Contains match (substring)
	if strings.Contains(text, pattern) {
		// Score based on position and length ratio
		idx := strings.Index(text, pattern)
		posScore := 1.0 - float64(idx)/float64(len(text))
		lenScore := float64(len(pattern)) / float64(len(text))
		return 0.5 + (posScore+lenScore)/4
	}

	// Prefix match
	if strings.HasPrefix(text, pattern) {
		return 0.9
	}

	// Fuzzy character matching
	patternIdx := 0
	matchCount := 0
	consecutiveBonus := 0.0
	lastMatchIdx := -2

	for i := 0; i < len(text) && patternIdx < len(pattern); i++ {
		if text[i] == pattern[patternIdx] {
			matchCount++
			if i == lastMatchIdx+1 {
				consecutiveBonus += 0.1
			}
			lastMatchIdx = i
			patternIdx++
		}
	}

	if patternIdx < len(pattern) {
		return 0 // Not all pattern chars matched
	}

	baseScore := float64(matchCount) / float64(len(text))
	return baseScore*0.3 + consecutiveBonus
}
