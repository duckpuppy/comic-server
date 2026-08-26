package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/duckpuppy/comic-server/internal/cbzconvert"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/trash"
)

// CBZConvertResult is the response for POST .../convert-cbz.
type CBZConvertResult struct {
	Processed int      `json:"processed"`
	Converted int      `json:"converted"`
	Errors    []string `json:"errors,omitempty"`
}

// handleRunCBZConvert converts every book currently matched by one smart
// list into CBZ, repacking pages and embedding ComicInfo.xml - mirroring
// how the user manually selects a smart list's results in ComicRack and
// runs its built-in Convert to CBZ action over them (comic-server-43b, see
// comic-server-pkk.2's research). comic-server's first feature that writes
// to and retires a comic archive file, so - like scan-info
// (comic-server-pkk.1) - it's off by default and additionally requires
// server.trash_path to be configured (see comic-server-1up).
// POST /api/library/lists/:listId/convert-cbz
func (s *Server) handleRunCBZConvert(w http.ResponseWriter, r *http.Request) {
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

	tr, err := trash.New(cfg.Server.TrashPath, cfg.Server.TrashRetentionDays)
	if err != nil {
		log.Error().Err(err).Msg("Invalid trash configuration for cbz-convert")
		http.Error(w, "Invalid trash configuration", http.StatusInternalServerError)
		return
	}

	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	listID := listIDFromCBZConvertSubPath(r.URL.Path)
	list, err := s.backend.FindListByID(listID)
	if err != nil {
		log.Error().Err(err).Str("list_id", listID).Msg("Error looking up smart list for cbz-convert run")
		http.Error(w, "Error looking up smart list", http.StatusInternalServerError)
		return
	}
	if list == nil {
		http.Error(w, "List not found", http.StatusNotFound)
		return
	}

	books, err := s.backend.MatchBooks(list)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to match books: %v", err), http.StatusInternalServerError)
		return
	}

	result := CBZConvertResult{Processed: len(books)}
	var toUpdate []*library.ComicBook

	for _, book := range books {
		converted, err := cbzconvert.Convert(book, s.resolveBookFilePath, tr)
		if err != nil {
			log.Error().Err(err).Str("book_id", book.ID).Str("file_path", book.FilePath).Msg("cbz-convert failed for book")
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", book.ID, err))
			continue
		}
		book.FilePath = converted.NewFilePath
		book.PageCount = converted.PageCount
		toUpdate = append(toUpdate, book)
		result.Converted++
	}

	if len(toUpdate) > 0 {
		if err := s.backend.UpdateBooks(toUpdate); err != nil {
			log.Error().Err(err).Msg("Failed to save cbz-convert updates")
			result.Errors = append(result.Errors, err.Error())
		}
	}

	s.writeJSON(w, http.StatusOK, result)
}

func listIDFromCBZConvertSubPath(path string) string {
	suffix := strings.TrimPrefix(path, "/api/library/lists/")
	return strings.TrimSuffix(suffix, "/convert-cbz")
}

// needsConvertCount returns how many of list's currently-matched books
// would actually be touched by cbzconvert.Convert (see
// cbzconvert.NeedsConversion), or nil if server.cbz_convert isn't
// enabled - see ListDetail.NeedsConvertCount's doc comment for why this
// is gated. Uses the same MatchBooks call handleRunCBZConvert itself
// uses, so the count always matches what an actual conversion run would
// do; errors are logged and treated as "unknown" (nil) rather than
// failing the list-detail page over what's just a UI nicety.
func (s *Server) needsConvertCount(list *library.ComicListItem) *int {
	s.configMu.RLock()
	cfg := s.config
	s.configMu.RUnlock()

	if cfg == nil || !cfg.Server.CBZConvert.Enabled || s.backend == nil {
		return nil
	}

	books, err := s.backend.MatchBooks(list)
	if err != nil {
		log.Warn().Err(err).Str("list_id", list.ID).Msg("Failed to evaluate needs-convert count")
		return nil
	}

	count := 0
	for _, book := range books {
		if cbzconvert.NeedsConversion(book) {
			count++
		}
	}
	return &count
}
