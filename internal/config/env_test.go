package config

import (
	"os"
	"testing"
)

func TestApplyEnvironment(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	envVars := []string{
		"COMIC_SERVER_LIBRARY_PATH",
		"COMIC_SERVER_PORT",
		"COMIC_SERVER_DISCOVERY_PORT",
		"COMIC_SERVER_BIND_ADDRESS",
		"COMIC_SERVER_IGNORE_DEVICES",
		"COMIC_SERVER_AUTO_SYNC",
		"COMIC_SERVER_MAX_CONCURRENT_SYNC",
		"COMIC_SERVER_LOG_LEVEL",
		"COMIC_SERVER_LOG_FORMAT",
	}
	for _, key := range envVars {
		originalEnv[key] = os.Getenv(key)
	}

	// Cleanup function
	cleanup := func() {
		for key, val := range originalEnv {
			if val == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, val)
			}
		}
	}
	defer cleanup()

	t.Run("library path override", func(t *testing.T) {
		cleanup()
		os.Setenv("COMIC_SERVER_LIBRARY_PATH", "/custom/path/ComicDb.xml")

		cfg := NewConfig()
		if err := cfg.ApplyEnvironment(); err != nil {
			t.Fatalf("ApplyEnvironment() error = %v", err)
		}

		if cfg.Server.LibraryPath != "/custom/path/ComicDb.xml" {
			t.Errorf("LibraryPath = %v, want /custom/path/ComicDb.xml", cfg.Server.LibraryPath)
		}
	})

	t.Run("server port override", func(t *testing.T) {
		cleanup()
		os.Setenv("COMIC_SERVER_PORT", "8080")

		cfg := NewConfig()
		if err := cfg.ApplyEnvironment(); err != nil {
			t.Fatalf("ApplyEnvironment() error = %v", err)
		}

		if cfg.Server.ServerPort != 8080 {
			t.Errorf("ServerPort = %v, want 8080", cfg.Server.ServerPort)
		}
	})

	t.Run("discovery port override", func(t *testing.T) {
		cleanup()
		os.Setenv("COMIC_SERVER_DISCOVERY_PORT", "8888")

		cfg := NewConfig()
		if err := cfg.ApplyEnvironment(); err != nil {
			t.Fatalf("ApplyEnvironment() error = %v", err)
		}

		if cfg.Server.DiscoveryPort != 8888 {
			t.Errorf("DiscoveryPort = %v, want 8888", cfg.Server.DiscoveryPort)
		}
	})

	t.Run("bind address override", func(t *testing.T) {
		cleanup()
		os.Setenv("COMIC_SERVER_BIND_ADDRESS", "192.168.1.100")

		cfg := NewConfig()
		if err := cfg.ApplyEnvironment(); err != nil {
			t.Fatalf("ApplyEnvironment() error = %v", err)
		}

		if cfg.Server.BindAddress != "192.168.1.100" {
			t.Errorf("BindAddress = %v, want 192.168.1.100", cfg.Server.BindAddress)
		}
	})

	t.Run("ignore devices override", func(t *testing.T) {
		cleanup()
		os.Setenv("COMIC_SERVER_IGNORE_DEVICES", "192.168.0.24, SM-T970, My Tablet")

		cfg := NewConfig()
		if err := cfg.ApplyEnvironment(); err != nil {
			t.Fatalf("ApplyEnvironment() error = %v", err)
		}

		expected := []string{"192.168.0.24", "SM-T970", "My Tablet"}
		if len(cfg.Server.IgnoreDevices) != len(expected) {
			t.Errorf("IgnoreDevices length = %v, want %v", len(cfg.Server.IgnoreDevices), len(expected))
		}
		for i := range expected {
			if cfg.Server.IgnoreDevices[i] != expected[i] {
				t.Errorf("IgnoreDevices[%d] = %v, want %v", i, cfg.Server.IgnoreDevices[i], expected[i])
			}
		}
	})

	t.Run("auto sync override", func(t *testing.T) {
		cleanup()
		os.Setenv("COMIC_SERVER_AUTO_SYNC", "true")

		cfg := NewConfig()
		if err := cfg.ApplyEnvironment(); err != nil {
			t.Fatalf("ApplyEnvironment() error = %v", err)
		}

		if !cfg.Server.AutoSync {
			t.Errorf("AutoSync = %v, want true", cfg.Server.AutoSync)
		}
	})

	t.Run("max concurrent sync override", func(t *testing.T) {
		cleanup()
		os.Setenv("COMIC_SERVER_MAX_CONCURRENT_SYNC", "5")

		cfg := NewConfig()
		if err := cfg.ApplyEnvironment(); err != nil {
			t.Fatalf("ApplyEnvironment() error = %v", err)
		}

		if cfg.Server.MaxConcurrentSync != 5 {
			t.Errorf("MaxConcurrentSync = %v, want 5", cfg.Server.MaxConcurrentSync)
		}
	})

	t.Run("log level override", func(t *testing.T) {
		cleanup()
		os.Setenv("COMIC_SERVER_LOG_LEVEL", "DEBUG")

		cfg := NewConfig()
		if err := cfg.ApplyEnvironment(); err != nil {
			t.Fatalf("ApplyEnvironment() error = %v", err)
		}

		if cfg.Server.LogLevel != "debug" {
			t.Errorf("LogLevel = %v, want debug", cfg.Server.LogLevel)
		}
	})

	t.Run("log format override", func(t *testing.T) {
		cleanup()
		os.Setenv("COMIC_SERVER_LOG_FORMAT", "JSON")

		cfg := NewConfig()
		if err := cfg.ApplyEnvironment(); err != nil {
			t.Fatalf("ApplyEnvironment() error = %v", err)
		}

		if cfg.Server.LogFormat != "json" {
			t.Errorf("LogFormat = %v, want json", cfg.Server.LogFormat)
		}
	})

	t.Run("multiple overrides", func(t *testing.T) {
		cleanup()
		os.Setenv("COMIC_SERVER_PORT", "9090")
		os.Setenv("COMIC_SERVER_LOG_LEVEL", "warn")
		os.Setenv("COMIC_SERVER_AUTO_SYNC", "true")

		cfg := NewConfig()
		if err := cfg.ApplyEnvironment(); err != nil {
			t.Fatalf("ApplyEnvironment() error = %v", err)
		}

		if cfg.Server.ServerPort != 9090 {
			t.Errorf("ServerPort = %v, want 9090", cfg.Server.ServerPort)
		}
		if cfg.Server.LogLevel != "warn" {
			t.Errorf("LogLevel = %v, want warn", cfg.Server.LogLevel)
		}
		if !cfg.Server.AutoSync {
			t.Errorf("AutoSync = %v, want true", cfg.Server.AutoSync)
		}
	})

	t.Run("invalid port number", func(t *testing.T) {
		cleanup()
		os.Setenv("COMIC_SERVER_PORT", "invalid")

		cfg := NewConfig()
		err := cfg.ApplyEnvironment()
		if err == nil {
			t.Error("ApplyEnvironment() expected error for invalid port, got nil")
		}
	})

	t.Run("invalid bool value", func(t *testing.T) {
		cleanup()
		os.Setenv("COMIC_SERVER_AUTO_SYNC", "maybe")

		cfg := NewConfig()
		err := cfg.ApplyEnvironment()
		if err == nil {
			t.Error("ApplyEnvironment() expected error for invalid bool, got nil")
		}
	})
}
