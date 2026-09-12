package configdb

import (
	"database/sql"
	"errors"
	"fmt"
)

// TrashSettings is the stored TrashPath/TrashRetentionDays pair - the
// first UI/API surface for these two ServerConfig fields (comic-server-4hsz),
// previously config.yaml-hand-edit-only. Unlike ScanInfoConfig these two
// fields have no dedicated config.* type of their own (they're flat
// ServerConfig fields), so this type exists purely at the configdb layer.
type TrashSettings struct {
	Path          string
	RetentionDays int
}

// createTrashSettingsTable creates the trash_settings table - a single-row
// store (id fixed at 1), same shape as scan_info/ui_settings: one row,
// upserted wholesale, no per-field endpoints.
func (db *DB) createTrashSettingsTable() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS trash_settings (
		id             INTEGER PRIMARY KEY CHECK (id = 1),
		path           TEXT NOT NULL DEFAULT '',
		retention_days INTEGER NOT NULL DEFAULT 30
	)`)
	if err != nil {
		return fmt.Errorf("create trash_settings table: %w", err)
	}
	return nil
}

// GetTrashSettings returns the stored trash settings, or nil if config.db
// has never been given any - callers should fall back to config.yaml's
// Server.TrashPath/TrashRetentionDays in that case (see
// api.Server.effectiveTrashConfig).
func (db *DB) GetTrashSettings() (*TrashSettings, error) {
	var s TrashSettings
	err := db.QueryRow(`SELECT path, retention_days FROM trash_settings WHERE id = 1`).
		Scan(&s.Path, &s.RetentionDays)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get trash settings: %w", err)
	}
	return &s, nil
}

// UpsertTrashSettings replaces the stored trash settings wholesale.
func (db *DB) UpsertTrashSettings(s TrashSettings) error {
	_, err := db.Exec(`
		INSERT INTO trash_settings (id, path, retention_days)
		VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			path = excluded.path,
			retention_days = excluded.retention_days
	`, s.Path, s.RetentionDays)
	if err != nil {
		return fmt.Errorf("upsert trash settings: %w", err)
	}
	return nil
}
