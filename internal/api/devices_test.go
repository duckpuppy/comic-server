package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/device"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/syncstate"
	ws "github.com/duckpuppy/comic-server/internal/websocket"
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
	registry.Add(deviceInfo, "192.168.1.100")

	cfg := &config.Config{}

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
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)

	// Create cache
	cache := library.NewListCache(5 * time.Minute)
	cache.SetCount("list-1", 45)

	configDB, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("failed to open test config db: %v", err)
	}
	t.Cleanup(func() { configDB.Close() })
	if err := configDB.UpsertDevice("device-123", "My Tablet", time.Time{}, nil); err != nil {
		t.Fatalf("failed to seed test device: %v", err)
	}
	if err := configDB.AddDeviceList("device-123", configdb.DeviceList{ListID: "list-1", ListName: "Currently Reading", Enabled: true}); err != nil {
		t.Fatalf("failed to seed test device list: %v", err)
	}

	server := &Server{
		registry:    registry,
		config:      cfg,
		configDB:    configDB,
		backend:     backend,
		listCache:   cache,
		syncManager: syncstate.NewManager(100),
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
	cfg := &config.Config{}

	configDB, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("failed to open test config db: %v", err)
	}
	t.Cleanup(func() { configDB.Close() })

	server := &Server{
		registry:    registry,
		config:      cfg,
		configDB:    configDB,
		syncManager: syncstate.NewManager(100),
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
	cfg := &config.Config{}

	configDB, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("failed to open test config db: %v", err)
	}
	t.Cleanup(func() { configDB.Close() })
	if err := configDB.UpsertDevice("device-123", "Offline Tablet", time.Time{}, nil); err != nil {
		t.Fatalf("failed to seed test device: %v", err)
	}

	server := &Server{
		registry:    registry,
		config:      cfg,
		configDB:    configDB,
		backend:     library.NewXMLBackendFromLibrary(&library.ComicLibrary{}, "", nil),
		listCache:   library.NewListCache(5 * time.Minute),
		syncManager: syncstate.NewManager(100),
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

func TestHandleDevicesRouter(t *testing.T) {
	// Setup server with minimal config
	registry := device.NewRegistry()
	cfg := &config.Config{}

	configDB, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("failed to open test config db: %v", err)
	}
	t.Cleanup(func() { configDB.Close() })

	server := &Server{
		registry:    registry,
		config:      cfg,
		configDB:    configDB,
		backend:     library.NewXMLBackendFromLibrary(&library.ComicLibrary{}, "", nil),
		listCache:   library.NewListCache(5 * time.Minute),
		syncManager: syncstate.NewManager(100),
	}

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		description    string
	}{
		{
			name:           "Device detail route",
			path:           "/api/devices/device-123",
			expectedStatus: http.StatusNotFound, // Device doesn't exist
			description:    "Should route to handleGetDeviceDetail",
		},
		{
			name:           "Sync history route",
			path:           "/api/devices/device-123/sync-history",
			expectedStatus: http.StatusOK, // Now implemented
			description:    "Should route to handleGetDeviceSyncHistory",
		},
		{
			name:           "Empty device ID",
			path:           "/api/devices/",
			expectedStatus: http.StatusOK, // Routes to handleDevices (list all)
			description:    "Should route to handleDevices for empty ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			server.handleDevicesRouter(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("%s: Expected status %d, got %d", tt.description, tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestHandleGetDeviceDetail_EmptyDeviceID(t *testing.T) {
	registry := device.NewRegistry()
	cfg := &config.Config{
		Devices: map[string]*config.DeviceConfig{},
	}

	server := &Server{
		registry:    registry,
		config:      cfg,
		syncManager: syncstate.NewManager(100),
	}

	// Test with empty device ID (just the trailing slash)
	req := httptest.NewRequest("GET", "/api/devices/", nil)
	w := httptest.NewRecorder()

	server.handleGetDeviceDetail(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for empty device ID, got %d", w.Code)
	}
}

func TestHandleGetDeviceSyncHistory(t *testing.T) {
	manager := syncstate.NewManager(100)

	// Add sync history by starting and completing syncs
	manager.StartSync("device-123", "192.168.1.100", "Test Tablet")
	manager.CompleteSync("device-123", 10, 0, 0, 0)

	manager.StartSync("device-123", "192.168.1.100", "Test Tablet")
	manager.CompleteSync("device-123", 5, 0, 0, 0)

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
	manager := syncstate.NewManager(100)

	// Add 15 sessions by starting and completing syncs
	for i := 0; i < 15; i++ {
		manager.StartSync("device-123", "192.168.1.100", "Test Tablet")
		manager.CompleteSync("device-123", 1, 0, 0, 0)
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
		syncManager: syncstate.NewManager(100),
	}

	req := httptest.NewRequest("GET", "/api/devices/device-123/sync-history?limit=100", nil)
	w := httptest.NewRecorder()

	server.handleGetDeviceSyncHistory(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestHandleDeviceListAdd_AcceptsNonSmartLists is the regression test for
// device wireless sync silently or outright failing when a device is
// assigned a real ID list ("To Read", ComicIdListItem) rather than a smart
// list: handleDeviceListAdd used to reject anything whose Type didn't
// contain "SmartList" with a misleading "Smart list not found in library"
// 404, even though the list existed - and cmd/server.go's
// applyDeviceConfig had the identical restriction, which made that
// device's ENTIRE sync fail outright (not just skip that one list) the
// next time it connected. Both were relaxed under the mistaken belief
// this reflected a real ComicRack wireless-sync protocol constraint;
// confirmed against ComicRackCE's own source (DeviceSyncSettings.SharedList
// stores only an opaque ListId, no type restriction) that no such
// constraint exists. Any list with real book membership (smart list, ID
// list, reading list) should be acceptable; only folders (a grouping of
// other lists, not a set of books) should still be rejected. Same class
// of bug as comic-server-vwl's Komga-target fix.
func TestHandleDeviceListAdd_AcceptsNonSmartLists(t *testing.T) {
	newServer := func() *Server {
		registry := device.NewRegistry()
		registry.Add(&device.Info{ID: "device-1", Name: "Test Tablet"}, "192.168.1.100")

		lib := &library.ComicLibrary{
			ComicLists: []library.ComicListItem{
				{ID: "idlist-1", Name: "To Read", Type: "ComicIdListItem", BookIds: []string{"book-1"}},
				{ID: "readinglist-1", Name: "My Reading List", Type: "ComicReadingList"},
				{ID: "folder-1", Name: "A Folder", Type: "ComicListItemFolder"},
			},
		}
		backend := library.NewXMLBackendFromLibrary(lib, "", nil)

		configDB, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
		if err != nil {
			t.Fatalf("failed to open test config db: %v", err)
		}
		t.Cleanup(func() { configDB.Close() })

		s := &Server{
			backend:           backend,
			registry:          registry,
			registeredDevices: map[string]bool{"device-1": true},
			config:            &config.Config{},
			configDB:          configDB,
			configPath:        filepath.Join(t.TempDir(), "config.yaml"),
			wsHub:             ws.NewHub(),
		}
		return s
	}

	doAdd := func(t *testing.T, s *Server, listID, listName string) *httptest.ResponseRecorder {
		t.Helper()
		body, _ := json.Marshal(AddListRequest{ListID: listID, ListName: listName, Enabled: true})
		req := httptest.NewRequest(http.MethodPost, "/api/devices/lists/device-1", bytes.NewReader(body))
		w := httptest.NewRecorder()
		s.handleDeviceListAdd(w, req)
		return w
	}

	if w := doAdd(t, newServer(), "idlist-1", "To Read"); w.Code != http.StatusOK {
		t.Errorf("expected an ID list to be accepted for device sync, got %d: %s", w.Code, w.Body.String())
	}
	if w := doAdd(t, newServer(), "readinglist-1", "My Reading List"); w.Code != http.StatusOK {
		t.Errorf("expected a reading list to be accepted for device sync, got %d: %s", w.Code, w.Body.String())
	}
	if w := doAdd(t, newServer(), "folder-1", "A Folder"); w.Code != http.StatusNotFound {
		t.Errorf("expected a folder to still be rejected, got %d: %s", w.Code, w.Body.String())
	}
}

func newTriggerSyncTestServer(t *testing.T) *Server {
	t.Helper()
	configDB, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("failed to open test config db: %v", err)
	}
	t.Cleanup(func() { configDB.Close() })

	return &Server{
		registry:    device.NewRegistry(),
		config:      &config.Config{},
		configDB:    configDB,
		backend:     library.NewXMLBackendFromLibrary(&library.ComicLibrary{}, "", nil),
		listCache:   library.NewListCache(5 * time.Minute),
		syncManager: syncstate.NewManager(100),
	}
}

func TestHandleTriggerSync_NoTriggerWired(t *testing.T) {
	server := newTriggerSyncTestServer(t)
	// server.triggerSync intentionally left nil - matches production
	// startup before SetSyncTrigger is called, and any deployment that
	// never wires it.

	req := httptest.NewRequest(http.MethodPost, "/api/devices/device-1/sync", nil)
	w := httptest.NewRecorder()
	server.handleDevicesRouter(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 with no trigger wired, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTriggerSync_DeviceNotConnected(t *testing.T) {
	server := newTriggerSyncTestServer(t)
	server.SetSyncTrigger(func(deviceID string) error {
		return device.ErrNotConnected
	})

	req := httptest.NewRequest(http.MethodPost, "/api/devices/device-1/sync", nil)
	w := httptest.NewRecorder()
	server.handleDevicesRouter(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for a disconnected device, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTriggerSync_AlreadySyncing(t *testing.T) {
	server := newTriggerSyncTestServer(t)
	server.SetSyncTrigger(func(deviceID string) error {
		return &syncstate.DeviceAlreadySyncingError{DeviceID: deviceID}
	})

	req := httptest.NewRequest(http.MethodPost, "/api/devices/device-1/sync", nil)
	w := httptest.NewRecorder()
	server.handleDevicesRouter(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 for a device already syncing, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTriggerSync_Success(t *testing.T) {
	server := newTriggerSyncTestServer(t)
	var gotDeviceID string
	server.SetSyncTrigger(func(deviceID string) error {
		gotDeviceID = deviceID
		return nil
	})

	req := httptest.NewRequest(http.MethodPost, "/api/devices/device-1/sync", nil)
	w := httptest.NewRecorder()
	server.handleDevicesRouter(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d: %s", w.Code, w.Body.String())
	}
	if gotDeviceID != "device-1" {
		t.Errorf("expected triggerSync to be called with device-1, got %q", gotDeviceID)
	}

	var response struct {
		Status   string `json:"status"`
		DeviceID string `json:"device_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response.DeviceID != "device-1" {
		t.Errorf("expected response device_id device-1, got %q", response.DeviceID)
	}
}

func TestHandleTriggerSync_WrongMethod(t *testing.T) {
	server := newTriggerSyncTestServer(t)
	server.SetSyncTrigger(func(deviceID string) error { return nil })

	req := httptest.NewRequest(http.MethodGet, "/api/devices/device-1/sync", nil)
	w := httptest.NewRecorder()
	server.handleDevicesRouter(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleTriggerSync_DoesNotShadowSyncHistoryRoute guards against a
// regression where a broad "/sync" suffix check could accidentally
// swallow the pre-existing "/sync-history" route (both start with
// "/sync", but only one actually ends with it).
func TestHandleTriggerSync_DoesNotShadowSyncHistoryRoute(t *testing.T) {
	server := newTriggerSyncTestServer(t)
	server.SetSyncTrigger(func(deviceID string) error { return nil })

	req := httptest.NewRequest(http.MethodGet, "/api/devices/device-1/sync-history", nil)
	w := httptest.NewRecorder()
	server.handleDevicesRouter(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected /sync-history to still route to sync history (200), got %d: %s", w.Code, w.Body.String())
	}
}
