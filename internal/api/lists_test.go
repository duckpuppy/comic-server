package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
					{Type: "Series", Operator: "Contains", ArgumentValue: "Batman"},
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
					{Type: "Series", Operator: "1", ArgumentValue: "Batman"},
					{Type: "Year", Operator: "2", ArgumentValue: "2020"},
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
		ID                   string   `json:"id"`
		Name                 string   `json:"name"`
		MatcherMode          string   `json:"matcher_mode"`
		MatcherModeFormatted string   `json:"matcher_mode_formatted"`
		BookCount            int      `json:"book_count"`
		Matchers             []string `json:"matchers"`
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
