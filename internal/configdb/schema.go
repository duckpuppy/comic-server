package configdb

import "fmt"

// Schema version for migrations. Version 1 had no tables (comic-server-ihb,
// the open/migrate foundation only). Version 2 adds devices/device_lists
// (comic-server-3ek). Version 3 adds komga_targets (comic-server-cde).
// Version 4 adds sync_history (comic-server-7vu).
const schemaVersion = 4

// initSchema brings the database up to schemaVersion. No-ops if already
// current - safe to call on every Open, every server startup.
func (db *DB) initSchema() error {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("get schema version: %w", err)
	}

	if version >= schemaVersion {
		return nil
	}

	if version == 0 {
		if err := db.createTables(); err != nil {
			return err
		}
	} else {
		if version < 2 {
			if err := db.migrateV1ToV2(); err != nil {
				return fmt.Errorf("migrate v1→v2: %w", err)
			}
		}
		if version < 3 {
			if err := db.migrateV2ToV3(); err != nil {
				return fmt.Errorf("migrate v2→v3: %w", err)
			}
		}
		if version < 4 {
			if err := db.migrateV3ToV4(); err != nil {
				return fmt.Errorf("migrate v3→v4: %w", err)
			}
		}
	}

	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}

	return nil
}

// createTables creates the full current schema for a fresh database.
func (db *DB) createTables() error {
	if err := db.createDeviceTables(); err != nil {
		return err
	}
	if err := db.createKomgaTargetsTable(); err != nil {
		return err
	}
	return db.createSyncHistoryTable()
}

// migrateV1ToV2 adds the devices/device_lists tables for a database that
// was created under schemaVersion 1 (comic-server-ihb's foundation-only
// release, which shipped with no tables at all).
func (db *DB) migrateV1ToV2() error {
	return db.createDeviceTables()
}

// migrateV2ToV3 adds the komga_targets table for a database that was
// created under schemaVersion 2 (comic-server-3ek, devices/device_lists
// only).
func (db *DB) migrateV2ToV3() error {
	return db.createKomgaTargetsTable()
}

// migrateV3ToV4 adds the sync_history table for a database that was
// created under schemaVersion 3 (comic-server-cde, devices/device_lists/
// komga_targets only).
func (db *DB) migrateV3ToV4() error {
	return db.createSyncHistoryTable()
}

// createSyncHistoryTable creates the sync_history table - an append-only
// log of completed/failed/aborted syncs, distinct from the devices/
// device_lists/komga_targets tables which hold current desired state
// rather than history. Backs syncstate.Manager's in-memory history so it
// survives a restart (comic-server-7vu).
func (db *DB) createSyncHistoryTable() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS sync_history (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id     TEXT NOT NULL,
		device_ip     TEXT NOT NULL DEFAULT '',
		device_name   TEXT NOT NULL DEFAULT '',
		start_time    TEXT NOT NULL,
		end_time      TEXT,
		status        TEXT NOT NULL,
		progress      INTEGER NOT NULL DEFAULT 0,
		books_total   INTEGER NOT NULL DEFAULT 0,
		books_added   INTEGER NOT NULL DEFAULT 0,
		books_updated INTEGER NOT NULL DEFAULT 0,
		books_deleted INTEGER NOT NULL DEFAULT 0,
		error_count   INTEGER NOT NULL DEFAULT 0,
		error_message TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		return fmt.Errorf("create sync_history table: %w", err)
	}
	return nil
}

// createDeviceTables creates the devices and device_lists tables -
// factored out since both a fresh install (createTables) and an upgrade
// from schemaVersion 1 (migrateV1ToV2) need to create them identically.
func (db *DB) createDeviceTables() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS devices (
			device_id        TEXT PRIMARY KEY,
			friendly_name    TEXT NOT NULL DEFAULT '',
			last_seen        TEXT,
			default_settings TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS device_lists (
			device_id  TEXT NOT NULL REFERENCES devices(device_id) ON DELETE CASCADE,
			list_id    TEXT NOT NULL,
			list_name  TEXT NOT NULL DEFAULT '',
			enabled    INTEGER NOT NULL DEFAULT 1,
			settings   TEXT,
			PRIMARY KEY (device_id, list_id)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("create device tables: %w", err)
		}
	}
	return nil
}

// createKomgaTargetsTable creates the komga_targets table - factored out
// since both a fresh install (createTables) and an upgrade from
// schemaVersion 2 (migrateV2ToV3) need to create it identically. One row
// per list (list_id is the primary key) since a list can have at most one
// Komga target, matching the existing REST API's duplicate-rejection
// behavior.
func (db *DB) createKomgaTargetsTable() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS komga_targets (
		list_id          TEXT PRIMARY KEY,
		list_name        TEXT NOT NULL DEFAULT '',
		type             TEXT NOT NULL,
		komga_name       TEXT NOT NULL,
		enabled          INTEGER NOT NULL DEFAULT 1,
		sync_read_status INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		return fmt.Errorf("create komga_targets table: %w", err)
	}
	return nil
}
