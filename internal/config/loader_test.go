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
	settings := &sync.SharedListSettings{
		OnlyUnread: true,
		Limit:      true,
		LimitValue: 50,
	}
	device := &DeviceConfig{
		DeviceID:     "device-789",
		FriendlyName: "Save Test",
		Lists: []SharedListConfig{
			{ListID: "list-guid-3", ListName: "Save List", Enabled: true, Settings: settings},
		},
	}
	cfg.Devices["device-789"] = device

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
	cfg.Devices["device-toml"] = &DeviceConfig{
		DeviceID:     "device-toml",
		FriendlyName: "TOML Save",
	}

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

func TestLoadServerConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "server-config.yaml")

	yamlContent := `server:
  library_path: /test/library/ComicDb.xml
  server_port: 8080
  discovery_port: 8888
  bind_address: 192.168.1.100
  ignore_devices:
    - device-1
    - device-2
  auto_sync: true
  max_concurrent_sync: 3
  log_level: debug
  log_format: json
`

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test YAML: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify server settings
	if cfg.Server.LibraryPath != "/test/library/ComicDb.xml" {
		t.Errorf("LibraryPath = %v, want /test/library/ComicDb.xml", cfg.Server.LibraryPath)
	}
	if cfg.Server.ServerPort != 8080 {
		t.Errorf("ServerPort = %v, want 8080", cfg.Server.ServerPort)
	}
	if cfg.Server.DiscoveryPort != 8888 {
		t.Errorf("DiscoveryPort = %v, want 8888", cfg.Server.DiscoveryPort)
	}
	if cfg.Server.BindAddress != "192.168.1.100" {
		t.Errorf("BindAddress = %v, want 192.168.1.100", cfg.Server.BindAddress)
	}
	if len(cfg.Server.IgnoreDevices) != 2 {
		t.Errorf("IgnoreDevices length = %v, want 2", len(cfg.Server.IgnoreDevices))
	}
	if !cfg.Server.AutoSync {
		t.Errorf("AutoSync = %v, want true", cfg.Server.AutoSync)
	}
	if cfg.Server.MaxConcurrentSync != 3 {
		t.Errorf("MaxConcurrentSync = %v, want 3", cfg.Server.MaxConcurrentSync)
	}
	if cfg.Server.LogLevel != "debug" {
		t.Errorf("LogLevel = %v, want debug", cfg.Server.LogLevel)
	}
	if cfg.Server.LogFormat != "json" {
		t.Errorf("LogFormat = %v, want json", cfg.Server.LogFormat)
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "minimal.yaml")

	yamlContent := `server:
  library_path: /test/ComicDb.xml
`

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test YAML: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Should have defaults applied
	if cfg.Server.ServerPort != 7620 {
		t.Errorf("ServerPort = %v, want 7620 (default)", cfg.Server.ServerPort)
	}
	if cfg.Server.DiscoveryPort != 7615 {
		t.Errorf("DiscoveryPort = %v, want 7615 (default)", cfg.Server.DiscoveryPort)
	}
	if cfg.Server.LogLevel != "info" {
		t.Errorf("LogLevel = %v, want info (default)", cfg.Server.LogLevel)
	}
	if cfg.Server.LogFormat != "text" {
		t.Errorf("LogFormat = %v, want text (default)", cfg.Server.LogFormat)
	}
}

func TestLoadValidationFailure(t *testing.T) {
	t.Run("invalid server port", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "invalid-port.yaml")

		yamlContent := `server:
  server_port: 99999
`
		if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
			t.Fatalf("Failed to write test YAML: %v", err)
		}

		_, err := Load(configPath)
		if err == nil {
			t.Error("Load() expected validation error for invalid port, got nil")
		}
	})

	t.Run("invalid discovery port", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "invalid-discovery.yaml")

		yamlContent := `server:
  discovery_port: 99999
`
		if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
			t.Fatalf("Failed to write test YAML: %v", err)
		}

		_, err := Load(configPath)
		if err == nil {
			t.Error("Load() expected validation error for invalid discovery port, got nil")
		}
	})

	t.Run("invalid log level", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "invalid-log.yaml")

		yamlContent := `server:
  log_level: invalid
`
		if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
			t.Fatalf("Failed to write test YAML: %v", err)
		}

		_, err := Load(configPath)
		if err == nil {
			t.Error("Load() expected validation error for invalid log level, got nil")
		}
	})

	t.Run("invalid log format", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "invalid-format.yaml")

		yamlContent := `server:
  log_format: xml
`
		if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
			t.Fatalf("Failed to write test YAML: %v", err)
		}

		_, err := Load(configPath)
		if err == nil {
			t.Error("Load() expected validation error for invalid log format, got nil")
		}
	})
}
