package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/sync"
)

func TestLoadNonexistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "nonexistent.yaml")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() should not error for nonexistent file, got: %v", err)
	}

	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}

	if len(cfg.Devices) != 0 {
		t.Errorf("Load() should return empty config for nonexistent file, got %d devices", len(cfg.Devices))
	}
}

func TestLoadYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `devices:
  device-123:
    device_id: device-123
    friendly_name: Test Device
    lists:
      - list_id: list-guid-1
        list_name: My List
        enabled: true
        settings:
          only_unread: true
          limit: true
          limit_value: 100
          limit_value_type: books
`

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test YAML: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() YAML error = %v", err)
	}

	if len(cfg.Devices) != 1 {
		t.Fatalf("Load() device count = %d, want 1", len(cfg.Devices))
	}

	device := cfg.Devices["device-123"]
	if device == nil {
		t.Fatal("Load() device not found")
	}

	if device.FriendlyName != "Test Device" {
		t.Errorf("Load() FriendlyName = %v, want Test Device", device.FriendlyName)
	}

	if len(device.Lists) != 1 {
		t.Fatalf("Load() list count = %d, want 1", len(device.Lists))
	}

	list := device.Lists[0]
	if list.ListID != "list-guid-1" {
		t.Errorf("Load() ListID = %v, want list-guid-1", list.ListID)
	}

	if !list.Enabled {
		t.Error("Load() list not enabled")
	}

	if list.Settings == nil {
		t.Fatal("Load() settings is nil")
	}

	if !list.Settings.OnlyUnread {
		t.Error("Load() OnlyUnread not set")
	}

	if list.Settings.LimitValue != 100 {
		t.Errorf("Load() LimitValue = %d, want 100", list.Settings.LimitValue)
	}
}

func TestLoadTOML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	tomlContent := `[devices.device-456]
device_id = "device-456"
friendly_name = "TOML Device"

[[devices.device-456.lists]]
list_id = "list-guid-2"
list_name = "TOML List"
enabled = true

[devices.device-456.lists.settings]
only_checked = true
sort = true
list_sort_type = "series"
`

	if err := os.WriteFile(configPath, []byte(tomlContent), 0644); err != nil {
		t.Fatalf("Failed to write test TOML: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() TOML error = %v", err)
	}

	if len(cfg.Devices) != 1 {
		t.Fatalf("Load() device count = %d, want 1", len(cfg.Devices))
	}

	device := cfg.Devices["device-456"]
	if device == nil {
		t.Fatal("Load() device not found")
	}

	if device.FriendlyName != "TOML Device" {
		t.Errorf("Load() FriendlyName = %v, want TOML Device", device.FriendlyName)
	}

	if len(device.Lists) != 1 {
		t.Fatalf("Load() list count = %d, want 1", len(device.Lists))
	}

	list := device.Lists[0]
	if list.Settings == nil {
		t.Fatal("Load() settings is nil")
	}

	if !list.Settings.OnlyChecked {
		t.Error("Load() OnlyChecked not set")
	}

	if list.Settings.ListSortType != sync.SortTypeSeries {
		t.Errorf("Load() ListSortType = %v, want %v", list.Settings.ListSortType, sync.SortTypeSeries)
	}
}

func TestLoadInvalidFormat(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	if err := os.WriteFile(configPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("Load() should error for unsupported format")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("Load() should error for invalid YAML")
	}
}

func TestSaveYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := NewConfig()
	device := cfg.AddDevice("device-789", "Save Test")
	settings := &sync.SharedListSettings{
		OnlyUnread: true,
		Limit:      true,
		LimitValue: 50,
	}
	device.AddList("list-guid-3", "Save List", settings)

	err := Save(cfg, configPath)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("Save() did not create file: %v", err)
	}

	// Load it back and verify
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() after Save() error = %v", err)
	}

	if len(loaded.Devices) != 1 {
		t.Fatalf("Load() after Save() device count = %d, want 1", len(loaded.Devices))
	}

	loadedDevice := loaded.Devices["device-789"]
	if loadedDevice == nil {
		t.Fatal("Load() after Save() device not found")
	}

	if loadedDevice.FriendlyName != "Save Test" {
		t.Errorf("Load() after Save() FriendlyName = %v, want Save Test", loadedDevice.FriendlyName)
	}

	if len(loadedDevice.Lists) != 1 {
		t.Fatalf("Load() after Save() list count = %d, want 1", len(loadedDevice.Lists))
	}

	loadedList := loadedDevice.Lists[0]
	if loadedList.Settings.LimitValue != 50 {
		t.Errorf("Load() after Save() LimitValue = %d, want 50", loadedList.Settings.LimitValue)
	}
}

func TestSaveTOML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	cfg := NewConfig()
	cfg.AddDevice("device-toml", "TOML Save")

	err := Save(cfg, configPath)
	if err != nil {
		t.Fatalf("Save() TOML error = %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("Save() did not create TOML file: %v", err)
	}

	// Load it back
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() after Save() TOML error = %v", err)
	}

	if len(loaded.Devices) != 1 {
		t.Fatalf("Load() after Save() TOML device count = %d, want 1", len(loaded.Devices))
	}
}

func TestSaveInvalidFormat(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := NewConfig()
	err := Save(cfg, configPath)
	if err == nil {
		t.Error("Save() should error for unsupported format")
	}
}
