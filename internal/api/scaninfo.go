package api

import (
	"fmt"
	"net/http"
	"reflect"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/scaninfo"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

// ScanInfoResult is the response for POST .../scan-info.
type ScanInfoResult struct {
	Processed int      `json:"processed"`
	Updated   int      `json:"updated"`
	Skipped   int      `json:"skipped"`
	Errors    []string `json:"errors,omitempty"`
}

// handleRunScanInfoWorkflow runs scan-info detection over every book
// currently at workflow.StageScanInfo, replacing "select the '03 Add
// Scanner Info' smart list, run scan-info" with a single button on the
// workflow dashboard (comic-server-1iv.3). The old per-list version of
// this action (POST /api/library/lists/:listId/scan-info) was removed in
// comic-server-3hu8 once the Workflow pipeline made it redundant.
// POST /api/library/workflow/scan-info
func (s *Server) handleRunScanInfoWorkflow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg, err := s.effectiveScanInfo()
	if err != nil {
		log.Error().Err(err).Msg("Failed to load scan info config")
		http.Error(w, "Failed to load scan info config", http.StatusInternalServerError)
		return
	}
	if !cfg.Enabled {
		http.Error(w, "scan_info is not enabled in config", http.StatusServiceUnavailable)
		return
	}
	detector, err := scaninfo.NewDetector(cfg.Scanners, cfg.Blacklist, cfg.Prefix, cfg.Unknown)
	if err != nil {
		log.Error().Err(err).Msg("Failed to build scan-info detector from config")
		http.Error(w, "Invalid scan_info configuration", http.StatusInternalServerError)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	books, err := s.booksAtStage(workflow.StageScanInfo)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load books: %v", err), http.StatusInternalServerError)
		return
	}

	result, toUpdate := s.runScanInfoOverBooks(books, detector)

	if len(toUpdate) > 0 {
		if err := s.backend.UpdateBooks(toUpdate); err != nil {
			log.Error().Err(err).Msg("Failed to save scan-info updates")
			result.Errors = append(result.Errors, err.Error())
		}
		s.InvalidateWorkflowCache()
	}

	s.writeJSON(w, http.StatusOK, result)
}

// runScanInfoOverBooks is the shared core both scan-info entry points
// (one smart list, or every book at StageScanInfo) run: detect a tag for
// each book, merge it in, and advance the workflow stage on every
// SUCCESSFUL detection (comic-server-1iv.2) whether or not the stored
// value actually changed. Returns the result plus every book that needs
// persisting - callers own the actual UpdateBooks call and its own error
// handling, since only the caller knows which log/response context it's
// in.
func (s *Server) runScanInfoOverBooks(books []*library.ComicBook, detector *scaninfo.Detector) (ScanInfoResult, []*library.ComicBook) {
	result := ScanInfoResult{Processed: len(books)}
	var toUpdate []*library.ComicBook
	rulesets := s.loadWorkflowRulesets()

	for _, book := range books {
		tag, ok := detector.DetectTag(toScanInfoBook(book))
		if !ok {
			result.Skipped++
			continue
		}
		merged, changed := scaninfo.MergeTag(book.ScanInformation, tag)
		if changed {
			book.ScanInformation = merged
			result.Updated++
		} else {
			result.Skipped++
		}

		stageAdvanced := workflow.AdvanceIfAtOrBefore(book, workflow.StageScanInfo, rulesets)
		if changed || stageAdvanced {
			toUpdate = append(toUpdate, book)
		}
	}

	return result, toUpdate
}

// toScanInfoBook builds a scaninfo.Book from a library.ComicBook, including
// every other populated string field as OtherFields for the
// false-positive guard - comic-server's stand-in for ComicRack's
// GetComicFields() reflection (see scaninfo.DetectTag's guard 1).
func toScanInfoBook(book *library.ComicBook) scaninfo.Book {
	var other []string
	v := reflect.ValueOf(*book)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Type.Kind() != reflect.String {
			continue
		}
		switch field.Name {
		case "ID", "FilePath", "ScanInformation":
			continue
		}
		if val := v.Field(i).String(); val != "" {
			other = append(other, val)
		}
	}

	return scaninfo.Book{
		FilePath:        book.FilePath,
		Series:          book.Series,
		Title:           book.Title,
		AlternateSeries: book.AlternateSeries,
		ScanInformation: book.ScanInformation,
		OtherFields:     other,
	}
}
