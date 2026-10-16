package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// Bulk edit (comic-server-k1bt.2): ComicRack's multi-book Info dialog for
// the books selected in Browse.
//
// POST /api/library/browse/bulk-edit
//
// The scope is either explicit book_ids (rows the user ticked) or the Browse
// filter itself ("select all N matching"). The edit is the single-book edit
// (BookEditRequest), so field checks, dirty flags and stage regression are the
// same. Every book is computed first and nothing is written if any value is
// rejected. apply=false only reports what would change.

// maxBulkEditBooks caps one bulk edit. A larger set is refused, not cut.
const maxBulkEditBooks = 10000

// bulkEditWriteBatch is how many books go to UpdateBooks at a time.
const bulkEditWriteBatch = 500

// BulkEditRequest is the POST body.
type BulkEditRequest struct {
	// BookIDs, when not empty, is the scope. Otherwise Browse is.
	BookIDs []string      `json:"book_ids,omitempty"`
	Browse  BrowseRequest `json:"browse"`
	// ExpectedCount is the number of books the user saw selected. An apply
	// whose scope now holds a different number is refused (409), so a change
	// to the library between preview and apply cannot widen the edit.
	ExpectedCount int             `json:"expected_count"`
	Edit          BookEditRequest `json:"edit"`
	Apply         bool            `json:"apply"`
}

// BulkEditSample is one book in the preview.
type BulkEditSample struct {
	ID      string   `json:"id"`
	Series  string   `json:"series"`
	Number  string   `json:"number"`
	Changed []string `json:"changed"`
}

// BulkEditResponse reports a preview or an applied edit.
type BulkEditResponse struct {
	Applied   bool             `json:"applied"`
	Total     int              `json:"total"`   // books in scope
	Changed   int              `json:"changed"` // books that change (or changed)
	Unchanged int              `json:"unchanged"`
	Fields    map[string]int   `json:"fields"` // books changed per field
	Sample    []BulkEditSample `json:"sample"`
}

const bulkEditSampleSize = 20

// handleBulkEdit POST /api/library/browse/bulk-edit
func (s *Server) handleBulkEdit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	var req BulkEditRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Edit.Fields)+len(req.Edit.ListAdd)+len(req.Edit.ListRemove) == 0 && req.Edit.Read == "" {
		http.Error(w, "Nothing to change", http.StatusBadRequest)
		return
	}

	ids, err := s.bulkEditScope(req)
	if err != nil {
		log.Error().Err(err).Msg("Failed to resolve bulk edit scope")
		http.Error(w, "Failed to resolve the selection", http.StatusInternalServerError)
		return
	}
	if len(ids) == 0 {
		http.Error(w, "No books selected", http.StatusBadRequest)
		return
	}
	if len(ids) > maxBulkEditBooks {
		http.Error(w, fmt.Sprintf("%d books selected; the limit is %d. Narrow the filter.", len(ids), maxBulkEditBooks), http.StatusRequestEntityTooLarge)
		return
	}
	if req.Apply && req.ExpectedCount != len(ids) {
		http.Error(w, fmt.Sprintf("The selection now holds %d books, not the %d you saw. Preview again.", len(ids), req.ExpectedCount), http.StatusConflict)
		return
	}

	// Edit copies through the audited wrapper (its GetBook returns a copy), so
	// the audit diff sees the old values and a preview never touches the library.
	backend := s.audited(audit.ActionEdit)
	now := time.Now()
	resp := BulkEditResponse{Applied: req.Apply, Total: len(ids), Fields: map[string]int{}, Sample: []BulkEditSample{}}
	var changedBooks []*library.ComicBook
	for _, id := range ids {
		book, err := backend.GetBook(id)
		if err != nil || book == nil {
			continue // removed since the scope was resolved
		}
		changed, err := applyBookEdit(book, req.Edit, now)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(changed) == 0 {
			resp.Unchanged++
			continue
		}
		resp.Changed++
		for _, f := range changed {
			resp.Fields[f]++
		}
		if len(resp.Sample) < bulkEditSampleSize {
			resp.Sample = append(resp.Sample, BulkEditSample{ID: book.ID, Series: book.Series, Number: book.Number, Changed: changed})
		}
		changedBooks = append(changedBooks, book)
	}

	if req.Apply {
		for chunk := range slices.Chunk(changedBooks, bulkEditWriteBatch) {
			if err := backend.UpdateBooks(chunk); err != nil {
				log.Error().Err(err).Int("books", len(chunk)).Msg("Bulk edit write failed")
				http.Error(w, "Failed to save some books; the edit is partly applied. Preview again to see what is left.", http.StatusInternalServerError)
				return
			}
		}
		if len(changedBooks) > 0 {
			s.InvalidateWorkflowCache()
		}
	}
	s.writeJSON(w, http.StatusOK, resp)
}

// bulkEditScope returns the ids the edit covers: the ticked rows, or every
// book the Browse filter matches (matchers/show_all, query and facets).
func (s *Server) bulkEditScope(req BulkEditRequest) ([]string, error) {
	if len(req.BookIDs) > 0 {
		seen := make(map[string]bool, len(req.BookIDs))
		ids := make([]string, 0, len(req.BookIDs))
		for _, id := range req.BookIDs {
			if id != "" && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		return ids, nil
	}
	if req.Browse.MatcherMode == "" {
		req.Browse.MatcherMode = "And"
	}
	books, err := s.resolveBrowseCandidates(req.Browse)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(books))
	for i, b := range books {
		ids[i] = b.ID
	}
	return ids, nil
}
