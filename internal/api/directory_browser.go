package api

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/duckpuppy/comic-server/internal/log"
)

// DirectoryBrowseResponse is the wire shape for GET
// /api/system/browse-directory - a read-only directory listing for any
// UI that needs to pick a SERVER-side path (comic-server-obe's Watch
// Folders section is the first caller; comic-server-38f7 and
// comic-server-fkq are expected to reuse this same endpoint rather than
// building their own). A native browser file picker can't see the
// server's filesystem (especially meaningless under Docker, where the
// container's filesystem differs from the host), so this is a small
// server-side directory-listing API the picker UI navigates instead.
//
// Only directories are ever listed (never files) - this exists purely
// for picking a folder path, not for browsing file contents. Dotfiles/
// dot-directories are skipped as UI noise, not as a security boundary.
//
// This endpoint carries no additional access restriction beyond whatever
// already protects the rest of the API: comic-server has no
// authentication today, and several existing settings (trash_path,
// library_path, database_path - see restart_required_settings.go) already
// accept an arbitrary server-side absolute path with no listing needed
// first, so a read-only directory listing doesn't raise the trust bar
// beyond where it already sits.
type DirectoryBrowseResponse struct {
	Path        string   `json:"path"`
	Parent      string   `json:"parent,omitempty"`
	Directories []string `json:"directories"`
}

// handleBrowseDirectory serves GET /api/system/browse-directory?path=...
// - an empty/missing path lists "/" (the filesystem root), matching how a
// native file picker starting point works. A path that doesn't exist, or
// exists but isn't a directory, is a 400 - this endpoint has no way to
// distinguish "typo" from "not created yet" and isn't trying to.
func (s *Server) handleBrowseDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	requested := r.URL.Query().Get("path")
	if requested == "" {
		requested = "/"
	}
	abs, err := filepath.Abs(requested)
	if err != nil {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	info, err := os.Stat(abs)
	if err != nil {
		http.Error(w, "Path not found: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !info.IsDir() {
		http.Error(w, "Path is not a directory", http.StatusBadRequest)
		return
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		log.Error().Err(err).Str("path", abs).Msg("Failed to list directory")
		http.Error(w, "Failed to list directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	dirs := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dirs = append(dirs, e.Name())
	}
	sort.Strings(dirs)

	parent := filepath.Dir(abs)
	if parent == abs {
		parent = "" // already at the root - nothing to go up to
	}

	s.writeJSON(w, http.StatusOK, DirectoryBrowseResponse{Path: abs, Parent: parent, Directories: dirs})
}
