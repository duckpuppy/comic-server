package configdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// ServerMiscSettings bundles small, previously config.yaml-only server
// settings (comic-server-wp8k, the second slice of comic-server-4hsz's
// Settings UI push; AutoSync added in comic-server-r769): CBZ Convert's
// enabled flag, the ignore-devices list, and auto-sync-on-connect. All
// three are read fresh at the point of use in cmd/server.go rather than
// baked into an object at startup, so - unlike library_path/ports/the
// ComicVine API key/Komga connection settings - they can take effect
// live once moved here, no restart needed. Bundled into one row/table
// rather than separate ones, matching scan_info's own "callers always
// want the whole thing at once" reasoning, even though these fields are
// otherwise unrelated - each is a single small value with no natural
// larger settings section of their own yet.
type ServerMiscSettings struct {
	CBZConvertEnabled bool
	IgnoreDevices     []string
	AutoSync          bool
	// CountCoveredAsOwned: issues a collected edition contains count as
	// owned in Missing Issues and the ComicVine smart-list matchers (tracker #46).
	CountCoveredAsOwned bool
	// LocalIdentityRecheck: a slow background ComicVine search for books
	// with a local identity, queued as suggestions (tracker #47). Off by default.
	LocalIdentityRecheck bool
	// ExportPath: the folder Browse "Export…" writes CBZ copies into
	// (tracker #48). Empty means "not set here": the config file / env
	// value, if any, is used instead.
	ExportPath string
}

// createServerMiscSettingsTable creates the server_misc_settings table - a
// single-row store (id fixed at 1), same shape as trash_settings/scan_info.
func (db *DB) createServerMiscSettingsTable() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS server_misc_settings (
		id                  INTEGER PRIMARY KEY CHECK (id = 1),
		cbz_convert_enabled INTEGER NOT NULL DEFAULT 0,
		ignore_devices      TEXT NOT NULL DEFAULT '[]',
		auto_sync           INTEGER NOT NULL DEFAULT 0,
		count_covered_as_owned INTEGER NOT NULL DEFAULT 0,
		local_identity_recheck INTEGER NOT NULL DEFAULT 0,
		export_path            TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		return fmt.Errorf("create server_misc_settings table: %w", err)
	}
	return nil
}

// addAutoSyncColumn adds the auto_sync column to an existing
// server_misc_settings table (comic-server-r769) - guarded by hasColumn
// so a database migrating through this step twice, or one whose table was
// just freshly created by createServerMiscSettingsTable (which already
// defines the column), doesn't hit a "duplicate column" error.
func (db *DB) addAutoSyncColumn() error {
	hasIt, err := db.hasColumn("server_misc_settings", "auto_sync")
	if err != nil {
		return fmt.Errorf("add auto_sync column: %w", err)
	}
	if hasIt {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE server_misc_settings ADD COLUMN auto_sync INTEGER NOT NULL DEFAULT 0`); err != nil {
		return fmt.Errorf("add auto_sync column: %w", err)
	}
	return nil
}

// addCountCoveredColumn adds count_covered_as_owned (tracker #46), guarded
// like addAutoSyncColumn.
func (db *DB) addCountCoveredColumn() error {
	hasIt, err := db.hasColumn("server_misc_settings", "count_covered_as_owned")
	if err != nil {
		return fmt.Errorf("add count_covered_as_owned column: %w", err)
	}
	if hasIt {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE server_misc_settings ADD COLUMN count_covered_as_owned INTEGER NOT NULL DEFAULT 0`); err != nil {
		return fmt.Errorf("add count_covered_as_owned column: %w", err)
	}
	return nil
}

// addLocalIdentityRecheckColumn adds local_identity_recheck (tracker #47),
// guarded like addAutoSyncColumn.
func (db *DB) addLocalIdentityRecheckColumn() error {
	hasIt, err := db.hasColumn("server_misc_settings", "local_identity_recheck")
	if err != nil {
		return fmt.Errorf("add local_identity_recheck column: %w", err)
	}
	if hasIt {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE server_misc_settings ADD COLUMN local_identity_recheck INTEGER NOT NULL DEFAULT 0`); err != nil {
		return fmt.Errorf("add local_identity_recheck column: %w", err)
	}
	return nil
}

// addExportPathColumn adds export_path (tracker #48), guarded like addAutoSyncColumn.
func (db *DB) addExportPathColumn() error {
	hasIt, err := db.hasColumn("server_misc_settings", "export_path")
	if err != nil {
		return fmt.Errorf("add export_path column: %w", err)
	}
	if hasIt {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE server_misc_settings ADD COLUMN export_path TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add export_path column: %w", err)
	}
	return nil
}

// GetServerMiscSettings returns the stored settings, or nil if config.db
// has never been given any - callers should fall back to config.yaml's
// Server.CBZConvert.Enabled/Server.IgnoreDevices/Server.AutoSync in that
// case.
func (db *DB) GetServerMiscSettings() (*ServerMiscSettings, error) {
	var enabled, autoSync, countCovered, recheck bool
	var ignoreJSON, exportPath string
	err := db.QueryRow(`SELECT cbz_convert_enabled, ignore_devices, auto_sync, count_covered_as_owned, local_identity_recheck, export_path FROM server_misc_settings WHERE id = 1`).
		Scan(&enabled, &ignoreJSON, &autoSync, &countCovered, &recheck, &exportPath)
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
	return &ServerMiscSettings{CBZConvertEnabled: enabled, IgnoreDevices: ignoreDevices, AutoSync: autoSync, CountCoveredAsOwned: countCovered, LocalIdentityRecheck: recheck, ExportPath: exportPath}, nil
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
		INSERT INTO server_misc_settings (id, cbz_convert_enabled, ignore_devices, auto_sync, count_covered_as_owned, local_identity_recheck, export_path)
		VALUES (1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			cbz_convert_enabled = excluded.cbz_convert_enabled,
			ignore_devices = excluded.ignore_devices,
			auto_sync = excluded.auto_sync,
			count_covered_as_owned = excluded.count_covered_as_owned,
			local_identity_recheck = excluded.local_identity_recheck,
			export_path = excluded.export_path
	`, s.CBZConvertEnabled, string(ignoreJSON), s.AutoSync, s.CountCoveredAsOwned, s.LocalIdentityRecheck, s.ExportPath)
	if err != nil {
		return fmt.Errorf("upsert server misc settings: %w", err)
	}
	return nil
}
