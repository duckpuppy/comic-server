package api

import (
	"encoding/json"
	"net/http"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/log"
)

// RestartRequiredSettings is the subset of config.yaml that's baked into an
// object or listener socket once at process startup (comic-server-yvbh) -
// unlike trash_path/CBZConvert.Enabled/ignore_devices (comic-server-wp8k)
// or scan_info (comic-server-4ms), none of these are read fresh per
// request, so saving a new value here can never take effect until the
// process restarts:
//
//   - LibraryPath - which backend gets constructed (internal/library vs
//     internal/storage) happens once in cmd/server.go's startup sequence.
//   - ServerPort/DiscoveryPort/BindAddress - sockets are bound once at
//     startup; SIGHUP does not rebind them (see cmd/server.go's SIGHUP
//     handler, which only reloads settings that ARE safe to hot-swap).
//   - ComicVineAPIKey - wireScraperAPI/startComicVineSync build the
//     ComicVine client and sync goroutine once at startup.
//   - Komga connection settings - buildKomgaSyncer constructs the
//     *komga.Syncer once at startup with these baked into its
//     SyncOptions. Komga TARGETS (the list of sync destinations) are a
//     separate, already-live config.db-backed feature (comic-server-3ek)
//     with its own UI - not this.
//
// The two API key fields are write-only: GET never returns the actual
// key, only whether one is currently set, so the value never round-trips
// through the browser/network tab unnecessarily. On PUT, an empty key
// field means "leave the existing key untouched" (not "clear it") - the
// form is always submitted with every field, so treating empty as a no-op
// is what lets a user change the port without accidentally wiping a key
// they can't see.
type RestartRequiredSettings struct {
	LibraryPath          string `json:"library_path"`
	DatabasePath         string `json:"database_path"`
	CoverCacheDir        string `json:"cover_cache_dir"`
	ServerPort           int    `json:"server_port"`
	DiscoveryPort        int    `json:"discovery_port"`
	BindAddress          string `json:"bind_address"`
	ComicVineAPIKeySet   bool   `json:"comicvine_api_key_set"`
	KomgaEnabled         bool   `json:"komga_enabled"`
	KomgaBaseURL         string `json:"komga_base_url"`
	KomgaAPIKeySet       bool   `json:"komga_api_key_set"`
	KomgaSyncIntervalSec int    `json:"komga_sync_interval_sec"`

	// The four fields below are baked once into a long-lived object at
	// startup (the connection semaphore and the two rate.Limiter
	// instances - see cmd/server.go) rather than read fresh per request,
	// same restart-required reasoning as everything else here (audited
	// for comic-server-obe).
	MaxConcurrentConnections     int `json:"max_concurrent_connections"`
	MaxConnectionsPerIP          int `json:"max_connections_per_ip"`
	MaxRequestsPerDevice         int `json:"max_requests_per_device"`
	RateLimitWindowSeconds       int `json:"rate_limit_window_seconds"`
	LibraryCacheFlushIntervalSec int `json:"library_cache_flush_interval_sec"`
}

// RestartRequiredSettingsResponse is the GET .../restart-required wire
// shape: Saved is what's currently in config.yaml (and will be used on the
// NEXT start), Active is what this running process actually started with,
// and RestartRequired is true the moment those two differ in any field -
// the UI's cue to show "restart needed" rather than pretending the change
// already took effect.
type RestartRequiredSettingsResponse struct {
	Saved           RestartRequiredSettings `json:"saved"`
	Active          RestartRequiredSettings `json:"active"`
	RestartRequired bool                    `json:"restart_required"`
	// RestartSupported is true when this process can restart itself in
	// place via POST /api/system/restart (comic-server-9klu) - the UI
	// shows a "Restart now" button when true and its existing
	// manual-restart message when false (e.g. Windows).
	RestartSupported bool `json:"restart_supported"`
}

// restartRequiredSettingsFromConfig extracts the wire shape from a full
// config.Config - used both for the live "Saved" side (s.config, which PUT
// mutates in place) and, via SetActiveRestartRequiredSettings, to snapshot
// the "Active" side once at startup before anything can mutate it.
func restartRequiredSettingsFromConfig(cfg *config.Config) RestartRequiredSettings {
	if cfg == nil {
		return RestartRequiredSettings{}
	}
	return RestartRequiredSettings{
		LibraryPath:                  cfg.Server.LibraryPath,
		DatabasePath:                 cfg.Server.DatabasePath,
		CoverCacheDir:                cfg.Server.CoverCacheDir,
		ServerPort:                   cfg.Server.ServerPort,
		DiscoveryPort:                cfg.Server.DiscoveryPort,
		BindAddress:                  cfg.Server.BindAddress,
		ComicVineAPIKeySet:           cfg.Server.ComicVineAPIKey != "",
		KomgaEnabled:                 cfg.Server.Komga.Enabled,
		KomgaBaseURL:                 cfg.Server.Komga.BaseURL,
		KomgaAPIKeySet:               cfg.Server.Komga.APIKey != "",
		KomgaSyncIntervalSec:         cfg.Server.Komga.SyncIntervalSec,
		MaxConcurrentConnections:     cfg.Server.MaxConcurrentConnections,
		MaxConnectionsPerIP:          cfg.Server.MaxConnectionsPerIP,
		MaxRequestsPerDevice:         cfg.Server.MaxRequestsPerDevice,
		RateLimitWindowSeconds:       cfg.Server.RateLimitWindowSeconds,
		LibraryCacheFlushIntervalSec: cfg.Server.LibraryCacheFlushIntervalSec,
	}
}

// SetActiveRestartRequiredSettings snapshots the restart-required settings
// this process actually started with. Call this once at startup, right
// after the config that built the backend/HTTP listeners/ComicVine
// client/Komga syncer is finalized and before any PUT to
// /api/settings/restart-required can mutate s.config - otherwise the
// "Active" side of the response would drift to match "Saved" and the
// restart-required indicator could never fire.
func (s *Server) SetActiveRestartRequiredSettings(cfg *config.Config) {
	s.activeRestartRequiredSettings = restartRequiredSettingsFromConfig(cfg)
}

// restartRequiredSettingsPutRequest is the PUT body - a plain-text
// comicvine_api_key/komga_api_key rather than the *Set booleans GET
// returns, since PUT is the one direction these secrets actually need to
// travel in.
type restartRequiredSettingsPutRequest struct {
	LibraryPath                  string `json:"library_path"`
	DatabasePath                 string `json:"database_path"`
	CoverCacheDir                string `json:"cover_cache_dir"`
	ServerPort                   int    `json:"server_port"`
	DiscoveryPort                int    `json:"discovery_port"`
	BindAddress                  string `json:"bind_address"`
	ComicVineAPIKey              string `json:"comicvine_api_key"`
	KomgaEnabled                 bool   `json:"komga_enabled"`
	KomgaBaseURL                 string `json:"komga_base_url"`
	KomgaAPIKey                  string `json:"komga_api_key"`
	KomgaSyncIntervalSec         int    `json:"komga_sync_interval_sec"`
	MaxConcurrentConnections     int    `json:"max_concurrent_connections"`
	MaxConnectionsPerIP          int    `json:"max_connections_per_ip"`
	MaxRequestsPerDevice         int    `json:"max_requests_per_device"`
	RateLimitWindowSeconds       int    `json:"rate_limit_window_seconds"`
	LibraryCacheFlushIntervalSec int    `json:"library_cache_flush_interval_sec"`
}

// handleRestartRequiredSettings serves GET/PUT /api/settings/restart-required.
func (s *Server) handleRestartRequiredSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetRestartRequiredSettings(w, r)
	case http.MethodPut:
		s.handlePutRestartRequiredSettings(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGetRestartRequiredSettings(w http.ResponseWriter, r *http.Request) {
	s.configMu.RLock()
	saved := restartRequiredSettingsFromConfig(s.config)
	s.configMu.RUnlock()

	active := s.activeRestartRequiredSettings

	s.writeJSON(w, http.StatusOK, RestartRequiredSettingsResponse{
		Saved:            saved,
		Active:           active,
		RestartRequired:  saved != active,
		RestartSupported: s.restartSupported(),
	})
}

func (s *Server) handlePutRestartRequiredSettings(w http.ResponseWriter, r *http.Request) {
	var req restartRequiredSettingsPutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	s.configMu.Lock()
	if s.config == nil {
		s.configMu.Unlock()
		http.Error(w, "Configuration not available", http.StatusServiceUnavailable)
		return
	}

	// Validate against a copy first so a bad request never partially
	// mutates the live in-memory config.
	candidate := *s.config
	candidate.Server.LibraryPath = req.LibraryPath
	candidate.Server.DatabasePath = req.DatabasePath
	candidate.Server.CoverCacheDir = req.CoverCacheDir
	candidate.Server.ServerPort = req.ServerPort
	candidate.Server.DiscoveryPort = req.DiscoveryPort
	candidate.Server.BindAddress = req.BindAddress
	candidate.Server.Komga.Enabled = req.KomgaEnabled
	candidate.Server.Komga.BaseURL = req.KomgaBaseURL
	candidate.Server.Komga.SyncIntervalSec = req.KomgaSyncIntervalSec
	candidate.Server.MaxConcurrentConnections = req.MaxConcurrentConnections
	candidate.Server.MaxConnectionsPerIP = req.MaxConnectionsPerIP
	candidate.Server.MaxRequestsPerDevice = req.MaxRequestsPerDevice
	candidate.Server.RateLimitWindowSeconds = req.RateLimitWindowSeconds
	candidate.Server.LibraryCacheFlushIntervalSec = req.LibraryCacheFlushIntervalSec
	if req.ComicVineAPIKey != "" {
		candidate.Server.ComicVineAPIKey = req.ComicVineAPIKey
	}
	if req.KomgaAPIKey != "" {
		candidate.Server.Komga.APIKey = req.KomgaAPIKey
	}

	if err := candidate.Validate(); err != nil {
		s.configMu.Unlock()
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	*s.config = candidate
	cfgCopy := *s.config
	s.configMu.Unlock()

	if err := config.Save(&cfgCopy, s.configPath); err != nil {
		log.Error().Err(err).Msg("Failed to save config.yaml after updating restart-required settings")
		http.Error(w, "Failed to save configuration", http.StatusInternalServerError)
		return
	}

	s.handleGetRestartRequiredSettings(w, r)
}
