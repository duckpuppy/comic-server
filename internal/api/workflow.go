package api

import (
	"net/http"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/libraryorganizer"
	"github.com/duckpuppy/comic-server/internal/log"
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

	entry, err := s.getOrBuildWorkflowCache()
	if err != nil {
		http.Error(w, "Failed to load books", http.StatusInternalServerError)
		return
	}

	summary := WorkflowSummary{CVDBSkip: entry.cvdbSkip}
	for _, st := range workflow.Stages {
		summary.Stages = append(summary.Stages, WorkflowStageSummary{
			Stage: st.String(),
			Label: st.Label(),
			Count: len(entry.booksByStage[st]),
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

	entry, err := s.getOrBuildWorkflowCache()
	if err != nil {
		http.Error(w, "Failed to load books", http.StatusInternalServerError)
		return
	}
	matched := entry.booksByStage[stage]

	limit, offset := parseLimitOffset(r, 20, 100)
	total := len(matched)
	start := min(offset, total)
	end := min(start+limit, total)
	page := matched[start:end]

	// Current path is free (book.FilePath); target path takes an actual
	// Library Organizer plan run, so it's only computed for the one stage
	// where it means anything, and only for this page - not the whole
	// (possibly much larger) matched set, same "scope the work to what's
	// actually being shown" lesson as comic-server-n5d.
	var planByID map[string]libraryorganizer.PlannedMove
	var planUnavailableReason string
	if stage == workflow.StageToMove {
		planByID, planUnavailableReason = s.planToMoveBooks(page)
	}

	previews := make([]ComicPreview, 0, len(page))
	for _, book := range page {
		preview := ComicPreview{
			ID:          book.ID,
			Series:      book.Series,
			Number:      book.Number,
			Title:       book.Title,
			Volume:      book.Volume,
			Publisher:   book.Publisher,
			Year:        book.Year,
			Unread:      book.IsUnread(),
			CurrentPath: book.FilePath,
		}
		switch {
		case planByID != nil:
			// A profile was found and Plan ran - every input book gets
			// exactly one PlannedMove back, so this lookup always hits.
			plan := planByID[book.ID]
			switch {
			case plan.Skipped:
				preview.TargetPathNote = "excluded by profile rules"
			case plan.Failed:
				preview.TargetPathNote = plan.FailReason
			case plan.Collision:
				preview.TargetPathNote = plan.CollisionReason
			default:
				preview.TargetPath = plan.NewRawPath
			}
		case planUnavailableReason != "":
			preview.TargetPathNote = planUnavailableReason
		}
		previews = append(previews, preview)
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"comics":   previews,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"has_more": end < total,
	})
}

// planToMoveBooks runs Library Organizer's Plan against books using the
// first configured profile (by sort_order, same default selection
// organizePage.js's own picker starts with) - this is a read-only
// informational preview embedded in the workflow drill-in, not the real
// apply flow, so there's no profile picker here. Never fails the whole
// book list over configDB being unavailable, no profile being configured
// yet, or the profile failing to load - but always returns a reason
// string explaining which of those happened, so a caller never has to
// guess why byID came back nil (comic-server-1qb's follow-up: a blank
// target path with no explanation looked like a silent bug even when it
// was a legitimate "nothing to plan against yet" state).
func (s *Server) planToMoveBooks(books []*library.ComicBook) (byID map[string]libraryorganizer.PlannedMove, unavailableReason string) {
	if s.configDB == nil {
		return nil, "config database not available"
	}
	profiles, err := s.configDB.ListLOProfiles()
	if err != nil {
		return nil, "failed to load Library Organizer profiles"
	}
	if len(profiles) == 0 {
		return nil, "no Library Organizer profile configured"
	}

	opts, _, errMsg, _ := s.loadLOPlanOptions(profiles[0].ID)
	if errMsg != "" {
		log.Warn().Str("reason", errMsg).Msg("Skipping target-path preview for workflow drill-in")
		return nil, "failed to load the active Library Organizer profile"
	}
	opts.FileExists = s.libraryOrganizerFileExists

	moves := libraryorganizer.Plan(books, opts)
	byID = make(map[string]libraryorganizer.PlannedMove, len(moves))
	for _, m := range moves {
		byID[m.BookID] = m
	}
	return byID, ""
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
