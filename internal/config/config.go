package config

import (
	"time"

	"github.com/duckpuppy/comic-server/internal/sync"
)

// Config is the root configuration structure
type Config struct {
	Devices map[string]*DeviceConfig `yaml:"devices" toml:"devices"`
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
	ListID   string                   `yaml:"list_id" toml:"list_id"`       // GUID from library
	ListName string                   `yaml:"list_name" toml:"list_name"`   // Cached name for display
	Enabled  bool                     `yaml:"enabled" toml:"enabled"`       // Allow disable without deleting
	Settings *sync.SharedListSettings `yaml:"settings,omitempty" toml:"settings,omitempty"` // Per-list settings (nil = use defaults)
}

// NewConfig creates a new empty configuration
func NewConfig() *Config {
	return &Config{
		Devices: make(map[string]*DeviceConfig),
	}
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
