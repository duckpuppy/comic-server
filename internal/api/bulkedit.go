package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/datamanager"
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
	// Diffs holds the before and after text of each changed field except
	// Read (tracker #31), so an edit can be checked book by book.
	Diffs []BulkEditDiff `json:"diffs,omitempty"`
}

// BulkEditDiff is one field's value before and after an edit.
type BulkEditDiff struct {
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// BulkEditResponse reports a preview or an applied edit.
type BulkEditResponse struct {
	Applied   bool             `json:"applied"`
	Total     int              `json:"total"`   // books in scope
	Changed   int              `json:"changed"` // books that change (or changed)
	Unchanged int              `json:"unchanged"`
	Fields    map[string]int   `json:"fields"` // books changed per field
	Sample    []BulkEditSample `json:"sample"`
	// Skipped counts books an autonumber left alone (they keep their number);
	// SkippedBooks lists the first few with the reason (tracker #30).
	Skipped      int              `json:"skipped,omitempty"`
	SkippedBooks []autonumberSkip `json:"skipped_books,omitempty"`
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
	if len(req.Edit.Fields)+len(req.Edit.ListAdd)+len(req.Edit.ListRemove) == 0 && req.Edit.FindReplace == nil && req.Edit.Autonumber == nil && req.Edit.Read == "" {
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

	// Autonumber needs the whole scope: plan every number first.
	var numbers map[string]string
	var preloaded map[string]*library.ComicBook
	var skipped map[string]bool
	var skippedList []autonumberSkip
	if a := req.Edit.Autonumber; a != nil {
		if len(req.Edit.Fields)+len(req.Edit.ListAdd)+len(req.Edit.ListRemove) > 0 || req.Edit.FindReplace != nil || req.Edit.Read != "" {
			http.Error(w, "Autonumber cannot be combined with other changes", http.StatusBadRequest)
			return
		}
		if err := a.validate(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		preloaded = make(map[string]*library.ComicBook, len(ids))
		ordered := make([]*library.ComicBook, 0, len(ids))
		for _, id := range ids {
			if b, err := backend.GetBook(id); err == nil && b != nil {
				preloaded[id] = b
				ordered = append(ordered, b)
			}
		}
		numbers, skippedList = planAutonumber(ordered, *a)
		skipped = make(map[string]bool, len(skippedList))
		for _, sk := range skippedList {
			skipped[sk.ID] = true
		}
	}
	resp := BulkEditResponse{Applied: req.Apply, Total: len(ids), Fields: map[string]int{}, Sample: []BulkEditSample{}}
	var changedBooks []*library.ComicBook
	for _, id := range ids {
		var book *library.ComicBook
		if preloaded != nil {
			book = preloaded[id]
		} else {
			book, _ = backend.GetBook(id)
		}
		if book == nil {
			continue // removed since the scope was resolved
		}
		edit := req.Edit
		if req.Edit.Autonumber != nil {
			if skipped[id] {
				continue
			}
			edit = BookEditRequest{Fields: map[string]any{"Number": numbers[id]}}
		}
		var before map[string]string
		if len(resp.Sample) < bulkEditSampleSize {
			before = bulkEditSnapshot(book, edit)
		}
		changed, err := applyBookEdit(book, edit, now)
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
			sample := BulkEditSample{ID: book.ID, Series: book.Series, Number: book.Number, Changed: changed}
			for _, f := range changed {
				if old, ok := before[f]; ok {
					after, _ := datamanager.GetFieldString(book, f)
					sample.Diffs = append(sample.Diffs, BulkEditDiff{Field: f, Before: old, After: after})
				}
			}
			resp.Sample = append(resp.Sample, sample)
		}
		changedBooks = append(changedBooks, book)
	}

	resp.Skipped = len(skippedList)
	resp.SkippedBooks = skippedList[:min(len(skippedList), bulkEditSampleSize)]

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

// bulkEditSnapshot reads the current value of every field the edit names, so
// the preview can show before and after.
func bulkEditSnapshot(book *library.ComicBook, edit BookEditRequest) map[string]string {
	out := map[string]string{}
	add := func(f string) {
		v, _ := datamanager.GetFieldString(book, f)
		out[f] = v
	}
	for f := range edit.Fields {
		add(f)
	}
	for f := range edit.ListAdd {
		add(f)
	}
	for f := range edit.ListRemove {
		add(f)
	}
	if edit.FindReplace != nil {
		add(edit.FindReplace.Field)
	}
	return out
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
