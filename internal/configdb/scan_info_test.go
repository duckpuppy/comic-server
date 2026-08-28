package configdb

import (
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
)

func TestScanInfo_GetReturnsNilWhenUnset(t *testing.T) {
	db := newTestDB(t)

	got, err := db.GetScanInfo()
	if err != nil {
		t.Fatalf("GetScanInfo failed: %v", err)
	}
	if got != nil {
		t.Errorf("GetScanInfo on empty db = %+v, want nil", got)
	}
}

func TestScanInfo_UpsertAndGet(t *testing.T) {
	db := newTestDB(t)

	cfg := config.ScanInfoConfig{
		Enabled:   true,
		Scanners:  []string{"DCP", "TLK-Empire"},
		Blacklist: []string{"digital", "\\d{4}"},
		Prefix:    "Scanner:",
		Unknown:   "Unknown",
	}
	if err := db.UpsertScanInfo(cfg); err != nil {
		t.Fatalf("UpsertScanInfo failed: %v", err)
	}

	got, err := db.GetScanInfo()
	if err != nil {
		t.Fatalf("GetScanInfo failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetScanInfo returned nil after Upsert")
	}
	if got.Enabled != cfg.Enabled || got.Prefix != cfg.Prefix || got.Unknown != cfg.Unknown {
		t.Errorf("scalar fields mismatch: got %+v, want %+v", got, cfg)
	}
	if len(got.Scanners) != 2 || got.Scanners[0] != "DCP" || got.Scanners[1] != "TLK-Empire" {
		t.Errorf("Scanners = %v, want %v", got.Scanners, cfg.Scanners)
	}
	if len(got.Blacklist) != 2 || got.Blacklist[0] != "digital" || got.Blacklist[1] != "\\d{4}" {
		t.Errorf("Blacklist = %v, want %v", got.Blacklist, cfg.Blacklist)
	}
}

func TestScanInfo_UpsertReplacesWholesale(t *testing.T) {
	db := newTestDB(t)

	if err := db.UpsertScanInfo(config.ScanInfoConfig{
		Enabled:  true,
		Scanners: []string{"A", "B"},
	}); err != nil {
		t.Fatalf("first UpsertScanInfo failed: %v", err)
	}

	if err := db.UpsertScanInfo(config.ScanInfoConfig{
		Enabled:  false,
		Scanners: []string{"C"},
	}); err != nil {
		t.Fatalf("second UpsertScanInfo failed: %v", err)
	}

	got, err := db.GetScanInfo()
	if err != nil {
		t.Fatalf("GetScanInfo failed: %v", err)
	}
	if got.Enabled {
		t.Error("expected Enabled=false after second upsert")
	}
	if len(got.Scanners) != 1 || got.Scanners[0] != "C" {
		t.Errorf("expected Scanners=[C] after second upsert (full replace, not merge), got %v", got.Scanners)
	}
}

func TestScanInfo_UpsertNilSlicesStoreAsEmpty(t *testing.T) {
	db := newTestDB(t)

	if err := db.UpsertScanInfo(config.ScanInfoConfig{Enabled: false}); err != nil {
		t.Fatalf("UpsertScanInfo failed: %v", err)
	}

	got, err := db.GetScanInfo()
	if err != nil {
		t.Fatalf("GetScanInfo failed: %v", err)
	}
	if got.Scanners == nil || len(got.Scanners) != 0 {
		t.Errorf("expected empty (non-nil) Scanners slice, got %v", got.Scanners)
	}
	if got.Blacklist == nil || len(got.Blacklist) != 0 {
		t.Errorf("expected empty (non-nil) Blacklist slice, got %v", got.Blacklist)
	}
}
