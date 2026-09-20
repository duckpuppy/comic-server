package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/trash"
)

// TrashEntryResponse is the JSON representation of a trash.Entry, for the
// web UI's trash browser (comic-server-tfs). DeletesAt is computed
// server-side from the SAME RetentionDays that built the *trash.Trash
// this entry came from (comic-server-ci31) - QuarantinedAt plus
// RetentionDays, the exact cutoff Sweep itself uses, so the UI never has
// to duplicate that arithmetic or fetch retention_days separately.
type TrashEntryResponse struct {
	ID            string    `json:"id"`
	OriginalPath  string    `json:"original_path"`
	QuarantinedAt time.Time `json:"quarantined_at"`
	DeletesAt     time.Time `json:"deletes_at"`
	Size          int64     `json:"size"`
}

func toTrashEntryResponse(e trash.Entry, retentionDays int) TrashEntryResponse {
	return TrashEntryResponse{
		ID:            e.ID,
		OriginalPath:  e.OriginalPath,
		QuarantinedAt: e.QuarantinedAt,
		DeletesAt:     e.QuarantinedAt.AddDate(0, 0, retentionDays),
		Size:          e.Size,
	}
}

// handleListTrash returns every quarantined file, newest first.
// GET /api/trash
//
// "Not configured" (errTrashNotConfigured) is a normal state - trash is
// optional - reported as 200 with "configured": false rather than a 5xx,
// which the browser logs as a console error on every page load of an
// unconfigured server regardless of how the JS handles it
// (comic-server-hono). A genuine config/setup error still 5xxs.
func (s *Server) handleListTrash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tr, err := s.newTrashFromConfig()
	if errors.Is(err, errTrashNotConfigured) {
		s.writeJSON(w, http.StatusOK, map[string]any{"configured": false, "entries": []TrashEntryResponse{}})
		return
	}
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
		resp = append(resp, toTrashEntryResponse(e, tr.RetentionDays))
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"configured": true, "entries": resp})
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

// TrashDeleteRequest is the body for POST /api/trash/delete - same
// {"ids": [...]} shape as TrashRestoreRequest, one endpoint for both the
// per-item and multi-select "Delete Permanently" actions.
type TrashDeleteRequest struct {
	IDs []string `json:"ids"`
}

// TrashDeleteResult reports the outcome of a delete request, mirroring
// TrashRestoreResult's Processed/Errors shape.
type TrashDeleteResult struct {
	Deleted int      `json:"deleted"`
	Errors  []string `json:"errors,omitempty"`
}

// handlePostTrashDelete permanently deletes one or more quarantined files
// on demand (comic-server-2y3p) - the manual counterpart to the
// background age-based Sweep. Unlike Restore, this is NOT undoable: the
// web UI is expected to confirm before calling this, same destructive-
// action pattern used elsewhere (e.g. list/ruleset deletes).
// POST /api/trash/delete
func (s *Server) handlePostTrashDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TrashDeleteRequest
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

	result := TrashDeleteResult{}
	for _, id := range req.IDs {
		if err := tr.DeleteNow(id); err != nil {
			log.Error().Err(err).Str("id", id).Msg("Failed to permanently delete trash entry")
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		result.Deleted++
	}

	s.writeJSON(w, http.StatusOK, result)
}
