package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/trash"
)

// TrashEntryResponse is the JSON representation of a trash.Entry, for the
// web UI's trash browser (comic-server-tfs).
type TrashEntryResponse struct {
	ID            string    `json:"id"`
	OriginalPath  string    `json:"original_path"`
	QuarantinedAt time.Time `json:"quarantined_at"`
	Size          int64     `json:"size"`
}

func toTrashEntryResponse(e trash.Entry) TrashEntryResponse {
	return TrashEntryResponse{
		ID:            e.ID,
		OriginalPath:  e.OriginalPath,
		QuarantinedAt: e.QuarantinedAt,
		Size:          e.Size,
	}
}

// newTrashFromConfig builds a *trash.Trash from the server's current
// config, same construction handleRunCBZConvert already uses - trash
// config is kept live-reloadable via SIGHUP like the rest of the app, so
// this is built fresh per request rather than cached on Server.
func (s *Server) newTrashFromConfig() (*trash.Trash, error) {
	s.configMu.RLock()
	cfg := s.config
	s.configMu.RUnlock()

	if cfg == nil || cfg.Server.TrashPath == "" {
		return nil, fmt.Errorf("trash is not configured (server.trash_path)")
	}
	return trash.New(cfg.Server.TrashPath, cfg.Server.TrashRetentionDays)
}

// handleListTrash returns every quarantined file, newest first.
// GET /api/trash
func (s *Server) handleListTrash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tr, err := s.newTrashFromConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	entries, err := tr.List()
	if err != nil {
		log.Error().Err(err).Msg("Failed to list trash")
		http.Error(w, "Failed to list trash", http.StatusInternalServerError)
		return
	}

	resp := make([]TrashEntryResponse, 0, len(entries))
	for _, e := range entries {
		resp = append(resp, toTrashEntryResponse(e))
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"entries": resp})
}

// TrashRestoreRequest is the body for POST /api/trash/restore. A single
// item is just {"ids": ["<id>"]} - one endpoint serves both the per-item
// and multi-select Restore actions in the web UI.
type TrashRestoreRequest struct {
	IDs []string `json:"ids"`
}

// TrashRestoreResult reports the outcome of a restore request, one entry
// per failure (matching CBZConvertResult's Processed/Converted/Errors
// shape - the closest existing precedent for a bulk-op response in this
// codebase).
type TrashRestoreResult struct {
	Restored int      `json:"restored"`
	Errors   []string `json:"errors,omitempty"`
}

// handlePostTrashRestore restores one or more quarantined files - moving
// each back to its original path, quarantining whatever currently
// occupies that path first if it's no longer free (see trash.Restore).
// POST /api/trash/restore
func (s *Server) handlePostTrashRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TrashRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.IDs) == 0 {
		http.Error(w, "ids is required", http.StatusBadRequest)
		return
	}

	tr, err := s.newTrashFromConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	result := TrashRestoreResult{}
	for _, id := range req.IDs {
		if err := tr.Restore(id); err != nil {
			log.Error().Err(err).Str("id", id).Msg("Failed to restore trash entry")
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		result.Restored++
	}

	s.writeJSON(w, http.StatusOK, result)
}
