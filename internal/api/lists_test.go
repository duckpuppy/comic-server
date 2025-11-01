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
