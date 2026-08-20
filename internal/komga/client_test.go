package komga

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, "test-api-key")
	return c, srv
}

func TestListAllSeries_Pagination(t *testing.T) {
	pages := [][]Series{
		{{ID: "1", Name: "A"}, {ID: "2", Name: "B"}},
		{{ID: "3", Name: "C"}},
	}

	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/series" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-api-key" {
			t.Errorf("missing/wrong X-API-Key header: %q", r.Header.Get("X-API-Key"))
		}

		page := r.URL.Query().Get("page")
		var pageNum int
		fmt.Sscanf(page, "%d", &pageNum)

		resp := pageResponse[Series]{
			Content: pages[pageNum],
			Number:  pageNum,
			Last:    pageNum == len(pages)-1,
		}
		json.NewEncoder(w).Encode(resp)
	})

	result, err := c.ListAllSeries(context.Background())
	if err != nil {
		t.Fatalf("ListAllSeries() error = %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 series across pages, got %d", len(result))
	}
	if result[0].ID != "1" || result[2].ID != "3" {
		t.Errorf("unexpected series order/content: %+v", result)
	}
}

func TestListAllBooks_Pagination(t *testing.T) {
	pages := [][]Book{
		{{ID: "b1", SeriesID: "s1"}},
		{{ID: "b2", SeriesID: "s1"}, {ID: "b3", SeriesID: "s2"}},
	}

	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/books" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		page := r.URL.Query().Get("page")
		var pageNum int
		fmt.Sscanf(page, "%d", &pageNum)

		resp := pageResponse[Book]{
			Content: pages[pageNum],
			Number:  pageNum,
			Last:    pageNum == len(pages)-1,
		}
		json.NewEncoder(w).Encode(resp)
	})

	result, err := c.ListAllBooks(context.Background())
	if err != nil {
		t.Fatalf("ListAllBooks() error = %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 books across pages, got %d", len(result))
	}
}

func TestUpsertCollection_CreatesWhenNotFound(t *testing.T) {
	var createBody map[string]any
	created := false

	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/collections":
			// Substring search returns no exact match.
			json.NewEncoder(w).Encode(pageResponse[collectionDto]{
				Content: []collectionDto{{ID: "other", Name: "Some Other Collection"}},
				Last:    true,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/collections":
			created = true
			json.NewDecoder(r.Body).Decode(&createBody)
			json.NewEncoder(w).Encode(collectionDto{ID: "new-id", Name: "My Collection"})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := c.UpsertCollection(context.Background(), "My Collection", []string{"s1", "s2"})
	if err != nil {
		t.Fatalf("UpsertCollection() error = %v", err)
	}
	if !created {
		t.Error("expected POST to create a new collection")
	}
	if createBody["name"] != "My Collection" {
		t.Errorf("unexpected create body name: %v", createBody["name"])
	}
}

func TestUpsertCollection_UpdatesWhenFoundByExactName(t *testing.T) {
	var patchBody map[string]any
	patched := false

	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/collections":
			// Substring search returns both an exact match and a decoy.
			json.NewEncoder(w).Encode(pageResponse[collectionDto]{
				Content: []collectionDto{
					{ID: "decoy", Name: "My Collection 2"},
					{ID: "exact-id", Name: "My Collection"},
				},
				Last: true,
			})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/collections/exact-id":
			patched = true
			json.NewDecoder(r.Body).Decode(&patchBody)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := c.UpsertCollection(context.Background(), "My Collection", []string{"s1"})
	if err != nil {
		t.Fatalf("UpsertCollection() error = %v", err)
	}
	if !patched {
		t.Error("expected PATCH to the exact-name-matched collection")
	}
	seriesIDs, _ := patchBody["seriesIds"].([]any)
	if len(seriesIDs) != 1 || seriesIDs[0] != "s1" {
		t.Errorf("unexpected patch body seriesIds: %v", patchBody["seriesIds"])
	}
}

func TestUpsertReadList_CreatesWhenNotFound(t *testing.T) {
	created := false

	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/readlists":
			json.NewEncoder(w).Encode(pageResponse[readListDto]{Last: true})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/readlists":
			created = true
			json.NewEncoder(w).Encode(readListDto{ID: "new-id", Name: "My Read List"})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := c.UpsertReadList(context.Background(), "My Read List", []string{"b1"})
	if err != nil {
		t.Fatalf("UpsertReadList() error = %v", err)
	}
	if !created {
		t.Error("expected POST to create a new read list")
	}
}

func TestUpsertReadList_UpdatesWhenFoundByExactName(t *testing.T) {
	patched := false

	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/readlists":
			json.NewEncoder(w).Encode(pageResponse[readListDto]{
				Content: []readListDto{{ID: "rl-id", Name: "My Read List"}},
				Last:    true,
			})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/readlists/rl-id":
			patched = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := c.UpsertReadList(context.Background(), "My Read List", []string{"b1", "b2"})
	if err != nil {
		t.Fatalf("UpsertReadList() error = %v", err)
	}
	if !patched {
		t.Error("expected PATCH to the exact-name-matched read list")
	}
}

func TestClient_ErrorOnNon2xx(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"invalid api key"}`))
	})

	_, err := c.ListAllSeries(context.Background())
	if err == nil {
		t.Fatal("expected error on 401 response, got nil")
	}
}
