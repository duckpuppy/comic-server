package api

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/watchfolder"
	"github.com/duckpuppy/comic-server/internal/workflow"
	"github.com/google/uuid"
)

// handleGetWatchFolderNewFiles returns every comic archive currently
// sitting in a configured server.watch_folders directory that isn't yet
// backed by a library book - comic-server-chh's "stage 0", before any
// book record exists at all. GET /api/library/workflow/new-files
func (s *Server) handleGetWatchFolderNewFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	s.configMu.RLock()
	folders := []string{}
	if s.config != nil {
		folders = s.config.Server.WatchFolders
	}
	s.configMu.RUnlock()

	if len(folders) == 0 {
		s.writeJSON(w, http.StatusOK, map[string]any{"files": []watchfolder.DiscoveredFile{}, "total": 0})
		return
	}

	found, err := s.scanWatchFolders(folders)
	if err != nil {
		http.Error(w, "Failed to scan watch folders", http.StatusInternalServerError)
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{"files": found, "total": len(found)})
}

// scanWatchFolders builds the known-FilePath set from every current book
// and scans folders for anything not in it.
func (s *Server) scanWatchFolders(folders []string) ([]watchfolder.DiscoveredFile, error) {
	allBooks, err := s.backend.GetAllBooks()
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(allBooks))
	for i := range allBooks {
		paths[i] = allBooks[i].FilePath
	}
	known := watchfolder.KnownPathSet(paths)
	return watchfolder.Scan(folders, known)
}

// startProcessingRequest is the body for POST
// /api/library/workflow/new-files/start.
type startProcessingRequest struct {
	Paths []string `json:"paths"`
}

// startProcessingResult reports what happened to one requested path.
type startProcessingResult struct {
	Path   string `json:"path"`
	BookID string `json:"book_id,omitempty"`
	Error  string `json:"error,omitempty"`
}

// handleStartProcessingNewFiles promotes one or more watch-folder files
// into real library book records, then lets them flow through the normal
// workflow pipeline (comic-server-chh). Each path is independently
// verified against the CURRENT watch-folder scan right before creating
// its book - never trusts the client's list at face value, since the
// underlying file could have been moved, converted, or already imported
// by something else between the drill-in load and this request.
// POST /api/library/workflow/new-files/start
func (s *Server) handleStartProcessingNewFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	var req startProcessingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Paths) == 0 {
		http.Error(w, "paths must not be empty", http.StatusBadRequest)
		return
	}

	s.configMu.RLock()
	folders := []string{}
	if s.config != nil {
		folders = s.config.Server.WatchFolders
	}
	s.configMu.RUnlock()

	found, err := s.scanWatchFolders(folders)
	if err != nil {
		http.Error(w, "Failed to scan watch folders", http.StatusInternalServerError)
		return
	}
	stillNew := make(map[string]watchfolder.DiscoveredFile, len(found))
	for _, f := range found {
		stillNew[f.Path] = f
	}

	rulesets := s.loadWorkflowRulesets()
	results := make([]startProcessingResult, 0, len(req.Paths))
	created := 0
	for _, path := range req.Paths {
		if _, ok := stillNew[path]; !ok {
			results = append(results, startProcessingResult{Path: path, Error: "no longer a new watch-folder file (already imported, moved, or removed)"})
			continue
		}

		book := newBookFromWatchFolderFile(path)
		workflow.SetStage(&book, workflow.InferStage(&book, rulesets))

		if err := s.backend.CreateBook(&book); err != nil {
			log.Error().Err(err).Str("path", path).Msg("Failed to create book from watch folder file")
			results = append(results, startProcessingResult{Path: path, Error: "failed to create book record"})
			continue
		}
		results = append(results, startProcessingResult{Path: path, BookID: book.ID})
		created++
	}

	if created > 0 {
		s.InvalidateWorkflowCache()
	}

	s.writeJSON(w, http.StatusOK, map[string]any{"created": created, "results": results})
}

// newBookFromWatchFolderFile builds the initial book record for a comic
// archive found on disk with no library metadata at all yet -
// comicvine.ParseFilename does the same best-effort Series/Number/Year
// guess already used to seed a ComicVine scrape search, so a freshly
// created book starts with something more useful than a blank record.
func newBookFromWatchFolderFile(path string) library.ComicBook {
	parsed := comicvine.ParseFilename(path)
	book := library.ComicBook{
		ID:       "{" + uuid.New().String() + "}",
		FilePath: path,
		Series:   parsed.Series,
		Number:   parsed.IssueNumber,
		Year:     parsed.Year,
		Volume:   parsed.Volume,
	}
	if info, err := os.Stat(path); err == nil {
		book.FileSize = info.Size()
		book.FileModifiedTime = library.ComicTime{Time: info.ModTime()}
	}
	return book
}
