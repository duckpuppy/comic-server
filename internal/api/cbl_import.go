// CBL reading-list import (comic-server-zc0g, spec
// docs/plans/2026-09-20-cbl-reading-list-import.md §6 phase 1). Local
// file/upload only - fetching from a git-hosted source (e.g.
// DieselTech/CBL-ReadingLists) is a separate, not-yet-built feature
// (comic-server-oprf).
package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/duckpuppy/comic-server/internal/cbl"
	"github.com/duckpuppy/comic-server/internal/storage"
)

// maxCBLUploadBytes caps a single CBL upload - real CBLs top out around
// a few hundred KB even for a full-series reading order (the largest
// sampled DieselTech file, 1156 books, is ~154KB); this is generous
// headroom, matching the reasoning behind maxLibraryImportUploadBytes.
const maxCBLUploadBytes = 8 << 20 // 8MB

// CBLImportEntryWire is the wire shape of one storage.CBLImportEntry.
type CBLImportEntryWire struct {
	Series    string `json:"series"`
	Number    string `json:"number"`
	Volume    int    `json:"volume,omitempty"`
	Year      int    `json:"year,omitempty"`
	Format    string `json:"format,omitempty"`
	CVIssueID int    `json:"cv_issue_id,omitempty"`
	MatchPath string `json:"match_path"` // "cv_id" / "series_number" / "none"
	BookID    string `json:"book_id,omitempty"`
}

// CBLImportResultWire is the wire shape of storage.CBLImportResult.
type CBLImportResultWire struct {
	ListID       string               `json:"list_id"`
	Name         string               `json:"name"`
	MatchedCVID  int                  `json:"matched_cv_id"`
	MatchedOther int                  `json:"matched_series_number"`
	Unmatched    int                  `json:"unmatched"`
	Entries      []CBLImportEntryWire `json:"entries"`
}

func toCBLImportResultWire(name string, r *storage.CBLImportResult) CBLImportResultWire {
	entries := make([]CBLImportEntryWire, len(r.Entries))
	for i, e := range r.Entries {
		entries[i] = CBLImportEntryWire{
			Series: e.Series, Number: e.Number, Volume: e.Volume, Year: e.Year, Format: e.Format,
			CVIssueID: e.CVIssueID, MatchPath: e.Path.String(), BookID: e.BookID,
		}
	}
	return CBLImportResultWire{
		ListID: r.ListID, Name: name,
		MatchedCVID: r.MatchedCVID, MatchedOther: r.MatchedOther, Unmatched: r.Unmatched,
		Entries: entries,
	}
}

// handleImportCBL serves POST /api/library/import-cbl - uploads and
// imports a single CBL file (multipart form field "file"), matching every
// entry against the live library and creating a new reading list from
// whatever matched. See internal/cbl for the matcher, storage.ImportCBL
// for the transactional write.
func (s *Server) handleImportCBL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sb, ok := s.backend.(*storage.SQLiteBackend)
	if !ok {
		http.Error(w, "CBL import requires the SQLite library backend", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxCBLUploadBytes)
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing or invalid \"file\" form field: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	rl, err := cbl.Parse(file)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, fmt.Sprintf("CBL file exceeds %d byte limit", maxCBLUploadBytes), http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "failed to parse CBL file: "+err.Error(), http.StatusBadRequest)
		return
	}

	if rl.IsSmartList() {
		http.Error(w, storage.ErrCBLIsSmartList.Error(), http.StatusUnprocessableEntity)
		return
	}
	if len(rl.Books) == 0 {
		http.Error(w, "CBL file has no books", http.StatusUnprocessableEntity)
		return
	}

	result, err := sb.DB().ImportCBL(rl, storage.CBLImportSource{Source: "local_file"})
	if err != nil {
		if err == storage.ErrCBLIsSmartList {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		http.Error(w, "import failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.InvalidateListCache()

	s.writeJSON(w, http.StatusOK, toCBLImportResultWire(rl.Name, result))
}
