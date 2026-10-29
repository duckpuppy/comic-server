package configdb

import (
	"path/filepath"
	"testing"
	"time"
)

func TestValidateDeviceAddress(t *testing.T) {
	good := map[string]string{
		"":                        "",
		"  ":                      "",
		"192.168.0.171":           "192.168.0.171",
		"100.85.206.109":          "100.85.206.109",
		" tablet ":                "tablet",
		"sm-t970.tail1234.ts.net": "sm-t970.tail1234.ts.net",
		"fd7a:115c:a1e0::1":       "fd7a:115c:a1e0::1",
	}
	for in, want := range good {
		got, err := ValidateDeviceAddress(in)
		if err != nil || got != want {
			t.Errorf("ValidateDeviceAddress(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"host:7614", "-bad.example", "bad_name", "a b", "http://x", "x/y", ".x"} {
		if _, err := ValidateDeviceAddress(in); err == nil {
			t.Errorf("ValidateDeviceAddress(%q) accepted", in)
		}
	}
}

func TestDeviceAddress_SavedAndSurvivesUpsert(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if err := db.UpsertDevice("d1", "Tab", time.Time{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.SetDeviceAddress("d1", "100.85.206.109"); err != nil {
		t.Fatal(err)
	}
	// A later upsert (last_seen update from discovery) must not wipe it.
	if err := db.UpsertDevice("d1", "Tab 2", time.Now(), nil); err != nil {
		t.Fatal(err)
	}

	d, err := db.GetDevice("d1")
	if err != nil || d == nil {
		t.Fatalf("GetDevice: %v, %v", d, err)
	}
	if d.Address != "100.85.206.109" {
		t.Errorf("GetDevice Address = %q", d.Address)
	}
	all, err := db.ListDevices()
	if err != nil || len(all) != 1 || all[0].Address != "100.85.206.109" {
		t.Errorf("ListDevices = %+v, %v", all, err)
	}

	if err := db.SetDeviceAddress("d1", ""); err != nil {
		t.Fatal(err)
	}
	if d, _ = db.GetDevice("d1"); d.Address != "" {
		t.Errorf("cleared Address = %q", d.Address)
	}
}

func TestSetDeviceAddress_UnknownDevice(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.SetDeviceAddress("nope", "1.2.3.4"); err == nil {
		t.Error("expected an error for an unregistered device")
	}
}

func TestMigrateV16ToV17_AddsAddressColumn(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	// Rebuild the old shape: devices without the address column.
	for _, stmt := range []string{
		`PRAGMA foreign_keys = OFF`,
		`ALTER TABLE devices DROP COLUMN address`,
		`PRAGMA foreign_keys = ON`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	if err := db.migrateV16ToV17(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.migrateV16ToV17(); err != nil {
		t.Fatalf("migrate is not repeatable: %v", err)
	}
	if ok, err := db.hasColumn("devices", "address"); err != nil || !ok {
		t.Errorf("address column missing: %v, %v", ok, err)
	}
}
