package configdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// ServerMiscSettings bundles two small, previously config.yaml-only server
// settings (comic-server-wp8k, the second slice of comic-server-4hsz's
// Settings UI push): CBZ Convert's enabled flag, and the ignore-devices
// list. Both are read fresh at the point of use in cmd/server.go rather
// than baked into an object at startup, so - unlike library_path/ports/
// the ComicVine API key/Komga connection settings - they can take effect
// live once moved here, no restart needed. Bundled into one row/table
// rather than two, matching scan_info's own "callers always want the
// whole thing at once" reasoning, even though these two fields are
// otherwise unrelated - both are single small values with no natural
// larger settings section of their own yet.
type ServerMiscSettings struct {
	CBZConvertEnabled bool
	IgnoreDevices     []string
}

// createServerMiscSettingsTable creates the server_misc_settings table - a
// single-row store (id fixed at 1), same shape as trash_settings/scan_info.
func (db *DB) createServerMiscSettingsTable() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS server_misc_settings (
		id                  INTEGER PRIMARY KEY CHECK (id = 1),
		cbz_convert_enabled INTEGER NOT NULL DEFAULT 0,
		ignore_devices      TEXT NOT NULL DEFAULT '[]'
	)`)
	if err != nil {
		return fmt.Errorf("create server_misc_settings table: %w", err)
	}
	return nil
}

// GetServerMiscSettings returns the stored settings, or nil if config.db
// has never been given any - callers should fall back to config.yaml's
// Server.CBZConvert.Enabled/Server.IgnoreDevices in that case.
func (db *DB) GetServerMiscSettings() (*ServerMiscSettings, error) {
	var enabled bool
	var ignoreJSON string
	err := db.QueryRow(`SELECT cbz_convert_enabled, ignore_devices FROM server_misc_settings WHERE id = 1`).
		Scan(&enabled, &ignoreJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get server misc settings: %w", err)
	}
	var ignoreDevices []string
	if err := json.Unmarshal([]byte(ignoreJSON), &ignoreDevices); err != nil {
		return nil, fmt.Errorf("unmarshal server misc settings ignore_devices: %w", err)
	}
	return &ServerMiscSettings{CBZConvertEnabled: enabled, IgnoreDevices: ignoreDevices}, nil
}

// UpsertServerMiscSettings replaces the stored settings wholesale.
func (db *DB) UpsertServerMiscSettings(s ServerMiscSettings) error {
	ignoreDevices := s.IgnoreDevices
	if ignoreDevices == nil {
		ignoreDevices = []string{}
	}
	ignoreJSON, err := json.Marshal(ignoreDevices)
	if err != nil {
		return fmt.Errorf("marshal server misc settings ignore_devices: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO server_misc_settings (id, cbz_convert_enabled, ignore_devices)
		VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			cbz_convert_enabled = excluded.cbz_convert_enabled,
			ignore_devices = excluded.ignore_devices
	`, s.CBZConvertEnabled, string(ignoreJSON))
	if err != nil {
		return fmt.Errorf("upsert server misc settings: %w", err)
	}
	return nil
}
