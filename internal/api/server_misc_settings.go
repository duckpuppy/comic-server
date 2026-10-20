package api

import (
	"encoding/json"
	"net/http"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// ServerMiscSettingsResponse is the GET/PUT /api/settings/server-misc wire
// shape - small, otherwise-unrelated settings (comic-server-wp8k,
// AutoSync added in comic-server-r769) bundled the same way they're
// stored (configdb.ServerMiscSettings).
type ServerMiscSettingsResponse struct {
	CBZConvertEnabled bool     `json:"cbz_convert_enabled"`
	IgnoreDevices     []string `json:"ignore_devices"`
	AutoSync          bool     `json:"auto_sync"`
	// CountCoveredAsOwned: issues a collected edition contains count as
	// owned in Missing Issues and the CV smart-list matchers (tracker #46).
	CountCoveredAsOwned bool `json:"count_covered_as_owned"`
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
		resp.AutoSync = stored.AutoSync
		resp.CountCoveredAsOwned = stored.CountCoveredAsOwned
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
			resp.AutoSync = s.config.Server.AutoSync
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

	before := s.countCoveredAsOwned()
	if err := s.configDB.UpsertServerMiscSettings(configdb.ServerMiscSettings{
		CBZConvertEnabled:   req.CBZConvertEnabled,
		IgnoreDevices:       req.IgnoreDevices,
		AutoSync:            req.AutoSync,
		CountCoveredAsOwned: req.CountCoveredAsOwned,
	}); err != nil {
		log.Error().Err(err).Msg("Failed to save server misc settings")
		http.Error(w, "Failed to save server misc settings", http.StatusInternalServerError)
		return
	}

	// Apply immediately in-memory too, same as the config.db write, so
	// every existing cmd-package call site reading cfg.Server.CBZConvert.
	// Enabled / cfg.Server.IgnoreDevices / cfg.Server.AutoSync directly
	// (they were never rewritten to read through configDB - see
	// cmd/server.go's applyServerMiscSettings) sees the new value on its
	// very next read, with no restart and no SIGHUP needed.
	s.configMu.Lock()
	if s.config != nil {
		s.config.Server.CBZConvert.Enabled = req.CBZConvertEnabled
		s.config.Server.IgnoreDevices = req.IgnoreDevices
		s.config.Server.AutoSync = req.AutoSync
	}
	s.configMu.Unlock()

	if req.CountCoveredAsOwned != before {
		s.invalidateVolumeSummaries()
		go s.RebuildCVData() // the CV smart-list matchers read this; one pass over the library
	}

	if req.IgnoreDevices == nil {
		req.IgnoreDevices = []string{}
	}
	s.writeJSON(w, http.StatusOK, req)
}

// countCoveredAsOwned reads the "count covered issues as owned" setting
// (tracker #46). Off when config.db has never been given settings.
func (s *Server) countCoveredAsOwned() bool {
	if s.configDB == nil {
		return false
	}
	st, err := s.configDB.GetServerMiscSettings()
	return err == nil && st != nil && st.CountCoveredAsOwned
}

// completenessOptions are the options Missing Issues and the CV matchers use.
func (s *Server) completenessOptions() comicvine.CompletenessOptions {
	return comicvine.CompletenessOptions{CountCoveredAsOwned: s.countCoveredAsOwned()}
}

// cvDataSetter is implemented by both library backends.
type cvDataSetter interface {
	SetCVData(map[string]*library.CVCompleteness)
}

// RebuildCVData recomputes the per-book ComicVine completeness the smart-list
// matchers use, with the current settings. It does nothing without a
// ComicVine cache. The startup sync does the same through cmd.
func (s *Server) RebuildCVData() {
	if s.cvCache == nil || s.backend == nil {
		return
	}
	setter, ok := s.backend.(cvDataSetter)
	if !ok {
		return
	}
	books, err := s.backend.GetAllBooks()
	if err != nil {
		log.Error().Err(err).Msg("ComicVine completeness rebuild: could not read books")
		return
	}
	data, err := comicvine.BuildCompletenessMapOpts(books, s.cvCache, s.completenessOptions())
	if err != nil {
		log.Error().Err(err).Msg("ComicVine completeness rebuild failed")
		return
	}
	setter.SetCVData(data)
	log.Info().Int("books_with_cv_data", len(data)).Bool("covered_as_owned", s.countCoveredAsOwned()).Msg("ComicVine completeness data rebuilt")
}
