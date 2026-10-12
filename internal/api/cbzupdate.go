package api

import (
	"fmt"
	"github.com/duckpuppy/comic-server/internal/audit"
	"net/http"

	"github.com/duckpuppy/comic-server/internal/cbzconvert"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// StaleWritesStatus is the response for GET .../workflow/stale-writes.
type StaleWritesStatus struct {
	Enabled bool `json:"enabled"`
	Stale   int  `json:"stale"`
}

// WriteMetadataResult is the response for POST .../workflow/write-metadata.
type WriteMetadataResult struct {
	Processed int      `json:"processed"`
	Written   int      `json:"written"`
	Errors    []string `json:"errors,omitempty"`
}

// staleBooks returns every book whose CBZ is out of date relative to its
// library record (comic-server-os6x). Only CBZ/ZIP files can be rewritten in
// place, so other formats never appear here.
func (s *Server) staleBooks(embedComicBook bool) ([]*library.ComicBook, error) {
	allBooks, err := s.backend.GetAllBooks()
	if err != nil {
		return nil, err
	}
	var stale []*library.ComicBook
	for i := range allBooks {
		if cbzconvert.IsStale(&allBooks[i], embedComicBook) {
			stale = append(stale, &allBooks[i])
		}
	}
	return stale, nil
}

// handleStaleWrites reports how many CBZ files are out of date.
// GET /api/library/workflow/stale-writes
func (s *Server) handleStaleWrites(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.configMu.RLock()
	cfg := s.config
	s.configMu.RUnlock()

	if cfg == nil || !cfg.Server.CBZConvert.Enabled || s.backend == nil {
		s.writeJSON(w, http.StatusOK, StaleWritesStatus{})
		return
	}
	stale, err := s.staleBooks(cfg.Server.CBZConvert.EmbedComicBook)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load books: %v", err), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, http.StatusOK, StaleWritesStatus{Enabled: true, Stale: len(stale)})
}

// handleWriteMetadata rewrites every stale CBZ so its embedded metadata
// matches the library.
// POST /api/library/workflow/write-metadata
func (s *Server) handleWriteMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.configMu.RLock()
	cfg := s.config
	s.configMu.RUnlock()

	if cfg == nil || !cfg.Server.CBZConvert.Enabled {
		http.Error(w, "cbz_convert is not enabled in config", http.StatusServiceUnavailable)
		return
	}
	tr, err := s.newTrashFromConfig()
	if err != nil {
		log.Error().Err(err).Msg("Invalid trash configuration for write-metadata")
		http.Error(w, "Invalid trash configuration", http.StatusInternalServerError)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	embed := cfg.Server.CBZConvert.EmbedComicBook
	books, err := s.staleBooks(embed)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load books: %v", err), http.StatusInternalServerError)
		return
	}

	result := WriteMetadataResult{Processed: len(books)}
	var toUpdate []*library.ComicBook
	for _, book := range books {
		updated, err := cbzconvert.UpdateMetadata(book, s.resolveBookFilePath, tr, cbzconvert.Options{EmbedComicBook: embed})
		if err != nil {
			log.Error().Err(err).Str("book_id", book.ID).Str("file_path", book.FilePath).Msg("write-metadata failed for book")
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", book.ID, err))
			continue
		}
		book.PageCount = updated.PageCount
		cbzconvert.MarkWritten(book, embed)
		toUpdate = append(toUpdate, book)
		result.Written++
	}

	if len(toUpdate) > 0 {
		if err := s.audited(audit.ActionWriteMetadata).UpdateBooks(toUpdate); err != nil {
			log.Error().Err(err).Msg("Failed to save write-metadata updates")
			result.Errors = append(result.Errors, err.Error())
		}
		s.InvalidateWorkflowCache()
	}
	s.writeJSON(w, http.StatusOK, result)
}
