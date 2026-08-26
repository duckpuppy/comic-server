// Package configdb provides a small, always-open SQLite database for
// record-shaped configuration (device registrations, list assignments,
// Komga targets) that's already CRUD'd via REST and persisted immediately
// - as opposed to bootstrap-critical settings (library path, ports, log
// level) which stay in config.yaml/env/CLI, since a DB config table can't
// describe where to find itself.
//
// This is deliberately a separate database from internal/storage's
// library DB and internal/comicvine's cache DB - neither of those is
// unconditionally open (the library DB only exists with --db/database_path
// set, the ComicVine cache only with an API key configured), so config.db
// is opened at every server startup regardless of which library backend
// is active. See comic-server-745 for the design record and
// comic-server-ihb for this package's own issue.
package configdb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps a SQLite database connection for config storage.
type DB struct {
	*sql.DB
	path string
}

// Open opens or creates the config database at the given path. Safe to
// call at every server startup - creates the parent directory and file if
// missing, and initSchema's migrations are idempotent (a no-op once the
// schema is already current).
func Open(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create config db directory: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open config db: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping config db: %w", err)
	}

	configDB := &DB{
		DB:   db,
		path: path,
	}

	if err := configDB.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize config db schema: %w", err)
	}

	return configDB, nil
}

// Path returns the database file path.
func (db *DB) Path() string {
	return db.path
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.DB.Close()
}
