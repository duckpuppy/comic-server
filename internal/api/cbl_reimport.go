// CBL watch/reimport (comic-server-zw0o, spec §5/§6 phase 3). Detects
// whether a git-sourced CBL-imported list's upstream file changed since
// it was imported, and re-applies it on request. See
// storage.CBLReimportPolicy for why reimport is always a full replace of
// list membership, never a merge - comic-server has no feature that lets
// a user hand-edit a reading list's membership in the first place.
//
// Two-step, never-silent flow (matches this codebase's other
// destructive-adjacent actions, e.g. CBZ convert's confirm()):
//  1. GET  .../cbl-reimport-check  - preview only, no write.
//  2. POST .../lists/:id/cbl-reimport - applies it, only after the caller
//     (the web UI, behind its own confirm) has seen the preview.
package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/cbl"
	"github.com/duckpuppy/comic-server/internal/cblrepo"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/storage"
)

const cblReimportCheckTimeout = 2 * time.Minute

// gitCBLSourcePrefix is the CBLImportSource.Source prefix used for
// anything imported via the configured git clone (handleCBLRepoImport) -
// see spec §4. "local_file" sources have no upstream to watch.
const gitCBLSourcePrefix = "git:"

// parseGitCBLSource splits a "git:<repo-url>:<path>" source string.
// Returns ok=false for anything else (empty, local_file, or a URL
// containing no path separator, which shouldn't happen in practice).
func parseGitCBLSource(source string) (repoURL, path string, ok bool) {
	if !strings.HasPrefix(source, gitCBLSourcePrefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(source, gitCBLSourcePrefix)
	// repo-url itself contains colons (https://...) but never a path
	// component with one, so split on the LAST colon.
	i := strings.LastIndex(rest, ":")
	if i < 0 {
		return "", "", false
	}
	return rest[:i], rest[i+1:], true
}

// CBLReimportCandidateWire is one list's watch-check result.
type CBLReimportCandidateWire struct {
	ListID  string `json:"list_id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Status  string `json:"status"` // unchanged | modified | renamed | orphaned | base_unknown | error
	NewPath string `json:"new_path,omitempty"`
	Error   string `json:"error,omitempty"`
}

// handleCBLReimportCheck serves GET /api/library/cbl-repo/reimport-check
// - a read-only sweep of every git-sourced CBL-imported list, reporting
// which ones changed upstream. Local-file imports are skipped entirely
// (no upstream to check). Syncs the clone first so the check runs
// against a current HEAD, not a stale one.
func (s *Server) handleCBLReimportCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cblRepo == nil {
		http.Error(w, "CBL repo browsing is not configured (server.cbl_repo.url)", http.StatusServiceUnavailable)
		return
	}
	sb, ok := s.backend.(*storage.SQLiteBackend)
	if !ok {
		http.Error(w, "CBL reimport requires the SQLite library backend", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), cblReimportCheckTimeout)
	defer cancel()
	if err := s.cblRepo.Sync(ctx); err != nil {
		if errors.Is(err, cblrepo.ErrGitNotFound) {
			http.Error(w, "git binary not found - CBL reimport requires git to be installed on this host", http.StatusServiceUnavailable)
			return
		}
		// Sync already falls back to the last good clone on a network
		// failure (spec §3) - it only errors when there's nothing usable
		// at all, which IS worth surfacing here.
		http.Error(w, "sync failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	lists, err := sb.DB().ListCBLImportedLists()
	if err != nil {
		http.Error(w, "list cbl-imported lists: "+err.Error(), http.StatusInternalServerError)
		return
	}

	out := make([]CBLReimportCandidateWire, 0, len(lists))
	for _, l := range lists {
		repoURL, path, ok := parseGitCBLSource(l.Source)
		if !ok {
			continue // local_file source - nothing to watch
		}
		c := CBLReimportCandidateWire{ListID: l.ListID, Name: l.Name, Path: path}
		if repoURL != s.cblRepo.URL {
			// Not this server's configured repo (e.g. config changed
			// since import) - can't diff against a clone we don't have.
			c.Status = "error"
			c.Error = "list's source repo is not the currently configured CBL repo"
			out = append(out, c)
			continue
		}
		change, err := s.cblRepo.CheckPathChange(ctx, l.SourceRef, path)
		if err != nil {
			c.Status = "error"
			c.Error = err.Error()
			out = append(out, c)
			continue
		}
		c.Status = change.Status.String()
		c.NewPath = change.NewPath
		out = append(out, c)
	}

	s.writeJSON(w, http.StatusOK, out)
}

// handleCBLReimport serves POST /api/library/lists/:id/cbl-reimport -
// re-parses the list's upstream CBL file at the clone's current HEAD,
// re-matches it, and fully replaces the list's membership (see
// storage.CBLReimportPolicy). The caller is expected to have already
// shown the user a preview from handleCBLReimportCheck and gotten
// confirmation - this endpoint does not ask again.
func (s *Server) handleCBLReimport(w http.ResponseWriter, r *http.Request, listID string) {
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
		http.Error(w, "CBL reimport requires the SQLite library backend", http.StatusServiceUnavailable)
		return
	}

	source, err := sb.DB().GetCBLSource(listID)
	if err != nil {
		http.Error(w, "get cbl source: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if source == nil {
		http.Error(w, storage.ErrListNotCBLImported.Error(), http.StatusUnprocessableEntity)
		return
	}
	repoURL, path, ok := parseGitCBLSource(source.Source)
	if !ok {
		http.Error(w, "list has no git upstream to reimport from (local_file import)", http.StatusUnprocessableEntity)
		return
	}
	if repoURL != s.cblRepo.URL {
		http.Error(w, "list's source repo is not the currently configured CBL repo", http.StatusUnprocessableEntity)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), cblReimportCheckTimeout)
	defer cancel()
	change, err := s.cblRepo.CheckPathChange(ctx, source.SourceRef, path)
	if err != nil {
		http.Error(w, "check path change: "+err.Error(), http.StatusInternalServerError)
		return
	}

	fetchPath := path
	switch change.Status {
	case cblrepo.PathUnchanged:
		http.Error(w, "nothing to reimport - upstream file has not changed", http.StatusConflict)
		return
	case cblrepo.PathRenamed:
		fetchPath = change.NewPath
	case cblrepo.PathOrphaned:
		http.Error(w, "upstream file was deleted or changed too much for git to trace as a rename - reimport refused; the list is left as-is (see comic-server-zw0o policy: never silently drop or guess a successor)", http.StatusConflict)
		return
	case cblrepo.PathBaseUnknown:
		http.Error(w, "upstream history was rewritten since this list's last import - cannot diff; delete and freshly re-import this list instead", http.StatusConflict)
		return
	}

	f, err := s.cblRepo.Open(fetchPath)
	if err != nil {
		http.Error(w, "open upstream file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	rl, err := cbl.Parse(f)
	if err != nil {
		http.Error(w, "failed to parse upstream CBL file: "+err.Error(), http.StatusBadRequest)
		return
	}
	if rl.IsSmartList() {
		http.Error(w, storage.ErrCBLIsSmartList.Error(), http.StatusUnprocessableEntity)
		return
	}

	newHead, err := s.cblRepo.HeadCommit()
	if err != nil {
		http.Error(w, "get clone head commit: "+err.Error(), http.StatusInternalServerError)
		return
	}
	newSource := storage.CBLImportSource{
		Source:    fmt.Sprintf("git:%s:%s", s.cblRepo.URL, fetchPath),
		SourceRef: newHead,
	}

	result, err := sb.DB().ReimportCBL(listID, rl, newSource)
	if err != nil {
		http.Error(w, "reimport failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Info().Str("list_id", listID).Str("path", fetchPath).
		Int("matched_cv_id", result.MatchedCVID).Int("matched_other", result.MatchedOther).
		Int("unmatched", result.Unmatched).Msg("CBL list reimported")

	s.InvalidateListCache()
	s.writeJSON(w, http.StatusOK, toCBLImportResultWire(rl.Name, result))
}
