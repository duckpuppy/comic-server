package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/localid"
)

func recheckTestServer(t *testing.T) (*Server, *scrapeAPIFixture) {
	t.Helper()
	b := library.ComicBook{ID: "b1", Series: "Epic Collection", Number: "1", FilePath: "Epic Collection 1.cbz"}
	localid.Set(&b, localid.Identity{Volume: "lv-0000000001", Issue: "li-0000000001"})
	fx := &scrapeAPIFixture{searchResults: map[string][]comicvine.Volume{
		"Epic Collection": {{ID: 9, Name: "Epic Collection", StartYear: "2021", CountOfIssues: 5}},
	}}
	return newScrapeAPITestServer(t, []library.ComicBook{b}, fx), fx
}

func setRecheck(t *testing.T, s *Server, on bool) {
	t.Helper()
	body, _ := json.Marshal(ServerMiscSettingsResponse{LocalIdentityRecheck: on})
	w := httptest.NewRecorder()
	s.handleServerMiscSettings(w, httptest.NewRequest(http.MethodPut, "/api/settings/server-misc", bytes.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("put: %d %s", w.Code, w.Body.String())
	}
}

func reviewItems(t *testing.T, s *Server) []scrapeReviewItem {
	t.Helper()
	w := httptest.NewRecorder()
	s.handleScrapeReviewList(w, httptest.NewRequest(http.MethodGet, "/api/scrape/review", nil))
	var items []scrapeReviewItem
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	return items
}

func TestLocalRecheck_SettingDefaultsOffAndPassDoesNothingWhenOff(t *testing.T) {
	s, _ := recheckTestServer(t)
	if s.localIdentityRecheckOn() {
		t.Fatal("must default to off")
	}
	s.localRecheckPass(context.Background())
	if n, _ := s.cvCache.GetPendingReviewBooks(""); len(n) != 0 {
		t.Errorf("a pass with the setting off queued %+v", n)
	}
	setRecheck(t, s, true)
	if !s.localIdentityRecheckOn() {
		t.Error("setting not stored")
	}
	setRecheck(t, s, false)
	if s.localIdentityRecheckOn() {
		t.Error("setting not cleared")
	}
}

func TestLocalRecheck_SuggestionShowsInScrapeReviewAndDismisses(t *testing.T) {
	s, _ := recheckTestServer(t)
	setRecheck(t, s, true)
	s.localRecheckPass(context.Background())

	items := reviewItems(t, s)
	if len(items) != 1 || !items[0].LocalRecheck || items[0].Candidates[0].VolumeID != 9 {
		t.Fatalf("items = %+v", items)
	}

	w := httptest.NewRecorder()
	s.handleRecheckDismiss(w, httptest.NewRequest(http.MethodPost, "/api/scrape/recheck/dismiss/b1", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("dismiss: %d %s", w.Code, w.Body.String())
	}
	if items := reviewItems(t, s); len(items) != 0 {
		t.Errorf("dismissed suggestion still listed: %+v", items)
	}
	// Not queued again, and the identity is untouched.
	s.localRecheckPass(context.Background())
	if items := reviewItems(t, s); len(items) != 0 {
		t.Errorf("re-queued after dismiss: %+v", items)
	}
	b, _ := s.backend.GetBook("b1")
	if !localid.Has(b) {
		t.Error("dismiss must keep the local identity")
	}
}

func TestLocalRecheck_StaleSuggestionHiddenOnceIdentityRetired(t *testing.T) {
	s, _ := recheckTestServer(t)
	setRecheck(t, s, true)
	s.localRecheckPass(context.Background())
	b, _ := s.backend.GetBook("b1")
	localid.Clear(b)
	if err := s.backend.UpdateBook(b); err != nil {
		t.Fatal(err)
	}
	if items := reviewItems(t, s); len(items) != 0 {
		t.Errorf("stale suggestion listed: %+v", items)
	}
}

func TestLocalRecheck_OrdinaryLocalIdentityBookStillHiddenFromReview(t *testing.T) {
	s, _ := recheckTestServer(t)
	row := &comicvine.BookScrapeResult{BookID: "b1", Series: "x", Status: comicvine.BookStatusPendingReview,
		Candidates: []comicvine.ReviewCandidate{{VolumeID: 1}}}
	if err := s.cvCache.UpsertScrapeJobBook("job-1", row); err != nil {
		t.Fatal(err)
	}
	if items := reviewItems(t, s); len(items) != 0 {
		t.Errorf("an old pending row of a local-identity book must stay hidden: %+v", items)
	}
}

func TestLocalRecheck_DismissNeedsPostAndBookID(t *testing.T) {
	s, _ := recheckTestServer(t)
	w := httptest.NewRecorder()
	s.handleRecheckDismiss(w, httptest.NewRequest(http.MethodGet, "/api/scrape/recheck/dismiss/b1", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d", w.Code)
	}
	w = httptest.NewRecorder()
	s.handleRecheckDismiss(w, httptest.NewRequest(http.MethodPost, "/api/scrape/recheck/dismiss/", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty id = %d", w.Code)
	}
}
