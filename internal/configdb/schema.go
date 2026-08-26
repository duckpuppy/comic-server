package configdb

import "fmt"

// Schema version for migrations. Version 1 has no tables yet - this
// package is just the open/migrate foundation (comic-server-ihb).
// comic-server-3ek/comic-server-cde bump this and add real CREATE TABLE
// statements plus a migrateV1ToV2-shaped step, following the same pattern
// internal/storage/schema.go already uses for its own version bumps.
const schemaVersion = 1

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
	}
	// No incremental migrateVxToVy steps exist yet - schemaVersion is
	// still 1. Future bumps add "if version < N { migrateVN-1ToVN() }"
	// steps here, matching internal/storage/schema.go.

	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}

	return nil
}

// createTables creates the schema for a fresh database. Nothing to create
// yet at schemaVersion 1 - see comic-server-3ek/comic-server-cde for the
// first real tables (devices, device_lists, komga_targets).
func (db *DB) createTables() error {
	return nil
}
