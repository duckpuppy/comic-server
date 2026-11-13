package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

// TestGetOrBuildWorkflowCache_ReusesSnapshotUntilInvalidated proves the
// cache is actually being reused, not silently rebuilt on every call: a
// stage change made directly against the backend (bypassing every API
// handler, the same way an external process or a future code path might)
// is invisible to a second read until InvalidateWorkflowCache is called.
func TestGetOrBuildWorkflowCache_ReusesSnapshotUntilInvalidated(t *testing.T) {
	book := library.ComicBook{ID: "1", FilePath: "/comics/book1.cbz"}
	workflow.SetStage(&book, workflow.StageScrape)
	s := newWorkflowTestServer(t, []library.ComicBook{book})

	first, err := s.getOrBuildWorkflowCache()
	if err != nil {
		t.Fatalf("getOrBuildWorkflowCache: %v", err)
	}
	if len(first.booksByStage[workflow.StageScrape]) != 1 {
		t.Fatalf("expected 1 book at StageScrape, got %d", len(first.booksByStage[workflow.StageScrape]))
	}

	// Change the underlying book's stage directly, bypassing any API
	// handler (and therefore bypassing InvalidateWorkflowCache too).
	changed, err := s.backend.GetBook("1")
	if err != nil || changed == nil {
		t.Fatalf("GetBook: %v", err)
	}
	workflow.SetStage(changed, workflow.StageDataManager)
	if err := s.backend.UpdateBooks([]*library.ComicBook{changed}); err != nil {
		t.Fatalf("UpdateBooks: %v", err)
	}

	second, err := s.getOrBuildWorkflowCache()
	if err != nil {
		t.Fatalf("getOrBuildWorkflowCache: %v", err)
	}
	if second != first {
		t.Fatalf("expected the SAME cached entry back (no rebuild), got a different pointer")
	}
	if len(second.booksByStage[workflow.StageScrape]) != 1 {
		t.Errorf("expected stale cache to still report 1 book at StageScrape, got %d", len(second.booksByStage[workflow.StageScrape]))
	}

	s.InvalidateWorkflowCache()

	third, err := s.getOrBuildWorkflowCache()
	if err != nil {
		t.Fatalf("getOrBuildWorkflowCache: %v", err)
	}
	if third == first {
		t.Fatal("expected a freshly rebuilt entry after InvalidateWorkflowCache, got the same one back")
	}
	if len(third.booksByStage[workflow.StageScrape]) != 0 {
		t.Errorf("expected 0 books at StageScrape after the real change, got %d", len(third.booksByStage[workflow.StageScrape]))
	}
	if len(third.booksByStage[workflow.StageDataManager]) != 1 {
		t.Errorf("expected 1 book at StageDataManager after the real change, got %d", len(third.booksByStage[workflow.StageDataManager]))
	}
}

// TestHandleRunScanInfoWorkflow_SummaryReflectsChangeImmediately is the
// end-to-end version of the same guarantee: a real workflow action
// (scan-info) must invalidate the cache ITSELF, without a test manually
// calling InvalidateWorkflowCache, so the dashboard reflects what a user
// just did the moment they look at it - "any workflow step processing
// should retrigger the workflow lists to update" (comic-server-4te's
// broader loading-indicator issue calls out this exact class of staleness
// bug).
func TestHandleRunScanInfoWorkflow_SummaryReflectsChangeImmediately(t *testing.T) {
	scanInfoCfg := config.ScanInfoConfig{
		Enabled:  true,
		Scanners: []string{"FakeScanCo"},
		Prefix:   "Scanner:",
		Unknown:  "Unknown",
	}
	book := library.ComicBook{ID: "1", Series: "Batman", FilePath: `Batman 001 (2016) (Zeta-Fictscans).cbz`}
	workflow.SetStage(&book, workflow.StageScanInfo)
	s := newScanInfoTestServer(t, scanInfoCfg, []library.ComicBook{book})

	// Warm the cache with the pre-action state, same as a user having the
	// Workflow dashboard open before clicking "Run Scan Info".
	summaryBefore := httptest.NewRecorder()
	s.handleGetWorkflowSummary(summaryBefore, httptest.NewRequest(http.MethodGet, "/api/library/workflow", nil))
	var before WorkflowSummary
	if err := json.NewDecoder(summaryBefore.Body).Decode(&before); err != nil {
		t.Fatalf("decode: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/library/workflow/scan-info", nil)
	w := httptest.NewRecorder()
	s.handleRunScanInfoWorkflow(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	summaryAfter := httptest.NewRecorder()
	s.handleGetWorkflowSummary(summaryAfter, httptest.NewRequest(http.MethodGet, "/api/library/workflow", nil))
	var after WorkflowSummary
	if err := json.NewDecoder(summaryAfter.Body).Decode(&after); err != nil {
		t.Fatalf("decode: %v", err)
	}

	countFor := func(sum WorkflowSummary, stage string) int {
		for _, s := range sum.Stages {
			if s.Stage == stage {
				return s.Count
			}
		}
		return -1
	}
	if countFor(before, "scan_info") != 1 {
		t.Fatalf("expected 1 book at scan_info before the run, got %d", countFor(before, "scan_info"))
	}
	if countFor(after, "scan_info") != 0 {
		t.Errorf("expected 0 books at scan_info after the run (cache must have been invalidated), got %d", countFor(after, "scan_info"))
	}
	if countFor(after, "data_manager") != 1 {
		t.Errorf("expected 1 book advanced to data_manager after the run, got %d", countFor(after, "data_manager"))
	}
}

// gatedBackend counts GetAllBooks calls and can hold them until released.
type gatedBackend struct {
	library.Backend
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (g *gatedBackend) GetAllBooks() ([]library.ComicBook, error) {
	g.calls.Add(1)
	g.started <- struct{}{}
	<-g.release
	return g.Backend.GetAllBooks()
}

func newGatedServer(t *testing.T) (*Server, *gatedBackend) {
	t.Helper()
	book := library.ComicBook{ID: "1", FilePath: "/comics/book1.cbz"}
	workflow.SetStage(&book, workflow.StageScrape)
	s := newWorkflowTestServer(t, []library.ComicBook{book})
	g := &gatedBackend{Backend: s.backend, started: make(chan struct{}, 8), release: make(chan struct{})}
	s.backend = g
	return s, g
}

// Requests that arrive while a build runs must share it, not each load every book.
func TestGetOrBuildWorkflowCache_ConcurrentRequestsShareOneBuild(t *testing.T) {
	s, g := newGatedServer(t)

	results := make(chan *workflowCacheEntry, 3)
	for i := 0; i < 3; i++ {
		go func() {
			e, err := s.getOrBuildWorkflowCache()
			if err != nil {
				t.Errorf("getOrBuildWorkflowCache: %v", err)
			}
			results <- e
		}()
		if i == 0 {
			<-g.started // first build is now running and blocked
		}
	}
	time.Sleep(50 * time.Millisecond) // let the other two reach the wait
	close(g.release)

	first := <-results
	for i := 0; i < 2; i++ {
		if e := <-results; e != first {
			t.Errorf("request %d got a different snapshot", i+2)
		}
	}
	if n := g.calls.Load(); n != 1 {
		t.Errorf("GetAllBooks called %d times, want 1", n)
	}
}

// While a rebuild runs, a request gets the previous snapshot instead of waiting.
func TestGetOrBuildWorkflowCache_ServesPreviousSnapshotDuringRebuild(t *testing.T) {
	s, g := newGatedServer(t)
	close(g.release)
	old, err := s.getOrBuildWorkflowCache()
	if err != nil {
		t.Fatal(err)
	}
	<-g.started // drain the signal of the first build

	g.release = make(chan struct{}) // block the next build
	s.InvalidateWorkflowCache()
	rebuilt := make(chan *workflowCacheEntry, 1)
	go func() {
		e, _ := s.getOrBuildWorkflowCache()
		rebuilt <- e
	}()
	<-g.started

	got := make(chan *workflowCacheEntry, 1)
	go func() {
		e, _ := s.getOrBuildWorkflowCache()
		got <- e
	}()
	select {
	case e := <-got:
		if e != old {
			t.Error("expected the previous snapshot while the rebuild runs")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request waited for the rebuild instead of getting the previous snapshot")
	}

	close(g.release)
	fresh := <-rebuilt
	if fresh == old {
		t.Error("rebuild returned the old snapshot")
	}
	if again, _ := s.getOrBuildWorkflowCache(); again != fresh {
		t.Error("the rebuilt snapshot was not cached")
	}
}

// A build that races an invalidation must not be stored as current.
func TestGetOrBuildWorkflowCache_InvalidationDuringBuildDiscardsResult(t *testing.T) {
	s, g := newGatedServer(t)
	done := make(chan struct{})
	go func() {
		_, _ = s.getOrBuildWorkflowCache()
		close(done)
	}()
	<-g.started
	s.InvalidateWorkflowCache()
	close(g.release)
	<-done

	s.workflowCacheMu.RLock()
	cached := s.workflowCache
	s.workflowCacheMu.RUnlock()
	if cached != nil {
		t.Error("a build that raced an invalidation was stored as current")
	}
}

// A record removed while a snapshot build is running must not come back:
// the build started before the removal, so its result is discarded, and the
// next request does not get the old snapshot either.
func TestForgetMissing_RemovedRecordNotListedAfterRacingBuild(t *testing.T) {
	s, g := newGatedServer(t)
	done := make(chan struct{})
	go func() {
		_, _ = s.getOrBuildWorkflowCache()
		close(done)
	}()
	<-g.started // a build that has already read the old book list is running

	if err := s.backend.(*gatedBackend).Backend.DeleteBook("1"); err != nil {
		t.Fatal(err)
	}
	s.forgetMissing("1")
	close(g.release)
	<-done

	entry, err := s.getOrBuildWorkflowCache()
	if err != nil {
		t.Fatal(err)
	}
	for stage, books := range entry.booksByStage {
		for _, b := range books {
			if b.ID == "1" {
				t.Errorf("removed book still listed at stage %v", stage)
			}
		}
	}
}
