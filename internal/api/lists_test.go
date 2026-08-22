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

// TestGetListCounts_ServesStaleWhileRefreshingInBackground verifies that
// once a list's cached count has been computed at least once, a later
// invalidation (TTL expiry or InvalidateAll on library reload) doesn't
// force the next request to block on a full recompute: getListCounts
// serves the last-known (here, deliberately wrong) value immediately and
// only the background refresh converges on the real count - see
// comic-server-cg1.
func TestGetListCounts_ServesStaleWhileRefreshingInBackground(t *testing.T) {
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "book-1", Series: "Batman"},
		},
		ComicLists: []library.ComicListItem{
			{
				ID:          "list-1",
				Name:        "Batman",
				Type:        "ComicSmartListItem",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "Series", MatchOperator: "0", MatchValue: "Batman"},
				},
			},
		},
	}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	cache := library.NewListCache(5 * time.Minute)

	server := &Server{backend: backend, listCache: cache}
	list := &lib.ComicLists[0]

	// Seed a deliberately wrong cached value, then invalidate it - this
	// simulates a real count having gone stale (TTL expiry or a library
	// reload), not a first-ever computation.
	cache.SetCounts("list-1", 999, 999)
	cache.Invalidate("list-1")

	count, unread := server.getListCounts(list)
	if count != 999 || unread != 999 {
		t.Fatalf("expected getListCounts to serve the stale value (999, 999) immediately, got (%d, %d)", count, unread)
	}

	// The background refresh should converge on the real count shortly.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if c, _, found := cache.GetCounts("list-1"); found && c == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for background refresh to update the cache with the real count (1)")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

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

	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	server := &Server{
		backend:   backend,
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

// TestHandleGetListTree_MatchesHandleGetLists guards against the nav badge
// (backed by /api/library/list-tree) diverging from the dashboard's "Smart
// Lists" stat (backed by /api/library/lists) - they must count the same set
// of leaf lists. A non-smart, non-folder item (e.g. a plain reading list)
// must be excluded from both, not just one.
func TestHandleGetListTree_MatchesHandleGetLists(t *testing.T) {
	lib := &library.ComicLibrary{
		ComicLists: []library.ComicListItem{
			{ID: "list-1", Name: "Currently Reading", Type: "ComicSmartListItem"},
			{ID: "list-2", Name: "Some Reading List", Type: "ComicReadingListItem"},
			{
				ID: "folder-1", Name: "Folder", Type: "ComicListFolderItem",
				ChildItems: []library.ComicListItem{
					{ID: "list-3", Name: "Nested Smart List", Type: "ComicSmartListItem"},
				},
			},
		},
	}

	cache := library.NewListCache(5 * time.Minute)
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	server := &Server{backend: backend, listCache: cache}

	listsReq := httptest.NewRequest("GET", "/api/library/lists", nil)
	listsW := httptest.NewRecorder()
	server.handleGetLists(listsW, listsReq)

	var listsResp struct {
		Total int `json:"total"`
	}
	if err := json.NewDecoder(listsW.Body).Decode(&listsResp); err != nil {
		t.Fatalf("Failed to decode /api/library/lists response: %v", err)
	}

	treeReq := httptest.NewRequest("GET", "/api/library/list-tree", nil)
	treeW := httptest.NewRecorder()
	server.handleGetListTree(treeW, treeReq)

	var treeResp struct {
		Tree []ListTreeNode `json:"tree"`
	}
	if err := json.NewDecoder(treeW.Body).Decode(&treeResp); err != nil {
		t.Fatalf("Failed to decode /api/library/list-tree response: %v", err)
	}

	var countLeaves func(nodes []ListTreeNode) int
	countLeaves = func(nodes []ListTreeNode) int {
		n := 0
		for _, node := range nodes {
			if node.IsFolder {
				n += countLeaves(node.Children)
			} else {
				n++
			}
		}
		return n
	}
	treeCount := countLeaves(treeResp.Tree)

	if listsResp.Total != 2 {
		t.Errorf("Expected /api/library/lists to count 2 smart lists (excluding the reading list), got %d", listsResp.Total)
	}
	if treeCount != listsResp.Total {
		t.Errorf("Nav tree leaf count (%d) diverged from /api/library/lists total (%d)", treeCount, listsResp.Total)
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

	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	server := &Server{
		backend:   backend,
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

	cache := library.NewListCache(5 * time.Minute)
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	server := &Server{
		backend:   backend,
		listCache: cache,
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

	cache := library.NewListCache(5 * time.Minute)
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	server := &Server{
		backend:   backend,
		listCache: cache,
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

// TestHandleGetListPreview_UnreadField verifies the preview endpoint
// exposes each comic's read state (comic-server-9xg), so the list detail
// page can visually distinguish read from unread comics.
func TestHandleGetListPreview_UnreadField(t *testing.T) {
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "comic-unread", Series: "Batman", Number: "1"}, // never opened
			{ID: "comic-read", Series: "Batman", Number: "2", OpenCount: 1, PageCount: 20, LastPageRead: 19},
			{ID: "comic-partial", Series: "Batman", Number: "3", OpenCount: 1, PageCount: 20, LastPageRead: 5},
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

	cache := library.NewListCache(5 * time.Minute)
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	server := &Server{backend: backend, listCache: cache}

	req := httptest.NewRequest("GET", "/api/library/lists/list-123/preview", nil)
	w := httptest.NewRecorder()
	server.handleGetListPreview(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var response struct {
		Comics []struct {
			ID     string `json:"id"`
			Unread bool   `json:"unread"`
		} `json:"comics"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	byID := make(map[string]bool)
	for _, c := range response.Comics {
		byID[c.ID] = c.Unread
	}
	if !byID["comic-unread"] {
		t.Error("expected comic-unread to be unread")
	}
	if byID["comic-read"] {
		t.Error("expected comic-read to NOT be unread")
	}
	if !byID["comic-partial"] {
		t.Error("expected comic-partial (not fully read) to be unread")
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
