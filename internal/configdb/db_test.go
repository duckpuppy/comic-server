package configdb

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpen_CreatesFileAndDirectory(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nested", "dir", "config.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	if _, err := os.Stat(filepath.Dir(dbPath)); err != nil {
		t.Errorf("expected parent directory to exist: %v", err)
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("expected db file to exist: %v", err)
	}
	if db.Path() != dbPath {
		t.Errorf("expected Path() to return %q, got %q", dbPath, db.Path())
	}
}

func TestOpen_IsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "config.db")

	db1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("first Open failed: %v", err)
	}
	db1.Close()

	// Simulates a second server start against the same config dir.
	db2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second Open failed: %v", err)
	}
	defer db2.Close()

	var version int
	if err := db2.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("query user_version: %v", err)
	}
	if version != schemaVersion {
		t.Errorf("expected schema version %d after reopen, got %d", schemaVersion, version)
	}
}

func TestOpen_SchemaVersionIsSet(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "config.db")

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
		t.Errorf("expected schema version %d, got %d", schemaVersion, version)
	}
}
