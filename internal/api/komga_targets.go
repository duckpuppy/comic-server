package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/duckpuppy/comic-server/internal/config"
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
	ListID    string `json:"list_id"`
	ListName  string `json:"list_name"`
	Type      string `json:"type"`
	KomgaName string `json:"komga_name"`
	Enabled   bool   `json:"enabled"`
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
	Type      string `json:"type"`
	KomgaName string `json:"komga_name"`
	Enabled   bool   `json:"enabled"`
}

func toKomgaTargetResponse(t config.KomgaTarget) *KomgaTargetResponse {
	return &KomgaTargetResponse{
		ListID:    t.ListID,
		ListName:  t.ListName,
		Type:      string(t.Type),
		KomgaName: t.KomgaName,
		Enabled:   t.Enabled,
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
	defer s.configMu.RUnlock()

	resp := KomgaTargetForListResponse{KomgaEnabled: s.config.Server.Komga.Enabled}
	for _, t := range s.config.Server.Komga.Targets {
		if t.ListID == listID {
			resp.Target = toKomgaTargetResponse(t)
			break
		}
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
		log.Error().Err(err).Str("list_id", listID).Msg("Error looking up smart list for Komga target")
		http.Error(w, "Error looking up smart list", http.StatusInternalServerError)
		return
	}
	if list == nil || !strings.Contains(list.Type, "SmartList") {
		http.Error(w, "Smart list not found in library", http.StatusNotFound)
		return
	}

	s.configMu.Lock()
	for _, t := range s.config.Server.Komga.Targets {
		if t.ListID == listID {
			s.configMu.Unlock()
			http.Error(w, "Komga target already exists for this list", http.StatusConflict)
			return
		}
	}
	newTarget := config.KomgaTarget{
		ListID:    listID,
		ListName:  list.Name,
		Type:      targetType,
		KomgaName: req.KomgaName,
		Enabled:   req.Enabled,
	}
	s.config.Server.Komga.Targets = append(s.config.Server.Komga.Targets, newTarget)
	saveErr := config.Save(s.config, s.configPath)
	s.configMu.Unlock()

	if saveErr != nil {
		log.Error().Err(saveErr).Msg("Failed to save config after adding Komga target")
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

	s.configMu.Lock()
	var updated *config.KomgaTarget
	targets := s.config.Server.Komga.Targets
	for i := range targets {
		if targets[i].ListID == listID {
			targets[i].Type = targetType
			targets[i].KomgaName = req.KomgaName
			targets[i].Enabled = req.Enabled
			updated = &targets[i]
			break
		}
	}
	if updated == nil {
		s.configMu.Unlock()
		http.Error(w, "No Komga target configured for this list", http.StatusNotFound)
		return
	}
	result := *updated
	saveErr := config.Save(s.config, s.configPath)
	s.configMu.Unlock()

	if saveErr != nil {
		log.Error().Err(saveErr).Msg("Failed to save config after updating Komga target")
	}
	s.applyKomgaTargets()

	log.Info().Str("list_id", listID).Str("komga_name", req.KomgaName).Bool("enabled", req.Enabled).Msg("Komga target updated")
	s.writeJSON(w, http.StatusOK, toKomgaTargetResponse(result))
}

// handleDeleteListKomgaTarget removes the Komga target for one list.
// DELETE /api/library/lists/:listId/komga
func (s *Server) handleDeleteListKomgaTarget(w http.ResponseWriter, r *http.Request) {
	listID := listIDFromKomgaSubPath(r.URL.Path)

	s.configMu.Lock()
	targets := s.config.Server.Komga.Targets
	idx := -1
	for i := range targets {
		if targets[i].ListID == listID {
			idx = i
			break
		}
	}
	if idx == -1 {
		s.configMu.Unlock()
		http.Error(w, "No Komga target configured for this list", http.StatusNotFound)
		return
	}
	s.config.Server.Komga.Targets = append(targets[:idx], targets[idx+1:]...)
	saveErr := config.Save(s.config, s.configPath)
	s.configMu.Unlock()

	if saveErr != nil {
		log.Error().Err(saveErr).Msg("Failed to save config after removing Komga target")
	}
	s.applyKomgaTargets()

	log.Info().Str("list_id", listID).Msg("Komga target removed")
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// applyKomgaTargets pushes the current set of enabled Komga targets from
// config into the live Syncer (if one is wired up via SetKomgaSyncer) and
// triggers an immediate sync pass, so a target add/update/remove made
// through the web UI takes effect right away instead of waiting for the
// next scheduled interval or a restart. A no-op if Komga sync isn't running
// (komgaSyncer is nil) - the change is still persisted to config for when it
// next starts.
func (s *Server) applyKomgaTargets() {
	if s.komgaSyncer == nil {
		return
	}

	s.configMu.RLock()
	cfgTargets := append([]config.KomgaTarget(nil), s.config.Server.Komga.Targets...)
	s.configMu.RUnlock()

	targets := make([]komga.Target, 0, len(cfgTargets))
	for _, t := range cfgTargets {
		if !t.Enabled {
			continue
		}
		var targetType komga.TargetType
		switch t.Type {
		case config.KomgaTargetCollection:
			targetType = komga.TargetCollection
		case config.KomgaTargetReadList:
			targetType = komga.TargetReadList
		default:
			continue
		}
		targets = append(targets, komga.Target{
			ListID:    t.ListID,
			KomgaName: t.KomgaName,
			Type:      targetType,
		})
	}

	s.komgaSyncer.SetTargets(targets)
	s.komgaSyncer.TriggerNow()
}
