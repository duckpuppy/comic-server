package syncstate

import (
	"sync"
	"time"
)

// SyncStatus represents the current status of a sync operation
type SyncStatus string

const (
	StatusStarting   SyncStatus = "starting"
	StatusInProgress SyncStatus = "in_progress"
	StatusCompleted  SyncStatus = "completed"
	StatusFailed     SyncStatus = "failed"
	StatusAborted    SyncStatus = "aborted"
)

// SyncState represents the state of an active or completed sync operation
type SyncState struct {
	DeviceID     string     `json:"device_id"`
	DeviceIP     string     `json:"device_ip"`
	DeviceName   string     `json:"device_name,omitempty"`
	StartTime    time.Time  `json:"start_time"`
	EndTime      *time.Time `json:"end_time,omitempty"`
	Status       SyncStatus `json:"status"`
	Progress     int        `json:"progress"`      // 0-100
	BooksTotal   int        `json:"books_total"`   // Total operations planned
	BooksAdded   int        `json:"books_added"`   // Books added to device
	BooksUpdated int        `json:"books_updated"` // Books updated on device
	BooksDeleted int        `json:"books_deleted"` // Books deleted from device
	ErrorCount   int        `json:"error_count"`   // Number of errors
	ErrorMessage string     `json:"error_message,omitempty"`
}

// Manager manages sync state for all devices
type Manager struct {
	mu          sync.RWMutex
	activeSyncs map[string]*SyncState // deviceID -> current sync state
	history     []*SyncState          // Recently completed syncs (FIFO)
	maxHistory  int                   // Maximum history entries to keep
}

// NewManager creates a new sync state manager
func NewManager(maxHistory int) *Manager {
	if maxHistory <= 0 {
		maxHistory = 100 // Default: keep last 100 syncs
	}
	return &Manager{
		activeSyncs: make(map[string]*SyncState),
		history:     make([]*SyncState, 0, maxHistory),
		maxHistory:  maxHistory,
	}
}

// StartSync registers a new sync operation and returns an error if the device is already syncing
func (m *Manager) StartSync(deviceID, deviceIP, deviceName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if device is already syncing
	if _, exists := m.activeSyncs[deviceID]; exists {
		return &DeviceAlreadySyncingError{DeviceID: deviceID}
	}

	// Create new sync state
	state := &SyncState{
		DeviceID:   deviceID,
		DeviceIP:   deviceIP,
		DeviceName: deviceName,
		StartTime:  time.Now(),
		Status:     StatusStarting,
		Progress:   0,
	}

	m.activeSyncs[deviceID] = state
	return nil
}

// UpdateProgress updates the progress of an active sync
func (m *Manager) UpdateProgress(deviceID string, progress, total, added, updated, deleted, errorCount int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.activeSyncs[deviceID]; exists {
		state.Progress = progress
		state.BooksTotal = total
		state.BooksAdded = added
		state.BooksUpdated = updated
		state.BooksDeleted = deleted
		state.ErrorCount = errorCount
		state.Status = StatusInProgress
	}
}

// CompleteSync marks a sync as completed successfully
func (m *Manager) CompleteSync(deviceID string, added, updated, deleted, errorCount int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.activeSyncs[deviceID]; exists {
		now := time.Now()
		state.EndTime = &now
		state.Status = StatusCompleted
		state.Progress = 100
		state.BooksAdded = added
		state.BooksUpdated = updated
		state.BooksDeleted = deleted
		state.ErrorCount = errorCount

		// Move to history
		m.addToHistory(state)
		delete(m.activeSyncs, deviceID)
	}
}

// FailSync marks a sync as failed
func (m *Manager) FailSync(deviceID string, errorMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.activeSyncs[deviceID]; exists {
		now := time.Now()
		state.EndTime = &now
		state.Status = StatusFailed
		state.ErrorMessage = errorMsg

		// Move to history
		m.addToHistory(state)
		delete(m.activeSyncs, deviceID)
	}
}

// AbortSync marks a sync as aborted
func (m *Manager) AbortSync(deviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.activeSyncs[deviceID]; exists {
		now := time.Now()
		state.EndTime = &now
		state.Status = StatusAborted

		// Move to history
		m.addToHistory(state)
		delete(m.activeSyncs, deviceID)
	}
}

// GetActiveSync returns the current sync state for a device
func (m *Manager) GetActiveSync(deviceID string) (*SyncState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.activeSyncs[deviceID]
	if !exists {
		return nil, false
	}

	// Return a copy to prevent external modification
	stateCopy := *state
	return &stateCopy, true
}

// GetAllActiveSyncs returns all currently active syncs
func (m *Manager) GetAllActiveSyncs() []*SyncState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	syncs := make([]*SyncState, 0, len(m.activeSyncs))
	for _, state := range m.activeSyncs {
		stateCopy := *state
		syncs = append(syncs, &stateCopy)
	}

	return syncs
}

// GetHistory returns recent sync history
func (m *Manager) GetHistory(limit int) []*SyncState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 || limit > len(m.history) {
		limit = len(m.history)
	}

	// Return most recent entries (history is FIFO, so take from end)
	start := len(m.history) - limit
	history := make([]*SyncState, limit)
	for i := 0; i < limit; i++ {
		stateCopy := *m.history[start+i]
		history[i] = &stateCopy
	}

	return history
}

// IsDeviceSyncing returns true if the device is currently syncing
func (m *Manager) IsDeviceSyncing(deviceID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.activeSyncs[deviceID]
	return exists
}

// GetActiveCount returns the number of currently active syncs
func (m *Manager) GetActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.activeSyncs)
}

// addToHistory adds a sync state to history (must be called with lock held)
func (m *Manager) addToHistory(state *SyncState) {
	// If history is full, remove oldest entry
	if len(m.history) >= m.maxHistory {
		m.history = m.history[1:]
	}

	m.history = append(m.history, state)
}

// DeviceAlreadySyncingError is returned when attempting to start a sync for a device that's already syncing
type DeviceAlreadySyncingError struct {
	DeviceID string
}

func (e *DeviceAlreadySyncingError) Error() string {
	return "device " + e.DeviceID + " is already syncing"
}
