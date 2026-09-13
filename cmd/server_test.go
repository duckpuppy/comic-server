package cmd

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/device"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/storage"
	csync "github.com/duckpuppy/comic-server/internal/sync"
	"github.com/duckpuppy/comic-server/internal/syncstate"
)

// TestApplyDeviceConfig_AcceptsIdList is the regression test for the most
// severe symptom of comic-server-vwl's device-sync bug: applyDeviceConfig
// used to hard-reject any assigned list whose Type didn't contain
// "SmartList" ("smart list %s (ID: %s) not found in library"), and that
// error aborts handleSyncRequest entirely - meaning a device with even one
// real ID list assigned (e.g. "To Read") had its WHOLE sync fail, not just
// that one list. Confirmed against ComicRackCE's own source that "devices
// only sync smart lists" isn't a real protocol constraint - any list type
// with real book membership should be accepted; only folders should still
// be rejected.
func TestApplyDeviceConfig_AcceptsIdList(t *testing.T) {
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "book1", Title: "Book 1"},
		},
		ComicLists: []library.ComicListItem{
			{ID: "idlist-1", Name: "To Read", Type: "ComicIdListItem", BookIds: []string{"book1"}},
		},
	}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := csync.NewSyncer(nil, backend)

	deviceConfig := &configdb.Device{
		DeviceID: "device-1",
		Lists: []configdb.DeviceList{
			{ListID: "idlist-1", ListName: "To Read", Enabled: true},
		},
	}

	if err := applyDeviceConfig(syncer, deviceConfig, backend); err != nil {
		t.Fatalf("expected an ID list to be accepted, got error: %v", err)
	}
}

// TestApplyDeviceConfig_RejectsFolder confirms a folder (which groups
// other lists rather than containing books itself) is still rejected,
// rather than the fix above having simply removed all validation.
func TestApplyDeviceConfig_RejectsFolder(t *testing.T) {
	lib := &library.ComicLibrary{
		ComicLists: []library.ComicListItem{
			{ID: "folder-1", Name: "A Folder", Type: "ComicListItemFolder"},
		},
	}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := csync.NewSyncer(nil, backend)

	deviceConfig := &configdb.Device{
		DeviceID: "device-1",
		Lists: []configdb.DeviceList{
			{ListID: "folder-1", ListName: "A Folder", Enabled: true},
		},
	}

	if err := applyDeviceConfig(syncer, deviceConfig, backend); err == nil {
		t.Error("expected an error assigning a folder, got nil")
	}
}

// TestNotReadyDeviceLists_BlocksOnColdIdListButNotWarmSmartList covers
// comic-server-jrn's device-scoped readiness gate: a device with both an ID
// list (always needs the shared snapshot) and a smart list whose matchers
// translate to a scoped SQL query (never needs it) assigned should only be
// blocked by the ID list while the backend is cold, and by neither once
// warm - regardless of the smart list ever needing it at all. Round-trips
// through a real Reload() rather than hand-building structs, per the
// comic-server-hha lesson that in-memory construction can hide bugs the
// real import path would catch.
func TestNotReadyDeviceLists_BlocksOnColdIdListButNotWarmSmartList(t *testing.T) {
	dir := t.TempDir()
	xmlPath := filepath.Join(dir, "ComicDb.xml")
	dbPath := filepath.Join(dir, "test.db")

	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "book1", Title: "Book 1", Series: "Batman", Year: 2020},
		},
		ComicLists: []library.ComicListItem{
			{ID: "idlist-1", Name: "To Read", Type: "ComicIdListItem", BookIds: []string{"book1"}},
			{
				ID: "smartlist-1", Name: "Batman", Type: "ComicSmartListItem", MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{{Type: "Series", MatchOperator: "0", MatchValue: "Batman"}},
			},
		},
	}
	if err := library.SaveLibrary(xmlPath, lib); err != nil {
		t.Fatalf("SaveLibrary: %v", err)
	}
	backend, err := storage.NewSQLiteBackend(dbPath, xmlPath)
	if err != nil {
		t.Fatalf("NewSQLiteBackend: %v", err)
	}
	t.Cleanup(func() { backend.Close() })
	if err := backend.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	deviceConfig := &configdb.Device{
		DeviceID: "device-1",
		Lists: []configdb.DeviceList{
			{ListID: "idlist-1", ListName: "To Read", Enabled: true},
			{ListID: "smartlist-1", ListName: "Batman", Enabled: true},
			{ListID: "smartlist-1", ListName: "Disabled dup", Enabled: false},
		},
	}

	notReady := notReadyDeviceLists(backend, deviceConfig)
	if len(notReady) != 1 || notReady[0] != "idlist-1" {
		t.Fatalf("expected only idlist-1 not-ready while cold, got %v", notReady)
	}

	if err := backend.WarmUp(); err != nil {
		t.Fatalf("WarmUp: %v", err)
	}
	if notReady := notReadyDeviceLists(backend, deviceConfig); len(notReady) != 0 {
		t.Fatalf("expected no not-ready lists once warm, got %v", notReady)
	}
}

// TestValidateCBZConvertAgainstEffectiveTrash_ChecksConfigDBFirst covers
// comic-server-dtu5: the check must consider config.db's trash_settings
// (if the user has ever saved one through the Settings UI) rather than
// only config.yaml's TrashPath - the whole point of moving this check out
// of Config.Validate() in the first place.
func TestValidateCBZConvertAgainstEffectiveTrash_ChecksConfigDBFirst(t *testing.T) {
	db := newTestConfigDB(t)
	cfg := &config.Config{}
	cfg.Server.CBZConvert.Enabled = true
	cfg.Server.TrashPath = "" // nothing in config.yaml

	if err := validateCBZConvertAgainstEffectiveTrash(cfg, db); err == nil {
		t.Error("expected an error with cbz_convert enabled and no trash path anywhere")
	}

	if err := db.UpsertTrashSettings(configdb.TrashSettings{Path: "/data/trash", RetentionDays: 30}); err != nil {
		t.Fatalf("UpsertTrashSettings: %v", err)
	}

	if err := validateCBZConvertAgainstEffectiveTrash(cfg, db); err != nil {
		t.Errorf("expected no error once config.db has a trash path, got: %v", err)
	}
}

func TestValidateCBZConvertAgainstEffectiveTrash_FallsBackToConfigYAML(t *testing.T) {
	db := newTestConfigDB(t)
	cfg := &config.Config{}
	cfg.Server.CBZConvert.Enabled = true
	cfg.Server.TrashPath = "/legacy/trash" // no config.db row yet

	if err := validateCBZConvertAgainstEffectiveTrash(cfg, db); err != nil {
		t.Errorf("expected the config.yaml fallback to satisfy the check, got: %v", err)
	}
}

// TestApplyServerMiscSettings_FirstRunMigratesAndClears covers
// comic-server-wp8k: on a database with no server_misc_settings row yet,
// the current in-memory config.yaml values are copied into config.db and
// then cleared from config.yaml (a clean break, matching trash_settings').
func TestApplyServerMiscSettings_FirstRunMigratesAndClears(t *testing.T) {
	db := newTestConfigDB(t)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &config.Config{}
	cfg.Server.CBZConvert.Enabled = true
	cfg.Server.IgnoreDevices = []string{"192.168.0.24"}
	if err := config.Save(cfg, configPath); err != nil {
		t.Fatalf("seed config.Save: %v", err)
	}

	if err := applyServerMiscSettings(cfg, db, configPath, false, nil); err != nil {
		t.Fatalf("applyServerMiscSettings: %v", err)
	}

	stored, err := db.GetServerMiscSettings()
	if err != nil {
		t.Fatalf("GetServerMiscSettings: %v", err)
	}
	if stored == nil || !stored.CBZConvertEnabled || len(stored.IgnoreDevices) != 1 || stored.IgnoreDevices[0] != "192.168.0.24" {
		t.Fatalf("stored settings = %+v, want migrated values", stored)
	}

	// The in-memory cfg still reflects the effective (now config.db-backed)
	// values, so existing call sites reading cfg.Server.* directly keep working.
	if !cfg.Server.CBZConvert.Enabled || len(cfg.Server.IgnoreDevices) != 1 {
		t.Errorf("in-memory cfg after migration = %+v, want unchanged effective values", cfg.Server)
	}

	reloaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if reloaded.Server.CBZConvert.Enabled || len(reloaded.Server.IgnoreDevices) != 0 {
		t.Errorf("config.yaml after migration = %+v, want cleared", reloaded.Server)
	}
}

// TestApplyServerMiscSettings_SteadyStateUsesConfigDB covers a later run:
// config.db already has a row, cfg (freshly loaded from a config.yaml
// that no longer carries these fields) gets the config.db values applied.
func TestApplyServerMiscSettings_SteadyStateUsesConfigDB(t *testing.T) {
	db := newTestConfigDB(t)
	if err := db.UpsertServerMiscSettings(configdb.ServerMiscSettings{
		CBZConvertEnabled: true,
		IgnoreDevices:     []string{"SM-T970"},
	}); err != nil {
		t.Fatalf("UpsertServerMiscSettings: %v", err)
	}

	cfg := &config.Config{} // simulates a fresh load from the now-cleared config.yaml
	if err := applyServerMiscSettings(cfg, db, filepath.Join(t.TempDir(), "config.yaml"), false, nil); err != nil {
		t.Fatalf("applyServerMiscSettings: %v", err)
	}

	if !cfg.Server.CBZConvert.Enabled {
		t.Error("expected CBZConvert.Enabled to be applied from config.db")
	}
	if len(cfg.Server.IgnoreDevices) != 1 || cfg.Server.IgnoreDevices[0] != "SM-T970" {
		t.Errorf("cfg.Server.IgnoreDevices = %v, want [SM-T970] from config.db", cfg.Server.IgnoreDevices)
	}
}

// TestApplyServerMiscSettings_CLIFlagOverridesAndPersists covers the
// --ignore-device flag: when explicitly passed, it wins over whatever's
// already in config.db AND is written back so it persists forward too.
func TestApplyServerMiscSettings_CLIFlagOverridesAndPersists(t *testing.T) {
	db := newTestConfigDB(t)
	if err := db.UpsertServerMiscSettings(configdb.ServerMiscSettings{
		IgnoreDevices: []string{"old-device"},
	}); err != nil {
		t.Fatalf("UpsertServerMiscSettings: %v", err)
	}

	cfg := &config.Config{}
	if err := applyServerMiscSettings(cfg, db, filepath.Join(t.TempDir(), "config.yaml"), true, []string{"new-device"}); err != nil {
		t.Fatalf("applyServerMiscSettings: %v", err)
	}

	if len(cfg.Server.IgnoreDevices) != 1 || cfg.Server.IgnoreDevices[0] != "new-device" {
		t.Errorf("cfg.Server.IgnoreDevices = %v, want [new-device] (CLI override)", cfg.Server.IgnoreDevices)
	}

	// The override must persist forward into config.db too.
	stored, err := db.GetServerMiscSettings()
	if err != nil {
		t.Fatalf("GetServerMiscSettings: %v", err)
	}
	if len(stored.IgnoreDevices) != 1 || stored.IgnoreDevices[0] != "new-device" {
		t.Errorf("stored.IgnoreDevices = %v, want [new-device] persisted", stored.IgnoreDevices)
	}
}

func newTestConfigDB(t *testing.T) *configdb.DB {
	t.Helper()
	db, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("failed to open test config db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestTriggerManualSync_DeviceNotConnected covers comic-server-yfp's manual
// sync trigger's synchronous pre-check: a device ID with nothing in the
// registry (never discovered, or discovered and then aged out) can't be
// synced - there's no IP to connect to.
func TestTriggerManualSync_DeviceNotConnected(t *testing.T) {
	registry := device.NewRegistry()
	syncManager := syncstate.NewManager(10)
	backend := library.NewXMLBackendFromLibrary(&library.ComicLibrary{}, "", nil)

	err := triggerManualSync("device-1", registry, syncManager, &config.Config{}, backend, newTestConfigDB(t), nil, nil)
	if !errors.Is(err, device.ErrNotConnected) {
		t.Errorf("expected device.ErrNotConnected, got %v", err)
	}
}

// TestTriggerManualSync_AlreadySyncing covers the other synchronous
// pre-check: a device that's mid-sync already shouldn't have a second one
// started concurrently for it (syncstate.Manager.StartSync would reject it
// anyway, but the pre-check here means the HTTP caller finds out
// immediately instead of after handleSyncRequest dials out and fails).
func TestTriggerManualSync_AlreadySyncing(t *testing.T) {
	registry := device.NewRegistry()
	registry.Add(&device.Info{ID: "device-1", Name: "Test Tablet"}, "192.168.1.100")

	syncManager := syncstate.NewManager(10)
	if err := syncManager.StartSync("device-1", "192.168.1.100", "Test Tablet"); err != nil {
		t.Fatalf("failed to seed an in-progress sync: %v", err)
	}

	backend := library.NewXMLBackendFromLibrary(&library.ComicLibrary{}, "", nil)

	err := triggerManualSync("device-1", registry, syncManager, &config.Config{}, backend, newTestConfigDB(t), nil, nil)
	var alreadySyncing *syncstate.DeviceAlreadySyncingError
	if !errors.As(err, &alreadySyncing) {
		t.Errorf("expected *syncstate.DeviceAlreadySyncingError, got %v", err)
	}
}

// TestTriggerManualSync_StartsInBackground confirms a connected, idle
// device passes both pre-checks and returns immediately (nil) without
// waiting for the sync itself to finish - the sync (which will fail fast
// here, since nothing is actually listening on the fake device's IP) runs
// in its own goroutine.
func TestTriggerManualSync_StartsInBackground(t *testing.T) {
	registry := device.NewRegistry()
	registry.Add(&device.Info{ID: "device-1", Name: "Test Tablet"}, "127.0.0.1")

	syncManager := syncstate.NewManager(10)
	backend := library.NewXMLBackendFromLibrary(&library.ComicLibrary{}, "", nil)

	// Deliberately not t.Cleanup(db.Close): triggerManualSync's whole point
	// is that the actual sync keeps running after this function returns,
	// in its own goroutine (it'll fail fast here since nothing's really
	// listening on 127.0.0.1's comic-server port, but it still touches
	// configDB on the way) - closing the DB on test exit would race that
	// goroutine's own use of it and log a spurious "database is closed"
	// error. The temp dir (and its still-open fd) is cleaned up by the OS
	// once the test binary exits.
	configDB, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("failed to open test config db: %v", err)
	}

	if err := triggerManualSync("device-1", registry, syncManager, &config.Config{}, backend, configDB, nil, nil); err != nil {
		t.Errorf("expected nil (sync started in the background), got %v", err)
	}
}
