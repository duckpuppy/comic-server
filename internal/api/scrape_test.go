package api

import (
	"encoding/json"
	"fmt"
	"github.com/duckpuppy/comic-server/internal/workflow"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/device"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/syncstate"
	"github.com/duckpuppy/comic-server/internal/websocket"
)

func newTestConfigDB(t *testing.T) *configdb.DB {
	t.Helper()
	db, err := configdb.Open(t.TempDir() + "/config.db")
	if err != nil {
		t.Fatalf("failed to open test config db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// scrapeAPIFixture is an in-memory ComicVine API fixture, mirroring the one
// used by internal/comicvine's scraper tests.
type scrapeAPIFixture struct {
	mu             sync.Mutex
	searchResults  map[string][]comicvine.Volume
	issuesByVolume map[int][]comicvine.Issue
	issueDetails   map[int]*comicvine.IssueDetail
}

func newScrapeAPITestServer(t *testing.T, books []library.ComicBook, fx *scrapeAPIFixture) *Server {
	t.Helper()

	cvSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.Path, "/")
		switch {
		case strings.HasPrefix(path, "/search"):
			fx.mu.Lock()
			results := fx.searchResults[r.URL.Query().Get("query")]
			fx.mu.Unlock()
			json.NewEncoder(w).Encode(map[string]any{"status_code": 1, "results": results})
		case strings.HasPrefix(path, "/issues"):
			var volID int
			fmt.Sscanf(r.URL.Query().Get("filter"), "volume:%d", &volID)
			fx.mu.Lock()
			issues := fx.issuesByVolume[volID]
			fx.mu.Unlock()
			json.NewEncoder(w).Encode(map[string]any{
				"status_code": 1, "results": issues,
				"number_of_total_results": len(issues), "number_of_page_results": len(issues),
			})
		case strings.HasPrefix(path, "/issue/4000-"):
			id, _ := strconv.Atoi(strings.TrimPrefix(path, "/issue/4000-"))
			fx.mu.Lock()
			detail := fx.issueDetails[id]
			fx.mu.Unlock()
			if detail == nil {
				json.NewEncoder(w).Encode(map[string]any{"status_code": 101, "error": "Object Not Found"})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"status_code": 1, "results": detail})
		case strings.HasPrefix(path, "/volume/4050-"):
			id, _ := strconv.Atoi(strings.TrimPrefix(path, "/volume/4050-"))
			fx.mu.Lock()
			var found *comicvine.Volume
			for _, vols := range fx.searchResults {
				for i := range vols {
					if vols[i].ID == id {
						found = &vols[i]
					}
				}
			}
			fx.mu.Unlock()
			if found == nil {
				json.NewEncoder(w).Encode(map[string]any{"status_code": 101, "error": "Object Not Found"})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"status_code": 1, "results": found})
		default:
			t.Errorf("unexpected ComicVine request path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(cvSrv.Close)

	lib := &library.ComicLibrary{Books: books}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncManager := syncstate.NewManager(10)
	registry := device.NewRegistry()
	cfg := &config.Config{}
	wsHub := websocket.NewHub()

	srv := NewServer(syncManager, registry, backend, cfg, "", VersionInfo{Version: "test"}, wsHub, newTestConfigDB(t))

	cache, err := comicvine.OpenCache(t.TempDir() + "/cv_cache.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cache.Close() })

	client := comicvine.NewClient("test-key", comicvine.WithBaseURL(cvSrv.URL))
	srv.SetScraper(client, cache)
	return srv
}

func TestHandleScrapeStart_NotConfigured(t *testing.T) {
	lib := &library.ComicLibrary{}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	srv := NewServer(syncstate.NewManager(10), device.NewRegistry(), backend, &config.Config{}, "", VersionInfo{}, websocket.NewHub(), newTestConfigDB(t))

	req := httptest.NewRequest(http.MethodPost, "/api/scrape", nil)
	w := httptest.NewRecorder()
	srv.handleScrapeStart(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestHandleScrapeStart_HighConfidence(t *testing.T) {
	books := []library.ComicBook{{ID: "book-1", FilePath: "Batman 001.cbz"}}
	fx := &scrapeAPIFixture{
		searchResults: map[string][]comicvine.Volume{
			"Batman": {{ID: 100, Name: "Batman", StartYear: "2016", CountOfIssues: 85}},
		},
		issuesByVolume: map[int][]comicvine.Issue{100: {{ID: 1001, IssueNumber: "1"}}},
		issueDetails:   map[int]*comicvine.IssueDetail{1001: {ID: 1001, IssueNumber: "1", Name: "I Am Gotham"}},
	}
	srv := newScrapeAPITestServer(t, books, fx)

	body := strings.NewReader(`{"book_ids":["book-1"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/scrape", body)
	w := httptest.NewRecorder()
	srv.handleScrapeStart(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["job_id"] == "" || resp["job_id"] == nil {
		t.Error("expected non-empty job_id")
	}

	// The scrape runs in a goroutine; poll briefly for completion.
	waitForJobStatus(t, srv, comicvine.JobStatusComplete)

	job := srv.scraper.CurrentJob()
	if job.Completed != 1 {
		t.Errorf("job = %+v", job)
	}
}

func TestHandleScrapeStart_MethodNotAllowed(t *testing.T) {
	srv := newScrapeAPITestServer(t, nil, &scrapeAPIFixture{})
	req := httptest.NewRequest(http.MethodGet, "/api/scrape", nil)
	w := httptest.NewRecorder()
	srv.handleScrapeStart(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleScrapeStatus_NoJobYet(t *testing.T) {
	srv := newScrapeAPITestServer(t, nil, &scrapeAPIFixture{})
	req := httptest.NewRequest(http.MethodGet, "/api/scrape/status", nil)
	w := httptest.NewRecorder()
	srv.handleScrapeStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "none" {
		t.Errorf("resp = %+v", resp)
	}
}

func TestHandleScrapeReviewList_AndResolve(t *testing.T) {
	book := library.ComicBook{ID: "book-1", FilePath: "Ambiguous Comic 001.cbz"}
	fx := &scrapeAPIFixture{
		searchResults: map[string][]comicvine.Volume{
			"Ambiguous Comic": {
				{ID: 1, Name: "Ambiguous Comic", StartYear: "2020", CountOfIssues: 150},
				{ID: 2, Name: "Ambiguous Comic", StartYear: "2020", CountOfIssues: 150},
			},
		},
		issuesByVolume: map[int][]comicvine.Issue{2: {{ID: 2001, IssueNumber: "1"}}},
		issueDetails:   map[int]*comicvine.IssueDetail{2001: {ID: 2001, IssueNumber: "1", Name: "Chosen"}},
	}
	srv := newScrapeAPITestServer(t, []library.ComicBook{book}, fx)

	req := httptest.NewRequest(http.MethodPost, "/api/scrape", strings.NewReader(`{"book_ids":["book-1"]}`))
	w := httptest.NewRecorder()
	srv.handleScrapeStart(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, body = %s", w.Code, w.Body.String())
	}
	waitForJobStatus(t, srv, comicvine.JobStatusComplete)

	// List pending reviews.
	reviewReq := httptest.NewRequest(http.MethodGet, "/api/scrape/review", nil)
	reviewW := httptest.NewRecorder()
	srv.handleScrapeReviewList(reviewW, reviewReq)
	if reviewW.Code != http.StatusOK {
		t.Fatalf("review status = %d, body = %s", reviewW.Code, reviewW.Body.String())
	}
	var pending []*comicvine.BookScrapeResult
	if err := json.Unmarshal(reviewW.Body.Bytes(), &pending); err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || len(pending[0].Candidates) != 2 {
		t.Fatalf("pending = %+v", pending)
	}

	// Resolve it.
	resolveReq := httptest.NewRequest(http.MethodPost, "/api/scrape/review/book-1", strings.NewReader(`{"volume_cv_id":2}`))
	resolveW := httptest.NewRecorder()
	srv.handleScrapeReviewResolve(resolveW, resolveReq)
	if resolveW.Code != http.StatusOK {
		t.Fatalf("resolve status = %d, body = %s", resolveW.Code, resolveW.Body.String())
	}
	var result comicvine.BookScrapeResult
	if err := json.Unmarshal(resolveW.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != comicvine.BookStatusScraped || result.VolumeID != 2 {
		t.Errorf("result = %+v", result)
	}
}

// An empty queue must serialize as [] (not null) so the web UI can iterate
// without a special case (comic-server-0nhp).
func TestHandleScrapeReviewList_EmptyIsArray(t *testing.T) {
	srv := newScrapeAPITestServer(t, nil, &scrapeAPIFixture{})
	req := httptest.NewRequest(http.MethodGet, "/api/scrape/review", nil)
	w := httptest.NewRecorder()
	srv.handleScrapeReviewList(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "[]" {
		t.Errorf("body = %q, want []", got)
	}
}

func TestHandleScrapeReviewResolve_MissingVolumeID(t *testing.T) {
	srv := newScrapeAPITestServer(t, nil, &scrapeAPIFixture{})
	req := httptest.NewRequest(http.MethodPost, "/api/scrape/review/book-1", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	srv.handleScrapeReviewResolve(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func waitForJobStatus(t *testing.T, srv *Server, want string) {
	t.Helper()
	waitUntil(t, func() bool {
		job := srv.scraper.CurrentJob()
		return job != nil && job.Status == want
	})
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

func TestHandleScrapeStart_WholeLibraryHoldsBackKnownProblems(t *testing.T) {
	books := []library.ComicBook{
		{ID: "new", FilePath: "Foo 001.cbz"},
		{ID: "skip", FilePath: "Foo 002.cbz", Tags: "CVDBSKIP"},
		{ID: "review", FilePath: "Foo 003.cbz"},
		{ID: "nomatch", FilePath: "Foo 004.cbz"},
		{ID: "transient", FilePath: "Foo 005.cbz"},
		{ID: "later-ok", FilePath: "Foo 006.cbz"},
	}
	srv := newScrapeAPITestServer(t, books, &scrapeAPIFixture{})
	old := &comicvine.ScrapeJob{ID: "old", Status: comicvine.JobStatusComplete, StartedAt: time.Now().Add(-2 * time.Hour), UpdatedAt: time.Now()}
	newer := &comicvine.ScrapeJob{ID: "newer", Status: comicvine.JobStatusComplete, StartedAt: time.Now().Add(-time.Hour), UpdatedAt: time.Now()}
	for _, j := range []*comicvine.ScrapeJob{old, newer} {
		if err := srv.cvCache.SaveScrapeJobMeta(j); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range []struct {
		job string
		res comicvine.BookScrapeResult
	}{
		{"old", comicvine.BookScrapeResult{BookID: "review", Status: comicvine.BookStatusPendingReview}},
		{"old", comicvine.BookScrapeResult{BookID: "nomatch", Status: comicvine.BookStatusFailed, Error: "no candidate volumes found"}},
		{"old", comicvine.BookScrapeResult{BookID: "transient", Status: comicvine.BookStatusFailed, Error: "search volumes: timeout"}},
		// Held back in an old job, but the newer job's outcome is what counts.
		{"old", comicvine.BookScrapeResult{BookID: "later-ok", Status: comicvine.BookStatusPendingReview}},
		{"newer", comicvine.BookScrapeResult{BookID: "later-ok", Status: comicvine.BookStatusFailed, Error: "fetch issues: 502"}},
	} {
		res := r.res
		if err := srv.cvCache.UpsertScrapeJobBook(r.job, &res); err != nil {
			t.Fatal(err)
		}
	}

	w := httptest.NewRecorder()
	srv.handleScrapeStart(w, httptest.NewRequest(http.MethodPost, "/api/scrape", nil))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		TotalBooks int            `json:"total_books"`
		HeldBack   map[string]int `json:"held_back"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"tagged CVDBSKIP": 1, "awaiting Scrape Review": 1, "no match on ComicVine": 1}
	if resp.TotalBooks != 3 || len(resp.HeldBack) != len(want) {
		t.Fatalf("resp = %+v", resp)
	}
	for k, v := range want {
		if resp.HeldBack[k] != v {
			t.Errorf("held_back[%q] = %d, want %d", k, resp.HeldBack[k], v)
		}
	}
	waitForJobStatus(t, srv, comicvine.JobStatusComplete)

	// retry_held_back brings back the past-outcome books, never CVDBSKIP.
	w = httptest.NewRecorder()
	srv.handleScrapeStart(w, httptest.NewRequest(http.MethodPost, "/api/scrape", strings.NewReader(`{"options":{"retry_held_back":true}}`)))
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if w.Code != http.StatusAccepted || resp.TotalBooks != 5 || resp.HeldBack["tagged CVDBSKIP"] != 1 {
		t.Fatalf("retry: status %d resp %+v", w.Code, resp)
	}
	waitForJobStatus(t, srv, comicvine.JobStatusComplete)
}

func TestFilterHeldBack_SkipsMarkedNotInComicVine(t *testing.T) {
	s := &Server{}
	marked := &library.ComicBook{ID: "m", FilePath: "a.cbz"}
	workflow.SetNoCVMatch(marked, "collected", "2026-10-16")
	plain := &library.ComicBook{ID: "p", FilePath: "b.cbz"}
	kept, counts := s.filterHeldBack([]*library.ComicBook{marked, plain}, true)
	if len(kept) != 1 || kept[0].ID != "p" {
		t.Fatalf("kept = %+v", kept)
	}
	if counts["marked not in ComicVine"] != 1 {
		t.Errorf("counts = %v (retry must not re-include marked books)", counts)
	}
}

func TestCVMatch_PreviewVolumeApplyAndErrors(t *testing.T) {
	d := &comicvine.IssueDetail{ID: 5001, IssueNumber: "1", Name: "Collected"}
	d.Volume.ID, d.Volume.Name = 200, "Young Omnibus"
	fx := &scrapeAPIFixture{
		searchResults:  map[string][]comicvine.Volume{"x": {{ID: 200, Name: "Young Omnibus", StartYear: "2012", Publisher: comicvine.Publisher{Name: "Marvel"}, CountOfIssues: 1}}},
		issuesByVolume: map[int][]comicvine.Issue{200: {{ID: 5001, IssueNumber: "1", Name: "Collected"}}},
		issueDetails:   map[int]*comicvine.IssueDetail{5001: d},
	}
	book := library.ComicBook{ID: "b1", Series: "Young Omnibus", FilePath: "a.cbz", PageCount: 121}
	workflow.SetNoCVMatch(&book, "collected", "2026-10-16")
	s := newScrapeAPITestServer(t, []library.ComicBook{book}, fx)

	post := func(body string, out any) int {
		w := httptest.NewRecorder()
		s.handleBooksRouter(w, httptest.NewRequest(http.MethodPost, "/api/library/books/b1/cv-match", strings.NewReader(body)))
		if out != nil && w.Code == http.StatusOK {
			_ = json.NewDecoder(w.Body).Decode(out)
		}
		return w.Code
	}

	var prev CVMatchResponse
	if c := post(`{"ref":"https://comicvine.gamespot.com/young/4000-5001/"}`, &prev); c != http.StatusOK {
		t.Fatalf("preview status %d", c)
	}
	if prev.Kind != "issue" || prev.Issue == nil || prev.Issue.VolumeName != "Young Omnibus" || prev.Applied || prev.Book.PageCount != 121 {
		t.Fatalf("issue preview = %+v", prev)
	}
	var d0 BookDetail
	getJSON(t, s, "/api/library/books/b1", &d0)
	if d0.ComicVine != nil {
		t.Fatal("preview changed the book")
	}

	var vol CVMatchResponse
	if c := post(`{"ref":"4050-200"}`, &vol); c != http.StatusOK || vol.Kind != "volume" || len(vol.Volume.Issues) != 1 {
		t.Fatalf("volume preview = %d %+v", c, vol)
	}

	for _, bad := range []string{`{"ref":"hello"}`, `{"ref":"https://comicvine.gamespot.com/x/4005-1/"}`, `{}`} {
		if c := post(bad, nil); c != http.StatusBadRequest {
			t.Errorf("%s -> %d, want 400", bad, c)
		}
	}
	if c := post(`{"ref":"4000-99999"}`, nil); c != http.StatusBadGateway {
		t.Errorf("unknown issue -> %d, want 502", c)
	}

	var done CVMatchResponse
	if c := post(`{"ref":"4050-200","issue_id":5001,"apply":true}`, &done); c != http.StatusOK || !done.Applied {
		t.Fatalf("apply = %d %+v", c, done)
	}
	var d1 BookDetail
	getJSON(t, s, "/api/library/books/b1", &d1)
	if d1.ComicVine == nil || d1.ComicVine.IssueID != 5001 || d1.ComicVine.VolumeID != 200 {
		t.Errorf("comicvine = %+v", d1.ComicVine)
	}
	if d1.NoCVMatch != nil {
		t.Errorf("not-in-ComicVine marker survived a real match: %+v", d1.NoCVMatch)
	}
}

func TestScrapeReviewList_FlagsUnnumberedBooks(t *testing.T) {
	books := []library.ComicBook{
		{ID: "u1", Series: "Godzilla", FilePath: "/c/Godzilla Here There Be Aliens (2026).cbz"},
		{ID: "n1", Series: "Batman", FilePath: "/c/Batman 005 (2016).cbz"},
	}
	s := newScrapeAPITestServer(t, books, &scrapeAPIFixture{})
	for _, id := range []string{"u1", "n1"} {
		r := &comicvine.BookScrapeResult{BookID: id, Filename: id, Status: comicvine.BookStatusPendingReview,
			Candidates: []comicvine.ReviewCandidate{{VolumeID: 1, Name: "V", IssueCount: 5, CoverURL: "https://img/x.jpg"}}}
		if err := s.cvCache.UpsertScrapeJobBook("j1", r); err != nil {
			t.Fatal(err)
		}
	}
	w := httptest.NewRecorder()
	s.handleScrapeReviewList(w, httptest.NewRequest(http.MethodGet, "/api/scrape/review", nil))
	var items []struct {
		BookID     string `json:"book_id"`
		NeedsIssue bool   `json:"needs_issue"`
		Book       struct {
			Series string `json:"series"`
		} `json:"book"`
		Candidates []struct {
			IssueCount int    `json:"issue_count"`
			CoverURL   string `json:"cover_url"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(w.Body).Decode(&items); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, it := range items {
		got[it.BookID] = it.NeedsIssue
		if len(it.Candidates) != 1 || it.Candidates[0].IssueCount != 5 || it.Candidates[0].CoverURL == "" {
			t.Errorf("%s candidates = %+v", it.BookID, it.Candidates)
		}
	}
	if len(got) != 2 || !got["u1"] || got["n1"] {
		t.Errorf("needs_issue = %v, want u1 true and n1 false", got)
	}
}
