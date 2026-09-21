// CBL repo browsing/import (comic-server-oprf, spec
// docs/plans/2026-09-20-cbl-reading-list-import.md §3). Browses a local
// clone of a git-hosted CBL collection (default: DieselTech/CBL-
// ReadingLists) and imports a chosen file via the same
// storage.DB.ImportCBL path local-file import uses (comic-server-zc0g).
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/duckpuppy/comic-server/internal/cbl"
	"github.com/duckpuppy/comic-server/internal/cblrepo"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/storage"
)

// cblRepoSyncTimeout bounds a sync triggered from an HTTP request - a
// full clone of a repo the size of DieselTech's takes a few seconds in
// practice (see internal/cblrepo's real-repo manual check), but this
// caps it well short of a typical client/proxy timeout regardless.
const cblRepoSyncTimeout = 4 * time.Minute

// CBLRepoStatusWire reports whether repo browsing is configured, and if
// so, whether it's ever been synced and how many files it currently
// sees. 200 always - "not configured" is a normal state, not an error
// (comic-server-hono).
type CBLRepoStatusWire struct {
	Configured bool   `json:"configured"`
	URL        string `json:"url,omitempty"`
	Synced     bool   `json:"synced"`
	FileCount  int    `json:"file_count,omitempty"`
	HeadCommit string `json:"head_commit,omitempty"`
	Error      string `json:"error,omitempty"`
}

func (s *Server) cblRepoStatus() CBLRepoStatusWire {
	if s.cblRepo == nil {
		return CBLRepoStatusWire{Configured: false}
	}
	status := CBLRepoStatusWire{Configured: true, URL: s.cblRepo.URL}
	files, err := s.cblRepo.ListCBLFiles()
	if err != nil {
		// Not yet synced (clone doesn't exist) is expected, not an
		// error worth surfacing - any other failure is.
		status.Synced = false
		return status
	}
	status.Synced = true
	status.FileCount = len(files)
	if commit, err := s.cblRepo.HeadCommit(); err == nil {
		status.HeadCommit = commit
	}
	return status
}

// handleCBLRepoStatus serves GET /api/library/cbl-repo/status.
func (s *Server) handleCBLRepoStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeJSON(w, http.StatusOK, s.cblRepoStatus())
}

// handleCBLRepoSync serves POST /api/library/cbl-repo/sync - explicit,
// user-triggered refresh (manual, not automatic - matches this feature's
// other manual-over-automatic decisions, e.g. wanted-list integration).
func (s *Server) handleCBLRepoSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cblRepo == nil {
		http.Error(w, "CBL repo browsing is not configured (server.cbl_repo.url)", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), cblRepoSyncTimeout)
	defer cancel()
	if err := s.cblRepo.Sync(ctx); err != nil {
		if errors.Is(err, cblrepo.ErrGitNotFound) {
			http.Error(w, "git binary not found - CBL repo browsing requires git to be installed on this host", http.StatusServiceUnavailable)
			return
		}
		log.Error().Err(err).Str("url", s.cblRepo.URL).Msg("CBL repo sync failed")
		http.Error(w, "sync failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, http.StatusOK, s.cblRepoStatus())
}

// CBLRepoBrowseResultWire is the response of GET /api/library/cbl-repo/browse.
type CBLRepoBrowseResultWire struct {
	Files []string `json:"files"`
}

// handleCBLRepoBrowse serves GET /api/library/cbl-repo/browse?q=... -
// lists (optionally filtered by a case-insensitive path substring) every
// .cbl file in the local clone. Does NOT sync on every call (that would
// mean a git fetch per search keystroke) - triggers exactly one sync if
// the clone has never been made yet (first real use), otherwise browses
// whatever's already on disk; use POST .../sync to refresh explicitly.
func (s *Server) handleCBLRepoBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cblRepo == nil {
		http.Error(w, "CBL repo browsing is not configured (server.cbl_repo.url)", http.StatusServiceUnavailable)
		return
	}

	if _, err := s.cblRepo.ListCBLFiles(); err != nil {
		// First real use - the clone doesn't exist on disk yet. Sync
		// once now rather than making the caller POST /sync first.
		ctx, cancel := context.WithTimeout(r.Context(), cblRepoSyncTimeout)
		defer cancel()
		if syncErr := s.cblRepo.Sync(ctx); syncErr != nil {
			if errors.Is(syncErr, cblrepo.ErrGitNotFound) {
				http.Error(w, "git binary not found - CBL repo browsing requires git to be installed on this host", http.StatusServiceUnavailable)
				return
			}
			http.Error(w, "initial clone failed: "+syncErr.Error(), http.StatusInternalServerError)
			return
		}
	}

	q := r.URL.Query().Get("q")
	files, err := s.cblRepo.Search(q)
	if err != nil {
		http.Error(w, "browse failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, http.StatusOK, CBLRepoBrowseResultWire{Files: files})
}

// cblRepoImportRequest is the body of POST /api/library/cbl-repo/import.
type cblRepoImportRequest struct {
	Path string `json:"path"`
}

// handleCBLRepoImport serves POST /api/library/cbl-repo/import - imports
// one file already present in the local clone (a path returned by
// handleCBLRepoBrowse), matching and creating a list exactly like
// handleImportCBL's local-upload path (comic-server-zc0g), but reading
// the CBL bytes from the clone instead of an uploaded file, and
// recording git provenance (repo URL + HEAD commit) on the resulting
// list.
func (s *Server) handleCBLRepoImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cblRepo == nil {
		http.Error(w, "CBL repo browsing is not configured (server.cbl_repo.url)", http.StatusServiceUnavailable)
		return
	}
	sb, ok := s.backend.(*storage.SQLiteBackend)
	if !ok {
		http.Error(w, "CBL import requires the SQLite library backend", http.StatusServiceUnavailable)
		return
	}

	var req cblRepoImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}

	f, err := s.cblRepo.Open(req.Path)
	if err != nil {
		if errors.Is(err, cblrepo.ErrPathEscapesClone) {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		http.Error(w, "file not found in clone: "+err.Error(), http.StatusNotFound)
		return
	}
	defer f.Close()

	rl, err := cbl.Parse(f)
	if err != nil {
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

	source := storage.CBLImportSource{Source: "git:" + s.cblRepo.URL + ":" + req.Path}
	if commit, err := s.cblRepo.HeadCommit(); err == nil {
		source.SourceRef = commit
	}

	result, err := sb.DB().ImportCBL(rl, source)
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
