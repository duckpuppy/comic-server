package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func startBrowseDMJob(t *testing.T, s *Server, apply bool, body string) string {
	t.Helper()
	path := "/api/library/browse/datamanager-preview"
	if apply {
		path = "/api/library/browse/datamanager-apply"
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	if apply {
		s.handleBrowseDataManagerApply(w, req)
	} else {
		s.handleBrowseDataManagerPreview(w, req)
	}
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	return resp["job_id"]
}

// TestHandleBrowseDataManager_ScopesToAdHocFilterNotWholeLibrary is the
// core of comic-server-w7ig: Browse's ad-hoc matcher set, not the whole
// library, decides which books Data Manager evaluates.
func TestHandleBrowseDataManager_ScopesToAdHocFilterNotWholeLibrary(t *testing.T) {
	books := []library.ComicBook{
		{ID: "1", Series: "Batman"},
		{ID: "2", Series: "Some Unrelated Series"},
	}
	s, db := newDataManagerLibraryTestServer(t, books)
	seedBatmanRuleset(t, db)

	body := `{"matcher_mode":"And","matchers":[{"Type":"ComicBookSeriesMatcher","MatchOperator":"0","MatchValue":"Batman"}]}`
	startBrowseDMJob(t, s, false, body)
	result := waitForDMJob(t, s, "")

	if result.Total != 1 {
		t.Errorf("Total = %d, want 1 (only the Batman-matching book, not the whole library)", result.Total)
	}
	if result.Changed != 1 || len(result.Books) != 1 || result.Books[0].BookID != "1" {
		t.Fatalf("expected exactly book 1 to have changed, got %+v", result)
	}
}

// TestHandleBrowseDataManager_ZeroMatchersMeansZeroBooks confirms the
// comic-server-haz safety rule still holds here: no matchers picked (and
// ShowAll not set) means nothing is in scope, not "everything".
func TestHandleBrowseDataManager_ZeroMatchersMeansZeroBooks(t *testing.T) {
	books := []library.ComicBook{{ID: "1", Series: "Batman"}}
	s, db := newDataManagerLibraryTestServer(t, books)
	seedBatmanRuleset(t, db)

	startBrowseDMJob(t, s, false, `{}`)
	result := waitForDMJob(t, s, "")

	if result.Total != 0 {
		t.Errorf("Total = %d, want 0 (zero matchers, ShowAll not set, must not match everything)", result.Total)
	}
}

// TestHandleBrowseDataManager_ShowAllScopesToWholeLibrary confirms the
// explicit "Show all books" opt-in works as its own separate path from
// the matcher list.
func TestHandleBrowseDataManager_ShowAllScopesToWholeLibrary(t *testing.T) {
	books := []library.ComicBook{
		{ID: "1", Series: "Batman"},
		{ID: "2", Series: "Some Unrelated Series"},
	}
	s, db := newDataManagerLibraryTestServer(t, books)
	seedBatmanRuleset(t, db)

	startBrowseDMJob(t, s, false, `{"show_all":true}`)
	result := waitForDMJob(t, s, "")

	if result.Total != 2 {
		t.Errorf("Total = %d, want 2 (Show All should scope to the whole library)", result.Total)
	}
	if result.Changed != 1 {
		t.Errorf("Changed = %d, want 1 (only the Batman book actually changes)", result.Changed)
	}
}

// TestHandleBrowseDataManagerApply_CommitsChanges confirms apply (not
// just preview) actually writes through Browse's own endpoint.
func TestHandleBrowseDataManagerApply_CommitsChanges(t *testing.T) {
	books := []library.ComicBook{{ID: "1", Series: "Batman"}}
	s, db := newDataManagerLibraryTestServer(t, books)
	seedBatmanRuleset(t, db)

	body := `{"matcher_mode":"And","matchers":[{"Type":"ComicBookSeriesMatcher","MatchOperator":"0","MatchValue":"Batman"}]}`
	startBrowseDMJob(t, s, true, body)
	result := waitForDMJob(t, s, "")

	if result.Changed != 1 {
		t.Fatalf("expected 1 book applied, got %+v", result)
	}
	book, err := s.backend.GetBook("1")
	if err != nil || book == nil || book.SeriesGroup != "Batman Family" {
		t.Errorf("expected book 1's SeriesGroup to be committed, got %+v (err=%v)", book, err)
	}
}

func TestHandleBrowseDataManager_NoRulesConfigured(t *testing.T) {
	s, _ := newDataManagerLibraryTestServer(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/library/browse/datamanager-preview", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	s.handleBrowseDataManagerPreview(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 with no rules configured, got %d: %s", w.Code, w.Body.String())
	}
}
