package configdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// WatchFolders is the config.db-backed store for server.watch_folders
// (comic-server-obe, formerly config.yaml-only). Read fresh at the point
// of use in internal/api/watchfolder.go rather than baked into an object
// at startup, so moving it here makes it live-editable - no restart
// needed, unlike library_path/ports/the ComicVine API key/Komga
// connection settings (see restart_required_settings.go).
type WatchFolders struct {
	Folders []string
}

// createWatchFoldersTable creates the watch_folders table - a single-row
// store (id fixed at 1), same shape as server_misc_settings.
func (db *DB) createWatchFoldersTable() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS watch_folders (
		id      INTEGER PRIMARY KEY CHECK (id = 1),
		folders TEXT NOT NULL DEFAULT '[]'
	)`)
	if err != nil {
		return fmt.Errorf("create watch_folders table: %w", err)
	}
	return nil
}

// GetWatchFolders returns the stored folders, or nil if config.db has
// never been given any - callers should fall back to config.yaml's
// Server.WatchFolders in that case.
func (db *DB) GetWatchFolders() (*WatchFolders, error) {
	var foldersJSON string
	err := db.QueryRow(`SELECT folders FROM watch_folders WHERE id = 1`).Scan(&foldersJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get watch folders: %w", err)
	}
	var folders []string
	if err := json.Unmarshal([]byte(foldersJSON), &folders); err != nil {
		return nil, fmt.Errorf("unmarshal watch folders: %w", err)
	}
	return &WatchFolders{Folders: folders}, nil
}

// UpsertWatchFolders replaces the stored folder list wholesale.
func (db *DB) UpsertWatchFolders(w WatchFolders) error {
	folders := w.Folders
	if folders == nil {
		folders = []string{}
	}
	foldersJSON, err := json.Marshal(folders)
	if err != nil {
		return fmt.Errorf("marshal watch folders: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO watch_folders (id, folders)
		VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET folders = excluded.folders
	`, string(foldersJSON))
	if err != nil {
		return fmt.Errorf("upsert watch folders: %w", err)
	}
	return nil
}
