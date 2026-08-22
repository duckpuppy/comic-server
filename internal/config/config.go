package config

import (
	"fmt"
	"time"

	"github.com/duckpuppy/comic-server/internal/sync"
)

// Config is the root configuration structure
type Config struct {
	Server  ServerConfig             `yaml:"server,omitempty" toml:"server,omitempty"`
	Devices map[string]*DeviceConfig `yaml:"devices" toml:"devices"`
}

// ServerConfig contains global server settings
type ServerConfig struct {
	// Library settings
	LibraryPath  string `yaml:"library_path,omitempty" toml:"library_path,omitempty"`   // Path to ComicDb.xml
	DatabasePath string `yaml:"database_path,omitempty" toml:"database_path,omitempty"` // Path to SQLite database (alternative to library_path)

	// CoverCacheDir overrides where resized cover thumbnails are cached.
	// Empty means use the XDG cache directory (config.GetCacheDir()) -
	// fine for a bare-metal install, but under Docker that directory isn't
	// one of the image's declared volumes (/config, /data, /comics), so the
	// cache is lost on every container recreate unless this is set to a
	// path under a mounted volume (e.g. /data/cover-cache).
	CoverCacheDir string `yaml:"cover_cache_dir,omitempty" toml:"cover_cache_dir,omitempty"`

	// Network settings
	ServerPort    int    `yaml:"server_port,omitempty" toml:"server_port,omitempty"`       // TCP control port (default: 7620)
	DiscoveryPort int    `yaml:"discovery_port,omitempty" toml:"discovery_port,omitempty"` // UDP multicast port (default: 7615)
	BindAddress   string `yaml:"bind_address,omitempty" toml:"bind_address,omitempty"`     // Network interface to bind (default: all)

	// Device filters
	IgnoreDevices []string `yaml:"ignore_devices,omitempty" toml:"ignore_devices,omitempty"` // Device IPs/IDs/names to ignore

	// Sync settings
	AutoSync                     bool `yaml:"auto_sync,omitempty" toml:"auto_sync,omitempty"`                                               // Enable automatic sync when devices connect
	MaxConcurrentSync            int  `yaml:"max_concurrent_sync,omitempty" toml:"max_concurrent_sync,omitempty"`                           // Max concurrent syncs (0 = unlimited)
	MaxConcurrentConnections     int  `yaml:"max_concurrent_connections,omitempty" toml:"max_concurrent_connections,omitempty"`             // Max concurrent connections (0 = unlimited)
	LibraryCacheFlushIntervalSec int  `yaml:"library_cache_flush_interval_sec,omitempty" toml:"library_cache_flush_interval_sec,omitempty"` // Library cache flush interval in seconds (0 = flush on every change)

	// Rate limiting settings
	MaxConnectionsPerIP    int `yaml:"max_connections_per_ip,omitempty" toml:"max_connections_per_ip,omitempty"`       // Max connection attempts per IP per minute (0 = unlimited)
	MaxRequestsPerDevice   int `yaml:"max_requests_per_device,omitempty" toml:"max_requests_per_device,omitempty"`     // Max requests per device per minute (0 = unlimited)
	RateLimitWindowSeconds int `yaml:"rate_limit_window_seconds,omitempty" toml:"rate_limit_window_seconds,omitempty"` // Rate limit time window in seconds

	// Logging settings
	LogLevel  string `yaml:"log_level,omitempty" toml:"log_level,omitempty"`   // Log level: debug, info, warn, error
	LogFormat string `yaml:"log_format,omitempty" toml:"log_format,omitempty"` // Log format: text, json

	// ComicVine integration
	ComicVineAPIKey string `yaml:"comicvine_api_key,omitempty" toml:"comicvine_api_key,omitempty"` // API key for ComicVine enrichment

	// Komga integration
	Komga KomgaConfig `yaml:"komga,omitempty" toml:"komga,omitempty"`
}

// KomgaConfig configures pushing comic-server smart lists into Komga
// collections and read lists, since Komga has no native smart-list concept
// of its own. comic-server and Komga read independent, synced copies of the
// same library (potentially on different machines/OSes), so books are
// matched by translating file paths between the two roots rather than by
// any shared ID - see LocalRoot/RemoteRoot.
type KomgaConfig struct {
	Enabled bool   `yaml:"enabled,omitempty" toml:"enabled,omitempty"`
	BaseURL string `yaml:"base_url,omitempty" toml:"base_url,omitempty"` // e.g. https://comics.example.com
	// APIKey can also be set via COMIC_SERVER_KOMGA_API_KEY; prefer the env
	// var over committing a real key to a config file.
	APIKey string `yaml:"api_key,omitempty" toml:"api_key,omitempty"`

	// SyncIntervalSec is how often each target's smart list is re-evaluated
	// and pushed to Komga. comic-server has no way to detect ComicRack
	// library changes while running (see comic-server-bwz), so this is a
	// scheduled push rather than change-triggered. Default: 900 (15 min).
	SyncIntervalSec int `yaml:"sync_interval_sec,omitempty" toml:"sync_interval_sec,omitempty"`

	// Path mapping: comic-server's library paths are rooted at LocalRoot;
	// Komga sees the same files rooted at RemoteRoot. Directory structure
	// below the root is assumed identical, so translation is a simple
	// prefix swap - the same approach as the *Arr apps' Remote Path
	// Mapping. Both roots are compared/joined using forward-slash-
	// normalized paths (matching how comic-server already normalizes
	// Directory/File/FullPath matchers).
	LocalRoot  string `yaml:"local_root,omitempty" toml:"local_root,omitempty"`
	RemoteRoot string `yaml:"remote_root,omitempty" toml:"remote_root,omitempty"`

	Targets []KomgaTarget `yaml:"targets,omitempty" toml:"targets,omitempty"`
}

// KomgaTargetType is which kind of Komga entity a smart list syncs into.
type KomgaTargetType string

const (
	KomgaTargetCollection KomgaTargetType = "collection" // series-level grouping
	KomgaTargetReadList   KomgaTargetType = "readlist"   // book-level grouping, can span series
)

// KomgaTarget maps one comic-server smart list to one Komga collection or
// read list. Books matched by the list but not found in Komga (via the
// LocalRoot/RemoteRoot path translation) are skipped and logged, not
// treated as a sync failure.
type KomgaTarget struct {
	ListID    string          `yaml:"list_id" toml:"list_id"`                         // Smart list GUID from the library
	ListName  string          `yaml:"list_name,omitempty" toml:"list_name,omitempty"` // Cached name for display
	Type      KomgaTargetType `yaml:"type" toml:"type"`
	KomgaName string          `yaml:"komga_name" toml:"komga_name"` // Name of the collection/read list in Komga
	Enabled   bool            `yaml:"enabled" toml:"enabled"`       // Allow disable without deleting
}

// DeviceConfig contains sync configuration for a specific device
type DeviceConfig struct {
	DeviceID        string                   `yaml:"device_id" toml:"device_id"`
	FriendlyName    string                   `yaml:"friendly_name,omitempty" toml:"friendly_name,omitempty"`
	LastSeen        time.Time                `yaml:"last_seen,omitempty" toml:"last_seen,omitempty"`
	Lists           []SharedListConfig       `yaml:"lists,omitempty" toml:"lists,omitempty"`
	DefaultSettings *sync.SharedListSettings `yaml:"default_settings,omitempty" toml:"default_settings,omitempty"`
}

// SharedListConfig contains configuration for a specific smart list on a device
type SharedListConfig struct {
	ListID   string                   `yaml:"list_id" toml:"list_id"`                       // GUID from library
	ListName string                   `yaml:"list_name" toml:"list_name"`                   // Cached name for display
	Enabled  bool                     `yaml:"enabled" toml:"enabled"`                       // Allow disable without deleting
	Settings *sync.SharedListSettings `yaml:"settings,omitempty" toml:"settings,omitempty"` // Per-list settings (nil = use defaults)
}

// NewConfig creates a new empty configuration with default server settings
func NewConfig() *Config {
	return &Config{
		Server:  DefaultServerConfig(),
		Devices: make(map[string]*DeviceConfig),
	}
}

// DefaultServerConfig returns the default server configuration
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		ServerPort:                   7620,
		DiscoveryPort:                7615,
		BindAddress:                  "", // Empty = bind to all interfaces
		IgnoreDevices:                []string{},
		AutoSync:                     false,
		MaxConcurrentSync:            0,   // 0 = unlimited (v0.2 has mutex limiting to 1)
		MaxConcurrentConnections:     5,   // Default: 5 concurrent connections
		LibraryCacheFlushIntervalSec: 30,  // Default: 30 seconds (balance between performance and data safety)
		MaxConnectionsPerIP:          10,  // Default: 10 connections/minute per IP
		MaxRequestsPerDevice:         100, // Default: 100 requests/minute per device
		RateLimitWindowSeconds:       60,  // Default: 60 seconds (1 minute)
		LogLevel:                     "info",
		LogFormat:                    "text",
	}
}

// ApplyDefaults fills in missing server configuration values with defaults
func (c *Config) ApplyDefaults() {
	defaults := DefaultServerConfig()

	if c.Server.ServerPort == 0 {
		c.Server.ServerPort = defaults.ServerPort
	}
	if c.Server.DiscoveryPort == 0 {
		c.Server.DiscoveryPort = defaults.DiscoveryPort
	}
	if c.Server.MaxConcurrentConnections == 0 {
		c.Server.MaxConcurrentConnections = defaults.MaxConcurrentConnections
	}
	if c.Server.MaxConnectionsPerIP == 0 {
		c.Server.MaxConnectionsPerIP = defaults.MaxConnectionsPerIP
	}
	if c.Server.MaxRequestsPerDevice == 0 {
		c.Server.MaxRequestsPerDevice = defaults.MaxRequestsPerDevice
	}
	if c.Server.RateLimitWindowSeconds == 0 {
		c.Server.RateLimitWindowSeconds = defaults.RateLimitWindowSeconds
	}
	if c.Server.LogLevel == "" {
		c.Server.LogLevel = defaults.LogLevel
	}
	if c.Server.LogFormat == "" {
		c.Server.LogFormat = defaults.LogFormat
	}
	if c.Server.IgnoreDevices == nil {
		c.Server.IgnoreDevices = []string{}
	}
	if c.Server.Komga.SyncIntervalSec == 0 {
		c.Server.Komga.SyncIntervalSec = 900 // 15 minutes
	}
}

// Validate checks the configuration for errors
func (c *Config) Validate() error {
	// Validate port ranges
	if c.Server.ServerPort < 1 || c.Server.ServerPort > 65535 {
		return fmt.Errorf("server_port must be between 1 and 65535, got %d", c.Server.ServerPort)
	}
	if c.Server.DiscoveryPort < 1 || c.Server.DiscoveryPort > 65535 {
		return fmt.Errorf("discovery_port must be between 1 and 65535, got %d", c.Server.DiscoveryPort)
	}

	// Validate rate limiting parameters (must be non-negative)
	if c.Server.MaxConcurrentConnections < 0 {
		return fmt.Errorf("max_concurrent_connections must be >= 0, got %d", c.Server.MaxConcurrentConnections)
	}
	if c.Server.MaxConnectionsPerIP < 0 {
		return fmt.Errorf("max_connections_per_ip must be >= 0, got %d", c.Server.MaxConnectionsPerIP)
	}
	if c.Server.MaxRequestsPerDevice < 0 {
		return fmt.Errorf("max_requests_per_device must be >= 0, got %d", c.Server.MaxRequestsPerDevice)
	}
	if c.Server.RateLimitWindowSeconds < 1 {
		return fmt.Errorf("rate_limit_window_seconds must be >= 1, got %d", c.Server.RateLimitWindowSeconds)
	}

	// Validate log level
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.Server.LogLevel] {
		return fmt.Errorf("log_level must be one of: debug, info, warn, error, got %q", c.Server.LogLevel)
	}

	// Validate log format
	validFormats := map[string]bool{"text": true, "json": true}
	if !validFormats[c.Server.LogFormat] {
		return fmt.Errorf("log_format must be one of: text, json, got %q", c.Server.LogFormat)
	}

	if err := c.Server.Komga.Validate(); err != nil {
		return err
	}

	return nil
}

// Validate checks the Komga configuration for errors. A no-op when Komga
// integration is disabled.
func (kc *KomgaConfig) Validate() error {
	if !kc.Enabled {
		return nil
	}
	if kc.BaseURL == "" {
		return fmt.Errorf("komga.base_url is required when komga.enabled is true")
	}
	if kc.APIKey == "" {
		return fmt.Errorf("komga.api_key is required when komga.enabled is true (set directly or via COMIC_SERVER_KOMGA_API_KEY)")
	}
	if kc.LocalRoot == "" || kc.RemoteRoot == "" {
		return fmt.Errorf("komga.local_root and komga.remote_root are both required when komga.enabled is true")
	}
	if kc.SyncIntervalSec < 0 {
		return fmt.Errorf("komga.sync_interval_sec must be >= 0, got %d", kc.SyncIntervalSec)
	}

	seen := make(map[string]bool, len(kc.Targets))
	for i, t := range kc.Targets {
		if t.ListID == "" {
			return fmt.Errorf("komga.targets[%d].list_id is required", i)
		}
		if seen[t.ListID] {
			return fmt.Errorf("komga.targets[%d]: duplicate list_id %q", i, t.ListID)
		}
		seen[t.ListID] = true

		if t.Type != KomgaTargetCollection && t.Type != KomgaTargetReadList {
			return fmt.Errorf("komga.targets[%d].type must be %q or %q, got %q", i, KomgaTargetCollection, KomgaTargetReadList, t.Type)
		}
		if t.KomgaName == "" {
			return fmt.Errorf("komga.targets[%d].komga_name is required", i)
		}
	}

	return nil
}

// GetDevice returns the configuration for a device by ID
// Returns nil if the device is not configured
func (c *Config) GetDevice(deviceID string) *DeviceConfig {
	return c.Devices[deviceID]
}

// AddDevice adds or updates a device configuration
func (c *Config) AddDevice(deviceID, friendlyName string) *DeviceConfig {
	if device, exists := c.Devices[deviceID]; exists {
		// Update existing device
		if friendlyName != "" {
			device.FriendlyName = friendlyName
		}
		device.LastSeen = time.Now()
		return device
	}

	// Create new device
	device := &DeviceConfig{
		DeviceID:        deviceID,
		FriendlyName:    friendlyName,
		LastSeen:        time.Now(),
		Lists:           []SharedListConfig{},
		DefaultSettings: sync.DefaultSettings(),
	}
	c.Devices[deviceID] = device
	return device
}

// UpdateDevice updates a device's friendly name and last seen timestamp
func (c *Config) UpdateDevice(deviceID, friendlyName string) {
	device, exists := c.Devices[deviceID]
	if exists {
		device.FriendlyName = friendlyName
		device.LastSeen = time.Now()
	}
}

// RemoveDevice removes a device configuration
func (c *Config) RemoveDevice(deviceID string) bool {
	if _, exists := c.Devices[deviceID]; exists {
		delete(c.Devices, deviceID)
		return true
	}
	return false
}
