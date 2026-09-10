package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
	book := library.ComicBook{ID: "1"}
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
