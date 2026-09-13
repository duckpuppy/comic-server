package api

import (
	"encoding/json"
	"net/http"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/log"
)

// ServerMiscSettingsResponse is the GET/PUT /api/settings/server-misc wire
// shape - two small, otherwise-unrelated settings (comic-server-wp8k)
// bundled the same way they're stored (configdb.ServerMiscSettings).
type ServerMiscSettingsResponse struct {
	CBZConvertEnabled bool     `json:"cbz_convert_enabled"`
	IgnoreDevices     []string `json:"ignore_devices"`
}

// handleServerMiscSettings serves GET/PUT /api/settings/server-misc.
func (s *Server) handleServerMiscSettings(w http.ResponseWriter, r *http.Request) {
	if s.configDB == nil {
		http.Error(w, "Config database not available", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetServerMiscSettings(w, r)
	case http.MethodPut:
		s.handlePutServerMiscSettings(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGetServerMiscSettings(w http.ResponseWriter, r *http.Request) {
	stored, err := s.configDB.GetServerMiscSettings()
	if err != nil {
		log.Error().Err(err).Msg("Failed to load server misc settings")
		http.Error(w, "Failed to load server misc settings", http.StatusInternalServerError)
		return
	}
	resp := ServerMiscSettingsResponse{IgnoreDevices: []string{}}
	if stored != nil {
		resp.CBZConvertEnabled = stored.CBZConvertEnabled
		resp.IgnoreDevices = stored.IgnoreDevices
	} else {
		// Never migrated (e.g. a fresh install with nothing in config.yaml
		// either) - fall back to whatever's currently in effect in memory,
		// same fallback shape effectiveTrashConfig uses.
		s.configMu.RLock()
		if s.config != nil {
			resp.CBZConvertEnabled = s.config.Server.CBZConvert.Enabled
			if s.config.Server.IgnoreDevices != nil {
				resp.IgnoreDevices = s.config.Server.IgnoreDevices
			}
		}
		s.configMu.RUnlock()
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handlePutServerMiscSettings(w http.ResponseWriter, r *http.Request) {
	var req ServerMiscSettingsResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.configDB.UpsertServerMiscSettings(configdb.ServerMiscSettings{
		CBZConvertEnabled: req.CBZConvertEnabled,
		IgnoreDevices:     req.IgnoreDevices,
	}); err != nil {
		log.Error().Err(err).Msg("Failed to save server misc settings")
		http.Error(w, "Failed to save server misc settings", http.StatusInternalServerError)
		return
	}

	// Apply immediately in-memory too, same as the config.db write, so
	// every existing cmd-package call site reading cfg.Server.CBZConvert.
	// Enabled / cfg.Server.IgnoreDevices directly (they were never
	// rewritten to read through configDB - see cmd/server.go's
	// applyServerMiscSettings) sees the new value on its very next read,
	// with no restart and no SIGHUP needed.
	s.configMu.Lock()
	if s.config != nil {
		s.config.Server.CBZConvert.Enabled = req.CBZConvertEnabled
		s.config.Server.IgnoreDevices = req.IgnoreDevices
	}
	s.configMu.Unlock()

	if req.IgnoreDevices == nil {
		req.IgnoreDevices = []string{}
	}
	s.writeJSON(w, http.StatusOK, req)
}
