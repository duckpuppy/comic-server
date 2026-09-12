package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/trash"
)

// errTrashNotConfigured is returned by newTrashFromConfig when no trash
// path is set (via config.db's trash_settings or config.yaml's
// server.trash_path) - callers surface this as a 503, same as before
// this settings UI existed.
var errTrashNotConfigured = errors.New("trash is not configured (Settings > Trash, or server.trash_path)")

// TrashSettingsResponse is the GET/PUT /api/settings/trash wire shape.
type TrashSettingsResponse struct {
	Path          string `json:"path"`
	RetentionDays int    `json:"retention_days"`
}

// effectiveTrashConfig returns the trash path/retention actually in
// effect: config.db's stored value if the user has ever saved one via
// GET/PUT /api/settings/trash, otherwise the config.yaml value loaded at
// startup (Server.TrashPath/TrashRetentionDays) - same fallback pattern
// effectiveScanInfo uses (comic-server-4hsz, comic-server-4ms's
// precedent). Every caller that builds a *trash.Trash (newTrashFromConfig,
// CBZ Convert, Library Organizer) should go through this rather than
// reading cfg.Server.TrashPath directly, so a setting saved through the
// UI takes effect without a restart.
func (s *Server) effectiveTrashConfig() (path string, retentionDays int, err error) {
	if s.configDB != nil {
		stored, err := s.configDB.GetTrashSettings()
		if err != nil {
			return "", 0, err
		}
		if stored != nil {
			return stored.Path, stored.RetentionDays, nil
		}
	}

	s.configMu.RLock()
	defer s.configMu.RUnlock()
	if s.config == nil {
		return "", 0, nil
	}
	return s.config.Server.TrashPath, s.config.Server.TrashRetentionDays, nil
}

// newTrashFromConfig builds a *trash.Trash from the effective trash
// config (config.db if ever saved through the UI, else config.yaml).
func (s *Server) newTrashFromConfig() (*trash.Trash, error) {
	path, retentionDays, err := s.effectiveTrashConfig()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, errTrashNotConfigured
	}
	return trash.New(path, retentionDays)
}

// handleTrashSettings serves GET/PUT /api/settings/trash.
func (s *Server) handleTrashSettings(w http.ResponseWriter, r *http.Request) {
	if s.configDB == nil {
		http.Error(w, "Config database not available", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetTrashSettings(w, r)
	case http.MethodPut:
		s.handlePutTrashSettings(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGetTrashSettings(w http.ResponseWriter, r *http.Request) {
	path, retentionDays, err := s.effectiveTrashConfig()
	if err != nil {
		log.Error().Err(err).Msg("Failed to load trash settings")
		http.Error(w, "Failed to load trash settings", http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, http.StatusOK, TrashSettingsResponse{Path: path, RetentionDays: retentionDays})
}

func (s *Server) handlePutTrashSettings(w http.ResponseWriter, r *http.Request) {
	var req TrashSettingsResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.RetentionDays < 0 {
		http.Error(w, "retention_days must be >= 0", http.StatusBadRequest)
		return
	}
	// An empty path is valid (disables the trash feature entirely, same
	// meaning as an unset server.trash_path in config.yaml) - only
	// reject a negative retention, matching config.Validate's own rule.
	if req.Path != "" {
		if _, err := trash.New(req.Path, req.RetentionDays); err != nil {
			http.Error(w, "Invalid trash configuration: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	if err := s.configDB.UpsertTrashSettings(configdb.TrashSettings{Path: req.Path, RetentionDays: req.RetentionDays}); err != nil {
		log.Error().Err(err).Msg("Failed to save trash settings")
		http.Error(w, "Failed to save trash settings", http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, http.StatusOK, req)
}
