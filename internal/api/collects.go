package api

import (
	"encoding/json"
	"net/http"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/collects"
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
	s.writeJSON(w, http.StatusOK, map[string]any{"book_id": id, "collects": collectsOrEmpty(book)})
}
