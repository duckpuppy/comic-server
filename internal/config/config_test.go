package config

import (
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/sync"
)

func TestNewConfig(t *testing.T) {
	cfg := NewConfig()

	if cfg == nil {
		t.Fatal("NewConfig() returned nil")
	}

	if cfg.Devices == nil {
		t.Error("NewConfig() Devices map is nil")
	}

	if len(cfg.Devices) != 0 {
		t.Errorf("NewConfig() Devices map not empty, got %d devices", len(cfg.Devices))
	}
}

func TestConfigAddDevice(t *testing.T) {
	cfg := NewConfig()

	deviceID := "test-device-123"
	friendlyName := "Test Tablet"

	device := cfg.AddDevice(deviceID, friendlyName)

	if device == nil {
		t.Fatal("AddDevice() returned nil")
	}

	if device.DeviceID != deviceID {
		t.Errorf("AddDevice() DeviceID = %v, want %v", device.DeviceID, deviceID)
	}

	if device.FriendlyName != friendlyName {
		t.Errorf("AddDevice() FriendlyName = %v, want %v", device.FriendlyName, friendlyName)
	}

	if len(device.Lists) != 0 {
		t.Errorf("AddDevice() Lists not empty, got %d", len(device.Lists))
	}

	// Verify device is in config
	if cfg.Devices[deviceID] != device {
		t.Error("AddDevice() did not add device to config map")
	}
}

func TestConfigUpdateDevice(t *testing.T) {
	cfg := NewConfig()
	deviceID := "test-device-123"

	// Add initial device
	cfg.AddDevice(deviceID, "Old Name")

	// Update with new name
	newName := "New Name"
	cfg.UpdateDevice(deviceID, newName)

	device := cfg.Devices[deviceID]
	if device.FriendlyName != newName {
		t.Errorf("UpdateDevice() FriendlyName = %v, want %v", device.FriendlyName, newName)
	}

	// Verify LastSeen was updated
	if device.LastSeen.IsZero() {
		t.Error("UpdateDevice() did not update LastSeen")
	}

	// Verify timestamp is recent (within 1 second)
	if time.Since(device.LastSeen) > time.Second {
		t.Error("UpdateDevice() LastSeen timestamp not recent")
	}
}

func TestDeviceConfigAddList(t *testing.T) {
	device := &DeviceConfig{
		DeviceID: "test-device",
		Lists:    []SharedListConfig{},
	}

	listID := "list-guid-123"
	listName := "My Smart List"
	settings := sync.DefaultSettings()

	err := device.AddList(listID, listName, settings)
	if err != nil {
		t.Fatalf("AddList() error = %v", err)
	}

	if len(device.Lists) != 1 {
		t.Fatalf("AddList() list count = %d, want 1", len(device.Lists))
	}

	list := device.Lists[0]
	if list.ListID != listID {
		t.Errorf("AddList() ListID = %v, want %v", list.ListID, listID)
	}

	if list.ListName != listName {
		t.Errorf("AddList() ListName = %v, want %v", list.ListName, listName)
	}

	if !list.Enabled {
		t.Error("AddList() list not enabled by default")
	}

	if list.Settings != settings {
		t.Error("AddList() settings not set correctly")
	}
}

func TestDeviceConfigAddListDuplicate(t *testing.T) {
	device := &DeviceConfig{
		DeviceID: "test-device",
		Lists: []SharedListConfig{
			{ListID: "existing-list", ListName: "Existing", Enabled: true},
		},
	}

	err := device.AddList("existing-list", "Duplicate", nil)
	if err == nil {
		t.Error("AddList() should return error for duplicate list")
	}
}

func TestDeviceConfigRemoveList(t *testing.T) {
	device := &DeviceConfig{
		DeviceID: "test-device",
		Lists: []SharedListConfig{
			{ListID: "list-1", ListName: "List 1", Enabled: true},
			{ListID: "list-2", ListName: "List 2", Enabled: true},
			{ListID: "list-3", ListName: "List 3", Enabled: true},
		},
	}

	// Remove middle list
	err := device.RemoveList("list-2")
	if err != nil {
		t.Fatalf("RemoveList() error = %v", err)
	}

	if len(device.Lists) != 2 {
		t.Fatalf("RemoveList() list count = %d, want 2", len(device.Lists))
	}

	// Verify correct list was removed
	for _, list := range device.Lists {
		if list.ListID == "list-2" {
			t.Error("RemoveList() did not remove the list")
		}
	}

	// Verify order preserved
	if device.Lists[0].ListID != "list-1" || device.Lists[1].ListID != "list-3" {
		t.Error("RemoveList() changed list order")
	}
}

func TestDeviceConfigRemoveListNotFound(t *testing.T) {
	device := &DeviceConfig{
		DeviceID: "test-device",
		Lists:    []SharedListConfig{},
	}

	err := device.RemoveList("nonexistent")
	if err == nil {
		t.Error("RemoveList() should return error for nonexistent list")
	}
}

func TestDeviceConfigGetList(t *testing.T) {
	settings := sync.DefaultSettings()
	device := &DeviceConfig{
		DeviceID: "test-device",
		Lists: []SharedListConfig{
			{ListID: "list-1", ListName: "List 1", Settings: settings},
		},
	}

	list := device.GetList("list-1")
	if list == nil {
		t.Fatal("GetList() returned nil for existing list")
	}

	if list.ListID != "list-1" {
		t.Error("GetList() returned wrong list")
	}

	// Test nonexistent list
	if device.GetList("nonexistent") != nil {
		t.Error("GetList() should return nil for nonexistent list")
	}
}

func TestDeviceConfigUpdateListSettings(t *testing.T) {
	oldSettings := sync.DefaultSettings()
	device := &DeviceConfig{
		DeviceID: "test-device",
		Lists: []SharedListConfig{
			{ListID: "list-1", ListName: "List 1", Settings: oldSettings},
		},
	}

	newSettings := &sync.SharedListSettings{
		OnlyUnread: true,
		Limit:      true,
		LimitValue: 100,
	}

	err := device.UpdateListSettings("list-1", newSettings)
	if err != nil {
		t.Fatalf("UpdateListSettings() error = %v", err)
	}

	list := device.GetList("list-1")
	if list.Settings != newSettings {
		t.Error("UpdateListSettings() did not update settings")
	}

	if !list.Settings.OnlyUnread {
		t.Error("UpdateListSettings() settings not applied correctly")
	}
}

func TestDeviceConfigEnableDisableList(t *testing.T) {
	device := &DeviceConfig{
		DeviceID: "test-device",
		Lists: []SharedListConfig{
			{ListID: "list-1", ListName: "List 1", Enabled: true},
		},
	}

	// Test disable
	err := device.DisableList("list-1")
	if err != nil {
		t.Fatalf("DisableList() error = %v", err)
	}

	list := device.GetList("list-1")
	if list.Enabled {
		t.Error("DisableList() did not disable list")
	}

	// Test enable
	err = device.EnableList("list-1")
	if err != nil {
		t.Fatalf("EnableList() error = %v", err)
	}

	list = device.GetList("list-1")
	if !list.Enabled {
		t.Error("EnableList() did not enable list")
	}
}
