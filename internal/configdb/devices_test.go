package configdb

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/sync"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestUpsertDevice_CreatesAndUpdates(t *testing.T) {
	db := newTestDB(t)

	now := time.Now().Truncate(time.Second)
	if err := db.UpsertDevice("device-1", "My Tablet", now, nil); err != nil {
		t.Fatalf("UpsertDevice (create) failed: %v", err)
	}

	d, err := db.GetDevice("device-1")
	if err != nil {
		t.Fatalf("GetDevice failed: %v", err)
	}
	if d == nil {
		t.Fatal("expected device to exist")
	}
	if d.FriendlyName != "My Tablet" || !d.LastSeen.Equal(now) || d.DefaultSettings != nil {
		t.Errorf("unexpected device: %+v", d)
	}

	settings := &sync.SharedListSettings{OnlyUnread: true, Limit: true, LimitValue: 25}
	if err := db.UpsertDevice("device-1", "Renamed Tablet", now, settings); err != nil {
		t.Fatalf("UpsertDevice (update) failed: %v", err)
	}
	d, err = db.GetDevice("device-1")
	if err != nil {
		t.Fatalf("GetDevice failed: %v", err)
	}
	if d.FriendlyName != "Renamed Tablet" {
		t.Errorf("expected updated friendly name, got %q", d.FriendlyName)
	}
	if d.DefaultSettings == nil || !d.DefaultSettings.OnlyUnread || d.DefaultSettings.LimitValue != 25 {
		t.Errorf("expected default settings to round-trip, got %+v", d.DefaultSettings)
	}
}

func TestGetDevice_NotFoundReturnsNil(t *testing.T) {
	db := newTestDB(t)

	d, err := db.GetDevice("does-not-exist")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if d != nil {
		t.Errorf("expected nil for unknown device, got %+v", d)
	}
}

func TestDeleteDevice_CascadesToLists(t *testing.T) {
	db := newTestDB(t)

	if err := db.UpsertDevice("device-1", "Tablet", time.Now(), nil); err != nil {
		t.Fatalf("UpsertDevice failed: %v", err)
	}
	if err := db.AddDeviceList("device-1", DeviceList{ListID: "list-1", ListName: "All Comics", Enabled: true}); err != nil {
		t.Fatalf("AddDeviceList failed: %v", err)
	}

	if err := db.DeleteDevice("device-1"); err != nil {
		t.Fatalf("DeleteDevice failed: %v", err)
	}

	d, err := db.GetDevice("device-1")
	if err != nil {
		t.Fatalf("GetDevice failed: %v", err)
	}
	if d != nil {
		t.Errorf("expected device to be gone, got %+v", d)
	}

	lists, err := db.ListDeviceLists("device-1")
	if err != nil {
		t.Fatalf("ListDeviceLists failed: %v", err)
	}
	if len(lists) != 0 {
		t.Errorf("expected cascade-deleted lists, got %+v", lists)
	}
}

func TestAddDeviceList_RejectsDuplicate(t *testing.T) {
	db := newTestDB(t)
	if err := db.UpsertDevice("device-1", "Tablet", time.Now(), nil); err != nil {
		t.Fatalf("UpsertDevice failed: %v", err)
	}

	dl := DeviceList{ListID: "list-1", ListName: "All Comics", Enabled: true}
	if err := db.AddDeviceList("device-1", dl); err != nil {
		t.Fatalf("first AddDeviceList failed: %v", err)
	}
	if err := db.AddDeviceList("device-1", dl); err == nil {
		t.Error("expected error adding a duplicate device/list pair")
	}
}

func TestRemoveDeviceList(t *testing.T) {
	db := newTestDB(t)
	if err := db.UpsertDevice("device-1", "Tablet", time.Now(), nil); err != nil {
		t.Fatalf("UpsertDevice failed: %v", err)
	}
	if err := db.AddDeviceList("device-1", DeviceList{ListID: "list-1", ListName: "All Comics", Enabled: true}); err != nil {
		t.Fatalf("AddDeviceList failed: %v", err)
	}

	if err := db.RemoveDeviceList("device-1", "list-1"); err != nil {
		t.Fatalf("RemoveDeviceList failed: %v", err)
	}

	lists, err := db.ListDeviceLists("device-1")
	if err != nil {
		t.Fatalf("ListDeviceLists failed: %v", err)
	}
	if len(lists) != 0 {
		t.Errorf("expected list removed, got %+v", lists)
	}

	// Removing again should not error (not-found is not an error here).
	if err := db.RemoveDeviceList("device-1", "list-1"); err != nil {
		t.Errorf("expected no error removing an already-removed list, got %v", err)
	}
}

func TestUpdateDeviceList_PartialUpdates(t *testing.T) {
	db := newTestDB(t)
	if err := db.UpsertDevice("device-1", "Tablet", time.Now(), nil); err != nil {
		t.Fatalf("UpsertDevice failed: %v", err)
	}
	if err := db.AddDeviceList("device-1", DeviceList{ListID: "list-1", ListName: "All Comics", Enabled: true}); err != nil {
		t.Fatalf("AddDeviceList failed: %v", err)
	}

	disabled := false
	if err := db.UpdateDeviceList("device-1", "list-1", &disabled, nil); err != nil {
		t.Fatalf("UpdateDeviceList (enabled only) failed: %v", err)
	}
	lists, err := db.ListDeviceLists("device-1")
	if err != nil {
		t.Fatalf("ListDeviceLists failed: %v", err)
	}
	if len(lists) != 1 || lists[0].Enabled {
		t.Fatalf("expected list disabled, got %+v", lists)
	}
	if lists[0].Settings != nil {
		t.Errorf("expected settings to remain nil (not passed in this update), got %+v", lists[0].Settings)
	}

	settings := &sync.SharedListSettings{Sort: true, ListSortType: sync.SortTypeAdded}
	if err := db.UpdateDeviceList("device-1", "list-1", nil, settings); err != nil {
		t.Fatalf("UpdateDeviceList (settings only) failed: %v", err)
	}
	lists, err = db.ListDeviceLists("device-1")
	if err != nil {
		t.Fatalf("ListDeviceLists failed: %v", err)
	}
	if lists[0].Enabled {
		t.Errorf("expected enabled to remain false (not passed in this update), got %+v", lists[0])
	}
	if lists[0].Settings == nil || lists[0].Settings.ListSortType != sync.SortTypeAdded {
		t.Errorf("expected settings to round-trip, got %+v", lists[0].Settings)
	}
}

func TestListDevices_IncludesListsPerDevice(t *testing.T) {
	db := newTestDB(t)
	if err := db.UpsertDevice("device-1", "Tablet 1", time.Now(), nil); err != nil {
		t.Fatalf("UpsertDevice failed: %v", err)
	}
	if err := db.UpsertDevice("device-2", "Tablet 2", time.Now(), nil); err != nil {
		t.Fatalf("UpsertDevice failed: %v", err)
	}
	if err := db.AddDeviceList("device-1", DeviceList{ListID: "list-1", ListName: "All Comics", Enabled: true}); err != nil {
		t.Fatalf("AddDeviceList failed: %v", err)
	}

	devices, err := db.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices failed: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}

	byID := make(map[string]Device)
	for _, d := range devices {
		byID[d.DeviceID] = d
	}
	if len(byID["device-1"].Lists) != 1 {
		t.Errorf("expected device-1 to have 1 list, got %+v", byID["device-1"].Lists)
	}
	if len(byID["device-2"].Lists) != 0 {
		t.Errorf("expected device-2 to have no lists, got %+v", byID["device-2"].Lists)
	}
}
