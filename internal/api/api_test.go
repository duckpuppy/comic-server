package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/device"
	"github.com/duckpuppy/comic-server/internal/syncstate"
)

func TestNewServer(t *testing.T) {
	syncManager := syncstate.NewManager(10)
	registry := device.NewRegistry()
	cfg := &config.Config{}
	version := VersionInfo{Version: "test", GitCommit: "abc123", BuildDate: "2024-01-01"}

	server := NewServer(syncManager, registry, cfg, version)
	if server == nil {
		t.Fatal("NewServer returned nil")
	}
	if server.syncManager != syncManager {
		t.Error("syncManager not set correctly")
	}
	if server.registry != registry {
		t.Error("registry not set correctly")
	}
	if server.config != cfg {
		t.Error("config not set correctly")
	}
	if server.version.Version != "test" {
		t.Error("version not set correctly")
	}
}

func TestHandleHealth(t *testing.T) {
	syncManager := syncstate.NewManager(10)
	registry := device.NewRegistry()
	cfg := &config.Config{}
	version := VersionInfo{Version: "test", GitCommit: "abc123", BuildDate: "2024-01-01"}
	server := NewServer(syncManager, registry, cfg, version)

	// Wait a bit to ensure uptime is > 0
	time.Sleep(10 * time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Status != "healthy" {
		t.Errorf("expected status 'healthy', got '%s'", response.Status)
	}
	if response.Uptime == "" {
		t.Error("uptime should not be empty")
	}
	if response.Version != "test" {
		t.Errorf("expected version 'test', got '%s'", response.Version)
	}
	if response.GitCommit != "abc123" {
		t.Errorf("expected git commit 'abc123', got '%s'", response.GitCommit)
	}
	if response.BuildDate != "2024-01-01" {
		t.Errorf("expected build date '2024-01-01', got '%s'", response.BuildDate)
	}
}

func TestHandleVersion(t *testing.T) {
	syncManager := syncstate.NewManager(10)
	registry := device.NewRegistry()
	cfg := &config.Config{}
	version := VersionInfo{Version: "v1.2.3", GitCommit: "def456", BuildDate: "2024-02-01"}
	server := NewServer(syncManager, registry, cfg, version)

	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	w := httptest.NewRecorder()

	server.handleVersion(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response VersionResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Version != "v1.2.3" {
		t.Errorf("expected version 'v1.2.3', got '%s'", response.Version)
	}
	if response.GitCommit != "def456" {
		t.Errorf("expected git commit 'def456', got '%s'", response.GitCommit)
	}
	if response.BuildDate != "2024-02-01" {
		t.Errorf("expected build date '2024-02-01', got '%s'", response.BuildDate)
	}
}

func TestHandleHealthMethodNotAllowed(t *testing.T) {
	syncManager := syncstate.NewManager(10)
	registry := device.NewRegistry()
	cfg := &config.Config{}
	version := VersionInfo{Version: "test", GitCommit: "abc123", BuildDate: "2024-01-01"}
	server := NewServer(syncManager, registry, cfg, version)

	req := httptest.NewRequest(http.MethodPost, "/api/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleSyncStatus(t *testing.T) {
	syncManager := syncstate.NewManager(10)
	registry := device.NewRegistry()
	cfg := &config.Config{}
	version := VersionInfo{Version: "test", GitCommit: "abc123", BuildDate: "2024-01-01"}
	server := NewServer(syncManager, registry, cfg, version)

	// Start some syncs
	syncManager.StartSync("device1", "192.168.1.100", "Tablet 1")
	syncManager.StartSync("device2", "192.168.1.101", "Tablet 2")

	req := httptest.NewRequest(http.MethodGet, "/api/sync/status", nil)
	w := httptest.NewRecorder()

	server.handleSyncStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response SyncStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.ActiveCount != 2 {
		t.Errorf("expected 2 active syncs, got %d", response.ActiveCount)
	}
	if len(response.ActiveSyncs) != 2 {
		t.Errorf("expected 2 syncs in array, got %d", len(response.ActiveSyncs))
	}
}

func TestHandleSyncHistory(t *testing.T) {
	syncManager := syncstate.NewManager(10)
	registry := device.NewRegistry()
	cfg := &config.Config{}
	version := VersionInfo{Version: "test", GitCommit: "abc123", BuildDate: "2024-01-01"}
	server := NewServer(syncManager, registry, cfg, version)

	// Complete some syncs
	syncManager.StartSync("device1", "192.168.1.100", "Tablet 1")
	syncManager.CompleteSync("device1", 10, 5, 2, 0)

	syncManager.StartSync("device2", "192.168.1.101", "Tablet 2")
	syncManager.CompleteSync("device2", 20, 10, 5, 1)

	req := httptest.NewRequest(http.MethodGet, "/api/sync/history", nil)
	w := httptest.NewRecorder()

	server.handleSyncHistory(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response SyncHistoryResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Count != 2 {
		t.Errorf("expected 2 history entries, got %d", response.Count)
	}
	if len(response.History) != 2 {
		t.Errorf("expected 2 syncs in array, got %d", len(response.History))
	}
}

func TestHandleSyncHistoryWithLimit(t *testing.T) {
	syncManager := syncstate.NewManager(100)
	registry := device.NewRegistry()
	cfg := &config.Config{}
	version := VersionInfo{Version: "test", GitCommit: "abc123", BuildDate: "2024-01-01"}
	server := NewServer(syncManager, registry, cfg, version)

	// Complete 10 syncs
	for i := 1; i <= 10; i++ {
		deviceID := "device"
		syncManager.StartSync(deviceID, "192.168.1.100", "Tablet")
		syncManager.CompleteSync(deviceID, i, i, i, 0)
		time.Sleep(1 * time.Millisecond) // Ensure different timestamps
	}

	// Request only last 5
	req := httptest.NewRequest(http.MethodGet, "/api/sync/history?limit=5", nil)
	w := httptest.NewRecorder()

	server.handleSyncHistory(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response SyncHistoryResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Count != 5 {
		t.Errorf("expected 5 history entries, got %d", response.Count)
	}
}

func TestHandleDevices(t *testing.T) {
	syncManager := syncstate.NewManager(10)
	registry := device.NewRegistry()
	cfg := &config.Config{}
	version := VersionInfo{Version: "test", GitCommit: "abc123", BuildDate: "2024-01-01"}
	server := NewServer(syncManager, registry, cfg, version)

	// Add devices to registry
	info1 := &device.Info{
		ID:           "device1",
		Name:         "Tablet 1",
		Model:        "SM-T970",
		Manufacturer: "Samsung",
		Edition:      device.EditionAndroidFull,
		Version:      100,
	}
	info2 := &device.Info{
		ID:           "device2",
		Name:         "Tablet 2",
		Model:        "iPad Pro",
		Manufacturer: "Apple",
		Edition:      device.EditionIOS,
		Version:      1,
	}

	registry.Add(info1, "192.168.1.100")
	registry.Add(info2, "192.168.1.101")

	// Start sync for device1
	syncManager.StartSync("device1", "192.168.1.100", "Tablet 1")

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	w := httptest.NewRecorder()

	server.handleDevices(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response DevicesResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Count != 2 {
		t.Errorf("expected 2 devices, got %d", response.Count)
	}
	if len(response.Devices) != 2 {
		t.Errorf("expected 2 devices in array, got %d", len(response.Devices))
	}

	// Find device1 in response
	var device1Info *DeviceInfo
	for i := range response.Devices {
		if response.Devices[i].ID == "device1" {
			device1Info = &response.Devices[i]
			break
		}
	}

	if device1Info == nil {
		t.Fatal("device1 not found in response")
	}

	if !device1Info.IsSyncing {
		t.Error("device1 should be marked as syncing")
	}
	if device1Info.Name != "Tablet 1" {
		t.Errorf("expected name 'Tablet 1', got '%s'", device1Info.Name)
	}
	if device1Info.Edition != string(device.EditionAndroidFull) {
		t.Errorf("expected edition '%s', got '%s'", device.EditionAndroidFull, device1Info.Edition)
	}
}

func TestHandleStats(t *testing.T) {
	syncManager := syncstate.NewManager(10)
	registry := device.NewRegistry()
	cfg := &config.Config{
		Server: config.ServerConfig{
			MaxConcurrentConnections: 5,
			MaxConnectionsPerIP:      10,
			MaxRequestsPerDevice:     100,
		},
	}
	version := VersionInfo{Version: "test", GitCommit: "abc123", BuildDate: "2024-01-01"}
	server := NewServer(syncManager, registry, cfg, version)

	// Add a device
	info := &device.Info{
		ID:           "device1",
		Name:         "Tablet 1",
		Model:        "SM-T970",
		Manufacturer: "Samsung",
		Edition:      device.EditionAndroidFull,
		Version:      100,
	}
	registry.Add(info, "192.168.1.100")

	// Start a sync
	syncManager.StartSync("device1", "192.168.1.100", "Tablet 1")

	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	w := httptest.NewRecorder()

	server.handleStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response StatsResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.ActiveSyncs != 1 {
		t.Errorf("expected 1 active sync, got %d", response.ActiveSyncs)
	}
	if response.RegisteredDevices != 1 {
		t.Errorf("expected 1 registered device, got %d", response.RegisteredDevices)
	}
	if response.MaxConcurrent != 5 {
		t.Errorf("expected max concurrent 5, got %d", response.MaxConcurrent)
	}
	if !response.RateLimitingEnabled {
		t.Error("rate limiting should be enabled")
	}
	if response.MaxConnectionsPerIP != 10 {
		t.Errorf("expected max connections per IP 10, got %d", response.MaxConnectionsPerIP)
	}
	if response.MaxRequestsPerDevice != 100 {
		t.Errorf("expected max requests per device 100, got %d", response.MaxRequestsPerDevice)
	}
}

func TestHandleMetrics(t *testing.T) {
	syncManager := syncstate.NewManager(10)
	registry := device.NewRegistry()
	cfg := &config.Config{}
	version := VersionInfo{Version: "test", GitCommit: "abc123", BuildDate: "2024-01-01"}
	server := NewServer(syncManager, registry, cfg, version)

	// Simulate some sync activity to generate metrics
	syncManager.StartSync("device1", "192.168.1.100", "Tablet 1")
	syncManager.CompleteSync("device1", 10, 5, 2, 0)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Verify Content-Type starts with text/plain for Prometheus format
	contentType := w.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/plain") {
		t.Errorf("expected Prometheus content type starting with 'text/plain', got '%s'", contentType)
	}

	body := w.Body.String()

	// Verify key metrics are present
	expectedMetrics := []string{
		"comic_server_active_syncs",
		"comic_server_syncs_total",
		"comic_server_books_processed_total",
		"comic_server_sync_duration_seconds",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(body, metric) {
			t.Errorf("expected metric '%s' not found in metrics output", metric)
		}
	}

	// Verify completed sync was recorded (metrics may be cumulative from other tests)
	if !strings.Contains(body, "comic_server_syncs_total{status=\"completed\"}") {
		t.Error("expected completed sync counter metric to exist")
	}

	// Verify books processed metrics exist
	if !strings.Contains(body, "comic_server_books_processed_total{operation=\"added\"}") {
		t.Error("expected books added counter metric to exist")
	}
	if !strings.Contains(body, "comic_server_books_processed_total{operation=\"updated\"}") {
		t.Error("expected books updated counter metric to exist")
	}
	if !strings.Contains(body, "comic_server_books_processed_total{operation=\"deleted\"}") {
		t.Error("expected books deleted counter metric to exist")
	}

	// Verify active syncs gauge exists (should be 0 since sync completed)
	if !strings.Contains(body, "comic_server_active_syncs") {
		t.Error("expected active syncs gauge metric to exist")
	}
}

func TestServeHTTP(t *testing.T) {
	syncManager := syncstate.NewManager(10)
	registry := device.NewRegistry()
	cfg := &config.Config{}
	version := VersionInfo{Version: "test", GitCommit: "abc123", BuildDate: "2024-01-01"}
	server := NewServer(syncManager, registry, cfg, version)

	// Test that routes are registered correctly
	tests := []struct {
		path           string
		method         string
		expectedStatus int
	}{
		{"/api/health", http.MethodGet, http.StatusOK},
		{"/api/version", http.MethodGet, http.StatusOK},
		{"/api/sync/status", http.MethodGet, http.StatusOK},
		{"/api/sync/history", http.MethodGet, http.StatusOK},
		{"/api/devices", http.MethodGet, http.StatusOK},
		{"/api/stats", http.MethodGet, http.StatusOK},
		{"/metrics", http.MethodGet, http.StatusOK},
		{"/api/health", http.MethodPost, http.StatusMethodNotAllowed},
		{"/api/version", http.MethodPost, http.StatusMethodNotAllowed},
		{"/api/nonexistent", http.MethodGet, http.StatusNotFound},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		w := httptest.NewRecorder()

		server.ServeHTTP(w, req)

		if w.Code != tt.expectedStatus {
			t.Errorf("%s %s: expected status %d, got %d", tt.method, tt.path, tt.expectedStatus, w.Code)
		}
	}
}
