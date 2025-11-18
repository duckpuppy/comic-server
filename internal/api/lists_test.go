package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/library"
)

func TestHandleGetLists(t *testing.T) {
	// Create test library with smart lists
	lib := &library.ComicLibrary{
		ComicLists: []library.ComicListItem{
			{
				ID:          "list-1",
				Name:        "Currently Reading",
				Type:        "ComicSmartListItem",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "Series", MatchOperator: "Contains", MatchValue: "Batman"},
				},
			},
			{
				ID:          "list-2",
				Name:        "To Read",
				Type:        "ComicSmartListItem",
				MatcherMode: "Or",
				Matchers:    []library.ComicBookMatcher{},
			},
		},
	}

	// Create cache
	cache := library.NewListCache(5 * time.Minute)
	cache.SetCount("list-1", 2847)
	cache.SetCount("list-2", 156)

	server := &Server{
		library:   lib,
		listCache: cache,
	}

	req := httptest.NewRequest("GET", "/api/library/lists", nil)
	w := httptest.NewRecorder()

	server.handleGetLists(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response struct {
		Lists []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Type        string `json:"type"`
			MatcherMode string `json:"matcher_mode"`
			BookCount   int    `json:"book_count"`
		} `json:"lists"`
	}

	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(response.Lists) != 2 {
		t.Errorf("Expected 2 lists, got %d", len(response.Lists))
	}

	if response.Lists[0].ID != "list-1" {
		t.Errorf("Expected list-1, got %s", response.Lists[0].ID)
	}

	if response.Lists[0].BookCount != 2847 {
		t.Errorf("Expected count 2847, got %d", response.Lists[0].BookCount)
	}
}

func TestHandleGetListDetail(t *testing.T) {
	lib := &library.ComicLibrary{
		ComicLists: []library.ComicListItem{
			{
				ID:          "list-123",
				Name:        "Currently Reading",
				Type:        "ComicSmartListItem",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "Series", MatchOperator: "1", MatchValue: "Batman"},
					{Type: "Year", MatchOperator: "2", MatchValue: "2020"},
				},
			},
		},
	}

	cache := library.NewListCache(5 * time.Minute)
	cache.SetCount("list-123", 2847)

	server := &Server{
		library:   lib,
		listCache: cache,
	}

	req := httptest.NewRequest("GET", "/api/library/lists/list-123", nil)
	w := httptest.NewRecorder()

	server.handleGetListDetail(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response struct {
		ID                   string                `json:"id"`
		Name                 string                `json:"name"`
		MatcherMode          string                `json:"matcher_mode"`
		MatcherModeFormatted string                `json:"matcher_mode_formatted"`
		BookCount            int                   `json:"book_count"`
		Matchers             []library.MatcherInfo `json:"matchers"`
	}

	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.ID != "list-123" {
		t.Errorf("Expected list-123, got %s", response.ID)
	}

	if len(response.Matchers) != 2 {
		t.Errorf("Expected 2 matchers, got %d", len(response.Matchers))
	}

	if response.MatcherModeFormatted != "Match ALL (AND)" {
		t.Errorf("Expected 'Match ALL (AND)', got %s", response.MatcherModeFormatted)
	}
}

func TestHandleGetListDetail_NotFound(t *testing.T) {
	lib := &library.ComicLibrary{
		ComicLists: []library.ComicListItem{},
	}

	server := &Server{
		library:   lib,
		listCache: library.NewListCache(5 * time.Minute),
	}

	req := httptest.NewRequest("GET", "/api/library/lists/nonexistent", nil)
	w := httptest.NewRecorder()

	server.handleGetListDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestHandleGetListPreview(t *testing.T) {
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "comic-1", Series: "Batman", Number: "1", Title: "Issue 1"},
			{ID: "comic-2", Series: "Batman", Number: "2", Title: "Issue 2"},
			{ID: "comic-3", Series: "Batman", Number: "3", Title: "Issue 3"},
		},
		ComicLists: []library.ComicListItem{
			{
				ID:          "list-123",
				Name:        "Batman",
				Type:        "ComicSmartListItem",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "Series", MatchOperator: "0", MatchValue: "Batman"},
				},
			},
		},
	}

	server := &Server{
		library:   lib,
		listCache: library.NewListCache(5 * time.Minute),
	}

	req := httptest.NewRequest("GET", "/api/library/lists/list-123/preview?limit=2", nil)
	w := httptest.NewRecorder()

	server.handleGetListPreview(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response struct {
		Comics []struct {
			GUID   string `json:"guid"`
			Series string `json:"series"`
			Number string `json:"number"`
			Title  string `json:"title"`
		} `json:"comics"`
		Total  int `json:"total"`
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	}

	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Total != 3 {
		t.Errorf("Expected total 3, got %d", response.Total)
	}

	if len(response.Comics) != 2 {
		t.Errorf("Expected 2 comics in preview, got %d", len(response.Comics))
	}

	if response.Limit != 2 {
		t.Errorf("Expected limit 2, got %d", response.Limit)
	}
}

func TestHandleGetListDevices(t *testing.T) {
	// Create config with device assigned to list
	cfg := &config.Config{
		Devices: map[string]*config.DeviceConfig{
			"device-1": {
				DeviceID:     "device-1",
				FriendlyName: "Samsung Tablet",
				Lists: []config.SharedListConfig{
					{ListID: "list-123", ListName: "Batman", Enabled: true},
				},
			},
			"device-2": {
				DeviceID:     "device-2",
				FriendlyName: "iPad Pro",
				Lists: []config.SharedListConfig{
					{ListID: "list-456", ListName: "Superman", Enabled: true},
				},
			},
		},
	}

	server := &Server{
		config: cfg,
	}

	req := httptest.NewRequest("GET", "/api/library/lists/list-123/devices", nil)
	w := httptest.NewRecorder()

	server.handleGetListDevices(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response struct {
		Devices []struct {
			DeviceID     string `json:"device_id"`
			FriendlyName string `json:"friendly_name"`
			Enabled      bool   `json:"enabled"`
		} `json:"devices"`
	}

	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(response.Devices) != 1 {
		t.Errorf("Expected 1 device, got %d", len(response.Devices))
	}

	if response.Devices[0].DeviceID != "device-1" {
		t.Errorf("Expected device-1, got %s", response.Devices[0].DeviceID)
	}
}
