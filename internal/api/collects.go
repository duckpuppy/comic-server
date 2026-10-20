package api

import (
	"encoding/json"
	"net/http"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/collects"
	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// collectsOrEmpty returns the book's collects links, never nil, so the JSON
// carries [] and the page does not need a null check.
func collectsOrEmpty(b *library.ComicBook) []collects.Entry {
	if e := collects.Get(b); e != nil {
		return e
	}
	return []collects.Entry{}
}

// CollectsRequest is the body of POST /api/library/books/:id/collects.
type CollectsRequest struct {
	Entries []collects.Entry `json:"entries"`
	// Preview checks the entries and returns warnings; nothing is saved.
	Preview bool `json:"preview"`
}

// CollectsLookupRequest is the body of POST /api/library/books/:id/collects/lookup.
type CollectsLookupRequest struct {
	Ref string `json:"ref"`
}

// CollectsLookupResponse is a ComicVine volume and its issue numbers, so the
// linking screen can fill the volume name and check a range.
type CollectsLookupResponse struct {
	VolumeID   int      `json:"volume_id"`
	Name       string   `json:"name"`
	StartYear  string   `json:"start_year"`
	Publisher  string   `json:"publisher"`
	IssueCount int      `json:"issue_count"`
	VolumeURL  string   `json:"volume_url"`
	Numbers    []string `json:"numbers"`
}

// handleCollectsLookup reads a ComicVine volume the user pasted. It changes
// nothing. A cached volume costs no API call; otherwise one paced call.
// POST /api/library/books/:id/collects/lookup
func (s *Server) handleCollectsLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.scraper == nil {
		http.Error(w, "ComicVine scraper is not configured", http.StatusServiceUnavailable)
		return
	}
	var req CollectsLookupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	ref, err := comicvine.ParseCVRef(req.Ref)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if ref.Kind != "volume" {
		http.Error(w, "paste a ComicVine volume link or id (it looks like 4050-12345)", http.StatusBadRequest)
		return
	}
	vp, err := s.scraper.PreviewVolume(cvInteractiveCtx(r), ref.ID)
	if err != nil {
		s.writeCVError(w, err)
		return
	}
	resp := CollectsLookupResponse{
		VolumeID: vp.VolumeID, Name: vp.Name, StartYear: vp.StartYear, Publisher: vp.Publisher,
		IssueCount: vp.IssueCount, VolumeURL: vp.VolumeURL, Numbers: make([]string, 0, len(vp.Issues)),
	}
	for _, i := range vp.Issues {
		resp.Numbers = append(resp.Numbers, i.Number)
	}
	s.writeJSON(w, http.StatusOK, resp)
}

// collectsWarnings checks each entry against its volume's cached issue list.
// One list of notes per entry; a volume that is not cached gives none.
func (s *Server) collectsWarnings(entries []collects.Entry) [][]string {
	out := make([][]string, len(entries))
	for i, e := range entries {
		var nums []string
		if e.Volume > 0 && s.scraper != nil {
			nums, _ = s.scraper.CachedIssueNumbers(e.Volume)
		}
		out[i] = collects.Warnings(e, nums)
	}
	return out
}

// handleBookCollects replaces the list of issues a collected edition contains
// (tracker #36, #38). An empty list clears it. The links are local to
// comic-server; nothing is written to the file or to ComicVine.
// POST /api/library/books/:id/collects
func (s *Server) handleBookCollects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	id := bookDetailID(r.URL.Path, "/collects")
	var req CollectsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if err := collects.ValidateAll(req.Entries); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Preview {
		s.writeJSON(w, http.StatusOK, map[string]any{"preview": true, "warnings": s.collectsWarnings(req.Entries)})
		return
	}
	backend := s.audited(audit.ActionCollects)
	book, err := backend.GetBook(id)
	if err != nil || book == nil {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	// The audited GetBook copy shares its slices with the stored record; the
	// custom value is a string, so replacing it does not touch the original.
	if collects.Set(book, req.Entries) {
		if err := backend.UpdateBooks([]*library.ComicBook{book}); err != nil {
			log.Error().Err(err).Str("book_id", id).Msg("Failed to save collects links")
			http.Error(w, "Failed to save book", http.StatusInternalServerError)
			return
		}
	}
	s.invalidateVolumeSummaries() // Missing Issues shows what a collection covers
	s.writeJSON(w, http.StatusOK, map[string]any{"book_id": id, "collects": collectsOrEmpty(book), "warnings": s.collectsWarnings(req.Entries)})
}
