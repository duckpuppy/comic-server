// CBL import match-correction UI (comic-server-a2hz, spec
// docs/plans/2026-09-20-cbl-reading-list-import.md §2d). Lets the web UI
// show a CBL-imported list's per-entry match results (which path matched
// each entry, and for an ambiguous string-fallback match, which
// candidates were tied) and re-point or unmatch a wrong one. Scoped to a
// single import's results, not a general-purpose relink tool - see
// storage.CorrectCBLImportEntry's doc comment.
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/storage"
)

// CBLCandidateBookWire is a small book summary - enough for the
// match-correction UI to display and let the user pick among, without
// pulling in every field of library.ComicBook.
type CBLCandidateBookWire struct {
	ID     string `json:"id"`
	Series string `json:"series"`
	Number string `json:"number"`
	Volume int    `json:"volume,omitempty"`
	Year   int    `json:"year,omitempty"`
	Format string `json:"format,omitempty"`
}

func toCBLCandidateBookWire(b *library.ComicBook) CBLCandidateBookWire {
	return CBLCandidateBookWire{
		ID: b.ID, Series: b.Series, Number: b.Number,
		Volume: b.Volume, Year: b.Year, Format: b.Format,
	}
}

// CBLImportEntryDetailWire is one entry's full match-correction detail -
// unlike CBLImportEntryWire (the plain import-summary shape), this
// resolves BookID/CandidateBookIDs into real book summaries so the UI
// doesn't need a second round-trip per entry.
type CBLImportEntryDetailWire struct {
	ID        string `json:"id"`
	Position  int    `json:"position"`
	MatchPath string `json:"match_path"` // "cv_id" / "series_number" / "manual" / "none"

	// Raw CBL entry fields, as imported.
	Series    string `json:"series"`
	Number    string `json:"number"`
	Volume    int    `json:"volume,omitempty"`
	Year      int    `json:"year,omitempty"`
	Format    string `json:"format,omitempty"`
	CVIssueID int    `json:"cv_issue_id,omitempty"`

	Book *CBLCandidateBookWire `json:"book,omitempty"`
	// Candidates is only populated for an ambiguous match_path=series_number
	// entry (see storage.CBLImportEntry.CandidateBookIDs) - what the
	// matcher's tie-break was choosing among, so the UI can offer them as
	// quick re-point options instead of a blind full-library search.
	Candidates []CBLCandidateBookWire `json:"candidates,omitempty"`
}

// resolveCBLImportEntryDetail turns one storage.CBLImportEntry into its
// wire shape, resolving BookID/CandidateBookIDs against backend. A book
// ID that no longer resolves (deleted since import) is silently omitted
// rather than erroring the whole request - the entry is still shown with
// its raw CBL fields either way.
func resolveCBLImportEntryDetail(backend library.Backend, e storage.CBLImportEntry) CBLImportEntryDetailWire {
	out := CBLImportEntryDetailWire{
		ID: e.ID, Position: e.Position, MatchPath: e.Path.String(),
		Series: e.Series, Number: e.Number, Volume: e.Volume, Year: e.Year,
		Format: e.Format, CVIssueID: e.CVIssueID,
	}
	if e.BookID != "" {
		if b, err := backend.GetBook(e.BookID); err == nil && b != nil {
			w := toCBLCandidateBookWire(b)
			out.Book = &w
		}
	}
	for _, id := range e.CandidateBookIDs {
		b, err := backend.GetBook(id)
		if err != nil || b == nil {
			continue
		}
		out.Candidates = append(out.Candidates, toCBLCandidateBookWire(b))
	}
	return out
}

// handleGetCBLImportEntries serves GET
// /api/library/lists/:listId/cbl-import-entries - every entry recorded
// for listID's CBL import, in original order, for the match-correction
// UI's main view.
func (s *Server) handleGetCBLImportEntries(w http.ResponseWriter, r *http.Request, listID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sb, ok := s.backend.(*storage.SQLiteBackend)
	if !ok {
		http.Error(w, "CBL import entries require the SQLite library backend", http.StatusServiceUnavailable)
		return
	}

	source, err := sb.DB().GetCBLSource(listID)
	if err != nil {
		http.Error(w, "get cbl source: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if source == nil {
		http.Error(w, "list was not created by a CBL import", http.StatusNotFound)
		return
	}

	entries, err := sb.DB().GetCBLImportEntries(listID)
	if err != nil {
		http.Error(w, "get cbl import entries: "+err.Error(), http.StatusInternalServerError)
		return
	}

	wire := make([]CBLImportEntryDetailWire, len(entries))
	for i, e := range entries {
		wire[i] = resolveCBLImportEntryDetail(s.backend, e)
	}
	s.writeJSON(w, http.StatusOK, struct {
		Entries []CBLImportEntryDetailWire `json:"entries"`
	}{Entries: wire})
}

// cblCorrectEntryRequest is the body of POST
// .../cbl-import-entries/:entryId/correct. BookID empty means "unmatch
// this entry" (spec §2c/§2d); non-empty re-points it to that book -
// which doesn't have to be one of the entry's recorded Candidates, though
// the UI is expected to offer those as the primary quick-pick options.
type cblCorrectEntryRequest struct {
	BookID string `json:"book_id"`
}

// handleCorrectCBLImportEntry serves POST
// /api/library/lists/:listId/cbl-import-entries/:entryId/correct - the
// write side of the match-correction UI (spec §2d). listID isn't used
// directly (the entry ID alone identifies the row - storage validates it
// exists) but is threaded through for a clearer route and consistent
// cache invalidation below.
func (s *Server) handleCorrectCBLImportEntry(w http.ResponseWriter, r *http.Request, listID, entryID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sb, ok := s.backend.(*storage.SQLiteBackend)
	if !ok {
		http.Error(w, "CBL import entries require the SQLite library backend", http.StatusServiceUnavailable)
		return
	}

	var req cblCorrectEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	corrected, err := sb.DB().CorrectCBLImportEntry(entryID, req.BookID)
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrCBLImportEntryNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		case errors.Is(err, storage.ErrCBLCandidateBookNotFound):
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			http.Error(w, "correct entry failed: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// A plain InvalidateListCache() here would only mark the count stale
	// (GetListCounts then serves the OLD count from GetStaleCounts and
	// kicks off an async recompute - the right call for a full library
	// reload, where re-evaluating every list live would be too slow to
	// wait on). This correction only ever touches ONE reading list's
	// membership by one book, so recompute and cache that list's count
	// synchronously instead - otherwise the UI's own immediate GET right
	// after this response would show a stale book_count until whatever
	// happened to trigger the next refresh (caught by this endpoint's own
	// browser smoke test, comic-server-a2hz).
	if list, err := s.backend.FindListByID(corrected.ListID); err == nil && list != nil {
		count, unread := s.evaluateListCounts(list)
		s.listCache.SetCounts(corrected.ListID, count, unread)
	} else {
		s.listCache.Invalidate(corrected.ListID)
	}

	s.writeJSON(w, http.StatusOK, resolveCBLImportEntryDetail(s.backend, *corrected))
}

// routeCBLImportEntries dispatches the /cbl-import-entries sub-tree of
// handleListsRouter - suffix is everything after
// "/api/library/lists/:listId/cbl-import-entries", i.e. "" for the list
// view or "/:entryId/correct" for a correction. Returns false if suffix
// doesn't match either shape, so the caller can fall through to
// http.NotFound.
func (s *Server) routeCBLImportEntries(w http.ResponseWriter, r *http.Request, listID, suffix string) bool {
	if suffix == "" {
		s.handleGetCBLImportEntries(w, r, listID)
		return true
	}
	rest := strings.TrimPrefix(suffix, "/")
	if entryID, ok := strings.CutSuffix(rest, "/correct"); ok && entryID != "" {
		s.handleCorrectCBLImportEntry(w, r, listID, entryID)
		return true
	}
	return false
}
