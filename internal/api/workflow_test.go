package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

func newWorkflowTestServer(t *testing.T, books []library.ComicBook) *Server {
	t.Helper()
	backend := library.NewXMLBackendFromLibrary(&library.ComicLibrary{Books: books}, "", nil)
	return &Server{backend: backend}
}

func TestHandleGetWorkflowSummary_CountsPerStageAndCVDBSkip(t *testing.T) {
	book1 := library.ComicBook{ID: "1"}
	workflow.SetStage(&book1, workflow.StageConvertToCBZ)
	book2 := library.ComicBook{ID: "2"}
	workflow.SetStage(&book2, workflow.StageConvertToCBZ)
	book3 := library.ComicBook{ID: "3"}
	workflow.SetStage(&book3, workflow.StageToMove)
	book4 := library.ComicBook{ID: "4", Tags: "CVDBSKIP"}
	// book4 has no explicit stage - StageUnknown, must still be counted.

	s := newWorkflowTestServer(t, []library.ComicBook{book1, book2, book3, book4})

	req := httptest.NewRequest(http.MethodGet, "/api/library/workflow", nil)
	w := httptest.NewRecorder()
	s.handleGetWorkflowSummary(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var summary WorkflowSummary
	if err := json.NewDecoder(w.Body).Decode(&summary); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	counts := map[string]int{}
	for _, s := range summary.Stages {
		counts[s.Stage] = s.Count
	}
	if counts["convert_cbz"] != 2 {
		t.Errorf("convert_cbz count = %d, want 2", counts["convert_cbz"])
	}
	if counts["to_move"] != 1 {
		t.Errorf("to_move count = %d, want 1", counts["to_move"])
	}
	if summary.CVDBSkip != 1 {
		t.Errorf("CVDBSkip = %d, want 1", summary.CVDBSkip)
	}
	// book4 (StageUnknown) isn't counted under any of the 5 real stages -
	// total real-stage count should be exactly 3, not 4.
	total := 0
	for _, c := range counts {
		total += c
	}
	if total != 3 {
		t.Errorf("total across all real stages = %d, want 3 (StageUnknown book excluded)", total)
	}
}

func TestHandleGetWorkflowStageBooks_ReturnsOnlyThatStagePaginated(t *testing.T) {
	books := make([]library.ComicBook, 5)
	for i := range books {
		books[i] = library.ComicBook{ID: string(rune('1' + i)), Series: "Batman"}
		workflow.SetStage(&books[i], workflow.StageScrape)
	}
	other := library.ComicBook{ID: "other"}
	workflow.SetStage(&other, workflow.StageToMove)
	books = append(books, other)

	s := newWorkflowTestServer(t, books)

	req := httptest.NewRequest(http.MethodGet, "/api/library/workflow/scrape/books?limit=2&offset=1", nil)
	w := httptest.NewRecorder()
	s.handleWorkflowStageSubRouter(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result struct {
		Comics  []ComicPreview `json:"comics"`
		Total   int            `json:"total"`
		Limit   int            `json:"limit"`
		Offset  int            `json:"offset"`
		HasMore bool           `json:"has_more"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Total != 5 {
		t.Errorf("Total = %d, want 5 (the ToMove book must be excluded)", result.Total)
	}
	if len(result.Comics) != 2 {
		t.Fatalf("expected a page of 2, got %d", len(result.Comics))
	}
	if !result.HasMore {
		t.Error("expected HasMore=true")
	}
}

func TestHandleGetWorkflowStageBooks_UnknownStageReturns404(t *testing.T) {
	s := newWorkflowTestServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/library/workflow/not-a-real-stage/books", nil)
	w := httptest.NewRecorder()
	s.handleWorkflowStageSubRouter(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleGetWorkflowSummary_MethodNotAllowed(t *testing.T) {
	s := newWorkflowTestServer(t, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/library/workflow", nil)
	w := httptest.NewRecorder()
	s.handleGetWorkflowSummary(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}
