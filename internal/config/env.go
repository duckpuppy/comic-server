package config

import (
	"os"
	"strconv"
	"strings"
)

// ApplyEnvironment overrides configuration values with environment variables
// Environment variables take precedence over config file values
// Variable names follow the pattern: COMIC_SERVER_<SETTING_NAME>
func (c *Config) ApplyEnvironment() error {
	// Library path
	if val := os.Getenv("COMIC_SERVER_LIBRARY_PATH"); val != "" {
		c.Server.LibraryPath = val
	}

	// SQLite database path
	if val := os.Getenv("COMIC_SERVER_DATABASE_PATH"); val != "" {
		c.Server.DatabasePath = val
	}

	// Cover thumbnail cache directory
	if val := os.Getenv("COMIC_SERVER_COVER_CACHE_DIR"); val != "" {
		c.Server.CoverCacheDir = val
	}

	// Network settings
	if val := os.Getenv("COMIC_SERVER_PORT"); val != "" {
		port, err := strconv.Atoi(val)
		if err != nil {
			return err
		}
		c.Server.ServerPort = port
	}

	if val := os.Getenv("COMIC_SERVER_DISCOVERY_PORT"); val != "" {
		port, err := strconv.Atoi(val)
		if err != nil {
			return err
		}
		c.Server.DiscoveryPort = port
	}

	if val := os.Getenv("COMIC_SERVER_BIND_ADDRESS"); val != "" {
		c.Server.BindAddress = val
	}

	// Device filters
	if val := os.Getenv("COMIC_SERVER_IGNORE_DEVICES"); val != "" {
		// Split comma-separated list
		devices := strings.Split(val, ",")
		for i := range devices {
			devices[i] = strings.TrimSpace(devices[i])
		}
		c.Server.IgnoreDevices = devices
	}

	// Sync settings
	if val := os.Getenv("COMIC_SERVER_AUTO_SYNC"); val != "" {
		autoSync, err := strconv.ParseBool(val)
		if err != nil {
			return err
		}
		c.Server.AutoSync = autoSync
	}

	if val := os.Getenv("COMIC_SERVER_MAX_CONCURRENT_SYNC"); val != "" {
		maxSync, err := strconv.Atoi(val)
		if err != nil {
			return err
		}
		c.Server.MaxConcurrentSync = maxSync
	}

	// Logging settings
	if val := os.Getenv("COMIC_SERVER_LOG_LEVEL"); val != "" {
		c.Server.LogLevel = strings.ToLower(val)
	}

	if val := os.Getenv("COMIC_SERVER_LOG_FORMAT"); val != "" {
		c.Server.LogFormat = strings.ToLower(val)
	}

	// Komga integration
	if val := os.Getenv("COMIC_SERVER_KOMGA_API_KEY"); val != "" {
		c.Server.Komga.APIKey = val
	}

	return nil
}
