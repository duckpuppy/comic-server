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

	for _, table := range []string{"devices", "device_lists", "komga_targets", "sync_history", "scan_info"} {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("expected table %q to exist after migration: %v", table, err)
		}
	}
}

// TestMigrateV2ToV3_AddsKomgaTargetsTable simulates a database created
// under schemaVersion 2 (comic-server-3ek, devices/device_lists only - no
// komga_targets table yet) and confirms opening it upgrades in place.
func TestMigrateV2ToV3_AddsKomgaTargetsTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	if _, err := raw.Exec("PRAGMA user_version = 2"); err != nil {
		t.Fatalf("set v2: %v", err)
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

	var name string
	if err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='komga_targets'").Scan(&name); err != nil {
		t.Errorf("expected table komga_targets to exist after migration: %v", err)
	}
}

// TestMigrateV3ToV4_AddsSyncHistoryTable simulates a database created under
// schemaVersion 3 (comic-server-cde, devices/device_lists/komga_targets
// only - no sync_history table yet) and confirms opening it upgrades in
// place.
func TestMigrateV3ToV4_AddsSyncHistoryTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	if _, err := raw.Exec("PRAGMA user_version = 3"); err != nil {
		t.Fatalf("set v3: %v", err)
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

	var name string
	if err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='sync_history'").Scan(&name); err != nil {
		t.Errorf("expected table sync_history to exist after migration: %v", err)
	}
}

// TestMigrateV4ToV5_AddsScanInfoTable simulates a database created under
// schemaVersion 4 (comic-server-7vu, devices/device_lists/komga_targets/
// sync_history only - no scan_info table yet) and confirms opening it
// upgrades in place.
func TestMigrateV4ToV5_AddsScanInfoTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	if _, err := raw.Exec("PRAGMA user_version = 4"); err != nil {
		t.Fatalf("set v4: %v", err)
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

	var name string
	if err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='scan_info'").Scan(&name); err != nil {
		t.Errorf("expected table scan_info to exist after migration: %v", err)
	}
}

func TestMigrateV5ToV6_AddsDataManagerTables(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	if _, err := raw.Exec("PRAGMA user_version = 5"); err != nil {
		t.Fatalf("set v5: %v", err)
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

	for _, table := range []string{"dm_groups", "dm_rulesets", "dm_rules", "dm_actions"} {
		var name string
		if err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name); err != nil {
			t.Errorf("expected table %s to exist after migration: %v", table, err)
		}
	}
}
