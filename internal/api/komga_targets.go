package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/komga"
	"github.com/duckpuppy/comic-server/internal/log"
)

// SetKomgaSyncer wires the live Komga Syncer into the API server, so target
// changes made through the web UI (comic-server-d3w) take effect
// immediately - no config-file edit, SIGHUP, or restart required. Call once
// at startup when Komga sync is enabled; without it, target mutations are
// still saved to config but only take effect on the next restart.
func (s *Server) SetKomgaSyncer(syncer *komga.Syncer) {
	s.komgaSyncer = syncer
}

// KomgaTargetResponse is the JSON representation of a config.KomgaTarget.
type KomgaTargetResponse struct {
	ListID         string `json:"list_id"`
	ListName       string `json:"list_name"`
	Type           string `json:"type"`
	KomgaName      string `json:"komga_name"`
	Enabled        bool   `json:"enabled"`
	SyncReadStatus bool   `json:"sync_read_status"`
}

// KomgaTargetForListResponse is the response for GET .../komga.
type KomgaTargetForListResponse struct {
	// KomgaEnabled reflects whether Komga integration itself is turned on
	// in config (komga.enabled) - a target can be saved even when this is
	// false, but it won't sync until Komga is enabled (requires a restart
	// to pick up base_url/api_key/local_root/remote_root).
	KomgaEnabled bool                 `json:"komga_enabled"`
	Target       *KomgaTargetResponse `json:"target"`
}

// KomgaTargetWriteRequest is the body for create/update requests.
type KomgaTargetWriteRequest struct {
	Type           string `json:"type"`
	KomgaName      string `json:"komga_name"`
	Enabled        bool   `json:"enabled"`
	SyncReadStatus bool   `json:"sync_read_status"`
}

func toKomgaTargetResponse(t configdb.KomgaTarget) *KomgaTargetResponse {
	return &KomgaTargetResponse{
		ListID:         t.ListID,
		ListName:       t.ListName,
		Type:           t.Type,
		KomgaName:      t.KomgaName,
		Enabled:        t.Enabled,
		SyncReadStatus: t.SyncReadStatus,
	}
}

// listIDFromKomgaSubPath extracts :listId from
// /api/library/lists/:listId/komga.
func listIDFromKomgaSubPath(path string) string {
	suffix := strings.TrimPrefix(path, "/api/library/lists/")
	return strings.TrimSuffix(suffix, "/komga")
}

// handleGetListKomgaTarget returns the Komga target (if any) configured for
// one list. GET /api/library/lists/:listId/komga
func (s *Server) handleGetListKomgaTarget(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	listID := listIDFromKomgaSubPath(r.URL.Path)

	s.configMu.RLock()
	resp := KomgaTargetForListResponse{KomgaEnabled: s.config.Server.Komga.Enabled}
	s.configMu.RUnlock()

	target, err := s.configDB.GetKomgaTarget(listID)
	if err != nil {
		log.Error().Err(err).Str("list_id", listID).Msg("Error looking up Komga target")
		http.Error(w, "Error looking up Komga target", http.StatusInternalServerError)
		return
	}
	if target != nil {
		resp.Target = toKomgaTargetResponse(*target)
	}
	s.writeJSON(w, http.StatusOK, resp)
}

// handleCreateListKomgaTarget creates a Komga target for one list.
// POST /api/library/lists/:listId/komga
func (s *Server) handleCreateListKomgaTarget(w http.ResponseWriter, r *http.Request) {
	listID := listIDFromKomgaSubPath(r.URL.Path)
	if listID == "" {
		http.Error(w, "list_id is required", http.StatusBadRequest)
		return
	}

	var req KomgaTargetWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	targetType := config.KomgaTargetType(req.Type)
	if targetType != config.KomgaTargetCollection && targetType != config.KomgaTargetReadList {
		http.Error(w, `type must be "collection" or "readlist"`, http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.KomgaName) == "" {
		http.Error(w, "komga_name is required", http.StatusBadRequest)
		return
	}

	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	list, err := s.backend.FindListByID(listID)
	if err != nil {
		log.Error().Err(err).Str("list_id", listID).Msg("Error looking up list for Komga target")
		http.Error(w, "Error looking up list", http.StatusInternalServerError)
		return
	}
	// Any list type with real book membership can sync to Komga - smart
	// lists, ID lists, and reading lists all evaluate via
	// backend.GetBooksForList (see komga.Syncer.syncTarget). Folders don't:
	// they're a grouping of other lists, not a set of books themselves.
	// Originally restricted to smart lists only (a leftover from before
	// GetBooksForList existed) - relaxed 2026-08-26 after a real "To Read"
	// ID list hit this and got the confusing "not found" error even though
	// the list existed.
	if list == nil || strings.Contains(list.Type, "Folder") {
		http.Error(w, "List not found, or is a folder (folders can't sync to Komga)", http.StatusNotFound)
		return
	}

	existing, err := s.configDB.GetKomgaTarget(listID)
	if err != nil {
		log.Error().Err(err).Str("list_id", listID).Msg("Error checking for existing Komga target")
		http.Error(w, "Error checking for existing Komga target", http.StatusInternalServerError)
		return
	}
	if existing != nil {
		http.Error(w, "Komga target already exists for this list", http.StatusConflict)
		return
	}

	newTarget := configdb.KomgaTarget{
		ListID:         listID,
		ListName:       list.Name,
		Type:           string(targetType),
		KomgaName:      req.KomgaName,
		Enabled:        req.Enabled,
		SyncReadStatus: req.SyncReadStatus,
	}
	if err := s.configDB.CreateKomgaTarget(newTarget); err != nil {
		log.Error().Err(err).Str("list_id", listID).Msg("Failed to save Komga target")
		http.Error(w, "Failed to save Komga target", http.StatusInternalServerError)
		return
	}
	s.applyKomgaTargets()

	log.Info().Str("list_id", listID).Str("komga_name", req.KomgaName).Str("type", req.Type).Msg("Komga target created")
	s.writeJSON(w, http.StatusOK, toKomgaTargetResponse(newTarget))
}

// handleUpdateListKomgaTarget updates the Komga target for one list.
// PUT /api/library/lists/:listId/komga
func (s *Server) handleUpdateListKomgaTarget(w http.ResponseWriter, r *http.Request) {
	listID := listIDFromKomgaSubPath(r.URL.Path)

	var req KomgaTargetWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	targetType := config.KomgaTargetType(req.Type)
	if targetType != config.KomgaTargetCollection && targetType != config.KomgaTargetReadList {
		http.Error(w, `type must be "collection" or "readlist"`, http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.KomgaName) == "" {
		http.Error(w, "komga_name is required", http.StatusBadRequest)
		return
	}

	if err := s.configDB.UpdateKomgaTarget(listID, string(targetType), req.KomgaName, req.Enabled, req.SyncReadStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "No Komga target configured for this list", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("list_id", listID).Msg("Failed to update Komga target")
		http.Error(w, "Failed to update Komga target", http.StatusInternalServerError)
		return
	}

	result, err := s.configDB.GetKomgaTarget(listID)
	if err != nil || result == nil {
		log.Error().Err(err).Str("list_id", listID).Msg("Failed to reload Komga target after update")
		http.Error(w, "Failed to reload Komga target after update", http.StatusInternalServerError)
		return
	}
	s.applyKomgaTargets()

	log.Info().Str("list_id", listID).Str("komga_name", req.KomgaName).Bool("enabled", req.Enabled).Msg("Komga target updated")
	s.writeJSON(w, http.StatusOK, toKomgaTargetResponse(*result))
}

// handleDeleteListKomgaTarget removes the Komga target for one list.
// DELETE /api/library/lists/:listId/komga
func (s *Server) handleDeleteListKomgaTarget(w http.ResponseWriter, r *http.Request) {
	listID := listIDFromKomgaSubPath(r.URL.Path)

	existing, err := s.configDB.GetKomgaTarget(listID)
	if err != nil {
		log.Error().Err(err).Str("list_id", listID).Msg("Error looking up Komga target")
		http.Error(w, "Error looking up Komga target", http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.Error(w, "No Komga target configured for this list", http.StatusNotFound)
		return
	}
	if err := s.configDB.DeleteKomgaTarget(listID); err != nil {
		log.Error().Err(err).Str("list_id", listID).Msg("Failed to remove Komga target")
		http.Error(w, "Failed to remove Komga target", http.StatusInternalServerError)
		return
	}
	s.applyKomgaTargets()

	log.Info().Str("list_id", listID).Msg("Komga target removed")
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// applyKomgaTargets pushes the current set of enabled Komga targets from
// config.db into the live Syncer (if one is wired up via SetKomgaSyncer)
// and triggers an immediate sync pass, so a target add/update/remove made
// through the web UI takes effect right away instead of waiting for the
// next scheduled interval or a restart. A no-op if Komga sync isn't
// running (komgaSyncer is nil) - the change is still persisted for when it
// next starts.
func (s *Server) applyKomgaTargets() {
	if s.komgaSyncer == nil {
		return
	}

	dbTargets, err := s.configDB.ListKomgaTargets()
	if err != nil {
		log.Error().Err(err).Msg("Failed to load Komga targets from config database")
		return
	}

	targets := make([]komga.Target, 0, len(dbTargets))
	for _, t := range dbTargets {
		if !t.Enabled {
			continue
		}
		var targetType komga.TargetType
		switch config.KomgaTargetType(t.Type) {
		case config.KomgaTargetCollection:
			targetType = komga.TargetCollection
		case config.KomgaTargetReadList:
			targetType = komga.TargetReadList
		default:
			continue
		}
		targets = append(targets, komga.Target{
			ListID:         t.ListID,
			KomgaName:      t.KomgaName,
			Type:           targetType,
			SyncReadStatus: t.SyncReadStatus,
		})
	}

	s.komgaSyncer.SetTargets(targets)
	s.komgaSyncer.TriggerNow()
}
