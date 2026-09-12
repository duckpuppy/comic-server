package configdb

import "fmt"

// Schema version for migrations. Version 1 had no tables (comic-server-ihb,
// the open/migrate foundation only). Version 2 adds devices/device_lists
// (comic-server-3ek). Version 3 adds komga_targets (comic-server-cde).
// Version 4 adds sync_history (comic-server-7vu). Version 5 adds scan_info
// (comic-server-4ms). Version 6 adds dm_groups/dm_rulesets/dm_rules/
// dm_actions (comic-server-764.4). Version 7 adds lo_profiles/
// lo_profile_items/lo_exclude_rules (comic-server-3bz.2). Version 8 adds
// ui_settings (comic-server-8qk). Version 9 adds a source column to
// dm_groups/dm_rulesets (comic-server-vkpq).
const schemaVersion = 9

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
		if version < 5 {
			if err := db.migrateV4ToV5(); err != nil {
				return fmt.Errorf("migrate v4→v5: %w", err)
			}
		}
		if version < 6 {
			if err := db.migrateV5ToV6(); err != nil {
				return fmt.Errorf("migrate v5→v6: %w", err)
			}
		}
		if version < 7 {
			if err := db.migrateV6ToV7(); err != nil {
				return fmt.Errorf("migrate v6→v7: %w", err)
			}
		}
		if version < 8 {
			if err := db.migrateV7ToV8(); err != nil {
				return fmt.Errorf("migrate v7→v8: %w", err)
			}
		}
		if version < 9 {
			if err := db.migrateV8ToV9(); err != nil {
				return fmt.Errorf("migrate v8→v9: %w", err)
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
	if err := db.createSyncHistoryTable(); err != nil {
		return err
	}
	if err := db.createScanInfoTable(); err != nil {
		return err
	}
	if err := db.createDataManagerTables(); err != nil {
		return err
	}
	if err := db.createLibraryOrganizerTables(); err != nil {
		return err
	}
	return db.createUISettingsTable()
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

// migrateV4ToV5 adds the scan_info table for a database that was created
// under schemaVersion 4 (comic-server-7vu, devices/device_lists/
// komga_targets/sync_history only).
func (db *DB) migrateV4ToV5() error {
	return db.createScanInfoTable()
}

// migrateV5ToV6 adds the dm_groups/dm_rulesets/dm_rules/dm_actions tables
// for a database that was created under schemaVersion 5 (comic-server-4ms,
// devices/device_lists/komga_targets/sync_history/scan_info only).
func (db *DB) migrateV5ToV6() error {
	return db.createDataManagerTables()
}

// migrateV6ToV7 adds the lo_profiles/lo_profile_items/lo_exclude_rules
// tables for a database created under schemaVersion 6.
func (db *DB) migrateV6ToV7() error {
	return db.createLibraryOrganizerTables()
}

// migrateV7ToV8 adds the ui_settings table for a database created under
// schemaVersion 7.
func (db *DB) migrateV7ToV8() error {
	return db.createUISettingsTable()
}

// migrateV8ToV9 adds a source column to dm_groups/dm_rulesets, so a
// re-import (comic-server-vkpq's import-merge semantics) can tell which
// rows it's safe to wipe (source='import') apart from rows a user created
// by hand in the native rule editor (source='manual'), which must survive
// a re-import untouched. New rows default to 'manual' (matches
// createDataManagerTables' own column default, for anything created going
// forward through the editor's CRUD API); every row that already existed
// before this migration ran is explicitly set to 'import' instead, since
// nothing could create a dm_groups/dm_rulesets row before comic-server-tj6o
// shipped the editor except the CLI import - preserving pre-upgrade
// wipe-replaces-everything behavior for anyone's existing imported data.
//
// hasColumn-guarded: a database migrating from a version at or below 5
// runs migrateV5ToV6 first in the SAME Open call, which calls
// createDataManagerTables - and that function already defines the source
// column directly (it's shared with the fresh-install path), so
// dm_groups/dm_rulesets already have it by the time this function runs.
// Skipping the ALTER in that case avoids a "duplicate column" error; the
// UPDATE below is a safe no-op either way since a table just created by
// createDataManagerTables in this same run is still empty. The leading
// createDataManagerTables() call is itself CREATE TABLE IF NOT EXISTS, so
// it's a no-op against a real database that already has these tables -
// it only matters for a synthetic test fixture that pins user_version
// without ever having actually run migrateV5ToV6 to create them.
func (db *DB) migrateV8ToV9() error {
	if err := db.createDataManagerTables(); err != nil {
		return fmt.Errorf("migrate v8→v9: %w", err)
	}
	groupsHasSource, err := db.hasColumn("dm_groups", "source")
	if err != nil {
		return fmt.Errorf("migrate v8→v9: %w", err)
	}
	if !groupsHasSource {
		if _, err := db.Exec(`ALTER TABLE dm_groups ADD COLUMN source TEXT NOT NULL DEFAULT 'manual'`); err != nil {
			return fmt.Errorf("migrate v8→v9: %w", err)
		}
	}
	rulesetsHasSource, err := db.hasColumn("dm_rulesets", "source")
	if err != nil {
		return fmt.Errorf("migrate v8→v9: %w", err)
	}
	if !rulesetsHasSource {
		if _, err := db.Exec(`ALTER TABLE dm_rulesets ADD COLUMN source TEXT NOT NULL DEFAULT 'manual'`); err != nil {
			return fmt.Errorf("migrate v8→v9: %w", err)
		}
	}
	if _, err := db.Exec(`UPDATE dm_groups SET source = 'import'`); err != nil {
		return fmt.Errorf("migrate v8→v9: %w", err)
	}
	if _, err := db.Exec(`UPDATE dm_rulesets SET source = 'import'`); err != nil {
		return fmt.Errorf("migrate v8→v9: %w", err)
	}
	return nil
}

// hasColumn reports whether table has a column named name, via
// PRAGMA table_info - used by migrations that ALTER TABLE ADD COLUMN to
// stay idempotent when a table might already have been created with that
// column by a shared create-table helper earlier in the same migration
// chain (see migrateV8ToV9).
func (db *DB) hasColumn(table, name string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return false, fmt.Errorf("table_info(%s): %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var colName, colType string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &dflt, &pk); err != nil {
			return false, fmt.Errorf("scan table_info(%s): %w", table, err)
		}
		if colName == name {
			return true, nil
		}
	}
	return false, rows.Err()
}

// createDataManagerTables creates the tables backing the Data Manager rule
// engine (comic-server-764): dm_groups mirrors dataman.dat's nested
// <group>/<disabled> folder hierarchy (self-referencing parent_id, same
// shape as comic-server's own smart-list folders), dm_rulesets are the
// named rule containers a group holds (or, with group_id NULL, a
// top-level ruleset - dataman.dat allows both), and dm_rules/dm_actions
// are each ruleset's flat condition/action lists (see
// internal/datamanager.Rule/Action for the engine that evaluates them -
// configdb intentionally doesn't import that package, matching how
// KomgaTarget mirrors config.KomgaTarget without importing internal/config).
//
// sort_order on every table preserves dataman.dat's real on-disk order:
// required for dm_groups/dm_rulesets/dm_actions, since Data Manager's
// real evaluation order is depth-first groups-before-rulesets (not file
// order) and a later action can overwrite an earlier one's write to the
// same field - losing that order on import would silently change
// behavior. dm_rules' order doesn't affect its own AND/OR result, but is
// preserved anyway for faithful round-tripping and display.
//
// disabled (on dm_groups and dm_rulesets) mirrors dataman.dat's real
// top-level <disabled> container, confirmed present in the user's actual
// file - holds an entire disabled group/ruleset subtree, not just a
// single flag on one ruleset. comment mirrors the (rare but real)
// comment="..." attribute ComicRack allows on <group>/<ruleset>.
func (db *DB) createDataManagerTables() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS dm_groups (
			id         TEXT PRIMARY KEY,
			parent_id  TEXT REFERENCES dm_groups(id) ON DELETE CASCADE,
			name       TEXT NOT NULL,
			comment    TEXT NOT NULL DEFAULT '',
			disabled   INTEGER NOT NULL DEFAULT 0,
			sort_order INTEGER NOT NULL DEFAULT 0,
			source     TEXT NOT NULL DEFAULT 'manual'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_dm_groups_parent ON dm_groups(parent_id)`,
		`CREATE TABLE IF NOT EXISTS dm_rulesets (
			id         TEXT PRIMARY KEY,
			group_id   TEXT REFERENCES dm_groups(id) ON DELETE CASCADE,
			name       TEXT NOT NULL,
			comment    TEXT NOT NULL DEFAULT '',
			mode       TEXT NOT NULL DEFAULT 'And',
			disabled   INTEGER NOT NULL DEFAULT 0,
			sort_order INTEGER NOT NULL DEFAULT 0,
			source     TEXT NOT NULL DEFAULT 'manual'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_dm_rulesets_group ON dm_rulesets(group_id)`,
		`CREATE TABLE IF NOT EXISTS dm_rules (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			ruleset_id TEXT NOT NULL REFERENCES dm_rulesets(id) ON DELETE CASCADE,
			field      TEXT NOT NULL,
			modifier   TEXT NOT NULL,
			value      TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_dm_rules_ruleset ON dm_rules(ruleset_id)`,
		`CREATE TABLE IF NOT EXISTS dm_actions (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			ruleset_id TEXT NOT NULL REFERENCES dm_rulesets(id) ON DELETE CASCADE,
			field      TEXT NOT NULL,
			modifier   TEXT NOT NULL,
			value      TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_dm_actions_ruleset ON dm_actions(ruleset_id)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("create data manager tables: %w", err)
		}
	}
	return nil
}

// createLibraryOrganizerTables creates the tables backing the Library
// Organizer file-mover port (comic-server-3bz): lo_profiles holds each
// profile's scalar settings (BaseFolder, FolderTemplate/FileTemplate,
// Mode, etc - see internal/libraryorganizer.Profile for the engine that
// consumes them; configdb intentionally doesn't import that package,
// same separation KomgaTarget established from internal/config).
//
// The real losettingsx.dat has SEVEN distinct Item-list collections per
// profile (IllegalCharacters, Months, Prefix, Postfix, Seperator,
// TextBox, EmptyData) plus ExcludedEmptyFolder/ExcludeFolders/
// FailedFields - all structurally identical "Name -> Value" pairs. Rather
// than one table per collection, lo_profile_items is one generic
// key-value side table with a `category` column distinguishing which
// collection a row belongs to - the same "one flexible table instead of
// seven near-identical ones" call already made for book_custom_values
// (internal/storage) elsewhere in this codebase.
//
// lo_exclude_rules is its own table (not folded into lo_profile_items)
// since it has real structure of its own (Field/Operator/Value, not just
// Name/Value) - see comic-server-3bz.3.
func (db *DB) createLibraryOrganizerTables() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS lo_profiles (
			id                      TEXT PRIMARY KEY,
			name                    TEXT NOT NULL,
			base_folder             TEXT NOT NULL DEFAULT '',
			folder_template         TEXT NOT NULL DEFAULT '',
			file_template           TEXT NOT NULL DEFAULT '',
			empty_folder            TEXT NOT NULL DEFAULT '',
			mode                    TEXT NOT NULL DEFAULT 'Move',
			copy_mode               INTEGER NOT NULL DEFAULT 0,
			use_folder              INTEGER NOT NULL DEFAULT 1,
			use_filename            INTEGER NOT NULL DEFAULT 1,
			replace_multiple_spaces INTEGER NOT NULL DEFAULT 1,
			auto_space_fields       INTEGER NOT NULL DEFAULT 1,
			remove_empty_folder     INTEGER NOT NULL DEFAULT 1,
			move_fileless           INTEGER NOT NULL DEFAULT 0,
			fileless_format         TEXT NOT NULL DEFAULT '',
			fail_empty_values       INTEGER NOT NULL DEFAULT 0,
			move_failed             INTEGER NOT NULL DEFAULT 0,
			failed_folder           TEXT NOT NULL DEFAULT '',
			exclude_mode            TEXT NOT NULL DEFAULT 'Do not',
			exclude_operator        TEXT NOT NULL DEFAULT 'Any',
			sort_order              INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS lo_profile_items (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			profile_id TEXT NOT NULL REFERENCES lo_profiles(id) ON DELETE CASCADE,
			category   TEXT NOT NULL,
			name       TEXT NOT NULL,
			value      TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_lo_profile_items_profile ON lo_profile_items(profile_id, category)`,
		`CREATE TABLE IF NOT EXISTS lo_exclude_rules (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			profile_id TEXT NOT NULL REFERENCES lo_profiles(id) ON DELETE CASCADE,
			field      TEXT NOT NULL,
			operator   TEXT NOT NULL,
			value      TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_lo_exclude_rules_profile ON lo_exclude_rules(profile_id)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("create library organizer tables: %w", err)
		}
	}
	return nil
}

// createScanInfoTable creates the scan_info table - a single-row store for
// Server.ScanInfo (Enabled/Scanners/Blacklist/Prefix/Unknown), the first
// UI/API surface for what was previously config.yaml-hand-edit-only
// (comic-server-4ms). A single row (id fixed at 1) rather than one table
// per list field: Scanners/Blacklist are both short, together-configured
// string lists with no per-entry metadata, so - per comic-server-745's own
// design note - storing them as JSON columns alongside the two scalar
// fields (prefix/unknown) they're configured together with is simpler
// than two extra many-row tables, and callers always want the whole
// struct at once (there's no per-entry lookup use case the way
// device_lists' per-device queries have).
// createUISettingsTable creates the ui_settings table - a single-row
// store for web UI preferences that should persist server-side and apply
// to every window by default. First (and currently only) field: the
// default theme (light/dark/system) - see comic-server-8qk. A per-window
// override lives in that window's own sessionStorage instead (see
// theme.js), never here, so a brand-new window always starts from this
// stored default.
func (db *DB) createUISettingsTable() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS ui_settings (
		id    INTEGER PRIMARY KEY CHECK (id = 1),
		theme TEXT NOT NULL DEFAULT 'system'
	)`)
	if err != nil {
		return fmt.Errorf("create ui_settings table: %w", err)
	}
	return nil
}

func (db *DB) createScanInfoTable() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS scan_info (
		id        INTEGER PRIMARY KEY CHECK (id = 1),
		enabled   INTEGER NOT NULL DEFAULT 0,
		scanners  TEXT NOT NULL DEFAULT '[]',
		blacklist TEXT NOT NULL DEFAULT '[]',
		prefix    TEXT NOT NULL DEFAULT '',
		unknown   TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		return fmt.Errorf("create scan_info table: %w", err)
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
