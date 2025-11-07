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
	registry.Add(deviceInfo, "192.168.1.100")

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
	cfg := &config.Config{
		Devices: map[string]*config.DeviceConfig{},
	}

	server := &Server{
		registry:    registry,
		config:      cfg,
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
