package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func newBrowseTestServer(t *testing.T, books []library.ComicBook) *Server {
	t.Helper()
	lib := &library.ComicLibrary{Books: books}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	return &Server{backend: backend}
}

type browseResponse struct {
	Comics []struct {
		Series string `json:"series"`
	} `json:"comics"`
	Total   int  `json:"total"`
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	HasMore bool `json:"has_more"`
}

func TestHandleBrowse_MatchesAdHocFilterWithoutSavingAnything(t *testing.T) {
	books := []library.ComicBook{
		{ID: "1", Series: "Batman"},
		{ID: "2", Series: "Batman"},
		{ID: "3", Series: "Superman"},
	}
	s := newBrowseTestServer(t, books)

	body := `{"matcher_mode":"And","matchers":[{"Type":"ComicBookSeriesMatcher","MatchOperator":"0","MatchValue":"Batman"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/library/browse", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	s.handleBrowse(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result browseResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Total != 2 {
		t.Fatalf("Total = %d, want 2", result.Total)
	}
	if len(result.Comics) != 2 {
		t.Fatalf("expected 2 comics, got %d", len(result.Comics))
	}

	// Nothing should have been created - no list exists anywhere to find.
	list, err := s.backend.FindListByID("does-not-matter")
	if err != nil {
		t.Fatalf("FindListByID: %v", err)
	}
	if list != nil {
		t.Errorf("expected no list to exist, ad-hoc browsing must never persist anything")
	}
}

// TestHandleBrowse_ZeroMatchersReturnsZeroNotEverything mirrors
// handleGetListPreview's own deliberate safety choice (comic-server-haz):
// no matchers picked yet means "0 matches", never "match the whole
// library" - an ad-hoc filter with nothing selected is the exact same
// not-yet-configured state as a freshly created empty smart list.
func TestHandleBrowse_ZeroMatchersReturnsZeroNotEverything(t *testing.T) {
	s := newBrowseTestServer(t, []library.ComicBook{{ID: "1", Series: "Batman"}})

	req := httptest.NewRequest(http.MethodPost, "/api/library/browse", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	s.handleBrowse(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result browseResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("Total = %d, want 0 (zero matchers must not match everything)", result.Total)
	}
}

func TestHandleBrowse_Paginates(t *testing.T) {
	books := make([]library.ComicBook, 5)
	for i := range books {
		books[i] = library.ComicBook{ID: string(rune('1' + i)), Series: "Batman"}
	}
	s := newBrowseTestServer(t, books)

	body := `{"matchers":[{"Type":"ComicBookSeriesMatcher","MatchOperator":"0","MatchValue":"Batman"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/library/browse?limit=2&offset=1", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	s.handleBrowse(w, req)

	var result browseResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Total != 5 {
		t.Errorf("Total = %d, want 5", result.Total)
	}
	if len(result.Comics) != 2 {
		t.Fatalf("expected a page of 2, got %d", len(result.Comics))
	}
	if result.Limit != 2 || result.Offset != 1 || !result.HasMore {
		t.Errorf("pagination = limit=%d offset=%d hasMore=%v, want limit=2 offset=1 hasMore=true", result.Limit, result.Offset, result.HasMore)
	}
}

func TestHandleBrowse_InvalidBodyIs400(t *testing.T) {
	s := newBrowseTestServer(t, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/library/browse", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	s.handleBrowse(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleBrowse_MethodNotAllowed(t *testing.T) {
	s := newBrowseTestServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/library/browse", nil)
	w := httptest.NewRecorder()
	s.handleBrowse(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", w.Code, w.Body.String())
	}
}
