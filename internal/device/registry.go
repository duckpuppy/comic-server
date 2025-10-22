package device

import (
	"fmt"
	"sync"
	"time"
)

// RegisteredDevice represents a device in the registry
type RegisteredDevice struct {
	Info      *Info
	IPAddress string
	LastSeen  time.Time
}

// Registry tracks discovered and validated devices
type Registry struct {
	mu      sync.RWMutex
	devices map[string]*RegisteredDevice // key: device ID
}

// NewRegistry creates a new device registry
func NewRegistry() *Registry {
	return &Registry{
		devices: make(map[string]*RegisteredDevice),
	}
}

// Add adds or updates a device in the registry
func (r *Registry) Add(info *Info, ipAddress string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.devices[info.ID] = &RegisteredDevice{
		Info:      info,
		IPAddress: ipAddress,
		LastSeen:  time.Now(),
	}
}

// Get retrieves a device by ID
func (r *Registry) Get(deviceID string) (*RegisteredDevice, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	device, exists := r.devices[deviceID]
	return device, exists
}

// GetByIP retrieves a device by IP address
func (r *Registry) GetByIP(ipAddress string) (*RegisteredDevice, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, device := range r.devices {
		if device.IPAddress == ipAddress {
			return device, true
		}
	}
	return nil, false
}

// List returns all registered devices
func (r *Registry) List() []*RegisteredDevice {
	r.mu.RLock()
	defer r.mu.RUnlock()

	devices := make([]*RegisteredDevice, 0, len(r.devices))
	for _, device := range r.devices {
		devices = append(devices, device)
	}
	return devices
}

// Count returns the number of registered devices
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.devices)
}

// Remove removes a device from the registry
func (r *Registry) Remove(deviceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.devices, deviceID)
}

// UpdateLastSeen updates the last seen timestamp for a device
func (r *Registry) UpdateLastSeen(deviceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	device, exists := r.devices[deviceID]
	if !exists {
		return fmt.Errorf("device %q not found in registry", deviceID)
	}

	device.LastSeen = time.Now()
	return nil
}

// RemoveStale removes devices that haven't been seen in the specified duration
func (r *Registry) RemoveStale(maxAge time.Duration) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	removed := 0

	for id, device := range r.devices {
		if now.Sub(device.LastSeen) > maxAge {
			delete(r.devices, id)
			removed++
		}
	}

	return removed
}

// String returns a human-readable summary of the registry
func (r *Registry) String() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return fmt.Sprintf("Registry: %d device(s)", len(r.devices))
}
