// CBL import -> wanted-books integration (comic-server-sx2d, spec §2e).
// A per-import, user-triggered action - never automatic (decision
// 2026-09-20, see the spec for the full rationale). Reuses the existing
// wanted-books mechanism (comic-server-38f7, wanted.go) rather than a
// bespoke placeholder-book concept.
package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/storage"
	"github.com/google/uuid"
)

// cblAddUnmatchedToWantedRequest is the body for POST
// /api/library/workflow/wanted/from-cbl-import.
type cblAddUnmatchedToWantedRequest struct {
	ListID string `json:"list_id"`
}

// CBLAddUnmatchedToWantedResultWire reports how many wanted-book records
// were created (and skipped, for entries a previous call already
// resolved - this action is idempotent, see
// storage.GetUnresolvedCBLImportEntries).
type CBLAddUnmatchedToWantedResultWire struct {
	Created int            `json:"created"`
	Books   []ComicPreview `json:"books"`
}

// handleAddCBLUnmatchedToWanted serves POST
// /api/library/workflow/wanted/from-cbl-import - creates a wanted book
// (comic-server-38f7: an ordinary book record with FilePath=="") for
// every entry from the given CBL import that didn't match a book in the
// library and hasn't already been added. Explicit, per-import trigger
// only; nothing about a CBL import calls this automatically.
func (s *Server) handleAddCBLUnmatchedToWanted(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	sb, ok := s.backend.(*storage.SQLiteBackend)
	if !ok {
		http.Error(w, "CBL import history requires the SQLite library backend", http.StatusServiceUnavailable)
		return
	}

	var req cblAddUnmatchedToWantedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.ListID == "" {
		http.Error(w, "list_id is required", http.StatusBadRequest)
		return
	}

	entries, err := sb.DB().GetUnresolvedCBLImportEntries(req.ListID)
	if err != nil {
		http.Error(w, "failed to load import entries: "+err.Error(), http.StatusInternalServerError)
		return
	}

	result := CBLAddUnmatchedToWantedResultWire{Books: make([]ComicPreview, 0, len(entries))}
	for _, e := range entries {
		book := library.ComicBook{
			ID:     "{" + uuid.New().String() + "}",
			Series: e.Series,
			Number: e.Number,
			Volume: e.Volume,
			Year:   e.Year,
			Format: e.Format,
		}
		if e.CVIssueID > 0 {
			book.CustomValuesStore = library.SetCustomValue(book.CustomValuesStore, "comicvine_issue", strconv.Itoa(e.CVIssueID))
		}
		if err := s.backend.CreateBook(&book); err != nil {
			log.Error().Err(err).Str("entry_id", e.ID).Msg("Failed to create wanted book from CBL import entry")
			continue
		}
		if err := sb.DB().MarkCBLImportEntryWanted(e.ID, book.ID); err != nil {
			log.Error().Err(err).Str("entry_id", e.ID).Msg("Failed to mark CBL import entry as wanted")
			continue
		}
		result.Created++
		result.Books = append(result.Books, ComicPreview{
			ID: book.ID, Series: book.Series, Number: book.Number, Volume: book.Volume, Year: book.Year,
		})
	}

	if result.Created > 0 {
		s.InvalidateWorkflowCache()
	}
	s.writeJSON(w, http.StatusOK, result)
}
