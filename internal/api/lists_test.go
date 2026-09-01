package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
	"path/filepath"
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

// TestBuildListTree_FoldersBeforeListsAlphabetical guards the tree ordering
// requested for the sidebar tree, the file-browser grid, and every picker
// dialog that reads /api/library/lists/tree (listPicker.js, dialogs.js's
// pickFolder) - they all share this one endpoint, so sorting once here
// covers all of them. At every level: folders first (A-Z), then lists
// (A-Z), recursively - the input here is deliberately out of order (not
// already alphabetical, not already folders-first) to prove the sort
// actually runs rather than happening to match insertion order.
func TestBuildListTree_FoldersBeforeListsAlphabetical(t *testing.T) {
	items := []library.ComicListItem{
		{ID: "list-zebra", Name: "Zebra List", Type: "ComicSmartListItem"},
		{
			ID: "folder-zeta", Name: "Zeta Folder", Type: "ComicListItemFolder",
			ChildItems: []library.ComicListItem{
				{ID: "nested-list-b", Name: "Bravo", Type: "ComicSmartListItem"},
				{ID: "nested-folder", Name: "Nested Folder", Type: "ComicListItemFolder"},
				{ID: "nested-list-a", Name: "Alpha", Type: "ComicSmartListItem"},
			},
		},
		{ID: "list-alpha", Name: "Alpha List", Type: "ComicSmartListItem"},
		{ID: "folder-alpha", Name: "Alpha Folder", Type: "ComicListItemFolder"},
	}

	backend := library.NewXMLBackendFromLibrary(&library.ComicLibrary{}, "", nil)
	server := &Server{backend: backend, listCache: library.NewListCache(5 * time.Minute)}
	tree := server.buildListTree(items)

	if len(tree) != 4 {
		t.Fatalf("expected 4 top-level nodes, got %d", len(tree))
	}
	gotNames := []string{tree[0].Name, tree[1].Name, tree[2].Name, tree[3].Name}
	wantNames := []string{"Alpha Folder", "Zeta Folder", "Alpha List", "Zebra List"}
	for i := range wantNames {
		if gotNames[i] != wantNames[i] {
			t.Fatalf("top-level order = %v, want %v", gotNames, wantNames)
		}
	}
	if !tree[0].IsFolder || !tree[1].IsFolder {
		t.Errorf("expected the first two top-level nodes to be folders, got IsFolder=%v, %v", tree[0].IsFolder, tree[1].IsFolder)
	}
	if tree[2].IsFolder || tree[3].IsFolder {
		t.Errorf("expected the last two top-level nodes to be lists, got IsFolder=%v, %v", tree[2].IsFolder, tree[3].IsFolder)
	}

	// The nested level (inside "Zeta Folder") must be sorted too.
	zeta := tree[1]
	if len(zeta.Children) != 3 {
		t.Fatalf("expected 3 children under Zeta Folder, got %d", len(zeta.Children))
	}
	gotChildNames := []string{zeta.Children[0].Name, zeta.Children[1].Name, zeta.Children[2].Name}
	wantChildNames := []string{"Nested Folder", "Alpha", "Bravo"}
	for i := range wantChildNames {
		if gotChildNames[i] != wantChildNames[i] {
			t.Fatalf("nested order = %v, want %v", gotChildNames, wantChildNames)
		}
	}
}

// TestHandleCreateList_ResponseHasLowercaseID is the regression test for
// comic-server-3rb: handleCreateList used to Encode() the raw
// library.ComicListItem directly, whose fields are only XML-tagged, so
// encoding/json fell back to their capitalized Go names ("ID", not "id").
// The frontend reads response.id right after creating a list to navigate
// to it (listsBrowser.js's showNewListDialog), so that silently sent the
// browser to /lists/undefined. This asserts the JSON body actually has a
// lowercase "id" that resolves back to a real list via a follow-up GET -
// not just that a field named ID exists somewhere in the struct.
func TestHandleCreateList_ResponseHasLowercaseID(t *testing.T) {
	lib := &library.ComicLibrary{}
	backend := library.NewXMLBackendFromLibrary(lib, filepath.Join(t.TempDir(), "ComicDb.xml"), nil)
	server := &Server{backend: backend, listCache: library.NewListCache(5 * time.Minute)}

	body := `{"name":"Currently Reading","type":"ComicSmartListItem","matcher_mode":"And","matchers":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/library/lists", strings.NewReader(body))
	w := httptest.NewRecorder()

	server.handleCreateList(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	// Decode into a map, not a tagged struct: encoding/json's Unmarshal
	// falls back to a case-INSENSITIVE field match, so a struct with a
	// `json:"id"` tag would happily accept a wire-format "ID" too and
	// mask the exact bug this test exists to catch - the frontend's
	// JS `created.id` property access is case-sensitive and would see
	// undefined for that same body.
	var raw map[string]any
	if err := json.NewDecoder(w.Body).Decode(&raw); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	id, ok := raw["id"].(string)
	if !ok || id == "" {
		t.Fatalf("expected a non-empty lowercase \"id\" key in the response body, got: %s", w.Body.String())
	}
	if name, _ := raw["name"].(string); name != "Currently Reading" {
		t.Errorf("expected lowercase \"name\" key %q, got %q (body: %s)", "Currently Reading", name, w.Body.String())
	}

	// The real-world failure mode: the frontend immediately GETs
	// /api/library/lists/<created id> to land on the new list's detail
	// page. Prove that ID actually resolves.
	getReq := httptest.NewRequest(http.MethodGet, "/api/library/lists/"+id, nil)
	getW := httptest.NewRecorder()
	server.handleGetListDetail(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("GET /api/library/lists/%s: expected 200, got %d: %s", id, getW.Code, getW.Body.String())
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

func TestHandleGetListDetail_NeedsConvertCountOmittedWhenDisabled(t *testing.T) {
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "1", Series: "Batman", FilePath: "/comics/batman.cbr"},
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
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	server := &Server{
		backend:   backend,
		listCache: library.NewListCache(5 * time.Minute),
		config:    &config.Config{Server: config.ServerConfig{CBZConvert: config.CBZConvertConfig{Enabled: false}}},
	}

	req := httptest.NewRequest("GET", "/api/library/lists/list-123", nil)
	w := httptest.NewRecorder()
	server.handleGetListDetail(w, req)

	var response ListDetail
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if response.NeedsConvertCount != nil {
		t.Errorf("expected NeedsConvertCount to be omitted when cbz_convert is disabled, got %v", *response.NeedsConvertCount)
	}
}

func TestHandleGetListDetail_NeedsConvertCountComputedWhenEnabled(t *testing.T) {
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "1", Series: "Batman", FilePath: "/comics/batman-1.cbr"},
			{ID: "2", Series: "Batman", FilePath: "/comics/batman-2.cbz"},
			{ID: "3", Series: "Batman", FilePath: "/comics/batman-3.cb7"},
			{ID: "4", Series: "Batman", FilePath: ""}, // no file - shouldn't count
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
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	server := &Server{
		backend:   backend,
		listCache: library.NewListCache(5 * time.Minute),
		config:    &config.Config{Server: config.ServerConfig{CBZConvert: config.CBZConvertConfig{Enabled: true}}},
	}

	req := httptest.NewRequest("GET", "/api/library/lists/list-123", nil)
	w := httptest.NewRecorder()
	server.handleGetListDetail(w, req)

	var response ListDetail
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if response.NeedsConvertCount == nil {
		t.Fatal("expected NeedsConvertCount to be set when cbz_convert is enabled")
	}
	if *response.NeedsConvertCount != 2 {
		t.Errorf("NeedsConvertCount = %d, want 2 (cbr + cb7, not cbz or the fileless entry)", *response.NeedsConvertCount)
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
	// Create config.db with devices assigned to lists
	configDB, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("failed to open test config db: %v", err)
	}
	t.Cleanup(func() { configDB.Close() })

	if err := configDB.UpsertDevice("device-1", "Samsung Tablet", time.Time{}, nil); err != nil {
		t.Fatalf("failed to seed device-1: %v", err)
	}
	if err := configDB.AddDeviceList("device-1", configdb.DeviceList{ListID: "list-123", ListName: "Batman", Enabled: true}); err != nil {
		t.Fatalf("failed to seed device-1 list: %v", err)
	}
	if err := configDB.UpsertDevice("device-2", "iPad Pro", time.Time{}, nil); err != nil {
		t.Fatalf("failed to seed device-2: %v", err)
	}
	if err := configDB.AddDeviceList("device-2", configdb.DeviceList{ListID: "list-456", ListName: "Superman", Enabled: true}); err != nil {
		t.Fatalf("failed to seed device-2 list: %v", err)
	}

	server := &Server{
		config:   &config.Config{},
		configDB: configDB,
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
