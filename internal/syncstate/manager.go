package syncstate

import (
	"fmt"
	"sync"
	"time"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus metrics
var (
	syncsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "comic_server_syncs_total",
			Help: "Total number of sync operations by status",
		},
		[]string{"status"},
	)
	activeSyncsGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "comic_server_active_syncs",
			Help: "Current number of active sync operations",
		},
	)
	booksProcessedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "comic_server_books_processed_total",
			Help: "Total number of books processed by operation",
		},
		[]string{"operation"},
	)
	syncDurationSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "comic_server_sync_duration_seconds",
			Help:    "Duration of sync operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)
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
	store       *configdb.DB          // optional: persists history across restarts
}

// NewManager creates a new sync state manager with in-memory-only history
// (lost on restart). Used by tests and any caller that doesn't have a
// config database to persist to.
func NewManager(maxHistory int) *Manager {
	return newManager(maxHistory, nil)
}

// NewManagerWithStore creates a new sync state manager whose history is
// persisted to store's sync_history table (comic-server-7vu) - every
// completed/failed/aborted sync is written through to the database, and
// the in-memory FIFO cache is warmed from the database at startup so
// GetHistory/GetHistoryPaginated/GetHistoryForDevice keep serving from
// memory unchanged.
func NewManagerWithStore(maxHistory int, store *configdb.DB) (*Manager, error) {
	m := newManager(maxHistory, store)

	records, err := store.LoadRecentSyncHistory(m.maxHistory)
	if err != nil {
		return nil, fmt.Errorf("load sync history: %w", err)
	}
	for _, rec := range records {
		m.history = append(m.history, syncStateFromRecord(rec))
	}

	return m, nil
}

func newManager(maxHistory int, store *configdb.DB) *Manager {
	if maxHistory <= 0 {
		maxHistory = 100 // Default: keep last 100 syncs
	}
	return &Manager{
		activeSyncs: make(map[string]*SyncState),
		history:     make([]*SyncState, 0, maxHistory),
		maxHistory:  maxHistory,
		store:       store,
	}
}

func syncStateToRecord(state *SyncState) configdb.SyncHistoryRecord {
	return configdb.SyncHistoryRecord{
		DeviceID:     state.DeviceID,
		DeviceIP:     state.DeviceIP,
		DeviceName:   state.DeviceName,
		StartTime:    state.StartTime,
		EndTime:      state.EndTime,
		Status:       string(state.Status),
		Progress:     state.Progress,
		BooksTotal:   state.BooksTotal,
		BooksAdded:   state.BooksAdded,
		BooksUpdated: state.BooksUpdated,
		BooksDeleted: state.BooksDeleted,
		ErrorCount:   state.ErrorCount,
		ErrorMessage: state.ErrorMessage,
	}
}

func syncStateFromRecord(rec configdb.SyncHistoryRecord) *SyncState {
	return &SyncState{
		DeviceID:     rec.DeviceID,
		DeviceIP:     rec.DeviceIP,
		DeviceName:   rec.DeviceName,
		StartTime:    rec.StartTime,
		EndTime:      rec.EndTime,
		Status:       SyncStatus(rec.Status),
		Progress:     rec.Progress,
		BooksTotal:   rec.BooksTotal,
		BooksAdded:   rec.BooksAdded,
		BooksUpdated: rec.BooksUpdated,
		BooksDeleted: rec.BooksDeleted,
		ErrorCount:   rec.ErrorCount,
		ErrorMessage: rec.ErrorMessage,
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

	// Update metrics
	syncsTotal.WithLabelValues(string(StatusStarting)).Inc()
	activeSyncsGauge.Inc()

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

		// Update metrics
		duration := time.Since(state.StartTime).Seconds()
		syncDurationSeconds.Observe(duration)
		syncsTotal.WithLabelValues(string(StatusCompleted)).Inc()
		activeSyncsGauge.Dec()
		booksProcessedTotal.WithLabelValues("added").Add(float64(added))
		booksProcessedTotal.WithLabelValues("updated").Add(float64(updated))
		booksProcessedTotal.WithLabelValues("deleted").Add(float64(deleted))

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

		// Update metrics
		duration := time.Since(state.StartTime).Seconds()
		syncDurationSeconds.Observe(duration)
		syncsTotal.WithLabelValues(string(StatusFailed)).Inc()
		activeSyncsGauge.Dec()

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

		// Update metrics
		duration := time.Since(state.StartTime).Seconds()
		syncDurationSeconds.Observe(duration)
		syncsTotal.WithLabelValues(string(StatusAborted)).Inc()
		activeSyncsGauge.Dec()

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

// HistoryPage represents a page of sync history with pagination metadata
type HistoryPage struct {
	History    []*SyncState `json:"history"`
	Total      int          `json:"total"`       // Total number of history entries
	Offset     int          `json:"offset"`      // Current offset
	Limit      int          `json:"limit"`       // Page size
	HasMore    bool         `json:"has_more"`    // Whether there are more entries
	NextOffset *int         `json:"next_offset"` // Offset for next page (nil if no more)
}

// PaginationMetadata contains pagination information
type PaginationMetadata struct {
	Total      int  `json:"total"`
	Offset     int  `json:"offset"`
	Limit      int  `json:"limit"`
	HasMore    bool `json:"has_more"`
	NextOffset *int `json:"next_offset,omitempty"`
}

// GetHistoryPaginated returns a page of sync history with pagination support.
// History is returned in chronological order (oldest first) within each page.
// offset=0 returns the most recent entries.
func (m *Manager) GetHistoryPaginated(limit, offset int) *HistoryPage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total := len(m.history)

	// Validate and adjust limit
	if limit <= 0 {
		limit = 20 // Default page size
	}
	if limit > 100 {
		limit = 100 // Cap at 100
	}

	// Validate offset
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		// Return empty page
		return &HistoryPage{
			History:    []*SyncState{},
			Total:      total,
			Offset:     offset,
			Limit:      limit,
			HasMore:    false,
			NextOffset: nil,
		}
	}

	// Calculate slice indices (offset from most recent)
	// history is FIFO, so most recent is at end
	// offset=0 means start from end, offset=1 means skip 1 from end, etc.
	end := total - offset
	start := end - limit
	if start < 0 {
		start = 0
	}

	// Extract page
	pageSize := end - start
	history := make([]*SyncState, pageSize)
	for i := 0; i < pageSize; i++ {
		stateCopy := *m.history[start+i]
		history[i] = &stateCopy
	}

	// Calculate pagination metadata
	hasMore := start > 0
	var nextOffset *int
	if hasMore {
		next := offset + pageSize
		nextOffset = &next
	}

	return &HistoryPage{
		History:    history,
		Total:      total,
		Offset:     offset,
		Limit:      limit,
		HasMore:    hasMore,
		NextOffset: nextOffset,
	}
}

// GetHistoryForDevice returns sync history filtered by device ID with pagination
func (m *Manager) GetHistoryForDevice(deviceID string, limit, offset int) ([]*SyncState, PaginationMetadata) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Filter history by device ID
	var filtered []*SyncState
	for _, state := range m.history {
		if state.DeviceID == deviceID {
			filtered = append(filtered, state)
		}
	}

	total := len(filtered)

	// Apply pagination
	start := offset
	if start > total {
		start = total
	}

	end := start + limit
	if end > total {
		end = total
	}

	// Extract page and create copies
	pageSize := end - start
	result := make([]*SyncState, pageSize)
	for i := 0; i < pageSize; i++ {
		stateCopy := *filtered[start+i]
		result[i] = &stateCopy
	}

	// Build metadata
	hasMore := end < total
	var nextOffset *int
	if hasMore {
		next := end
		nextOffset = &next
	}

	metadata := PaginationMetadata{
		Total:      total,
		Offset:     offset,
		Limit:      limit,
		HasMore:    hasMore,
		NextOffset: nextOffset,
	}

	return result, metadata
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

	if m.store != nil {
		if err := m.store.AppendSyncHistory(syncStateToRecord(state), m.maxHistory); err != nil {
			// Persistence is best-effort: the in-memory history (this
			// process's source of truth until restart) already has the
			// entry, so a DB write failure only risks losing it across
			// the next restart, not right now.
			log.Error().Err(err).Str("device_id", state.DeviceID).Msg("Failed to persist sync history")
		}
	}
}

// DeviceAlreadySyncingError is returned when attempting to start a sync for a device that's already syncing
type DeviceAlreadySyncingError struct {
	DeviceID string
}

func (e *DeviceAlreadySyncingError) Error() string {
	return "device " + e.DeviceID + " is already syncing"
}
