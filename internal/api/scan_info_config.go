package api

import (
	"encoding/json"
	"net/http"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/scaninfo"
)

// ScanInfoConfigResponse is the GET/PUT /api/settings/scan-info wire
// shape - snake_case, matching the rest of this API, unlike
// config.ScanInfoConfig itself, which only carries yaml/toml tags (it's a
// config.yaml type, not an API DTO).
type ScanInfoConfigResponse struct {
	Enabled   bool     `json:"enabled"`
	Scanners  []string `json:"scanners"`
	Blacklist []string `json:"blacklist"`
	Prefix    string   `json:"prefix"`
	Unknown   string   `json:"unknown"`
}

func toScanInfoConfigResponse(cfg config.ScanInfoConfig) ScanInfoConfigResponse {
	scanners := cfg.Scanners
	if scanners == nil {
		scanners = []string{}
	}
	blacklist := cfg.Blacklist
	if blacklist == nil {
		blacklist = []string{}
	}
	return ScanInfoConfigResponse{
		Enabled:   cfg.Enabled,
		Scanners:  scanners,
		Blacklist: blacklist,
		Prefix:    cfg.Prefix,
		Unknown:   cfg.Unknown,
	}
}

func (r ScanInfoConfigResponse) toConfig() config.ScanInfoConfig {
	return config.ScanInfoConfig{
		Enabled:   r.Enabled,
		Scanners:  r.Scanners,
		Blacklist: r.Blacklist,
		Prefix:    r.Prefix,
		Unknown:   r.Unknown,
	}
}

// effectiveScanInfo returns the ScanInfo config actually in effect:
// config.db's stored value if the user has ever saved one via
// GET/PUT /api/settings/scan-info, otherwise the config.yaml value loaded
// at startup (Server.ScanInfo). This is the first UI/API surface for this
// section (comic-server-4ms) - previously config.yaml-hand-edit-only, with
// no way to change it without a restart.
func (s *Server) effectiveScanInfo() (config.ScanInfoConfig, error) {
	stored, err := s.configDB.GetScanInfo()
	if err != nil {
		return config.ScanInfoConfig{}, err
	}
	if stored != nil {
		return *stored, nil
	}

	s.configMu.RLock()
	defer s.configMu.RUnlock()
	return s.config.Server.ScanInfo, nil
}

// handleScanInfoConfig serves GET/PUT /api/settings/scan-info.
func (s *Server) handleScanInfoConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetScanInfoConfig(w, r)
	case http.MethodPut:
		s.handlePutScanInfoConfig(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGetScanInfoConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.effectiveScanInfo()
	if err != nil {
		log.Error().Err(err).Msg("Failed to load scan info config")
		http.Error(w, "Failed to load scan info config", http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, http.StatusOK, toScanInfoConfigResponse(cfg))
}

func (s *Server) handlePutScanInfoConfig(w http.ResponseWriter, r *http.Request) {
	var resp ScanInfoConfigResponse
	if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	cfg := resp.toConfig()

	if err := cfg.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate() alone doesn't catch a scanner name or blacklist entry
	// that isn't valid regex syntax - both get compiled directly into
	// scaninfo.NewDetector's patterns, so build a real Detector here too
	// and reject anything it would reject, rather than only discovering
	// the bad entry the next time someone runs scan-info against a list.
	if cfg.Enabled {
		if _, err := scaninfo.NewDetector(cfg.Scanners, cfg.Blacklist, cfg.Prefix, cfg.Unknown); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	if err := s.configDB.UpsertScanInfo(cfg); err != nil {
		log.Error().Err(err).Msg("Failed to save scan info config")
		http.Error(w, "Failed to save scan info config", http.StatusInternalServerError)
		return
	}

	s.writeJSON(w, http.StatusOK, toScanInfoConfigResponse(cfg))
}
