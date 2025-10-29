package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/device"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/syncstate"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// VersionInfo contains build version information
type VersionInfo struct {
	Version   string
	GitCommit string
	BuildDate string
}

// Server provides REST API endpoints for monitoring and control
type Server struct {
	syncManager *syncstate.Manager
	registry    *device.Registry
	config      *config.Config
	version     VersionInfo
	mux         *http.ServeMux
	startTime   time.Time
}

// NewServer creates a new API server with version information
func NewServer(syncManager *syncstate.Manager, registry *device.Registry, cfg *config.Config, version VersionInfo) *Server {
	s := &Server{
		syncManager: syncManager,
		registry:    registry,
		config:      cfg,
		version:     version,
		mux:         http.NewServeMux(),
		startTime:   time.Now(),
	}

	s.registerRoutes()
	return s
}

// ServeHTTP implements http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// registerRoutes sets up all API endpoints
func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/version", s.handleVersion)
	s.mux.HandleFunc("/api/sync/status", s.handleSyncStatus)
	s.mux.HandleFunc("/api/sync/history", s.handleSyncHistory)
	s.mux.HandleFunc("/api/devices", s.handleDevices)
	s.mux.HandleFunc("/api/stats", s.handleStats)
	s.mux.Handle("/metrics", promhttp.Handler())
}

// Health check response
type HealthResponse struct {
	Status    string `json:"status"`
	Uptime    string `json:"uptime"`
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildDate string `json:"build_date"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uptime := time.Since(s.startTime)
	response := HealthResponse{
		Status:    "healthy",
		Uptime:    uptime.String(),
		Version:   s.version.Version,
		GitCommit: s.version.GitCommit,
		BuildDate: s.version.BuildDate,
	}

	s.writeJSON(w, http.StatusOK, response)
}

// Version response
type VersionResponse struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildDate string `json:"build_date"`
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := VersionResponse{
		Version:   s.version.Version,
		GitCommit: s.version.GitCommit,
		BuildDate: s.version.BuildDate,
	}

	s.writeJSON(w, http.StatusOK, response)
}

// Sync status response
type SyncStatusResponse struct {
	ActiveSyncs []*syncstate.SyncState `json:"active_syncs"`
	ActiveCount int                    `json:"active_count"`
}

func (s *Server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	syncs := s.syncManager.GetAllActiveSyncs()
	response := SyncStatusResponse{
		ActiveSyncs: syncs,
		ActiveCount: len(syncs),
	}

	s.writeJSON(w, http.StatusOK, response)
}

// Sync history response
type SyncHistoryResponse struct {
	History []*syncstate.SyncState `json:"history"`
	Count   int                    `json:"count"`
}

func (s *Server) handleSyncHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get limit from query parameter (default: 20)
	limit := 20
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		var parsedLimit int
		if _, err := fmt.Sscanf(limitStr, "%d", &parsedLimit); err == nil && parsedLimit > 0 {
			limit = parsedLimit
			if limit > 100 {
				limit = 100 // Cap at 100
			}
		}
	}

	history := s.syncManager.GetHistory(limit)
	response := SyncHistoryResponse{
		History: history,
		Count:   len(history),
	}

	s.writeJSON(w, http.StatusOK, response)
}

// Device info for API
type DeviceInfo struct {
	ID           string    `json:"id"`
	IP           string    `json:"ip"`
	Name         string    `json:"name"`
	Model        string    `json:"model"`
	Manufacturer string    `json:"manufacturer"`
	Edition      string    `json:"edition"`
	LastSeen     time.Time `json:"last_seen"`
	IsSyncing    bool      `json:"is_syncing"`
}

// Devices response
type DevicesResponse struct {
	Devices []DeviceInfo `json:"devices"`
	Count   int          `json:"count"`
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	devices := s.registry.List()
	deviceInfos := make([]DeviceInfo, 0, len(devices))

	for _, dev := range devices {
		info := DeviceInfo{
			ID:           dev.Info.ID,
			IP:           dev.IPAddress,
			Name:         dev.Info.Name,
			Model:        dev.Info.Model,
			Manufacturer: dev.Info.Manufacturer,
			Edition:      string(dev.Info.Edition),
			LastSeen:     dev.LastSeen,
			IsSyncing:    s.syncManager.IsDeviceSyncing(dev.Info.ID),
		}
		deviceInfos = append(deviceInfos, info)
	}

	response := DevicesResponse{
		Devices: deviceInfos,
		Count:   len(deviceInfos),
	}

	s.writeJSON(w, http.StatusOK, response)
}

// Stats response
type StatsResponse struct {
	Uptime               string `json:"uptime"`
	ActiveSyncs          int    `json:"active_syncs"`
	RegisteredDevices    int    `json:"registered_devices"`
	MaxConcurrent        int    `json:"max_concurrent_connections"`
	RateLimitingEnabled  bool   `json:"rate_limiting_enabled"`
	MaxConnectionsPerIP  int    `json:"max_connections_per_ip,omitempty"`
	MaxRequestsPerDevice int    `json:"max_requests_per_device,omitempty"`
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uptime := time.Since(s.startTime)
	devices := s.registry.List()

	response := StatsResponse{
		Uptime:               uptime.String(),
		ActiveSyncs:          s.syncManager.GetActiveCount(),
		RegisteredDevices:    len(devices),
		MaxConcurrent:        s.config.Server.MaxConcurrentConnections,
		RateLimitingEnabled:  s.config.Server.MaxConnectionsPerIP > 0 || s.config.Server.MaxRequestsPerDevice > 0,
		MaxConnectionsPerIP:  s.config.Server.MaxConnectionsPerIP,
		MaxRequestsPerDevice: s.config.Server.MaxRequestsPerDevice,
	}

	s.writeJSON(w, http.StatusOK, response)
}

// writeJSON writes a JSON response
func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Error().Err(err).Msg("Failed to encode JSON response")
	}
}

// parsePathParam extracts a path parameter after a prefix
// Example: "/api/sync/status/device123" with prefix "/api/sync/status/" returns "device123"
func parsePathParam(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	return strings.TrimPrefix(path, prefix)
}
