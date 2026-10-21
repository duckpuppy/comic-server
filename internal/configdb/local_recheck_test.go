package configdb

import (
	"path/filepath"
	"testing"
)

func TestServerMiscSettings_LocalIdentityRecheckDefaultsOffAndRoundTrips(t *testing.T) {
	db := newTestDB(t)
	if err := db.UpsertServerMiscSettings(ServerMiscSettings{CountCoveredAsOwned: true}); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetServerMiscSettings()
	if err != nil || got == nil || got.LocalIdentityRecheck {
		t.Fatalf("default = %+v, %v; want off", got, err)
	}
	if err := db.UpsertServerMiscSettings(ServerMiscSettings{LocalIdentityRecheck: true}); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetServerMiscSettings()
	if !got.LocalIdentityRecheck || got.CountCoveredAsOwned {
		t.Errorf("got %+v", got)
	}
}

// A v14 database (no local_identity_recheck column) gains it on open and
// keeps its stored settings (tracker #47).
func TestMigrateV14ToV15_AddsLocalIdentityRecheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertServerMiscSettings(ServerMiscSettings{AutoSync: true, CountCoveredAsOwned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE server_misc_settings DROP COLUMN local_identity_recheck`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 14`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db.Close()
	got, err := db.GetServerMiscSettings()
	if err != nil || got == nil {
		t.Fatalf("get: %+v %v", got, err)
	}
	if !got.AutoSync || !got.CountCoveredAsOwned || got.LocalIdentityRecheck {
		t.Errorf("got %+v, want old values kept and recheck off", got)
	}
	var v int
	_ = db.QueryRow("PRAGMA user_version").Scan(&v)
	if v != schemaVersion {
		t.Errorf("user_version = %d, want %d", v, schemaVersion)
	}
}

func TestServerMiscSettings_ExportPathRoundTrips(t *testing.T) {
	db := newTestDB(t)
	if err := db.UpsertServerMiscSettings(ServerMiscSettings{AutoSync: true}); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.GetServerMiscSettings(); got == nil || got.ExportPath != "" {
		t.Fatalf("default = %+v, want empty export path", got)
	}
	if err := db.UpsertServerMiscSettings(ServerMiscSettings{ExportPath: "/data/exports"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.GetServerMiscSettings(); got.ExportPath != "/data/exports" {
		t.Errorf("got %+v", got)
	}
}

// A v15 database (no export_path column) gains it on open and keeps its
// stored settings (tracker #48).
func TestMigrateV15ToV16_AddsExportPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertServerMiscSettings(ServerMiscSettings{AutoSync: true, LocalIdentityRecheck: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE server_misc_settings DROP COLUMN export_path`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 15`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db.Close()
	got, err := db.GetServerMiscSettings()
	if err != nil || got == nil || !got.AutoSync || !got.LocalIdentityRecheck || got.ExportPath != "" {
		t.Fatalf("got %+v, %v; want old values kept and no export path", got, err)
	}
}
