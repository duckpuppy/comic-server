package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

// maxBulkNoCVMatch caps one bulk request; the review queue never gets near it.
const maxBulkNoCVMatch = 5000

// CVNoMatchPreview is one marked book in the list, with the decision itself.
type CVNoMatchPreview struct {
	ComicPreview
	Reason string `json:"reason"`
	Date   string `json:"date"`
}

// CVNoMatchBulkRequest is the body of POST /api/library/workflow/cv-nomatch.
type CVNoMatchBulkRequest struct {
	BookIDs []string `json:"book_ids"`
	Set     bool     `json:"set"`
	Reason  string   `json:"reason"`
}

// CVNoMatchBulkSkip is a requested book that was left alone, and why.
type CVNoMatchBulkSkip struct {
	BookID string `json:"book_id"`
	Reason string `json:"reason"`
}

// CVNoMatchBulkResult reports what a bulk mark or undo did.
type CVNoMatchBulkResult struct {
	Changed   int                 `json:"changed"`
	Unchanged int                 `json:"unchanged"`
	Skipped   []CVNoMatchBulkSkip `json:"skipped"`
}

// handleCVNoMatchBooks lists (GET) or bulk-marks (POST) books marked not in
// ComicVine (comic-server-5wer phase 5).
// GET|POST /api/library/workflow/cv-nomatch
func (s *Server) handleCVNoMatchBooks(w http.ResponseWriter, r *http.Request) {
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listCVNoMatchBooks(w, r)
	case http.MethodPost:
		s.bulkCVNoMatch(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// listCVNoMatchBooks returns a page of marked books, most recently marked
// first, so a wrong mark is easy to find and undo.
func (s *Server) listCVNoMatchBooks(w http.ResponseWriter, r *http.Request) {
	entry, err := s.getOrBuildWorkflowCache()
	if err != nil {
		http.Error(w, "Failed to load books", http.StatusInternalServerError)
		return
	}
	marked := make([]CVNoMatchPreview, 0, len(entry.cvNoMatch))
	for _, book := range entry.cvNoMatch {
		reason, date, _ := workflow.NoCVMatch(book)
		marked = append(marked, CVNoMatchPreview{ComicPreview: attentionPreview(book), Reason: reason, Date: date})
	}
	slices.SortStableFunc(marked, func(a, b CVNoMatchPreview) int {
		if c := strings.Compare(b.Date, a.Date); c != 0 {
			return c
		}
		if c := strings.Compare(strings.ToLower(a.Series), strings.ToLower(b.Series)); c != 0 {
			return c
		}
		return strings.Compare(a.Number, b.Number)
	})

	limit, offset := parseLimitOffset(r, 20, 100)
	total := len(marked)
	start := min(offset, total)
	end := min(start+limit, total)
	s.writeJSON(w, http.StatusOK, map[string]any{
		"comics":   marked[start:end],
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"has_more": end < total,
	})
}

// bulkCVNoMatch sets or clears the mark on many books in one save, audited
// per book like the single-book endpoint.
func (s *Server) bulkCVNoMatch(w http.ResponseWriter, r *http.Request) {
	var req CVNoMatchBulkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.BookIDs) == 0 {
		http.Error(w, "book_ids is required", http.StatusBadRequest)
		return
	}
	if len(req.BookIDs) > maxBulkNoCVMatch {
		http.Error(w, "too many book_ids in one request", http.StatusBadRequest)
		return
	}
	if req.Set && !slices.Contains(workflow.NoCVMatchReasons, req.Reason) {
		http.Error(w, "reason must be one of: "+strings.Join(workflow.NoCVMatchReasons, ", "), http.StatusBadRequest)
		return
	}

	backend := s.audited(audit.ActionCVNoMatch)
	now := time.Now()
	result := CVNoMatchBulkResult{Skipped: []CVNoMatchBulkSkip{}}
	var changed []*library.ComicBook
	seen := make(map[string]bool, len(req.BookIDs))
	for _, id := range req.BookIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		book, err := backend.GetBook(id)
		if err != nil || book == nil {
			result.Skipped = append(result.Skipped, CVNoMatchBulkSkip{BookID: id, Reason: "book not found"})
			continue
		}
		ok, blocked := setNoCVMatch(book, req.Set, req.Reason, now)
		switch {
		case blocked != "":
			result.Skipped = append(result.Skipped, CVNoMatchBulkSkip{BookID: id, Reason: blocked})
		case ok:
			changed = append(changed, book)
		default:
			result.Unchanged++
		}
	}
	if len(changed) > 0 {
		if err := backend.UpdateBooks(changed); err != nil {
			log.Error().Err(err).Int("books", len(changed)).Msg("Failed to save ComicVine no-match marks")
			http.Error(w, "Failed to save books", http.StatusInternalServerError)
			return
		}
		s.InvalidateWorkflowCache()
	}
	result.Changed = len(changed)
	s.writeJSON(w, http.StatusOK, result)
}
