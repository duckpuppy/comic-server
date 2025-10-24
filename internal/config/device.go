package config

import (
	"fmt"

	"github.com/duckpuppy/comic-server/internal/sync"
)

// AddList adds a smart list to a device's sync configuration
func (d *DeviceConfig) AddList(listID, listName string, settings *sync.SharedListSettings) error {
	// Check if list already exists
	for i := range d.Lists {
		if d.Lists[i].ListID == listID {
			return fmt.Errorf("list %q is already configured for this device", listName)
		}
	}

	// Add new list
	listConfig := SharedListConfig{
		ListID:   listID,
		ListName: listName,
		Enabled:  true,
		Settings: settings,
	}

	d.Lists = append(d.Lists, listConfig)
	return nil
}

// RemoveList removes a smart list from a device's sync configuration
func (d *DeviceConfig) RemoveList(listID string) error {
	for i := range d.Lists {
		if d.Lists[i].ListID == listID {
			// Remove from slice
			d.Lists = append(d.Lists[:i], d.Lists[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("list not found in device configuration")
}

// GetList returns the configuration for a specific list
func (d *DeviceConfig) GetList(listID string) *SharedListConfig {
	for i := range d.Lists {
		if d.Lists[i].ListID == listID {
			return &d.Lists[i]
		}
	}
	return nil
}

// UpdateListSettings updates the settings for a specific list
func (d *DeviceConfig) UpdateListSettings(listID string, settings *sync.SharedListSettings) error {
	for i := range d.Lists {
		if d.Lists[i].ListID == listID {
			d.Lists[i].Settings = settings
			return nil
		}
	}

	return fmt.Errorf("list not found in device configuration")
}

// EnableList enables a list without removing it
func (d *DeviceConfig) EnableList(listID string) error {
	for i := range d.Lists {
		if d.Lists[i].ListID == listID {
			d.Lists[i].Enabled = true
			return nil
		}
	}

	return fmt.Errorf("list not found in device configuration")
}

// DisableList disables a list without removing it
func (d *DeviceConfig) DisableList(listID string) error {
	for i := range d.Lists {
		if d.Lists[i].ListID == listID {
			d.Lists[i].Enabled = false
			return nil
		}
	}

	return fmt.Errorf("list not found in device configuration")
}
