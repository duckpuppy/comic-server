package configdb

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestMigrateV1ToV2_AddsDeviceTables simulates a database created under
// schemaVersion 1 (comic-server-ihb's foundation-only release, which has
// no tables at all - this is mediaserver's actual current on-disk state)
// and confirms opening it upgrades in place: the devices/device_lists
// tables appear, and the schema version advances so the migration doesn't
// re-run.
func TestMigrateV1ToV2_AddsDeviceTables(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// Build a v1-shaped database by hand: opened, schema initialized, but
	// pinned back to user_version 1 (no tables) rather than left at
	// whatever Open would create fresh today.
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	if _, err := raw.Exec("PRAGMA user_version = 1"); err != nil {
		t.Fatalf("set v1: %v", err)
	}
	raw.Close()

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("query user_version: %v", err)
	}
	if version != schemaVersion {
		t.Errorf("expected schema version %d after migration, got %d", schemaVersion, version)
	}

	for _, table := range []string{"devices", "device_lists"} {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("expected table %q to exist after migration: %v", table, err)
		}
	}
}
