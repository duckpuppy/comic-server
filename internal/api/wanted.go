package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/duckpuppy/comic-server/internal/cbzconvert"
	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/workflow"
	"github.com/google/uuid"
)

// handleWantedBooks serves GET (list) and POST (create) on
// /api/library/workflow/wanted - "wanted" books (comic-server-38f7) are
// ordinary book records with FilePath=="", so a user can track an issue
// they don't own a file for yet and later link one in. Distinct from
// StageUnknown (which also includes ordinary never-backfilled real
// books): wanted books need their own direct FilePath=="" query, not a
// workflow stage - see the Wanted card doc comment in
// internal/api/web/js/workflowPage.js.
func (s *Server) handleWantedBooks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetWantedBooks(w, r)
	case http.MethodPost:
		s.handleCreateWantedBook(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetWantedBooks returns every book currently missing a file.
// GET /api/library/workflow/wanted
func (s *Server) handleGetWantedBooks(w http.ResponseWriter, r *http.Request) {
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	wanted, err := s.wantedBooks()
	if err != nil {
		http.Error(w, "Failed to load books", http.StatusInternalServerError)
		return
	}

	previews := make([]ComicPreview, 0, len(wanted))
	for _, book := range wanted {
		previews = append(previews, ComicPreview{
			ID:        book.ID,
			Series:    book.Series,
			Number:    book.Number,
			Title:     book.Title,
			Volume:    book.Volume,
			Publisher: book.Publisher,
			Year:      book.Year,
		})
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"comics": previews, "total": len(previews)})
}

// wantedBooks returns every book with an empty FilePath, working from a
// fresh GetAllBooks() snapshot - same "scan everything, filter in Go"
// pattern scanWatchFolders already uses, since there's no SQLite backend
// today to push this filter down into a query.
func (s *Server) wantedBooks() ([]*library.ComicBook, error) {
	allBooks, err := s.backend.GetAllBooks()
	if err != nil {
		return nil, err
	}
	var wanted []*library.ComicBook
	for i := range allBooks {
		if allBooks[i].FilePath == "" {
			wanted = append(wanted, &allBooks[i])
		}
	}
	return wanted, nil
}

// wantedCreateRequest is the body for POST /api/library/workflow/wanted.
type wantedCreateRequest struct {
	Series    string `json:"series"`
	Number    string `json:"number"`
	Volume    int    `json:"volume"`
	Year      int    `json:"year"`
	Publisher string `json:"publisher"`
}

// handleCreateWantedBook creates a placeholder book record with no file
// (comic-server-38f7's "Create" piece) - reuses library.Backend.CreateBook,
// already built for the watch-folder flow (comic-server-chh), just with
// an empty FilePath and metadata from the form instead of a filename
// guess/embedded ComicInfo.xml. Never enters the workflow pipeline (no
// SetStage call) until a file is actually linked - see handleLinkWantedBook.
func (s *Server) handleCreateWantedBook(w http.ResponseWriter, r *http.Request) {
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	var req wantedCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Series) == "" {
		http.Error(w, "series is required", http.StatusBadRequest)
		return
	}

	book := library.ComicBook{
		ID:        "{" + uuid.New().String() + "}",
		Series:    req.Series,
		Number:    req.Number,
		Volume:    req.Volume,
		Year:      req.Year,
		Publisher: req.Publisher,
	}
	if err := s.backend.CreateBook(&book); err != nil {
		log.Error().Err(err).Msg("Failed to create wanted book")
		http.Error(w, "Failed to create book", http.StatusInternalServerError)
		return
	}
	s.InvalidateWorkflowCache()

	s.writeJSON(w, http.StatusCreated, ComicPreview{
		ID: book.ID, Series: book.Series, Number: book.Number,
		Volume: book.Volume, Publisher: book.Publisher, Year: book.Year,
	})
}

// wantedLinkRequest is the body for POST /api/library/workflow/wanted/link.
type wantedLinkRequest struct {
	BookID   string `json:"book_id"`
	FilePath string `json:"file_path"`
}

// handleLinkWantedBook attaches a file (picked via the server-side
// browser, comic-server-obe) to an existing wanted book (comic-server-38f7's
// "Link" piece), then re-classifies it through the normal pipeline exactly
// like any other book - workflow.InferStage, not a special "New Files"
// path, since New Files is specifically for files with no book record at
// all (comic-server-chh) and doesn't apply once a book record already
// exists (confirmed with the user rather than assumed - see this issue's
// design notes). Any embedded ComicInfo.xml on the linked file overlays
// the book's placeholder metadata (Series/Number/etc typed at creation),
// same "the real file's own metadata wins" precedent as
// newBookFromWatchFolderFile.
// POST /api/library/workflow/wanted/link
func (s *Server) handleLinkWantedBook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	var req wantedLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.BookID == "" || req.FilePath == "" {
		http.Error(w, "book_id and file_path are required", http.StatusBadRequest)
		return
	}

	info, err := os.Stat(req.FilePath)
	if err != nil {
		http.Error(w, "File not found: "+err.Error(), http.StatusBadRequest)
		return
	}
	if info.IsDir() {
		http.Error(w, "file_path is a directory, not a file", http.StatusBadRequest)
		return
	}

	book, err := s.backend.GetBook(req.BookID)
	if err != nil {
		http.Error(w, "Failed to load book: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if book == nil {
		http.NotFound(w, r)
		return
	}
	if book.FilePath != "" {
		http.Error(w, "Book already has a file - not a wanted entry", http.StatusConflict)
		return
	}

	book.FilePath = req.FilePath
	book.FileSize = info.Size()
	book.FileModifiedTime = library.ComicTime{Time: info.ModTime()}
	if data, ok := comicvine.ReadComicInfoXMLBytes(req.FilePath); ok {
		if fromXML, ok := cbzconvert.ParseComicInfoXML(data); ok {
			overlayNonZeroFields(book, fromXML)
		}
	}

	rulesets := s.loadWorkflowRulesets()
	workflow.SetStage(book, workflow.InferStage(book, rulesets))

	if err := s.backend.UpdateBooks([]*library.ComicBook{book}); err != nil {
		log.Error().Err(err).Str("book_id", book.ID).Msg("Failed to save linked wanted book")
		http.Error(w, "Failed to save book", http.StatusInternalServerError)
		return
	}
	s.InvalidateWorkflowCache()

	s.writeJSON(w, http.StatusOK, ComicPreview{
		ID: book.ID, Series: book.Series, Number: book.Number, Title: book.Title,
		Volume: book.Volume, Publisher: book.Publisher, Year: book.Year, CurrentPath: book.FilePath,
	})
}
