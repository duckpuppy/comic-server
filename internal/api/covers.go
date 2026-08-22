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

	if strings.HasSuffix(suffix, "/cover/cache") {
		s.handleDeleteBookCoverCache(w, r)
		return
	}
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

	var data []byte
	contentType := "image/jpeg" // the cache always re-encodes thumbnails as JPEG
	if s.coverCache != nil {
		data, err = s.coverCache.Get(bookID, book.FilePath)
	} else {
		data, err = comicvine.ExtractCover(book.FilePath)
		if err == nil {
			contentType = http.DetectContentType(data)
		}
	}
	if err != nil {
		log.Warn().Err(err).Str("book_id", bookID).Str("file_path", book.FilePath).Msg("Failed to extract cover image")
		http.Error(w, "Cover not available", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Write(data)
}

// handleDeleteBookCoverCache drops a book's cached cover thumbnail (if any),
// forcing the next GET .../cover to re-extract it - for the rare case the
// automatic mtime/size invalidation (comic-server-0y6.2) doesn't catch, or
// for testing. Idempotent: succeeds whether or not anything was cached, and
// is a no-op (not an error) when no cover cache is configured at all.
// DELETE /api/library/books/:id/cover/cache
func (s *Server) handleDeleteBookCoverCache(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	suffix := strings.TrimPrefix(r.URL.Path, "/api/library/books/")
	bookID := strings.TrimSuffix(suffix, "/cover/cache")
	if bookID == "" {
		http.Error(w, "book id is required", http.StatusBadRequest)
		return
	}

	if s.coverCache != nil {
		if err := s.coverCache.Invalidate(bookID); err != nil {
			log.Error().Err(err).Str("book_id", bookID).Msg("Failed to invalidate cover cache")
			http.Error(w, "Failed to invalidate cover cache", http.StatusInternalServerError)
			return
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}
