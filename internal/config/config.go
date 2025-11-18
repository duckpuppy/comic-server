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
	LibraryPath string `yaml:"library_path,omitempty" toml:"library_path,omitempty"` // Path to ComicDb.xml

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
