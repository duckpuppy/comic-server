# Device Detail Page Implementation Plan

> **For Claude:** Use `${SUPERPOWERS_SKILLS_ROOT}/skills/collaboration/executing-plans/SKILL.md` to implement this plan task-by-task.

**Goal:** Add comprehensive device detail page showing device info, assigned smart lists with settings, and paginated sync history.

**Architecture:** Modular approach with two backend endpoints (`GET /api/devices/:deviceId` for core info, `GET /api/devices/:deviceId/sync-history` for paginated history), new frontend component with lazy loading for sync history, client-side routing using existing History API router.

**Tech Stack:** Go (backend API), Vanilla JavaScript (frontend), existing router.js, syncstate.Manager for history filtering.

---

## Phase 1: Backend - Sync History Filtering

### Task 1: Add Device History Filter Method

**Files:**
- Modify: `internal/syncstate/manager.go`
- Create: `internal/syncstate/manager_test.go` (if not exists, otherwise modify)

**Step 1: Write the failing test**

Add to `internal/syncstate/manager_test.go`:

```go
func TestGetHistoryForDevice(t *testing.T) {
	manager := NewManager()

	// Add history for multiple devices
	session1 := SyncSession{
		DeviceID:   "device-1",
		DeviceName: "Tablet 1",
		StartTime:  time.Now().Add(-3 * time.Hour),
		EndTime:    time.Now().Add(-2 * time.Hour),
		Status:     "completed",
	}
	session2 := SyncSession{
		DeviceID:   "device-2",
		DeviceName: "Tablet 2",
		StartTime:  time.Now().Add(-2 * time.Hour),
		EndTime:    time.Now().Add(-1 * time.Hour),
		Status:     "completed",
	}
	session3 := SyncSession{
		DeviceID:   "device-1",
		DeviceName: "Tablet 1",
		StartTime:  time.Now().Add(-1 * time.Hour),
		EndTime:    time.Now(),
		Status:     "completed",
	}

	manager.AddHistory(session1)
	manager.AddHistory(session2)
	manager.AddHistory(session3)

	// Get history for device-1
	history, metadata := manager.GetHistoryForDevice("device-1", 10, 0)

	if len(history) != 2 {
		t.Errorf("Expected 2 sessions for device-1, got %d", len(history))
	}

	if metadata.Total != 2 {
		t.Errorf("Expected total 2, got %d", metadata.Total)
	}

	if metadata.HasMore {
		t.Errorf("Expected HasMore false, got true")
	}

	// Verify most recent first
	if history[0].StartTime.Before(history[1].StartTime) {
		t.Error("Expected most recent session first")
	}
}

func TestGetHistoryForDevice_Pagination(t *testing.T) {
	manager := NewManager()

	// Add 5 sessions for same device
	for i := 0; i < 5; i++ {
		session := SyncSession{
			DeviceID:   "device-1",
			DeviceName: "Tablet",
			StartTime:  time.Now().Add(time.Duration(-i) * time.Hour),
			EndTime:    time.Now().Add(time.Duration(-i) * time.Hour).Add(30 * time.Minute),
			Status:     "completed",
		}
		manager.AddHistory(session)
	}

	// Get first page (limit 2)
	history, metadata := manager.GetHistoryForDevice("device-1", 2, 0)

	if len(history) != 2 {
		t.Errorf("Expected 2 sessions, got %d", len(history))
	}

	if metadata.Total != 5 {
		t.Errorf("Expected total 5, got %d", metadata.Total)
	}

	if !metadata.HasMore {
		t.Error("Expected HasMore true")
	}

	if metadata.NextOffset == nil || *metadata.NextOffset != 2 {
		t.Errorf("Expected NextOffset 2, got %v", metadata.NextOffset)
	}

	// Get second page
	history2, metadata2 := manager.GetHistoryForDevice("device-1", 2, 2)

	if len(history2) != 2 {
		t.Errorf("Expected 2 sessions on page 2, got %d", len(history2))
	}

	if !metadata2.HasMore {
		t.Error("Expected HasMore true on page 2")
	}

	// Get last page
	history3, metadata3 := manager.GetHistoryForDevice("device-1", 2, 4)

	if len(history3) != 1 {
		t.Errorf("Expected 1 session on last page, got %d", len(history3))
	}

	if metadata3.HasMore {
		t.Error("Expected HasMore false on last page")
	}

	if metadata3.NextOffset != nil {
		t.Error("Expected NextOffset nil on last page")
	}
}

func TestGetHistoryForDevice_NoHistory(t *testing.T) {
	manager := NewManager()

	history, metadata := manager.GetHistoryForDevice("nonexistent", 10, 0)

	if len(history) != 0 {
		t.Errorf("Expected empty history, got %d sessions", len(history))
	}

	if metadata.Total != 0 {
		t.Errorf("Expected total 0, got %d", metadata.Total)
	}

	if metadata.HasMore {
		t.Error("Expected HasMore false")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/syncstate -v -run TestGetHistoryForDevice`
Expected: FAIL with "undefined: Manager.GetHistoryForDevice"

**Step 3: Implement GetHistoryForDevice method**

Add to `internal/syncstate/manager.go`:

```go
// PaginationMetadata contains pagination information
type PaginationMetadata struct {
	Total      int  `json:"total"`
	Offset     int  `json:"offset"`
	Limit      int  `json:"limit"`
	HasMore    bool `json:"has_more"`
	NextOffset *int `json:"next_offset,omitempty"`
}

// GetHistoryForDevice returns sync history filtered by device ID with pagination
func (m *Manager) GetHistoryForDevice(deviceID string, limit, offset int) ([]SyncSession, PaginationMetadata) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Filter history by device ID
	var filtered []SyncSession
	for _, session := range m.history {
		if session.DeviceID == deviceID {
			filtered = append(filtered, session)
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

	result := filtered[start:end]

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
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/syncstate -v -run TestGetHistoryForDevice`
Expected: PASS (3 tests)

**Step 5: Commit**

```bash
git add internal/syncstate/manager.go internal/syncstate/manager_test.go
git commit -m "feat: add device history filtering to syncstate.Manager"
```

---

## Phase 2: Backend - Device Detail Endpoints

### Task 2: Create Device API Handlers

**Files:**
- Create: `internal/api/devices.go`
- Create: `internal/api/devices_test.go`

**Step 1: Write the failing test for GET /api/devices/:deviceId**

Create `internal/api/devices_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/device"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/syncstate"
)

func TestHandleGetDeviceDetail(t *testing.T) {
	// Create test registry with device
	registry := device.NewRegistry()
	deviceInfo := &device.Info{
		ID:           "device-123",
		Name:         "Test Tablet",
		Model:        "SM-T970",
		Manufacturer: "Samsung",
		Edition:      "Android Full",
	}
	registry.Register(deviceInfo, "192.168.1.100")

	// Create test config with assigned lists
	cfg := &config.Config{
		Devices: map[string]*config.DeviceConfig{
			"device-123": {
				DeviceID:     "device-123",
				FriendlyName: "My Tablet",
				Lists: []config.SharedListConfig{
					{
						ListID:   "list-1",
						ListName: "Currently Reading",
						Enabled:  true,
					},
				},
			},
		},
	}

	// Create test library with lists
	lib := &library.ComicLibrary{
		ComicLists: []library.ComicListItem{
			{
				ID:   "list-1",
				Name: "Currently Reading",
				Type: "ComicSmartListItem",
			},
		},
	}

	// Create cache
	cache := library.NewListCache(5 * time.Minute)
	cache.SetCount("list-1", 45)

	server := &Server{
		registry:    registry,
		config:      cfg,
		library:     lib,
		listCache:   cache,
		syncManager: syncstate.NewManager(),
	}

	req := httptest.NewRequest("GET", "/api/devices/device-123", nil)
	w := httptest.NewRecorder()

	server.handleGetDeviceDetail(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		FriendlyName string `json:"friendly_name"`
		Lists        []struct {
			ListID    string `json:"list_id"`
			ListName  string `json:"list_name"`
			Enabled   bool   `json:"enabled"`
			BookCount int    `json:"book_count"`
		} `json:"lists"`
	}

	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.ID != "device-123" {
		t.Errorf("Expected ID device-123, got %s", response.ID)
	}

	if response.FriendlyName != "My Tablet" {
		t.Errorf("Expected friendly name 'My Tablet', got %s", response.FriendlyName)
	}

	if len(response.Lists) != 1 {
		t.Errorf("Expected 1 list, got %d", len(response.Lists))
	}

	if response.Lists[0].BookCount != 45 {
		t.Errorf("Expected book count 45, got %d", response.Lists[0].BookCount)
	}
}

func TestHandleGetDeviceDetail_NotFound(t *testing.T) {
	registry := device.NewRegistry()
	cfg := &config.Config{
		Devices: map[string]*config.DeviceConfig{},
	}

	server := &Server{
		registry:    registry,
		config:      cfg,
		syncManager: syncstate.NewManager(),
	}

	req := httptest.NewRequest("GET", "/api/devices/nonexistent", nil)
	w := httptest.NewRecorder()

	server.handleGetDeviceDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestHandleGetDeviceDetail_OfflineDevice(t *testing.T) {
	// Device in config but not in registry (offline)
	registry := device.NewRegistry()
	cfg := &config.Config{
		Devices: map[string]*config.DeviceConfig{
			"device-123": {
				DeviceID:     "device-123",
				FriendlyName: "Offline Tablet",
				Lists:        []config.SharedListConfig{},
			},
		},
	}

	server := &Server{
		registry:    registry,
		config:      cfg,
		library:     &library.ComicLibrary{},
		listCache:   library.NewListCache(5 * time.Minute),
		syncManager: syncstate.NewManager(),
	}

	req := httptest.NewRequest("GET", "/api/devices/device-123", nil)
	w := httptest.NewRecorder()

	server.handleGetDeviceDetail(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response struct {
		ID           string `json:"id"`
		FriendlyName string `json:"friendly_name"`
	}

	json.NewDecoder(w.Body).Decode(&response)

	if response.FriendlyName != "Offline Tablet" {
		t.Errorf("Expected friendly name 'Offline Tablet', got %s", response.FriendlyName)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api -v -run TestHandleGetDeviceDetail`
Expected: FAIL with "undefined: Server.handleGetDeviceDetail"

**Step 3: Implement handleGetDeviceDetail**

Create `internal/api/devices.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/duckpuppy/comic-server/internal/log"
)

// DeviceDetail represents full device information
type DeviceDetail struct {
	// Device registry info
	ID           string `json:"id"`
	IP           string `json:"ip"`
	Name         string `json:"name"`
	Model        string `json:"model"`
	Manufacturer string `json:"manufacturer"`
	Edition      string `json:"edition"`
	LastSeen     string `json:"last_seen"`

	// Config info
	FriendlyName string             `json:"friendly_name"`
	Lists        []AssignedListInfo `json:"lists"`

	// Current status
	IsSyncing bool `json:"is_syncing"`
}

// AssignedListInfo represents a smart list assigned to a device
type AssignedListInfo struct {
	ListID    string `json:"list_id"`
	ListName  string `json:"list_name"`
	Enabled   bool   `json:"enabled"`
	BookCount int    `json:"book_count"`
}

// handleGetDeviceDetail returns detailed info for a specific device
func (s *Server) handleGetDeviceDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract device ID from URL path
	// URL: /api/devices/:deviceId
	deviceID := r.URL.Path[len("/api/devices/"):]
	if idx := strings.Index(deviceID, "/"); idx != -1 {
		deviceID = deviceID[:idx]
	}

	// Look up device in registry
	var detail DeviceDetail
	device, exists := s.registry.GetDevice(deviceID)

	// Also check config
	s.configMu.RLock()
	deviceConfig, inConfig := s.config.Devices[deviceID]
	s.configMu.RUnlock()

	// Device must be in registry OR config
	if !exists && !inConfig {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	// Populate basic info from registry (if available)
	if exists {
		detail.ID = device.ID
		detail.IP = device.IP
		detail.Name = device.Name
		detail.Model = device.Model
		detail.Manufacturer = device.Manufacturer
		detail.Edition = device.Edition
		detail.LastSeen = device.LastSeen.Format("2006-01-02T15:04:05Z07:00")
	} else {
		// Offline device - only have config info
		detail.ID = deviceID
		detail.Name = deviceConfig.FriendlyName
	}

	// Populate config info
	if inConfig {
		detail.FriendlyName = deviceConfig.FriendlyName

		// Build assigned lists with book counts
		detail.Lists = make([]AssignedListInfo, 0, len(deviceConfig.Lists))
		for _, listConfig := range deviceConfig.Lists {
			// Get book count from cache
			count, found := s.listCache.GetCount(listConfig.ListID)
			if !found && s.library != nil {
				// Not in cache - evaluate and cache
				for i := range s.library.ComicLists {
					if s.library.ComicLists[i].ID == listConfig.ListID {
						matches := s.library.EvaluateSmartList(&s.library.ComicLists[i])
						count = len(matches)
						s.listCache.SetCount(listConfig.ListID, count)
						break
					}
				}
			}

			detail.Lists = append(detail.Lists, AssignedListInfo{
				ListID:    listConfig.ListID,
				ListName:  listConfig.ListName,
				Enabled:   listConfig.Enabled,
				BookCount: count,
			})
		}
	}

	// Check if currently syncing
	detail.IsSyncing = s.syncManager.IsDeviceSyncing(deviceID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/api -v -run TestHandleGetDeviceDetail`
Expected: PASS (3 tests)

**Step 5: Commit**

```bash
git add internal/api/devices.go internal/api/devices_test.go
git commit -m "feat: add GET /api/devices/:deviceId endpoint"
```

---

### Task 3: Add Device Sync History Endpoint

**Files:**
- Modify: `internal/api/devices.go`
- Modify: `internal/api/devices_test.go`

**Step 1: Write the failing test**

Add to `internal/api/devices_test.go`:

```go
func TestHandleGetDeviceSyncHistory(t *testing.T) {
	manager := syncstate.NewManager()

	// Add sync history
	session1 := syncstate.SyncSession{
		DeviceID:   "device-123",
		DeviceName: "Test Tablet",
		StartTime:  time.Now().Add(-2 * time.Hour),
		EndTime:    time.Now().Add(-1 * time.Hour),
		Status:     "completed",
		FilesAdded: 10,
	}
	session2 := syncstate.SyncSession{
		DeviceID:   "device-123",
		DeviceName: "Test Tablet",
		StartTime:  time.Now().Add(-1 * time.Hour),
		EndTime:    time.Now(),
		Status:     "completed",
		FilesAdded: 5,
	}
	manager.AddHistory(session1)
	manager.AddHistory(session2)

	server := &Server{
		syncManager: manager,
	}

	req := httptest.NewRequest("GET", "/api/devices/device-123/sync-history?limit=10&offset=0", nil)
	w := httptest.NewRecorder()

	server.handleGetDeviceSyncHistory(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response struct {
		History []struct {
			DeviceID string `json:"device_id"`
			Status   string `json:"status"`
		} `json:"history"`
		Total   int  `json:"total"`
		HasMore bool `json:"has_more"`
	}

	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(response.History) != 2 {
		t.Errorf("Expected 2 history entries, got %d", len(response.History))
	}

	if response.Total != 2 {
		t.Errorf("Expected total 2, got %d", response.Total)
	}

	if response.HasMore {
		t.Error("Expected HasMore false")
	}
}

func TestHandleGetDeviceSyncHistory_Pagination(t *testing.T) {
	manager := syncstate.NewManager()

	// Add 15 sessions
	for i := 0; i < 15; i++ {
		session := syncstate.SyncSession{
			DeviceID:   "device-123",
			DeviceName: "Test Tablet",
			StartTime:  time.Now().Add(time.Duration(-i) * time.Hour),
			EndTime:    time.Now().Add(time.Duration(-i) * time.Hour).Add(30 * time.Minute),
			Status:     "completed",
		}
		manager.AddHistory(session)
	}

	server := &Server{
		syncManager: manager,
	}

	req := httptest.NewRequest("GET", "/api/devices/device-123/sync-history?limit=10&offset=0", nil)
	w := httptest.NewRecorder()

	server.handleGetDeviceSyncHistory(w, req)

	var response struct {
		History    []interface{} `json:"history"`
		Total      int           `json:"total"`
		HasMore    bool          `json:"has_more"`
		NextOffset *int          `json:"next_offset,omitempty"`
	}

	json.NewDecoder(w.Body).Decode(&response)

	if len(response.History) != 10 {
		t.Errorf("Expected 10 history entries, got %d", len(response.History))
	}

	if response.Total != 15 {
		t.Errorf("Expected total 15, got %d", response.Total)
	}

	if !response.HasMore {
		t.Error("Expected HasMore true")
	}

	if response.NextOffset == nil || *response.NextOffset != 10 {
		t.Errorf("Expected NextOffset 10, got %v", response.NextOffset)
	}
}

func TestHandleGetDeviceSyncHistory_InvalidLimit(t *testing.T) {
	server := &Server{
		syncManager: syncstate.NewManager(),
	}

	req := httptest.NewRequest("GET", "/api/devices/device-123/sync-history?limit=100", nil)
	w := httptest.NewRecorder()

	server.handleGetDeviceSyncHistory(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api -v -run TestHandleGetDeviceSyncHistory`
Expected: FAIL with "undefined: Server.handleGetDeviceSyncHistory"

**Step 3: Implement handleGetDeviceSyncHistory**

Add to `internal/api/devices.go`:

```go
// handleGetDeviceSyncHistory returns paginated sync history for a device
func (s *Server) handleGetDeviceSyncHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract device ID from URL
	path := r.URL.Path
	deviceID := path[len("/api/devices/"):]
	if idx := strings.Index(deviceID, "/"); idx != -1 {
		deviceID = deviceID[:idx]
	}

	// Parse query parameters
	query := r.URL.Query()
	limit := 10 // Default
	if l := query.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
			if limit > 50 {
				http.Error(w, "Limit cannot exceed 50", http.StatusBadRequest)
				return
			}
		}
	}

	offset := 0
	if o := query.Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	// Get filtered history
	history, metadata := s.syncManager.GetHistoryForDevice(deviceID, limit, offset)

	response := map[string]interface{}{
		"history":     history,
		"total":       metadata.Total,
		"limit":       metadata.Limit,
		"offset":      metadata.Offset,
		"has_more":    metadata.HasMore,
		"next_offset": metadata.NextOffset,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
```

Add import at top of `devices.go`:

```go
import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/duckpuppy/comic-server/internal/log"
)
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/api -v -run TestHandleGetDeviceSyncHistory`
Expected: PASS (3 tests)

**Step 5: Commit**

```bash
git add internal/api/devices.go internal/api/devices_test.go
git commit -m "feat: add GET /api/devices/:deviceId/sync-history endpoint"
```

---

### Task 4: Register Device Routes

**Files:**
- Modify: `internal/api/api.go`

**Step 1: Add route registration**

Find the route registration section in `internal/api/api.go` (look for where `/api/devices` is registered) and add the device detail routes:

```go
// Existing: http.HandleFunc("/api/devices", s.handleGetDevices)

// Add device detail routes
http.HandleFunc("/api/devices/", func(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// /api/devices/:deviceId
	if !strings.Contains(path[len("/api/devices/"):], "/") {
		s.handleGetDeviceDetail(w, r)
		return
	}

	// /api/devices/:deviceId/sync-history
	if strings.HasSuffix(path, "/sync-history") {
		s.handleGetDeviceSyncHistory(w, r)
		return
	}

	http.NotFound(w, r)
})
```

**Step 2: Verify routes work**

Run integration test:

```bash
go run main.go server --library testdata/ComicDb.xml &
SERVER_PID=$!
sleep 2

# Test device detail endpoint (use actual device ID from your config)
curl -s http://localhost:7620/api/devices/device-123 | jq '.friendly_name'

# Test sync history endpoint
curl -s http://localhost:7620/api/devices/device-123/sync-history | jq '.total'

# Cleanup
kill $SERVER_PID
```

Expected: Endpoints return JSON without errors

**Step 3: Commit**

```bash
git add internal/api/api.go
git commit -m "feat: register device detail API routes"
```

---

## Phase 3: Frontend - Device Detail Component

### Task 5: Create DeviceDetail Component

**Files:**
- Create: `internal/api/web/js/deviceDetail.js`

**Step 1: Create DeviceDetail class**

Create `internal/api/web/js/deviceDetail.js`:

```javascript
// Device Detail Page
class DeviceDetail {
    constructor(deviceId) {
        this.deviceId = deviceId;
        this.device = null;
        this.syncHistory = [];
        this.historyOffset = 0;
        this.historyLimit = 10;
        this.historyTotal = 0;
        this.historyLoading = false;
    }

    async init() {
        await this.loadDeviceInfo();
        if (this.device) {
            this.render();
            this.attachListeners();
            await this.loadSyncHistory();
        }
    }

    async loadDeviceInfo() {
        try {
            const response = await fetch(`/api/devices/${this.deviceId}`);

            if (response.status === 404) {
                this.showError("Device not found. It may have been unregistered.");
                return;
            }

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            this.device = await response.json();
        } catch (error) {
            console.error('Failed to load device:', error);
            this.showError("Failed to load device information. Please try again.");
        }
    }

    async loadSyncHistory() {
        if (this.historyLoading) return;

        this.historyLoading = true;
        this.renderHistoryLoading();

        try {
            const url = `/api/devices/${this.deviceId}/sync-history?limit=${this.historyLimit}&offset=${this.historyOffset}`;
            const response = await fetch(url);

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            const data = await response.json();

            if (this.historyOffset === 0) {
                this.syncHistory = data.history || [];
            } else {
                this.syncHistory = [...this.syncHistory, ...(data.history || [])];
            }

            this.historyTotal = data.total || 0;
            this.historyHasMore = data.has_more || false;
            this.historyNextOffset = data.next_offset;

            this.renderSyncHistory();
        } catch (error) {
            console.error('Failed to load sync history:', error);
            this.renderHistoryError();
        } finally {
            this.historyLoading = false;
        }
    }

    render() {
        const app = document.getElementById('app');

        app.innerHTML = `
            <div class="device-detail-page">
                <!-- Breadcrumb -->
                <nav class="breadcrumb">
                    <a href="/" onclick="router.navigate('/'); return false;">Dashboard</a>
                    <span class="separator">›</span>
                    <span class="current">${this.escapeHtml(this.device.friendly_name || this.device.name)}</span>
                </nav>

                <!-- Device Header -->
                <div class="device-detail-header">
                    <div class="device-header-main">
                        <h1>${this.escapeHtml(this.device.friendly_name || this.device.name)}</h1>
                        ${this.renderStatusBadge()}
                    </div>
                </div>

                <!-- Device Info Cards -->
                <div class="device-info-cards">
                    ${this.renderInfoCards()}
                </div>

                <!-- Assigned Lists Panel -->
                <div class="panel assigned-lists-panel">
                    <h2>Assigned Smart Lists</h2>
                    ${this.renderAssignedLists()}
                </div>

                <!-- Sync History Panel -->
                <div class="panel sync-history-panel">
                    <h2>Sync History</h2>
                    <div id="sync-history-content">
                        <div class="loading-spinner">Loading history...</div>
                    </div>
                </div>
            </div>
        `;
    }

    renderStatusBadge() {
        if (this.device.is_syncing) {
            return '<span class="status-badge syncing">Syncing</span>';
        }

        // Check last_seen to determine online/offline
        if (this.device.last_seen) {
            const lastSeen = new Date(this.device.last_seen);
            const minutesAgo = (Date.now() - lastSeen.getTime()) / 1000 / 60;

            if (minutesAgo < 2) {
                return '<span class="status-badge online">Online</span>';
            } else if (minutesAgo < 30) {
                return '<span class="status-badge idle">Idle</span>';
            }
        }

        return '<span class="status-badge offline">Offline</span>';
    }

    renderInfoCards() {
        return `
            <div class="info-card">
                <div class="info-card-label">Model</div>
                <div class="info-card-value">${this.escapeHtml(this.device.model || 'Unknown')}</div>
            </div>
            <div class="info-card">
                <div class="info-card-label">Manufacturer</div>
                <div class="info-card-value">${this.escapeHtml(this.device.manufacturer || 'Unknown')}</div>
            </div>
            <div class="info-card">
                <div class="info-card-label">Edition</div>
                <div class="info-card-value">${this.escapeHtml(this.device.edition || 'Unknown')}</div>
            </div>
            <div class="info-card">
                <div class="info-card-label">IP Address</div>
                <div class="info-card-value">${this.escapeHtml(this.device.ip || 'Unknown')}</div>
            </div>
            <div class="info-card">
                <div class="info-card-label">Last Seen</div>
                <div class="info-card-value">${this.formatTimestamp(this.device.last_seen)}</div>
            </div>
            <div class="info-card">
                <div class="info-card-label">Device ID</div>
                <div class="info-card-value device-id">${this.escapeHtml(this.device.id)}</div>
            </div>
        `;
    }

    renderAssignedLists() {
        if (!this.device.lists || this.device.lists.length === 0) {
            return `
                <div class="empty-state">
                    <p>No smart lists assigned to this device.</p>
                    <p class="help-text">Use the config command to assign lists.</p>
                </div>
            `;
        }

        return `
            <table class="assigned-lists-table">
                <thead>
                    <tr>
                        <th>List Name</th>
                        <th>Books</th>
                        <th>Status</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody>
                    ${this.device.lists.map(list => `
                        <tr>
                            <td>
                                <a href="/lists/${list.list_id}"
                                   onclick="router.navigate('/lists/${list.list_id}'); return false;"
                                   class="list-link">
                                    ${this.escapeHtml(list.list_name)}
                                </a>
                            </td>
                            <td>${list.book_count.toLocaleString()}</td>
                            <td>
                                <span class="status-indicator ${list.enabled ? 'enabled' : 'disabled'}">
                                    ${list.enabled ? 'Enabled' : 'Disabled'}
                                </span>
                            </td>
                            <td>
                                <button class="btn btn-small"
                                        onclick="router.navigate('/lists/${list.list_id}'); return false;">
                                    View List
                                </button>
                            </td>
                        </tr>
                    `).join('')}
                </tbody>
            </table>
        `;
    }

    renderHistoryLoading() {
        const content = document.getElementById('sync-history-content');
        if (content) {
            content.innerHTML = '<div class="loading-spinner">Loading history...</div>';
        }
    }

    renderHistoryError() {
        const content = document.getElementById('sync-history-content');
        if (content) {
            content.innerHTML = `
                <div class="error-message">
                    <p>Failed to load sync history.</p>
                    <button class="btn btn-secondary" onclick="location.reload()">Retry</button>
                </div>
            `;
        }
    }

    renderSyncHistory() {
        const content = document.getElementById('sync-history-content');
        if (!content) return;

        if (this.syncHistory.length === 0) {
            content.innerHTML = `
                <div class="empty-state">
                    <p>No sync history yet.</p>
                    <p class="help-text">Sync history will appear here after the first sync.</p>
                </div>
            `;
            return;
        }

        content.innerHTML = `
            <div class="sync-history-list">
                ${this.syncHistory.map(session => this.renderHistoryItem(session)).join('')}
            </div>
            ${this.renderLoadMoreButton()}
        `;
    }

    renderHistoryItem(session) {
        const startTime = new Date(session.start_time);
        const endTime = new Date(session.end_time);
        const duration = Math.round((endTime - startTime) / 1000); // seconds

        const statusClass = session.status === 'completed' ? 'success' :
                          session.status === 'failed' ? 'error' : 'warning';

        return `
            <div class="history-item">
                <div class="history-item-header">
                    <span class="history-timestamp">${this.formatTimestamp(session.start_time)}</span>
                    <span class="history-status status-${statusClass}">${session.status}</span>
                </div>
                <div class="history-item-stats">
                    <span class="history-stat">
                        <span class="stat-icon">📚</span>
                        ${session.files_added || 0} added
                    </span>
                    <span class="history-stat">
                        <span class="stat-icon">🔄</span>
                        ${session.files_updated || 0} updated
                    </span>
                    <span class="history-stat">
                        <span class="stat-icon">🗑️</span>
                        ${session.files_deleted || 0} deleted
                    </span>
                    <span class="history-stat">
                        <span class="stat-icon">⏱️</span>
                        ${this.formatDuration(duration)}
                    </span>
                </div>
            </div>
        `;
    }

    renderLoadMoreButton() {
        if (!this.historyHasMore) {
            return '';
        }

        return `
            <div class="load-more-container">
                <button id="load-more-history" class="btn btn-secondary">
                    Load More
                </button>
            </div>
        `;
    }

    attachListeners() {
        // Will attach load more listener after history renders
        this.attachHistoryListeners();
    }

    attachHistoryListeners() {
        const loadMoreBtn = document.getElementById('load-more-history');
        if (loadMoreBtn) {
            loadMoreBtn.addEventListener('click', async () => {
                loadMoreBtn.disabled = true;
                loadMoreBtn.textContent = 'Loading...';

                this.historyOffset = this.historyNextOffset;
                await this.loadSyncHistory();

                // Button will be re-rendered by renderSyncHistory
            });
        }
    }

    showError(message) {
        document.getElementById('app').innerHTML = `
            <div class="error-page">
                <h1>Error</h1>
                <p>${this.escapeHtml(message)}</p>
                <button onclick="router.navigate('/')" class="btn btn-primary">
                    Back to Dashboard
                </button>
            </div>
        `;
    }

    formatTimestamp(timestamp) {
        if (!timestamp) return 'Never';

        const date = new Date(timestamp);
        const now = new Date();
        const diffMs = now - date;
        const diffMins = Math.floor(diffMs / 60000);

        if (diffMins < 1) return 'Just now';
        if (diffMins < 60) return `${diffMins} minute${diffMins !== 1 ? 's' : ''} ago`;

        const diffHours = Math.floor(diffMins / 60);
        if (diffHours < 24) return `${diffHours} hour${diffHours !== 1 ? 's' : ''} ago`;

        const diffDays = Math.floor(diffHours / 24);
        if (diffDays < 7) return `${diffDays} day${diffDays !== 1 ? 's' : ''} ago`;

        // Format as date
        return date.toLocaleDateString() + ' ' + date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    }

    formatDuration(seconds) {
        if (seconds < 60) return `${seconds}s`;

        const mins = Math.floor(seconds / 60);
        const secs = seconds % 60;
        return `${mins}m ${secs}s`;
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text || '';
        return div.innerHTML;
    }
}
```

**Step 2: Verify file is created**

Run: `ls -l internal/api/web/js/deviceDetail.js`
Expected: File exists

**Step 3: Commit**

```bash
git add internal/api/web/js/deviceDetail.js
git commit -m "feat: add DeviceDetail component"
```

---

### Task 6: Add Device Detail Styling

**Files:**
- Create: `internal/api/web/css/deviceDetail.css`

**Step 1: Create CSS file**

Create `internal/api/web/css/deviceDetail.css`:

```css
/* Device Detail Page */
.device-detail-page {
    padding: 2rem;
    max-width: 1200px;
    margin: 0 auto;
}

/* Device Header */
.device-detail-header {
    margin-bottom: 2rem;
}

.device-header-main {
    display: flex;
    align-items: center;
    gap: 1rem;
}

.device-detail-header h1 {
    font-size: 2rem;
    font-weight: 700;
    margin: 0;
}

.status-badge {
    display: inline-block;
    padding: 0.375rem 0.75rem;
    border-radius: 0.375rem;
    font-size: 0.875rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.025em;
}

.status-badge.online {
    background: #d1fae5;
    color: #065f46;
}

.status-badge.idle {
    background: #fef3c7;
    color: #92400e;
}

.status-badge.offline {
    background: #fee2e2;
    color: #991b1b;
}

.status-badge.syncing {
    background: #dbeafe;
    color: #1e40af;
}

/* Device Info Cards */
.device-info-cards {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 1rem;
    margin-bottom: 2rem;
}

.info-card {
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 0.5rem;
    padding: 1.25rem;
}

.info-card-label {
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-secondary);
    margin-bottom: 0.5rem;
}

.info-card-value {
    font-size: 1rem;
    font-weight: 600;
    color: var(--text-primary);
    word-break: break-all;
}

.info-card-value.device-id {
    font-family: 'Monaco', 'Menlo', monospace;
    font-size: 0.875rem;
}

/* Assigned Lists Panel */
.assigned-lists-panel {
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 0.5rem;
    padding: 1.5rem;
    margin-bottom: 2rem;
}

.assigned-lists-panel h2 {
    font-size: 1.25rem;
    font-weight: 700;
    margin-bottom: 1.5rem;
}

.assigned-lists-table {
    width: 100%;
    border-collapse: collapse;
}

.assigned-lists-table thead {
    background: var(--bg-primary);
    border-bottom: 2px solid var(--border-color);
}

.assigned-lists-table th {
    text-align: left;
    padding: 0.75rem 1rem;
    font-size: 0.875rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-secondary);
}

.assigned-lists-table td {
    padding: 1rem;
    border-bottom: 1px solid var(--border-color);
}

.assigned-lists-table tbody tr:hover {
    background: var(--bg-primary);
}

.list-link {
    color: var(--primary-color);
    text-decoration: none;
    font-weight: 500;
}

.list-link:hover {
    text-decoration: underline;
}

.status-indicator {
    display: inline-block;
    padding: 0.25rem 0.75rem;
    border-radius: 0.25rem;
    font-size: 0.75rem;
    font-weight: 600;
}

.status-indicator.enabled {
    background: #d1fae5;
    color: #065f46;
}

.status-indicator.disabled {
    background: #f3f4f6;
    color: #6b7280;
}

/* Sync History Panel */
.sync-history-panel {
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 0.5rem;
    padding: 1.5rem;
}

.sync-history-panel h2 {
    font-size: 1.25rem;
    font-weight: 700;
    margin-bottom: 1.5rem;
}

.sync-history-list {
    display: flex;
    flex-direction: column;
    gap: 1rem;
}

.history-item {
    background: var(--bg-primary);
    border: 1px solid var(--border-color);
    border-radius: 0.375rem;
    padding: 1rem;
}

.history-item-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 0.75rem;
}

.history-timestamp {
    font-size: 0.875rem;
    font-weight: 500;
    color: var(--text-primary);
}

.history-status {
    padding: 0.25rem 0.75rem;
    border-radius: 0.25rem;
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
}

.history-status.status-success {
    background: #d1fae5;
    color: #065f46;
}

.history-status.status-error {
    background: #fee2e2;
    color: #991b1b;
}

.history-status.status-warning {
    background: #fef3c7;
    color: #92400e;
}

.history-item-stats {
    display: flex;
    gap: 1.5rem;
    flex-wrap: wrap;
}

.history-stat {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    font-size: 0.875rem;
    color: var(--text-secondary);
}

.stat-icon {
    font-size: 1rem;
}

.loading-spinner {
    text-align: center;
    padding: 2rem;
    color: var(--text-secondary);
}

.empty-state {
    text-align: center;
    padding: 3rem 2rem;
}

.empty-state p {
    color: var(--text-secondary);
    margin-bottom: 0.5rem;
}

.empty-state .help-text {
    font-size: 0.875rem;
    color: var(--text-tertiary);
}

.error-message {
    text-align: center;
    padding: 2rem;
}

.error-message p {
    color: var(--text-secondary);
    margin-bottom: 1rem;
}

.load-more-container {
    display: flex;
    justify-content: center;
    margin-top: 1.5rem;
}

/* Responsive */
@media (max-width: 768px) {
    .device-detail-page {
        padding: 1rem;
    }

    .device-info-cards {
        grid-template-columns: 1fr;
    }

    .assigned-lists-table {
        font-size: 0.875rem;
    }

    .assigned-lists-table th,
    .assigned-lists-table td {
        padding: 0.5rem;
    }

    .history-item-stats {
        gap: 1rem;
    }
}
```

**Step 2: Commit**

```bash
git add internal/api/web/css/deviceDetail.css
git commit -m "feat: add device detail page styling"
```

---

### Task 7: Register Device Detail Route

**Files:**
- Modify: `internal/api/web/index.html`
- Modify: `internal/api/web/js/app.js`

**Step 1: Add CSS to index.html**

Add to the `<head>` section of `internal/api/web/index.html`:

```html
<link rel="stylesheet" href="/css/deviceDetail.css">
```

Add to the scripts section (before `app.js`):

```html
<script src="/js/deviceDetail.js"></script>
```

**Step 2: Register route in app.js**

Add route registration in `internal/api/web/js/app.js` after existing routes:

```javascript
// Register /devices/:deviceId route
router.register('/devices/:deviceId', async (params) => {
    navigation.setActive('devices');
    dashboard.hide();
    const deviceDetail = new DeviceDetail(params.deviceId);
    await deviceDetail.init();
});
```

**Step 3: Update device card click handler**

In `internal/api/web/js/devices.js`, find the `renderDevice` method and update device card click to navigate to detail page:

Find this code:
```javascript
card.addEventListener('click', (e) => {
    // Existing click handler
});
```

Replace with:
```javascript
card.addEventListener('click', (e) => {
    // Don't navigate if clicking buttons
    if (e.target.tagName === 'BUTTON') {
        return;
    }
    router.navigate(`/devices/${device.id}`);
});
```

**Step 4: Commit**

```bash
git add internal/api/web/index.html internal/api/web/js/app.js internal/api/web/js/devices.js
git commit -m "feat: register device detail route and navigation"
```

---

## Phase 4: Testing & Documentation

### Task 8: Update API Documentation

**Files:**
- Modify: `docs/API.md`

**Step 1: Add device detail endpoints documentation**

Find the `### GET /api/devices` section in `docs/API.md` and add after it:

```markdown
---

### GET /api/devices/:deviceId

Get detailed information about a specific device.

**Path Parameters:**
- `deviceId` (string, required) - Device identifier

**Response:**
```json
{
  "id": "SM-T970",
  "ip": "192.168.0.100",
  "name": "Samsung Galaxy Tab",
  "model": "SM-T970",
  "manufacturer": "Samsung",
  "edition": "Android Full",
  "last_seen": "2025-01-15T14:30:00Z",
  "friendly_name": "My Tablet",
  "lists": [
    {
      "list_id": "list-guid-123",
      "list_name": "Currently Reading",
      "enabled": true,
      "book_count": 45
    }
  ],
  "is_syncing": false
}
```

**Fields:**
- `id` (string) - Device identifier
- `ip` (string) - Device IP address (if online)
- `name` (string) - Device name
- `model` (string) - Device model
- `manufacturer` (string) - Device manufacturer
- `edition` (string) - ComicRack edition
- `last_seen` (string) - Last discovery time (RFC3339)
- `friendly_name` (string) - User-configured friendly name
- `lists` (array) - Assigned smart lists
  - `list_id` (string) - Smart list identifier
  - `list_name` (string) - List display name
  - `enabled` (boolean) - Whether syncing is enabled
  - `book_count` (integer) - Number of comics in list
- `is_syncing` (boolean) - Whether device is currently syncing

**Status Codes:**
- `200 OK` - Success
- `404 Not Found` - Device not found

**Example:**
```bash
curl http://localhost:7620/api/devices/SM-T970
```

**Notes:**
- Returns data for offline devices if they exist in config
- Book counts are cached for 15 minutes

---

### GET /api/devices/:deviceId/sync-history

Get paginated sync history for a specific device.

**Path Parameters:**
- `deviceId` (string, required) - Device identifier

**Query Parameters:**
- `limit` (integer, optional) - Number of entries to return (default: 10, max: 50)
- `offset` (integer, optional) - Pagination offset (default: 0)

**Response:**
```json
{
  "history": [
    {
      "device_id": "SM-T970",
      "device_name": "Samsung Galaxy Tab",
      "start_time": "2025-01-15T14:30:00Z",
      "end_time": "2025-01-15T14:35:00Z",
      "status": "completed",
      "files_added": 10,
      "files_updated": 5,
      "files_deleted": 2
    }
  ],
  "total": 25,
  "limit": 10,
  "offset": 0,
  "has_more": true,
  "next_offset": 10
}
```

**Fields:**
- `history` (array) - Array of sync sessions
  - `device_id` (string) - Device identifier
  - `device_name` (string) - Device name at time of sync
  - `start_time` (string) - Sync start time (RFC3339)
  - `end_time` (string) - Sync end time (RFC3339)
  - `status` (string) - Sync status: "completed", "failed", "aborted"
  - `files_added` (integer) - Number of files added
  - `files_updated` (integer) - Number of files updated
  - `files_deleted` (integer) - Number of files deleted
- `total` (integer) - Total number of history entries for this device
- `limit` (integer) - Number of entries returned
- `offset` (integer) - Pagination offset
- `has_more` (boolean) - Whether more entries exist
- `next_offset` (integer, optional) - Offset for next page

**Status Codes:**
- `200 OK` - Success (empty array if no history)
- `400 Bad Request` - Invalid pagination parameters

**Example:**
```bash
# Get first page
curl http://localhost:7620/api/devices/SM-T970/sync-history?limit=10&offset=0

# Get second page
curl http://localhost:7620/api/devices/SM-T970/sync-history?limit=10&offset=10
```

**Notes:**
- Maximum limit is 50 entries per request
- Most recent syncs returned first
- Returns empty array for devices with no sync history
```

**Step 2: Commit**

```bash
git add docs/API.md
git commit -m "docs: add device detail API endpoints"
```

---

### Task 9: Manual Testing Verification

**Files:**
- None (manual testing)

**Step 1: Start the server**

```bash
just build
./comic-server server --library /path/to/ComicDb.xml
```

**Step 2: Manual testing checklist**

Verify in browser at http://localhost:7620:

**Navigation:**
- [ ] Click device from sidebar → navigates to `/devices/:deviceId`
- [ ] URL updates correctly
- [ ] Breadcrumb shows "Dashboard › Device Name"
- [ ] Browser back button works

**Device Info:**
- [ ] Header shows device name and status badge
- [ ] Info cards show all device properties
- [ ] Status badge shows correct state (online/offline/syncing)
- [ ] Timestamps formatted as relative time

**Assigned Lists:**
- [ ] Table shows all assigned lists
- [ ] Book counts display correctly
- [ ] Enabled/disabled status visible
- [ ] Click list name → navigates to list detail page
- [ ] Empty state shows when no lists assigned

**Sync History:**
- [ ] History loads after page renders (lazy)
- [ ] Shows most recent syncs first
- [ ] Each entry shows stats (added/updated/deleted/duration)
- [ ] "Load More" button appears if more than 10 entries
- [ ] Clicking "Load More" fetches next page
- [ ] Empty state shows when no history

**Error Handling:**
- [ ] Invalid device ID → shows error page with back button
- [ ] Offline device → shows with offline badge
- [ ] Network error → shows retry option

**Step 3: Test API endpoints directly**

```bash
# Test device detail
curl http://localhost:7620/api/devices/YOUR-DEVICE-ID | jq

# Test sync history
curl http://localhost:7620/api/devices/YOUR-DEVICE-ID/sync-history | jq

# Test pagination
curl "http://localhost:7620/api/devices/YOUR-DEVICE-ID/sync-history?limit=5&offset=0" | jq
```

**Step 4: Verify all tests still pass**

```bash
go test ./... -v
```

Expected: All tests pass

**Step 5: Final verification**

Create a checklist of what was built:
- [ ] Backend: GetHistoryForDevice method with tests
- [ ] Backend: GET /api/devices/:deviceId endpoint with tests
- [ ] Backend: GET /api/devices/:deviceId/sync-history endpoint with tests
- [ ] Backend: Routes registered
- [ ] Frontend: DeviceDetail component
- [ ] Frontend: Device detail CSS
- [ ] Frontend: Route registered
- [ ] Frontend: Navigation from device cards
- [ ] Documentation: API.md updated

All items should be checked.

---

## Completion

**Summary:**

This plan implements a comprehensive device detail page with:

1. **Backend (4 tasks):**
   - Sync history filtering by device ID with pagination
   - Device detail endpoint with device info + assigned lists
   - Sync history endpoint with pagination
   - Route registration

2. **Frontend (3 tasks):**
   - DeviceDetail component with lazy loading
   - Responsive CSS styling
   - Route registration and navigation

3. **Documentation (2 tasks):**
   - API endpoint documentation
   - Manual testing verification

**Commits:** 9 total commits following conventional commit format

**Testing:** All backend endpoints have unit tests, frontend has manual testing checklist

**Next Steps:** After completing this plan, you can move to the next enhancement (List Assignment Management - POST/DELETE endpoints).
