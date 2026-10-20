package api

import (
	"encoding/json"
	"net/http"
	"slices"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/localid"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

// LocalIdentityRequest is the body of POST /api/library/books/:id/local-identity.
// Set false removes the identity from this book only. Set true with Preview
// lists the sibling books that could share the volume key and saves nothing;
// without Preview it saves the book and the SiblingIDs the user kept.
type LocalIdentityRequest struct {
	Set        bool     `json:"set"`
	Preview    bool     `json:"preview"`
	SiblingIDs []string `json:"sibling_ids"`
}

// LocalIdentitySibling is a book that could share the volume key.
type LocalIdentitySibling struct {
	ID          string `json:"id"`
	Number      string `json:"number"`
	Title       string `json:"title"`
	HasIdentity bool   `json:"has_identity"`
}

// LocalIdentityPreview is what a confirm would do.
type LocalIdentityPreview struct {
	Series    string                 `json:"series"`
	Number    string                 `json:"number"`
	VolumeKey string                 `json:"volume_key,omitempty"` // empty: a new key is made on confirm
	Siblings  []LocalIdentitySibling `json:"siblings"`
}

// handleLocalIdentity gives a book that ComicVine never lists a local
// identity, or removes it (tracker #43). The book keeps its own Series and
// Number; the identity only adds a stable key that sibling books of one line
// share. Nothing is written to the file or to ComicVine.
// POST /api/library/books/:id/local-identity
func (s *Server) handleLocalIdentity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	id := bookDetailID(r.URL.Path, "/local-identity")
	var req LocalIdentityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	backend := s.audited(audit.ActionLocalIdentity)
	book, err := backend.GetBook(id)
	if err != nil || book == nil {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}

	if !req.Set {
		if workflow.ClearLocalIdentity(book) {
			if err := backend.UpdateBooks([]*library.ComicBook{book}); err != nil {
				log.Error().Err(err).Str("book_id", id).Msg("Failed to clear local identity")
				http.Error(w, "Failed to save book", http.StatusInternalServerError)
				return
			}
			s.InvalidateWorkflowCache()
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"book_id": id, "set": false})
		return
	}

	if book.FilePath == "" {
		http.Error(w, "this book has no file; a local identity is for books you own", http.StatusConflict)
		return
	}
	if comicvine.VolumeIDFromStore(book.CustomValuesStore) > 0 {
		http.Error(w, "this book already has a ComicVine volume", http.StatusConflict)
		return
	}
	if localid.SeriesKey(book.Series) == "" {
		http.Error(w, "this book has no series name; set it with Edit first", http.StatusBadRequest)
		return
	}
	all, err := backend.GetAllBooks()
	if err != nil {
		http.Error(w, "Failed to load books", http.StatusInternalServerError)
		return
	}
	sibs := localid.Siblings(all, book)
	volume := localid.ExistingVolumeKey(append([]*library.ComicBook{book}, sibs...))

	if req.Preview {
		p := LocalIdentityPreview{Series: book.Series, Number: book.Number, VolumeKey: volume, Siblings: []LocalIdentitySibling{}}
		for _, o := range sibs {
			p.Siblings = append(p.Siblings, LocalIdentitySibling{ID: o.ID, Number: o.Number, Title: o.Title, HasIdentity: localid.Has(o)})
		}
		s.writeJSON(w, http.StatusOK, p)
		return
	}

	if volume == "" {
		volume = localid.NewVolumeKey()
	}
	targets := []*library.ComicBook{book}
	for _, o := range sibs {
		if slices.Contains(req.SiblingIDs, o.ID) {
			targets = append(targets, o)
		}
	}
	var changed []*library.ComicBook
	for _, b := range targets {
		cur, has := localid.Get(b)
		ident := localid.Identity{Volume: volume, Issue: cur.Issue}
		if !has {
			ident.Issue = localid.NewIssueKey()
		}
		if workflow.SetLocalIdentity(b, ident) {
			changed = append(changed, b)
		}
	}
	if len(changed) > 0 {
		if err := backend.UpdateBooks(changed); err != nil {
			log.Error().Err(err).Str("book_id", id).Msg("Failed to save local identity")
			http.Error(w, "Failed to save book", http.StatusInternalServerError)
			return
		}
		s.InvalidateWorkflowCache()
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"book_id": id, "set": true, "volume_key": volume, "books_changed": len(changed)})
}
