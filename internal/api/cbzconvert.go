package api

import (
	"fmt"
	"net/http"

	"github.com/duckpuppy/comic-server/internal/cbzconvert"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/trash"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

// CBZConvertResult is the response for POST .../convert-cbz.
type CBZConvertResult struct {
	Processed int      `json:"processed"`
	Converted int      `json:"converted"`
	Errors    []string `json:"errors,omitempty"`
}

// handleRunCBZConvertWorkflow converts every book currently at
// workflow.StageConvertToCBZ, replacing "select the '01 Convert to CBZ'
// smart list, run convert" with a single button on the workflow
// dashboard (comic-server-1iv.3). The old per-list version of this action
// (POST /api/library/lists/:listId/convert-cbz) was removed in
// comic-server-3hu8 once the Workflow pipeline made it redundant.
// POST /api/library/workflow/convert-cbz
func (s *Server) handleRunCBZConvertWorkflow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.configMu.RLock()
	cfg := s.config
	s.configMu.RUnlock()

	if cfg == nil || !cfg.Server.CBZConvert.Enabled {
		http.Error(w, "cbz_convert is not enabled in config", http.StatusServiceUnavailable)
		return
	}
	tr, err := s.newTrashFromConfig()
	if err != nil {
		log.Error().Err(err).Msg("Invalid trash configuration for cbz-convert")
		http.Error(w, "Invalid trash configuration", http.StatusInternalServerError)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	books, err := s.booksAtStage(workflow.StageConvertToCBZ)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load books: %v", err), http.StatusInternalServerError)
		return
	}

	result, toUpdate := s.runCBZConvertOverBooks(books, tr)

	if len(toUpdate) > 0 {
		if err := s.backend.UpdateBooks(toUpdate); err != nil {
			log.Error().Err(err).Msg("Failed to save cbz-convert updates")
			result.Errors = append(result.Errors, err.Error())
		}
		s.InvalidateWorkflowCache()
	}

	s.writeJSON(w, http.StatusOK, result)
}

// runCBZConvertOverBooks is the core handleRunCBZConvertWorkflow runs.
func (s *Server) runCBZConvertOverBooks(books []*library.ComicBook, tr *trash.Trash) (CBZConvertResult, []*library.ComicBook) {
	result := CBZConvertResult{Processed: len(books)}
	var toUpdate []*library.ComicBook
	rulesets := s.loadWorkflowRulesets()

	for _, book := range books {
		converted, err := cbzconvert.Convert(book, s.resolveBookFilePath, tr)
		if err != nil {
			log.Error().Err(err).Str("book_id", book.ID).Str("file_path", book.FilePath).Msg("cbz-convert failed for book")
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", book.ID, err))
			continue
		}
		book.FilePath = converted.NewFilePath
		book.PageCount = converted.PageCount
		workflow.AdvanceIfAtOrBefore(book, workflow.StageConvertToCBZ, rulesets)
		toUpdate = append(toUpdate, book)
		result.Converted++
	}

	return result, toUpdate
}
