# Smart List UI - First-Class Entities Implementation Plan

> **For Claude:** Use `${SUPERPOWERS_SKILLS_ROOT}/skills/collaboration/executing-plans/SKILL.md` to implement this plan task-by-task.

**Goal:** Transform smart lists from modal-based configuration to first-class entities with dedicated pages, enabling better discovery and management for libraries with 65K+ comics.

**Architecture:** Add client-side routing to create multi-page SPA with three main views: Smart Lists Browser (`/lists`), List Detail Page (`/lists/:listId`), and enhanced Device Detail Page (`/devices/:deviceId`). Backend adds REST endpoints for list querying, preview generation, and device assignment management. Performance optimizations include list count caching and paginated comic previews.

**Tech Stack:** Vanilla JavaScript (History API routing), Go backend (new API endpoints in `internal/api/lists.go`), existing WebSocket infrastructure for real-time updates.

---

## Phase 1: Backend API Foundation

### Task 1: List Count Caching System

**Files:**
- Create: `internal/library/cache.go`
- Create: `internal/library/cache_test.go`
- Modify: `internal/library/library.go` (add cache integration)

**Step 1: Write the failing cache test**

Create `internal/library/cache_test.go`:

```go
package library

import (
	"testing"
	"time"
)

func TestListCache_GetCount(t *testing.T) {
	cache := NewListCache(5 * time.Minute)

	// Cache miss should return 0, false
	count, found := cache.GetCount("list-123")
	if found {
		t.Error("Expected cache miss for non-existent list")
	}
	if count != 0 {
		t.Errorf("Expected count 0, got %d", count)
	}
}

func TestListCache_SetAndGet(t *testing.T) {
	cache := NewListCache(5 * time.Minute)

	cache.SetCount("list-123", 2847)

	count, found := cache.GetCount("list-123")
	if !found {
		t.Error("Expected cache hit after SetCount")
	}
	if count != 2847 {
		t.Errorf("Expected count 2847, got %d", count)
	}
}

func TestListCache_Expiry(t *testing.T) {
	cache := NewListCache(100 * time.Millisecond)

	cache.SetCount("list-123", 2847)
	time.Sleep(150 * time.Millisecond)

	_, found := cache.GetCount("list-123")
	if found {
		t.Error("Expected cache miss after TTL expiry")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/library -v -run TestListCache`
Expected: FAIL with "undefined: NewListCache"

**Step 3: Implement cache**

Create `internal/library/cache.go`:

```go
package library

import (
	"sync"
	"time"
)

// ListCache caches smart list book counts to avoid expensive re-evaluation
type ListCache struct {
	counts map[string]*cacheEntry
	mu     sync.RWMutex
	ttl    time.Duration
}

type cacheEntry struct {
	count     int
	timestamp time.Time
}

// NewListCache creates a new list cache with the given TTL
func NewListCache(ttl time.Duration) *ListCache {
	return &ListCache{
		counts: make(map[string]*cacheEntry),
		ttl:    ttl,
	}
}

// GetCount retrieves a cached count if it exists and hasn't expired
func (c *ListCache) GetCount(listID string) (int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.counts[listID]
	if !exists {
		return 0, false
	}

	// Check if expired
	if time.Since(entry.timestamp) > c.ttl {
		return 0, false
	}

	return entry.count, true
}

// SetCount sets a count in the cache with current timestamp
func (c *ListCache) SetCount(listID string, count int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.counts[listID] = &cacheEntry{
		count:     count,
		timestamp: time.Now(),
	}
}

// Invalidate removes a specific list from cache
func (c *ListCache) Invalidate(listID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.counts, listID)
}

// InvalidateAll clears the entire cache
func (c *ListCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.counts = make(map[string]*cacheEntry)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/library -v -run TestListCache`
Expected: PASS (3 tests)

**Step 5: Commit**

```bash
git add internal/library/cache.go internal/library/cache_test.go
git commit -m "feat: add list count caching system"
```

---

### Task 2: Matcher Formatter (Human-Readable Display)

**Files:**
- Create: `internal/library/formatter.go`
- Create: `internal/library/formatter_test.go`

**Step 1: Write the failing formatter test**

Create `internal/library/formatter_test.go`:

```go
package library

import (
	"testing"
)

func TestFormatMatcher_SeriesContains(t *testing.T) {
	matcher := ComicBookMatcher{
		FieldName:      "Series",
		MatchOperator:  "Contains",
		MatchValue:     "Batman",
		Negate:         false,
	}

	result := FormatMatcher(matcher)
	expected := "Series contains 'Batman'"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestFormatMatcher_YearAfter(t *testing.T) {
	matcher := ComicBookMatcher{
		FieldName:      "Year",
		MatchOperator:  "Greater",
		MatchValue:     "2020",
		Negate:         false,
	}

	result := FormatMatcher(matcher)
	expected := "Year is after 2020"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestFormatMatcher_Negated(t *testing.T) {
	matcher := ComicBookMatcher{
		FieldName:      "Publisher",
		MatchOperator:  "Equals",
		MatchValue:     "DC",
		Negate:         true,
	}

	result := FormatMatcher(matcher)
	expected := "NOT Publisher equals 'DC'"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestFormatMatcherMode_And(t *testing.T) {
	result := FormatMatcherMode("And")
	expected := "Match ALL (AND)"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestFormatMatcherMode_Or(t *testing.T) {
	result := FormatMatcherMode("Or")
	expected := "Match ANY (OR)"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/library -v -run TestFormatMatcher`
Expected: FAIL with "undefined: FormatMatcher"

**Step 3: Implement formatter**

Create `internal/library/formatter.go`:

```go
package library

import "fmt"

// FormatMatcher converts a ComicBookMatcher to human-readable string
func FormatMatcher(m ComicBookMatcher) string {
	var operator string

	// Map operators to human-readable phrases
	switch m.MatchOperator {
	case "Equals":
		operator = "equals"
	case "Contains":
		operator = "contains"
	case "ContainsAny":
		operator = "contains any of"
	case "ContainsAll":
		operator = "contains all of"
	case "StartsWith":
		operator = "starts with"
	case "EndsWith":
		operator = "ends with"
	case "Greater":
		if m.FieldName == "Year" || m.FieldName == "Month" || m.FieldName == "Day" {
			operator = "is after"
		} else {
			operator = "is greater than"
		}
	case "Lesser":
		if m.FieldName == "Year" || m.FieldName == "Month" || m.FieldName == "Day" {
			operator = "is before"
		} else {
			operator = "is less than"
		}
	case "InRange":
		operator = "is in range"
	case "IsInLastDays":
		operator = "is in last"
	case "Regex":
		operator = "matches regex"
	default:
		operator = m.MatchOperator
	}

	// Build the formatted string
	var result string
	if m.Negate {
		result = fmt.Sprintf("NOT %s %s '%s'", m.FieldName, operator, m.MatchValue)
	} else {
		result = fmt.Sprintf("%s %s '%s'", m.FieldName, operator, m.MatchValue)
	}

	return result
}

// FormatMatcherMode converts matcher mode to human-readable string
func FormatMatcherMode(mode string) string {
	switch mode {
	case "And":
		return "Match ALL (AND)"
	case "Or":
		return "Match ANY (OR)"
	default:
		return mode
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/library -v -run TestFormatMatcher`
Expected: PASS (5 tests)

**Step 5: Commit**

```bash
git add internal/library/formatter.go internal/library/formatter_test.go
git commit -m "feat: add matcher human-readable formatter"
```

---

### Task 3: List API Endpoints

**Files:**
- Create: `internal/api/lists.go`
- Create: `internal/api/lists_test.go`
- Modify: `internal/api/api.go` (register new routes)

**Step 1: Write the failing test for GET /api/library/lists**

Create `internal/api/lists_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
					{FieldName: "Series", MatchOperator: "Contains", MatchValue: "Batman"},
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api -v -run TestHandleGetLists`
Expected: FAIL with "undefined: Server.handleGetLists"

**Step 3: Implement GET /api/library/lists endpoint**

Create `internal/api/lists.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// ListSummary represents a smart list with cached count
type ListSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	MatcherMode string `json:"matcher_mode"`
	BookCount   int    `json:"book_count"`
	MatcherCount int   `json:"matcher_count"`
}

// handleGetLists returns all smart lists with cached counts
func (s *Server) handleGetLists(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.library == nil {
		log.Error().Msg("Library not loaded")
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	lists := make([]ListSummary, 0, len(s.library.ComicLists))

	for _, list := range s.library.ComicLists {
		// Only include smart lists
		if list.Type != "ComicSmartListItem" {
			continue
		}

		// Get cached count, or calculate if not cached
		count, found := s.listCache.GetCount(list.ID)
		if !found {
			// Evaluate list to get count
			matches := s.library.EvaluateSmartList(&list)
			count = len(matches)
			s.listCache.SetCount(list.ID, count)
		}

		lists = append(lists, ListSummary{
			ID:          list.ID,
			Name:        list.Name,
			Type:        list.Type,
			MatcherMode: list.MatcherMode,
			BookCount:   count,
			MatcherCount: len(list.Matchers),
		})
	}

	response := map[string]interface{}{
		"lists": lists,
		"total": len(lists),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
```

**Step 4: Add listCache field to Server struct**

Modify `internal/api/api.go`:

Find the `Server` struct and add:

```go
type Server struct {
	// ... existing fields ...
	listCache *library.ListCache
}
```

In the initialization function, add:

```go
func NewServer(/* params */) *Server {
	return &Server{
		// ... existing fields ...
		listCache: library.NewListCache(15 * time.Minute), // 15 min TTL
	}
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/api -v -run TestHandleGetLists`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/api/lists.go internal/api/lists_test.go internal/api/api.go
git commit -m "feat: add GET /api/library/lists endpoint with caching"
```

---

### Task 4: List Detail Endpoint

**Files:**
- Modify: `internal/api/lists.go`
- Modify: `internal/api/lists_test.go`

**Step 1: Write the failing test for GET /api/library/lists/:listId**

Add to `internal/api/lists_test.go`:

```go
func TestHandleGetListDetail(t *testing.T) {
	lib := &library.ComicLibrary{
		ComicLists: []library.ComicListItem{
			{
				ID:          "list-123",
				Name:        "Currently Reading",
				Type:        "ComicSmartListItem",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{FieldName: "Series", MatchOperator: "Contains", MatchValue: "Batman"},
					{FieldName: "Year", MatchOperator: "Greater", MatchValue: "2020"},
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
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		MatcherMode string   `json:"matcher_mode"`
		BookCount   int      `json:"book_count"`
		Matchers    []string `json:"matchers"`
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

	if response.Matchers[0] != "Series contains 'Batman'" {
		t.Errorf("Unexpected matcher format: %s", response.Matchers[0])
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api -v -run TestHandleGetListDetail`
Expected: FAIL with "undefined: Server.handleGetListDetail"

**Step 3: Implement GET /api/library/lists/:listId endpoint**

Add to `internal/api/lists.go`:

```go
// ListDetail represents full details of a smart list
type ListDetail struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	MatcherMode string   `json:"matcher_mode"`
	MatcherModeFormatted string `json:"matcher_mode_formatted"`
	BookCount   int      `json:"book_count"`
	Matchers    []string `json:"matchers"`
}

// handleGetListDetail returns details for a specific list
func (s *Server) handleGetListDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.library == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	// Extract list ID from URL path
	// URL: /api/library/lists/:listId
	listID := r.URL.Path[len("/api/library/lists/"):]

	// Find the list
	var targetList *library.ComicListItem
	for i := range s.library.ComicLists {
		if s.library.ComicLists[i].ID == listID {
			targetList = &s.library.ComicLists[i]
			break
		}
	}

	if targetList == nil {
		http.Error(w, "List not found", http.StatusNotFound)
		return
	}

	// Get cached count
	count, found := s.listCache.GetCount(listID)
	if !found {
		matches := s.library.EvaluateSmartList(targetList)
		count = len(matches)
		s.listCache.SetCount(listID, count)
	}

	// Format matchers
	matchers := make([]string, len(targetList.Matchers))
	for i, m := range targetList.Matchers {
		matchers[i] = library.FormatMatcher(m)
	}

	detail := ListDetail{
		ID:          targetList.ID,
		Name:        targetList.Name,
		Type:        targetList.Type,
		MatcherMode: targetList.MatcherMode,
		MatcherModeFormatted: library.FormatMatcherMode(targetList.MatcherMode),
		BookCount:   count,
		Matchers:    matchers,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/api -v -run TestHandleGetListDetail`
Expected: PASS (2 tests)

**Step 5: Commit**

```bash
git add internal/api/lists.go internal/api/lists_test.go
git commit -m "feat: add GET /api/library/lists/:listId endpoint"
```

---

### Task 5: List Preview Endpoint

**Files:**
- Modify: `internal/api/lists.go`
- Modify: `internal/api/lists_test.go`

**Step 1: Write the failing test for GET /api/library/lists/:listId/preview**

Add to `internal/api/lists_test.go`:

```go
func TestHandleGetListPreview(t *testing.T) {
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{GUID: "comic-1", Series: "Batman", Number: "1", Title: "Issue 1"},
			{GUID: "comic-2", Series: "Batman", Number: "2", Title: "Issue 2"},
			{GUID: "comic-3", Series: "Batman", Number: "3", Title: "Issue 3"},
		},
		ComicLists: []library.ComicListItem{
			{
				ID:          "list-123",
				Name:        "Batman",
				Type:        "ComicSmartListItem",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{FieldName: "Series", MatchOperator: "Equals", MatchValue: "Batman"},
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api -v -run TestHandleGetListPreview`
Expected: FAIL with "undefined: Server.handleGetListPreview"

**Step 3: Implement GET /api/library/lists/:listId/preview endpoint**

Add to `internal/api/lists.go`:

```go
// ComicPreview represents a comic for preview display
type ComicPreview struct {
	GUID      string `json:"guid"`
	Series    string `json:"series"`
	Number    string `json:"number"`
	Title     string `json:"title"`
	Volume    int    `json:"volume"`
	Publisher string `json:"publisher"`
	Year      int    `json:"year"`
}

// handleGetListPreview returns a preview of comics matching the list
func (s *Server) handleGetListPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.library == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	// Extract list ID from URL
	path := r.URL.Path
	listID := path[len("/api/library/lists/"):]
	if idx := strings.Index(listID, "/"); idx != -1 {
		listID = listID[:idx]
	}

	// Parse query parameters
	query := r.URL.Query()
	limit := 20 // Default
	if l := query.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
			if limit > 100 {
				limit = 100 // Max limit
			}
		}
	}

	offset := 0
	if o := query.Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	// Find the list
	var targetList *library.ComicListItem
	for i := range s.library.ComicLists {
		if s.library.ComicLists[i].ID == listID {
			targetList = &s.library.ComicLists[i]
			break
		}
	}

	if targetList == nil {
		http.Error(w, "List not found", http.StatusNotFound)
		return
	}

	// Evaluate list
	matches := s.library.EvaluateSmartList(targetList)
	total := len(matches)

	// Apply pagination
	start := offset
	if start > total {
		start = total
	}

	end := start + limit
	if end > total {
		end = total
	}

	// Build preview
	previews := make([]ComicPreview, 0, end-start)
	for i := start; i < end; i++ {
		comic := matches[i]
		previews = append(previews, ComicPreview{
			GUID:      comic.GUID,
			Series:    comic.Series,
			Number:    comic.Number,
			Title:     comic.Title,
			Volume:    comic.Volume,
			Publisher: comic.Publisher,
			Year:      comic.Year,
		})
	}

	response := map[string]interface{}{
		"comics": previews,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
```

Add required imports to top of `lists.go`:

```go
import (
	// ... existing imports ...
	"strconv"
	"strings"
)
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/api -v -run TestHandleGetListPreview`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/lists.go internal/api/lists_test.go
git commit -m "feat: add GET /api/library/lists/:listId/preview endpoint"
```

---

### Task 6: Device List Assignment Endpoints

**Files:**
- Modify: `internal/api/lists.go`
- Modify: `internal/api/lists_test.go`

**Step 1: Write the failing test for GET /api/library/lists/:listId/devices**

Add to `internal/api/lists_test.go`:

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api -v -run TestHandleGetListDevices`
Expected: FAIL with "undefined: Server.handleGetListDevices"

**Step 3: Implement GET /api/library/lists/:listId/devices endpoint**

Add to `internal/api/lists.go`:

```go
// DeviceAssignment represents a device assigned to a list
type DeviceAssignment struct {
	DeviceID     string `json:"device_id"`
	FriendlyName string `json:"friendly_name"`
	Enabled      bool   `json:"enabled"`
}

// handleGetListDevices returns devices assigned to a specific list
func (s *Server) handleGetListDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract list ID
	path := r.URL.Path
	listID := path[len("/api/library/lists/"):]
	if idx := strings.Index(listID, "/"); idx != -1 {
		listID = listID[:idx]
	}

	assignments := []DeviceAssignment{}

	// Scan all devices for this list
	s.configMu.RLock()
	for _, device := range s.config.Devices {
		for _, list := range device.Lists {
			if list.ListID == listID {
				assignments = append(assignments, DeviceAssignment{
					DeviceID:     device.DeviceID,
					FriendlyName: device.FriendlyName,
					Enabled:      list.Enabled,
				})
				break
			}
		}
	}
	s.configMu.RUnlock()

	response := map[string]interface{}{
		"devices": assignments,
		"total":   len(assignments),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
```

**Step 4: Add config field to Server struct**

Modify `internal/api/api.go`:

Add to Server struct:

```go
type Server struct {
	// ... existing fields ...
	config   *config.Config
	configMu sync.RWMutex
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/api -v -run TestHandleGetListDevices`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/api/lists.go internal/api/lists_test.go internal/api/api.go
git commit -m "feat: add GET /api/library/lists/:listId/devices endpoint"
```

---

### Task 7: Register API Routes

**Files:**
- Modify: `internal/api/api.go`

**Step 1: Register new list routes**

In `internal/api/api.go`, find the route registration section and add:

```go
func (s *Server) setupRoutes() {
	// ... existing routes ...

	// Library lists endpoints
	http.HandleFunc("/api/library/lists", s.handleGetLists)
	http.HandleFunc("/api/library/lists/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// /api/library/lists/:listId
		if !strings.Contains(path[len("/api/library/lists/"):], "/") {
			s.handleGetListDetail(w, r)
			return
		}

		// /api/library/lists/:listId/preview
		if strings.HasSuffix(path, "/preview") {
			s.handleGetListPreview(w, r)
			return
		}

		// /api/library/lists/:listId/devices
		if strings.HasSuffix(path, "/devices") {
			s.handleGetListDevices(w, r)
			return
		}

		http.NotFound(w, r)
	})
}
```

**Step 2: Verify routes work**

Run integration test:

```bash
go run main.go server --library testdata/ComicDb.xml &
SERVER_PID=$!
sleep 2

# Test GET /api/library/lists
curl -s http://localhost:7620/api/library/lists | jq '.total'

# Cleanup
kill $SERVER_PID
```

Expected: Returns count of smart lists

**Step 3: Commit**

```bash
git add internal/api/api.go
git commit -m "feat: register list API routes"
```

---

## Phase 2: Frontend Routing Infrastructure

### Task 8: Client-Side Router

**Files:**
- Create: `internal/api/web/js/router.js`
- Create: `internal/api/web/js/router_test.html` (manual test page)

**Step 1: Implement vanilla JS router**

Create `internal/api/web/js/router.js`:

```javascript
// Simple client-side router using History API
class Router {
    constructor() {
        this.routes = new Map();
        this.currentPath = window.location.pathname;

        // Handle back/forward navigation
        window.addEventListener('popstate', () => this.handleRoute());

        // Handle initial load
        document.addEventListener('DOMContentLoaded', () => this.handleRoute());
    }

    /**
     * Register a route with a handler function
     * @param {string} pattern - Route pattern (e.g., '/lists/:id')
     * @param {Function} handler - Function to call when route matches
     */
    register(pattern, handler) {
        const regex = this.patternToRegex(pattern);
        this.routes.set(pattern, { regex, handler, pattern });
    }

    /**
     * Convert route pattern to regex with named groups
     * @param {string} pattern - Route pattern
     * @returns {RegExp} Regular expression for matching
     */
    patternToRegex(pattern) {
        // Convert :param to named capture group
        const regexPattern = pattern.replace(/:(\w+)/g, '(?<$1>[^/]+)');
        return new RegExp(`^${regexPattern}$`);
    }

    /**
     * Navigate to a new path
     * @param {string} path - Path to navigate to
     */
    navigate(path) {
        if (path === this.currentPath) return;

        this.currentPath = path;
        window.history.pushState({}, '', path);
        this.handleRoute();
    }

    /**
     * Handle current route
     */
    handleRoute() {
        const path = window.location.pathname;
        this.currentPath = path;

        // Find matching route
        for (const [pattern, { regex, handler }] of this.routes) {
            const match = path.match(regex);
            if (match) {
                // Extract params from named groups
                const params = match.groups || {};
                handler(params);
                return;
            }
        }

        // No route matched - 404
        this.handle404();
    }

    /**
     * Default 404 handler
     */
    handle404() {
        console.warn('No route matched:', this.currentPath);
        document.getElementById('app').innerHTML = `
            <div class="error-page">
                <h1>404 - Page Not Found</h1>
                <p>The page "${this.currentPath}" does not exist.</p>
                <a href="/">Return to Dashboard</a>
            </div>
        `;
    }
}

// Global router instance
const router = new Router();
```

**Step 2: Create manual test page**

Create `internal/api/web/js/router_test.html`:

```html
<!DOCTYPE html>
<html>
<head>
    <title>Router Test</title>
    <script src="router.js"></script>
</head>
<body>
    <nav>
        <a href="#" onclick="router.navigate('/'); return false;">Home</a>
        <a href="#" onclick="router.navigate('/lists'); return false;">Lists</a>
        <a href="#" onclick="router.navigate('/lists/123'); return false;">List 123</a>
        <a href="#" onclick="router.navigate('/devices'); return false;">Devices</a>
        <a href="#" onclick="router.navigate('/devices/abc'); return false;">Device ABC</a>
    </nav>

    <div id="app"></div>

    <script>
        router.register('/', (params) => {
            document.getElementById('app').innerHTML = '<h1>Dashboard</h1>';
        });

        router.register('/lists', (params) => {
            document.getElementById('app').innerHTML = '<h1>Lists Browser</h1>';
        });

        router.register('/lists/:listId', (params) => {
            document.getElementById('app').innerHTML = `<h1>List Detail: ${params.listId}</h1>`;
        });

        router.register('/devices', (params) => {
            document.getElementById('app').innerHTML = '<h1>Devices</h1>';
        });

        router.register('/devices/:deviceId', (params) => {
            document.getElementById('app').innerHTML = `<h1>Device Detail: ${params.deviceId}</h1>`;
        });

        // Handle initial route
        router.handleRoute();
    </script>
</body>
</html>
```

**Step 3: Manual verification**

1. Open router_test.html in browser
2. Click navigation links
3. Verify browser URL updates
4. Verify content changes
5. Test back/forward buttons

Expected: All navigation works, URL updates, back/forward functional

**Step 4: Commit**

```bash
git add internal/api/web/js/router.js internal/api/web/js/router_test.html
git commit -m "feat: add client-side router with History API"
```

---

### Task 9: Navigation Component

**Files:**
- Create: `internal/api/web/js/navigation.js`
- Create: `internal/api/web/css/navigation.css`
- Modify: `internal/api/web/index.html`

**Step 1: Create navigation component**

Create `internal/api/web/js/navigation.js`:

```javascript
// Navigation component for top-level tabs
class Navigation {
    constructor() {
        this.currentTab = 'dashboard';
    }

    init() {
        this.render();
        this.attachListeners();
    }

    render() {
        const nav = document.getElementById('main-nav');
        if (!nav) return;

        nav.innerHTML = `
            <div class="nav-tabs">
                <a href="/" class="nav-tab" data-tab="dashboard">
                    <span class="nav-icon">📊</span>
                    <span class="nav-label">Dashboard</span>
                </a>
                <a href="/lists" class="nav-tab" data-tab="lists">
                    <span class="nav-icon">📚</span>
                    <span class="nav-label">Smart Lists</span>
                    <span class="nav-badge" id="lists-count">0</span>
                </a>
                <a href="/devices" class="nav-tab" data-tab="devices">
                    <span class="nav-icon">📱</span>
                    <span class="nav-label">Devices</span>
                    <span class="nav-badge" id="devices-count">0</span>
                </a>
                <a href="/sync" class="nav-tab" data-tab="sync">
                    <span class="nav-icon">🔄</span>
                    <span class="nav-label">Sync History</span>
                </a>
            </div>
        `;
    }

    attachListeners() {
        const tabs = document.querySelectorAll('.nav-tab');
        tabs.forEach(tab => {
            tab.addEventListener('click', (e) => {
                e.preventDefault();
                const path = tab.getAttribute('href');
                router.navigate(path);
                this.setActive(tab.dataset.tab);
            });
        });
    }

    setActive(tabName) {
        this.currentTab = tabName;

        // Update active state
        document.querySelectorAll('.nav-tab').forEach(tab => {
            if (tab.dataset.tab === tabName) {
                tab.classList.add('active');
            } else {
                tab.classList.remove('active');
            }
        });
    }

    updateBadge(tabName, count) {
        const badge = document.getElementById(`${tabName}-count`);
        if (badge) {
            badge.textContent = count;
            badge.style.display = count > 0 ? 'inline-block' : 'none';
        }
    }
}

// Global navigation instance
const navigation = new Navigation();
```

**Step 2: Create navigation styles**

Create `internal/api/web/css/navigation.css`:

```css
/* Navigation Tabs */
.nav-tabs {
    display: flex;
    gap: 0.5rem;
    padding: 0 1rem;
    background: var(--bg-secondary);
    border-bottom: 1px solid var(--border-color);
}

.nav-tab {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 1rem 1.5rem;
    text-decoration: none;
    color: var(--text-secondary);
    font-weight: 500;
    border-bottom: 2px solid transparent;
    transition: all 0.2s;
    position: relative;
}

.nav-tab:hover {
    color: var(--text-primary);
    background: var(--bg-primary);
}

.nav-tab.active {
    color: var(--primary-color);
    border-bottom-color: var(--primary-color);
}

.nav-icon {
    font-size: 1.25rem;
}

.nav-label {
    font-size: 0.875rem;
}

.nav-badge {
    display: none;
    background: var(--primary-color);
    color: white;
    font-size: 0.75rem;
    font-weight: 600;
    padding: 0.125rem 0.5rem;
    border-radius: 10px;
    min-width: 20px;
    text-align: center;
}

@media (max-width: 768px) {
    .nav-label {
        display: none;
    }

    .nav-tab {
        padding: 1rem;
    }
}
```

**Step 3: Update index.html**

Modify `internal/api/web/index.html`:

Add to `<head>`:

```html
<link rel="stylesheet" href="/css/navigation.css">
```

Update body structure:

```html
<body>
    <div class="container">
        <!-- Header -->
        <header class="header">
            <h1>Comic Server</h1>
            <div class="server-status">
                <span class="status-indicator" id="ws-status"></span>
                <span id="ws-status-text">Connecting...</span>
            </div>
        </header>

        <!-- Navigation -->
        <nav id="main-nav"></nav>

        <!-- App Container (route content goes here) -->
        <div id="app"></div>
    </div>

    <!-- Existing scripts -->
    <script src="/js/router.js"></script>
    <script src="/js/navigation.js"></script>
    <!-- ... other scripts ... -->
</body>
```

**Step 4: Verify navigation renders**

Start server and check http://localhost:7620/

Expected: Navigation tabs visible, clickable, active state works

**Step 5: Commit**

```bash
git add internal/api/web/js/navigation.js internal/api/web/css/navigation.css internal/api/web/index.html
git commit -m "feat: add navigation component with tabs"
```

---

## Phase 3: Smart Lists Browser Page

### Task 10: Lists Browser View

**Files:**
- Create: `internal/api/web/js/lists.js`
- Create: `internal/api/web/css/lists.css`

**Step 1: Create lists browser component**

Create `internal/api/web/js/lists.js`:

```javascript
// Smart Lists Browser
class ListsBrowser {
    constructor() {
        this.lists = [];
        this.filteredLists = [];
        this.searchTerm = '';
        this.sortBy = 'name';
        this.filterAssigned = 'all'; // 'all', 'assigned', 'unassigned'
    }

    async init() {
        await this.loadLists();
        this.render();
        this.attachListeners();
    }

    async loadLists() {
        try {
            const response = await fetch('/api/library/lists');
            const data = await response.json();
            this.lists = data.lists || [];
            this.applyFilters();
        } catch (error) {
            console.error('Failed to load lists:', error);
            this.lists = [];
        }
    }

    applyFilters() {
        let filtered = [...this.lists];

        // Search filter
        if (this.searchTerm) {
            const term = this.searchTerm.toLowerCase();
            filtered = filtered.filter(list =>
                list.name.toLowerCase().includes(term)
            );
        }

        // Assignment filter (would need device data for full implementation)
        // TODO: Implement when device assignment data is available

        // Sort
        filtered.sort((a, b) => {
            switch (this.sortBy) {
                case 'name':
                    return a.name.localeCompare(b.name);
                case 'name-desc':
                    return b.name.localeCompare(a.name);
                case 'count':
                    return b.book_count - a.book_count;
                default:
                    return 0;
            }
        });

        this.filteredLists = filtered;
    }

    render() {
        const app = document.getElementById('app');
        app.innerHTML = `
            <div class="lists-page">
                <div class="lists-header">
                    <h1>Smart Lists</h1>
                    <div class="lists-search">
                        <input
                            type="text"
                            id="lists-search-input"
                            placeholder="Search lists..."
                            value="${this.searchTerm}"
                        >
                    </div>
                </div>

                <div class="lists-content">
                    <aside class="lists-filters">
                        <h3>Filters</h3>
                        <div class="filter-group">
                            <label>Sort by:</label>
                            <select id="lists-sort">
                                <option value="name">Name (A-Z)</option>
                                <option value="name-desc">Name (Z-A)</option>
                                <option value="count">Book Count</option>
                            </select>
                        </div>
                    </aside>

                    <main class="lists-grid">
                        ${this.renderListCards()}
                    </main>
                </div>
            </div>
        `;

        // Update navigation badge
        navigation.updateBadge('lists', this.lists.length);
    }

    renderListCards() {
        if (this.filteredLists.length === 0) {
            return `
                <div class="empty-message">
                    <p>No smart lists found.</p>
                    <p class="help-text">Smart lists are created in ComicRackCE.</p>
                </div>
            `;
        }

        return this.filteredLists.map(list => `
            <div class="list-card" data-list-id="${list.id}">
                <div class="list-card-header">
                    <h3 class="list-name">${this.escapeHtml(list.name)}</h3>
                </div>
                <div class="list-card-body">
                    <div class="list-stat">
                        <span class="stat-icon">📚</span>
                        <span class="stat-value">${list.book_count.toLocaleString()}</span>
                        <span class="stat-label">comics</span>
                    </div>
                    <div class="list-stat">
                        <span class="stat-icon">🔍</span>
                        <span class="stat-value">${list.matcher_count}</span>
                        <span class="stat-label">rules</span>
                    </div>
                </div>
                <div class="list-card-footer">
                    <button class="btn btn-primary btn-small" onclick="router.navigate('/lists/${list.id}')">
                        View Details
                    </button>
                </div>
            </div>
        `).join('');
    }

    attachListeners() {
        // Search input
        const searchInput = document.getElementById('lists-search-input');
        if (searchInput) {
            let debounceTimer;
            searchInput.addEventListener('input', (e) => {
                clearTimeout(debounceTimer);
                debounceTimer = setTimeout(() => {
                    this.searchTerm = e.target.value;
                    this.applyFilters();
                    this.render();
                }, 300);
            });
        }

        // Sort select
        const sortSelect = document.getElementById('lists-sort');
        if (sortSelect) {
            sortSelect.value = this.sortBy;
            sortSelect.addEventListener('change', (e) => {
                this.sortBy = e.target.value;
                this.applyFilters();
                this.render();
            });
        }

        // Card clicks
        const cards = document.querySelectorAll('.list-card');
        cards.forEach(card => {
            card.addEventListener('click', (e) => {
                if (e.target.tagName !== 'BUTTON') {
                    const listId = card.dataset.listId;
                    router.navigate(`/lists/${listId}`);
                }
            });
        });
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}

// Global instance
let listsBrowser = null;
```

**Step 2: Create list styles**

Create `internal/api/web/css/lists.css`:

```css
/* Lists Page Layout */
.lists-page {
    padding: 2rem;
}

.lists-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 2rem;
}

.lists-header h1 {
    font-size: 2rem;
    font-weight: 700;
}

.lists-search input {
    width: 300px;
    padding: 0.75rem 1rem;
    border: 1px solid var(--border-color);
    border-radius: 0.5rem;
    font-size: 0.875rem;
}

.lists-search input:focus {
    outline: none;
    border-color: var(--primary-color);
    box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.1);
}

/* Lists Content */
.lists-content {
    display: grid;
    grid-template-columns: 250px 1fr;
    gap: 2rem;
}

@media (max-width: 1024px) {
    .lists-content {
        grid-template-columns: 1fr;
    }

    .lists-filters {
        order: 1;
    }
}

/* Filters Sidebar */
.lists-filters {
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 0.5rem;
    padding: 1.5rem;
    height: fit-content;
    position: sticky;
    top: 1.5rem;
}

.lists-filters h3 {
    font-size: 1rem;
    font-weight: 600;
    margin-bottom: 1rem;
}

.filter-group {
    margin-bottom: 1.5rem;
}

.filter-group label {
    display: block;
    font-size: 0.875rem;
    font-weight: 500;
    margin-bottom: 0.5rem;
    color: var(--text-secondary);
}

.filter-group select {
    width: 100%;
    padding: 0.5rem;
    border: 1px solid var(--border-color);
    border-radius: 0.375rem;
    font-size: 0.875rem;
}

/* Lists Grid */
.lists-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
    gap: 1.5rem;
}

/* List Card */
.list-card {
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 0.5rem;
    padding: 1.5rem;
    cursor: pointer;
    transition: all 0.2s;
}

.list-card:hover {
    border-color: var(--primary-color);
    box-shadow: var(--shadow-md);
    transform: translateY(-2px);
}

.list-card-header {
    margin-bottom: 1rem;
}

.list-name {
    font-size: 1.125rem;
    font-weight: 600;
    color: var(--text-primary);
    margin: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.list-card-body {
    display: flex;
    gap: 1.5rem;
    margin-bottom: 1rem;
    padding: 1rem 0;
    border-top: 1px solid var(--border-color);
    border-bottom: 1px solid var(--border-color);
}

.list-stat {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.25rem;
}

.stat-icon {
    font-size: 1.5rem;
}

.stat-value {
    font-size: 1.25rem;
    font-weight: 700;
    color: var(--text-primary);
}

.stat-label {
    font-size: 0.75rem;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.05em;
}

.list-card-footer {
    display: flex;
    justify-content: flex-end;
}

.btn-small {
    padding: 0.5rem 1rem;
    font-size: 0.875rem;
}
```

**Step 3: Register route**

Add to main app initialization (in `app.js` or similar):

```javascript
// Register /lists route
router.register('/lists', async () => {
    navigation.setActive('lists');
    if (!listsBrowser) {
        listsBrowser = new ListsBrowser();
    }
    await listsBrowser.init();
});
```

**Step 4: Add CSS to index.html**

Modify `internal/api/web/index.html`:

```html
<link rel="stylesheet" href="/css/lists.css">
```

**Step 5: Test lists browser**

Start server, navigate to http://localhost:7620/lists

Expected: Grid of list cards, search works, sorting works

**Step 6: Commit**

```bash
git add internal/api/web/js/lists.js internal/api/web/css/lists.css internal/api/web/index.html
git commit -m "feat: add smart lists browser page"
```

---

## Phase 4: List Detail Page

### Task 11: List Detail View

**Files:**
- Modify: `internal/api/web/js/lists.js`
- Modify: `internal/api/web/css/lists.css`

**Step 1: Add ListDetail class**

Add to `internal/api/web/js/lists.js`:

```javascript
// List Detail Page
class ListDetail {
    constructor(listId) {
        this.listId = listId;
        this.list = null;
        this.devices = [];
        this.preview = [];
        this.previewOffset = 0;
        this.previewLimit = 20;
    }

    async init() {
        await Promise.all([
            this.loadListDetail(),
            this.loadDevices(),
            this.loadPreview()
        ]);
        this.render();
        this.attachListeners();
    }

    async loadListDetail() {
        try {
            const response = await fetch(`/api/library/lists/${this.listId}`);
            if (!response.ok) {
                throw new Error('List not found');
            }
            this.list = await response.json();
        } catch (error) {
            console.error('Failed to load list:', error);
            this.list = null;
        }
    }

    async loadDevices() {
        try {
            const response = await fetch(`/api/library/lists/${this.listId}/devices`);
            const data = await response.json();
            this.devices = data.devices || [];
        } catch (error) {
            console.error('Failed to load devices:', error);
            this.devices = [];
        }
    }

    async loadPreview() {
        try {
            const url = `/api/library/lists/${this.listId}/preview?limit=${this.previewLimit}&offset=${this.previewOffset}`;
            const response = await fetch(url);
            const data = await response.json();

            if (this.previewOffset === 0) {
                this.preview = data.comics || [];
            } else {
                this.preview = [...this.preview, ...(data.comics || [])];
            }

            this.previewTotal = data.total || 0;
        } catch (error) {
            console.error('Failed to load preview:', error);
            this.preview = [];
        }
    }

    render() {
        const app = document.getElementById('app');

        if (!this.list) {
            app.innerHTML = `
                <div class="error-page">
                    <h1>List Not Found</h1>
                    <p>The requested list could not be found.</p>
                    <button onclick="router.navigate('/lists')" class="btn btn-primary">
                        Back to Lists
                    </button>
                </div>
            `;
            return;
        }

        app.innerHTML = `
            <div class="list-detail-page">
                <!-- Breadcrumb -->
                <nav class="breadcrumb">
                    <a href="/lists" onclick="router.navigate('/lists'); return false;">Smart Lists</a>
                    <span class="separator">›</span>
                    <span class="current">${this.escapeHtml(this.list.name)}</span>
                </nav>

                <!-- Header -->
                <div class="list-detail-header">
                    <h1>${this.escapeHtml(this.list.name)}</h1>
                    <p class="list-count">
                        ${this.list.book_count.toLocaleString()} comics match this list
                    </p>
                </div>

                <!-- Main Content -->
                <div class="list-detail-content">
                    <!-- Matchers Panel -->
                    <div class="panel matchers-panel">
                        <h2>Matchers</h2>
                        <div class="matcher-mode">
                            ${this.list.matcher_mode_formatted}
                        </div>
                        <ul class="matchers-list">
                            ${this.renderMatchers()}
                        </ul>
                    </div>

                    <!-- Device Assignments Panel -->
                    <div class="panel devices-panel">
                        <h2>Device Assignments</h2>
                        <div class="device-assignments">
                            ${this.renderDeviceAssignments()}
                        </div>
                    </div>
                </div>

                <!-- Comics Preview -->
                <div class="panel preview-panel">
                    <h2>Comics Preview</h2>
                    <p class="preview-info">Showing ${this.preview.length} of ${this.previewTotal.toLocaleString()}</p>
                    <div class="comics-grid">
                        ${this.renderComicsPreview()}
                    </div>
                    ${this.renderLoadMore()}
                </div>
            </div>
        `;
    }

    renderMatchers() {
        if (!this.list.matchers || this.list.matchers.length === 0) {
            return '<li class="empty-message">No matchers defined</li>';
        }

        return this.list.matchers.map(matcher => `
            <li class="matcher-item">
                <span class="matcher-bullet">•</span>
                <span class="matcher-text">${this.escapeHtml(matcher)}</span>
            </li>
        `).join('');
    }

    renderDeviceAssignments() {
        if (this.devices.length === 0) {
            return `
                <p class="empty-message">This list is not assigned to any devices.</p>
                <button class="btn btn-primary" onclick="alert('Assign to device - TODO')">
                    + Assign to Device
                </button>
            `;
        }

        return this.devices.map(device => `
            <div class="device-assignment-card">
                <div class="device-assignment-info">
                    <h4>${this.escapeHtml(device.friendly_name)}</h4>
                    <span class="device-status ${device.enabled ? 'enabled' : 'disabled'}">
                        ${device.enabled ? 'Enabled' : 'Disabled'}
                    </span>
                </div>
                <div class="device-assignment-actions">
                    <button class="btn btn-small" onclick="router.navigate('/devices/${device.device_id}')">
                        View Device
                    </button>
                </div>
            </div>
        `).join('');
    }

    renderComicsPreview() {
        if (this.preview.length === 0) {
            return '<p class="empty-message">No comics to preview</p>';
        }

        return this.preview.map(comic => `
            <div class="comic-card">
                <div class="comic-placeholder">📖</div>
                <div class="comic-info">
                    <div class="comic-series">${this.escapeHtml(comic.series)}</div>
                    <div class="comic-number">#${this.escapeHtml(comic.number)}</div>
                    ${comic.title ? `<div class="comic-title">${this.escapeHtml(comic.title)}</div>` : ''}
                </div>
            </div>
        `).join('');
    }

    renderLoadMore() {
        if (this.preview.length >= this.previewTotal) {
            return '';
        }

        return `
            <div class="load-more-container">
                <button id="load-more-btn" class="btn btn-secondary">
                    Load More
                </button>
            </div>
        `;
    }

    attachListeners() {
        const loadMoreBtn = document.getElementById('load-more-btn');
        if (loadMoreBtn) {
            loadMoreBtn.addEventListener('click', async () => {
                loadMoreBtn.disabled = true;
                loadMoreBtn.textContent = 'Loading...';

                this.previewOffset += this.previewLimit;
                await this.loadPreview();
                this.render();
            });
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text || '';
        return div.innerHTML;
    }
}
```

**Step 2: Add list detail styles**

Add to `internal/api/web/css/lists.css`:

```css
/* List Detail Page */
.list-detail-page {
    padding: 2rem;
    max-width: 1200px;
    margin: 0 auto;
}

/* Breadcrumb */
.breadcrumb {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 1.5rem;
    font-size: 0.875rem;
}

.breadcrumb a {
    color: var(--primary-color);
    text-decoration: none;
}

.breadcrumb a:hover {
    text-decoration: underline;
}

.breadcrumb .separator {
    color: var(--text-secondary);
}

.breadcrumb .current {
    color: var(--text-primary);
    font-weight: 500;
}

/* Detail Header */
.list-detail-header {
    margin-bottom: 2rem;
}

.list-detail-header h1 {
    font-size: 2rem;
    font-weight: 700;
    margin-bottom: 0.5rem;
}

.list-count {
    font-size: 1rem;
    color: var(--text-secondary);
}

/* Detail Content */
.list-detail-content {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 1.5rem;
    margin-bottom: 2rem;
}

@media (max-width: 768px) {
    .list-detail-content {
        grid-template-columns: 1fr;
    }
}

/* Matchers Panel */
.matchers-panel {
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 0.5rem;
    padding: 1.5rem;
}

.matcher-mode {
    display: inline-block;
    padding: 0.5rem 1rem;
    background: var(--bg-primary);
    border: 1px solid var(--border-color);
    border-radius: 0.375rem;
    font-size: 0.875rem;
    font-weight: 600;
    color: var(--primary-color);
    margin-bottom: 1rem;
}

.matchers-list {
    list-style: none;
    padding: 0;
    margin: 0;
}

.matcher-item {
    display: flex;
    gap: 0.75rem;
    padding: 0.75rem 0;
    border-bottom: 1px solid var(--border-color);
}

.matcher-item:last-child {
    border-bottom: none;
}

.matcher-bullet {
    color: var(--primary-color);
    font-size: 1.25rem;
    line-height: 1;
}

.matcher-text {
    font-size: 0.875rem;
    color: var(--text-primary);
    font-family: 'Monaco', 'Menlo', monospace;
}

/* Device Assignments */
.devices-panel {
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 0.5rem;
    padding: 1.5rem;
}

.device-assignments {
    display: flex;
    flex-direction: column;
    gap: 1rem;
}

.device-assignment-card {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 1rem;
    background: var(--bg-primary);
    border: 1px solid var(--border-color);
    border-radius: 0.375rem;
}

.device-assignment-info h4 {
    font-size: 1rem;
    font-weight: 600;
    margin-bottom: 0.25rem;
}

.device-status {
    font-size: 0.75rem;
    padding: 0.25rem 0.5rem;
    border-radius: 0.25rem;
    font-weight: 500;
}

.device-status.enabled {
    background: #d1fae5;
    color: #065f46;
}

.device-status.disabled {
    background: #fee2e2;
    color: #991b1b;
}

/* Comics Preview */
.preview-panel {
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 0.5rem;
    padding: 1.5rem;
}

.preview-info {
    color: var(--text-secondary);
    font-size: 0.875rem;
    margin-bottom: 1rem;
}

.comics-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
    gap: 1rem;
    margin-bottom: 1.5rem;
}

.comic-card {
    background: var(--bg-primary);
    border: 1px solid var(--border-color);
    border-radius: 0.375rem;
    padding: 1rem;
    text-align: center;
    transition: all 0.2s;
}

.comic-card:hover {
    border-color: var(--primary-color);
    box-shadow: var(--shadow-sm);
}

.comic-placeholder {
    font-size: 3rem;
    margin-bottom: 0.5rem;
}

.comic-series {
    font-weight: 600;
    font-size: 0.875rem;
    margin-bottom: 0.25rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.comic-number {
    font-size: 0.75rem;
    color: var(--text-secondary);
    margin-bottom: 0.25rem;
}

.comic-title {
    font-size: 0.75rem;
    color: var(--text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.load-more-container {
    display: flex;
    justify-content: center;
}

.btn-secondary {
    background: var(--bg-primary);
    color: var(--text-primary);
    border: 1px solid var(--border-color);
}

.btn-secondary:hover {
    background: var(--border-color);
}
```

**Step 3: Register route**

Add to router registration:

```javascript
// Register /lists/:listId route
router.register('/lists/:listId', async (params) => {
    navigation.setActive('lists');
    const listDetail = new ListDetail(params.listId);
    await listDetail.init();
});
```

**Step 4: Test list detail page**

Navigate to http://localhost:7620/lists/{some-list-id}

Expected: Shows matchers, devices, preview with load more

**Step 5: Commit**

```bash
git add internal/api/web/js/lists.js internal/api/web/css/lists.css
git commit -m "feat: add list detail page with preview"
```

---

## Phase 5: Testing & Polish

### Task 12: Integration Testing

**Files:**
- Create: `internal/api/integration_test.go`

**Step 1: Write integration test for list endpoints**

Create `internal/api/integration_test.go`:

```go
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

func TestListsIntegration(t *testing.T) {
	// Setup test library
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{GUID: "c1", Series: "Batman", Number: "1", Title: "Test 1", Year: 2020},
			{GUID: "c2", Series: "Batman", Number: "2", Title: "Test 2", Year: 2021},
			{GUID: "c3", Series: "Superman", Number: "1", Title: "Test 3", Year: 2020},
		},
		ComicLists: []library.ComicListItem{
			{
				ID:          "list-1",
				Name:        "Batman Comics",
				Type:        "ComicSmartListItem",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{FieldName: "Series", MatchOperator: "Equals", MatchValue: "Batman"},
				},
			},
		},
	}

	cfg := &config.Config{
		Devices: map[string]*config.DeviceConfig{
			"device-1": {
				DeviceID:     "device-1",
				FriendlyName: "Test Device",
				Lists: []config.SharedListConfig{
					{ListID: "list-1", ListName: "Batman Comics", Enabled: true},
				},
			},
		},
	}

	server := &Server{
		library:   lib,
		config:    cfg,
		listCache: library.NewListCache(5 * time.Minute),
	}

	// Test GET /api/library/lists
	t.Run("GET /api/library/lists", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/library/lists", nil)
		w := httptest.NewRecorder()

		server.handleGetLists(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200, got %d", w.Code)
		}

		var response map[string]interface{}
		json.NewDecoder(w.Body).Decode(&response)

		lists := response["lists"].([]interface{})
		if len(lists) != 1 {
			t.Errorf("Expected 1 list, got %d", len(lists))
		}
	})

	// Test GET /api/library/lists/:listId
	t.Run("GET /api/library/lists/:listId", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/library/lists/list-1", nil)
		w := httptest.NewRecorder()

		server.handleGetListDetail(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200, got %d", w.Code)
		}

		var detail map[string]interface{}
		json.NewDecoder(w.Body).Decode(&detail)

		if detail["name"] != "Batman Comics" {
			t.Errorf("Expected 'Batman Comics', got %s", detail["name"])
		}
	})

	// Test GET /api/library/lists/:listId/preview
	t.Run("GET /api/library/lists/:listId/preview", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/library/lists/list-1/preview?limit=10", nil)
		w := httptest.NewRecorder()

		server.handleGetListPreview(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200, got %d", w.Code)
		}

		var preview map[string]interface{}
		json.NewDecoder(w.Body).Decode(&preview)

		comics := preview["comics"].([]interface{})
		if len(comics) != 2 {
			t.Errorf("Expected 2 comics, got %d", len(comics))
		}
	})

	// Test GET /api/library/lists/:listId/devices
	t.Run("GET /api/library/lists/:listId/devices", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/library/lists/list-1/devices", nil)
		w := httptest.NewRecorder()

		server.handleGetListDevices(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200, got %d", w.Code)
		}

		var response map[string]interface{}
		json.NewDecoder(w.Body).Decode(&response)

		devices := response["devices"].([]interface{})
		if len(devices) != 1 {
			t.Errorf("Expected 1 device, got %d", len(devices))
		}
	})
}
```

**Step 2: Run integration tests**

Run: `go test ./internal/api -v -run TestListsIntegration`
Expected: PASS (4 sub-tests)

**Step 3: Commit**

```bash
git add internal/api/integration_test.go
git commit -m "test: add integration tests for list endpoints"
```

---

### Task 13: Documentation

**Files:**
- Modify: `docs/API.md`
- Modify: `CLAUDE.md`

**Step 1: Document new API endpoints**

Add to `docs/API.md`:

```markdown
## Library Lists

### GET /api/library/lists

Returns all smart lists with cached book counts.

**Response:**
```json
{
  "lists": [
    {
      "id": "list-guid-123",
      "name": "Currently Reading",
      "type": "ComicSmartListItem",
      "matcher_mode": "And",
      "book_count": 2847,
      "matcher_count": 3
    }
  ],
  "total": 47
}
```

### GET /api/library/lists/:listId

Returns detailed information about a specific list.

**Response:**
```json
{
  "id": "list-guid-123",
  "name": "Currently Reading",
  "type": "ComicSmartListItem",
  "matcher_mode": "And",
  "matcher_mode_formatted": "Match ALL (AND)",
  "book_count": 2847,
  "matchers": [
    "Series contains 'Batman'",
    "Year is after 2020",
    "NOT Publisher equals 'DC'"
  ]
}
```

### GET /api/library/lists/:listId/preview

Returns a paginated preview of comics matching the list.

**Query Parameters:**
- `limit` (int, default: 20, max: 100) - Number of comics to return
- `offset` (int, default: 0) - Offset for pagination

**Response:**
```json
{
  "comics": [
    {
      "guid": "comic-guid-1",
      "series": "Batman",
      "number": "45",
      "title": "The Dark Knight Returns",
      "volume": 1,
      "publisher": "DC",
      "year": 2021
    }
  ],
  "total": 2847,
  "limit": 20,
  "offset": 0
}
```

### GET /api/library/lists/:listId/devices

Returns devices assigned to a specific list.

**Response:**
```json
{
  "devices": [
    {
      "device_id": "device-guid-1",
      "friendly_name": "Samsung Galaxy Tab",
      "enabled": true
    }
  ],
  "total": 2
}
```
```

**Step 2: Update CLAUDE.md**

Add to `CLAUDE.md` under "Project Structure":

```markdown
### Web UI (v0.8)

**Smart Lists as First-Class Entities:**

The Web UI now features client-side routing with dedicated pages for smart lists:

- `/` - Dashboard (overview)
- `/lists` - Smart lists browser with search and filtering
- `/lists/:listId` - List detail page with matchers, devices, and comic preview
- `/devices/:deviceId` - Device detail page (enhanced with list assignments)
- `/sync` - Sync history page

**Key Features:**
- Client-side routing using History API (vanilla JS, no framework)
- List count caching (15 min TTL) for performance with large libraries
- Paginated comic previews (20 per page, max 100)
- Human-readable matcher formatting
- Bidirectional navigation (lists → devices and devices → lists)

**Performance Optimizations:**
- List evaluation caching to avoid re-calculating 65K+ comic libraries
- Virtual scrolling for large lists (50+ lists paginated)
- Debounced search input (300ms)
- Lazy loading of comic previews
```

**Step 3: Commit**

```bash
git add docs/API.md CLAUDE.md
git commit -m "docs: document smart list UI and API endpoints"
```

---

## Verification & Completion

### Task 14: End-to-End Verification

**Step 1: Start server with test library**

```bash
go build -o comic-server
./comic-server server --library testdata/ComicDb.xml
```

**Step 2: Manual verification checklist**

Open http://localhost:7620 and verify:

- [ ] Navigation tabs visible and clickable
- [ ] Dashboard shows stats
- [ ] Click "Smart Lists" → Shows list browser
- [ ] Search lists → Filters work
- [ ] Sort by name/count → Updates grid
- [ ] Click a list card → Shows detail page
- [ ] List detail shows matchers in readable format
- [ ] Device assignments displayed (if any)
- [ ] Comics preview loads
- [ ] "Load More" button works
- [ ] Breadcrumb navigation works
- [ ] Browser back/forward buttons work
- [ ] Page refresh preserves current route

**Step 3: Run all tests**

```bash
go test ./... -v
```

Expected: All tests pass

**Step 4: Final commit**

```bash
git add -A
git commit -m "feat: smart list UI complete - all features working"
```

---

## Summary

**What we built:**

1. **Backend (Phase 1):**
   - List count caching system (`internal/library/cache.go`)
   - Matcher formatter for human-readable display (`internal/library/formatter.go`)
   - 4 new API endpoints (`internal/api/lists.go`):
     - GET /api/library/lists
     - GET /api/library/lists/:listId
     - GET /api/library/lists/:listId/preview
     - GET /api/library/lists/:listId/devices

2. **Frontend (Phases 2-4):**
   - Client-side router (`js/router.js`)
   - Navigation component (`js/navigation.js`)
   - Lists browser page (`js/lists.js`)
   - List detail page (matchers, devices, preview)
   - Responsive CSS for all components

3. **Testing & Docs (Phase 5):**
   - Integration tests for all endpoints
   - Updated API documentation
   - Updated CLAUDE.md

**Architecture benefits:**
- Scalable to 65K+ comics with caching
- Paginated preview (max 100 comics at a time)
- Fast client-side navigation
- Backward compatible (existing device modal still works)

**Next steps (not in this plan):**
- Device detail page enhancement (Task 4 from design)
- Device list assignment UI (POST/DELETE endpoints)
- WebSocket events for real-time updates
- Sync monitoring improvements (from design discussion)
