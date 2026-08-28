package cmd

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/device"
	"github.com/duckpuppy/comic-server/internal/library"
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
