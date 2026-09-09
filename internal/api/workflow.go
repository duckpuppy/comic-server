package api

import (
	"net/http"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

// WorkflowStageSummary is one stage's entry in the dashboard summary -
// comic-server-1iv.3's replacement for a manually-maintained pipeline
// smart list.
type WorkflowStageSummary struct {
	Stage string `json:"stage"` // Stage.String() - stable, matches the /books path segment
	Label string `json:"label"`
	Count int    `json:"count"`
}

// WorkflowSummary is the response for GET /api/library/workflow.
type WorkflowSummary struct {
	Stages   []WorkflowStageSummary `json:"stages"`
	CVDBSkip int                    `json:"cvdb_skip"` // informational only, never advances - see comic-server-3x3
}

// handleGetWorkflowSummary returns a live count of books at each pipeline
// stage, plus the CVDBSKIP informational count. GET /api/library/workflow
func (s *Server) handleGetWorkflowSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	books, err := s.backend.GetAllBooks()
	if err != nil {
		http.Error(w, "Failed to load books", http.StatusInternalServerError)
		return
	}

	counts := make(map[workflow.Stage]int, len(workflow.Stages))
	cvdbSkip := 0
	for i := range books {
		counts[workflow.GetStage(&books[i])]++
		if workflow.IsCVDBSkip(&books[i]) {
			cvdbSkip++
		}
	}

	summary := WorkflowSummary{CVDBSkip: cvdbSkip}
	for _, st := range workflow.Stages {
		summary.Stages = append(summary.Stages, WorkflowStageSummary{
			Stage: st.String(),
			Label: st.Label(),
			Count: counts[st],
		})
	}

	s.writeJSON(w, http.StatusOK, summary)
}

// handleGetWorkflowStageBooks returns a paginated list of books currently
// at one stage. GET /api/library/workflow/:stage/books
func (s *Server) handleGetWorkflowStageBooks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	stage, ok := stageFromWorkflowSubPath(r.URL.Path, "/books")
	if !ok {
		http.Error(w, "Unknown workflow stage", http.StatusNotFound)
		return
	}

	matched, err := s.booksAtStage(stage)
	if err != nil {
		http.Error(w, "Failed to load books", http.StatusInternalServerError)
		return
	}

	limit, offset := parseLimitOffset(r, 20, 100)
	total := len(matched)
	start := min(offset, total)
	end := min(start+limit, total)

	previews := make([]ComicPreview, 0, end-start)
	for _, book := range matched[start:end] {
		previews = append(previews, ComicPreview{
			ID:        book.ID,
			Series:    book.Series,
			Number:    book.Number,
			Title:     book.Title,
			Volume:    book.Volume,
			Publisher: book.Publisher,
			Year:      book.Year,
			Unread:    book.IsUnread(),
		})
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"comics":   previews,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"has_more": end < total,
	})
}

// handleWorkflowStageSubRouter dispatches /api/library/workflow/:stage/...
// - only "/books" exists today, but this keeps the shape open for a
// future per-stage action beyond the ones already covered by
// handleRunScanInfoWorkflow/handleRunCBZConvertWorkflow.
func (s *Server) handleWorkflowStageSubRouter(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/books") {
		s.handleGetWorkflowStageBooks(w, r)
		return
	}
	http.NotFound(w, r)
}

// booksAtStage returns every book currently at stage, as pointers into a
// GetAllBooks() snapshot - same "always work from a fresh snapshot, never
// a shared cached slice" pattern the whole-library Data Manager endpoints
// use (comic-server-dpq).
func (s *Server) booksAtStage(stage workflow.Stage) ([]*library.ComicBook, error) {
	allBooks, err := s.backend.GetAllBooks()
	if err != nil {
		return nil, err
	}
	var matched []*library.ComicBook
	for i := range allBooks {
		if workflow.GetStage(&allBooks[i]) == stage {
			matched = append(matched, &allBooks[i])
		}
	}
	return matched, nil
}

// stageFromWorkflowSubPath parses "/api/library/workflow/:stage/books"
// style paths, returning ok=false for an unrecognized stage string
// rather than defaulting to something misleading.
func stageFromWorkflowSubPath(path, suffix string) (workflow.Stage, bool) {
	trimmed := strings.TrimPrefix(path, "/api/library/workflow/")
	trimmed = strings.TrimSuffix(trimmed, suffix)
	for _, st := range workflow.Stages {
		if st.String() == trimmed {
			return st, true
		}
	}
	return workflow.StageUnknown, false
}
