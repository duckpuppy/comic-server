package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetConfigDir(t *testing.T) {
	tests := []struct {
		name       string
		xdgConfig  string
		wantSuffix string
	}{
		{
			name:       "with XDG_CONFIG_HOME set",
			xdgConfig:  "/custom/config",
			wantSuffix: "/custom/config/comic-server",
		},
		{
			name:       "without XDG_CONFIG_HOME",
			xdgConfig:  "",
			wantSuffix: ".config/comic-server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore original env
			originalXDG := os.Getenv("XDG_CONFIG_HOME")
			defer os.Setenv("XDG_CONFIG_HOME", originalXDG)

			if tt.xdgConfig != "" {
				os.Setenv("XDG_CONFIG_HOME", tt.xdgConfig)
			} else {
				os.Unsetenv("XDG_CONFIG_HOME")
			}

			got, err := GetConfigDir()
			if err != nil {
				t.Fatalf("GetConfigDir() error = %v", err)
			}

			if tt.xdgConfig != "" {
				// Exact match when XDG_CONFIG_HOME is set
				if got != tt.wantSuffix {
					t.Errorf("GetConfigDir() = %v, want %v", got, tt.wantSuffix)
				}
			} else {
				// Suffix match when using home directory
				if !endsWithPath(got, tt.wantSuffix) {
					t.Errorf("GetConfigDir() = %v, want path ending with %v", got, tt.wantSuffix)
				}
			}
		})
	}
}

func TestGetCacheDir(t *testing.T) {
	tests := []struct {
		name       string
		xdgCache   string
		wantSuffix string
	}{
		{
			name:       "with XDG_CACHE_HOME set",
			xdgCache:   "/custom/cache",
			wantSuffix: "/custom/cache/comic-server",
		},
		{
			name:       "without XDG_CACHE_HOME",
			xdgCache:   "",
			wantSuffix: ".cache/comic-server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalXDG := os.Getenv("XDG_CACHE_HOME")
			defer os.Setenv("XDG_CACHE_HOME", originalXDG)

			if tt.xdgCache != "" {
				os.Setenv("XDG_CACHE_HOME", tt.xdgCache)
			} else {
				os.Unsetenv("XDG_CACHE_HOME")
			}

			got, err := GetCacheDir()
			if err != nil {
				t.Fatalf("GetCacheDir() error = %v", err)
			}

			if tt.xdgCache != "" {
				if got != tt.wantSuffix {
					t.Errorf("GetCacheDir() = %v, want %v", got, tt.wantSuffix)
				}
			} else if !endsWithPath(got, tt.wantSuffix) {
				t.Errorf("GetCacheDir() = %v, want path ending with %v", got, tt.wantSuffix)
			}
		})
	}
}

func TestEnsureCacheDir(t *testing.T) {
	tmpDir := t.TempDir()
	originalXDG := os.Getenv("XDG_CACHE_HOME")
	defer os.Setenv("XDG_CACHE_HOME", originalXDG)
	os.Setenv("XDG_CACHE_HOME", tmpDir)

	dir, err := EnsureCacheDir()
	if err != nil {
		t.Fatalf("EnsureCacheDir() error = %v", err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Errorf("expected EnsureCacheDir() to create %v as a directory", dir)
	}
}

func TestGetDefaultConfigPath(t *testing.T) {
	// Create temporary directory for testing
	tmpDir := t.TempDir()

	// Save and restore original env
	originalXDG := os.Getenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", originalXDG)

	configDir := filepath.Join(tmpDir, "comic-server")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create config directory
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create test config dir: %v", err)
	}

	t.Run("prefers existing TOML file", func(t *testing.T) {
		tomlPath := filepath.Join(configDir, "config.toml")
		if err := os.WriteFile(tomlPath, []byte(""), 0644); err != nil {
			t.Fatalf("Failed to create test TOML file: %v", err)
		}
		defer os.Remove(tomlPath)

		got, err := GetDefaultConfigPath()
		if err != nil {
			t.Fatalf("GetDefaultConfigPath() error = %v", err)
		}

		if got != tomlPath {
			t.Errorf("GetDefaultConfigPath() = %v, want %v", got, tomlPath)
		}
	})

	t.Run("returns YAML path when no TOML exists", func(t *testing.T) {
		// Ensure no TOML file exists
		tomlPath := filepath.Join(configDir, "config.toml")
		os.Remove(tomlPath)

		got, err := GetDefaultConfigPath()
		if err != nil {
			t.Fatalf("GetDefaultConfigPath() error = %v", err)
		}

		wantYAML := filepath.Join(configDir, "config.yaml")
		if got != wantYAML {
			t.Errorf("GetDefaultConfigPath() = %v, want %v", got, wantYAML)
		}
	})
}

// endsWithPath checks if a path ends with the given suffix
func endsWithPath(path, suffix string) bool {
	cleanPath := filepath.Clean(path)
	cleanSuffix := filepath.Clean(suffix)
	return len(cleanPath) >= len(cleanSuffix) &&
		cleanPath[len(cleanPath)-len(cleanSuffix):] == cleanSuffix
}
