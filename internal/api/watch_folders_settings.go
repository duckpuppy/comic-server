package api

import (
	"encoding/json"
	"net/http"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/log"
)

// WatchFoldersResponse is the GET/PUT /api/settings/watch-folders wire
// shape (comic-server-obe).
type WatchFoldersResponse struct {
	Folders []string `json:"folders"`
}

// effectiveWatchFolders returns the watch folders actually in effect:
// config.db's stored value if the user has ever saved one via this
// settings UI, otherwise the config.yaml value loaded at startup - same
// fallback pattern effectiveTrashConfig/effectiveScanInfo use. Every
// reader of watch folders (handleGetWatchFolderNewFiles,
// handleStartProcessingNewFiles) goes through this rather than reading
// cfg.Server.WatchFolders directly, so a folder added through the UI is
// scanned on the very next request - no restart needed.
func (s *Server) effectiveWatchFolders() ([]string, error) {
	if s.configDB != nil {
		stored, err := s.configDB.GetWatchFolders()
		if err != nil {
			return nil, err
		}
		if stored != nil {
			return stored.Folders, nil
		}
	}

	s.configMu.RLock()
	defer s.configMu.RUnlock()
	if s.config == nil {
		return nil, nil
	}
	return s.config.Server.WatchFolders, nil
}

// handleWatchFoldersSettings serves GET/PUT /api/settings/watch-folders.
func (s *Server) handleWatchFoldersSettings(w http.ResponseWriter, r *http.Request) {
	if s.configDB == nil {
		http.Error(w, "Config database not available", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetWatchFoldersSettings(w, r)
	case http.MethodPut:
		s.handlePutWatchFoldersSettings(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGetWatchFoldersSettings(w http.ResponseWriter, r *http.Request) {
	folders, err := s.effectiveWatchFolders()
	if err != nil {
		log.Error().Err(err).Msg("Failed to load watch folders")
		http.Error(w, "Failed to load watch folders", http.StatusInternalServerError)
		return
	}
	if folders == nil {
		folders = []string{}
	}
	s.writeJSON(w, http.StatusOK, WatchFoldersResponse{Folders: folders})
}

func (s *Server) handlePutWatchFoldersSettings(w http.ResponseWriter, r *http.Request) {
	var req WatchFoldersResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.configDB.UpsertWatchFolders(configdb.WatchFolders{Folders: req.Folders}); err != nil {
		log.Error().Err(err).Msg("Failed to save watch folders")
		http.Error(w, "Failed to save watch folders", http.StatusInternalServerError)
		return
	}

	// Apply immediately in-memory too, same as server_misc_settings.go -
	// handleGetWatchFolderNewFiles/handleStartProcessingNewFiles currently
	// still read cfg.Server.WatchFolders directly rather than through
	// effectiveWatchFolders (they predate this settings UI), so this keeps
	// them seeing the new value on their very next read with no restart.
	s.configMu.Lock()
	if s.config != nil {
		s.config.Server.WatchFolders = req.Folders
	}
	s.configMu.Unlock()

	if req.Folders == nil {
		req.Folders = []string{}
	}
	s.writeJSON(w, http.StatusOK, req)
}
