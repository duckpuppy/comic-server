package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/device"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/syncstate"
	"github.com/duckpuppy/comic-server/internal/websocket"
)

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

	srv := NewServer(syncManager, registry, backend, cfg, "", VersionInfo{Version: "test"}, wsHub)

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
	srv := NewServer(syncstate.NewManager(10), device.NewRegistry(), backend, &config.Config{}, "", VersionInfo{}, websocket.NewHub())

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
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["job_id"] == "" {
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
