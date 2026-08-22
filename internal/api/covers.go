package api

import (
	"net/http"
	"strings"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/log"
)

// handleBooksRouter routes /api/library/books/:id/... requests.
func (s *Server) handleBooksRouter(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	suffix := strings.TrimPrefix(path, "/api/library/books/")

	if strings.HasSuffix(suffix, "/cover") {
		s.handleGetBookCover(w, r)
		return
	}

	http.NotFound(w, r)
}

// handleGetBookCover serves a comic's cover image, extracted from its
// archive file (CBZ/CBR/CB7) on every request via comicvine.ExtractCover -
// see comic-server-0y6.2 for the on-disk cache that avoids repeated
// extraction on every request.
// GET /api/library/books/:id/cover
func (s *Server) handleGetBookCover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	suffix := strings.TrimPrefix(r.URL.Path, "/api/library/books/")
	bookID := strings.TrimSuffix(suffix, "/cover")
	if bookID == "" {
		http.Error(w, "book id is required", http.StatusBadRequest)
		return
	}

	book, err := s.backend.GetBook(bookID)
	if err != nil {
		log.Error().Err(err).Str("book_id", bookID).Msg("Error looking up book for cover")
		http.Error(w, "Error looking up book", http.StatusInternalServerError)
		return
	}
	if book == nil {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	if book.FilePath == "" {
		http.Error(w, "Book has no file path", http.StatusNotFound)
		return
	}

	data, err := comicvine.ExtractCover(book.FilePath)
	if err != nil {
		log.Warn().Err(err).Str("book_id", bookID).Str("file_path", book.FilePath).Msg("Failed to extract cover image")
		http.Error(w, "Cover not available", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", http.DetectContentType(data))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Write(data)
}
