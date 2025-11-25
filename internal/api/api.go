package api

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/device"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
	csync "github.com/duckpuppy/comic-server/internal/sync"
	"github.com/duckpuppy/comic-server/internal/syncstate"
	ws "github.com/duckpuppy/comic-server/internal/websocket"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

//go:embed web
var webFS embed.FS

// VersionInfo contains build version information
type VersionInfo struct {
	Version   string
	GitCommit string
	BuildDate string
}

// Server provides REST API endpoints for monitoring and control
type Server struct {
	syncManager       *syncstate.Manager
	registry          *device.Registry
	backend           library.Backend       // Backend for library operations
	listCache         *library.ListCache    // Cache for smart list book counts
	config            *config.Config
	configPath        string // Path to config file for saving
	configMu          sync.RWMutex    // Protects config modifications
	version           VersionInfo
	mux               *http.ServeMux
	startTime         time.Time
	wsHub             *ws.Hub
	upgrader          websocket.Upgrader
	registeredDevices map[string]bool // In-memory registered device tracking
	mu                sync.RWMutex    // Protects registeredDevices
}

// NewServer creates a new API server with version information
func NewServer(syncManager *syncstate.Manager, registry *device.Registry, backend library.Backend, cfg *config.Config, configPath string, version VersionInfo, wsHub *ws.Hub) *Server {
	s := &Server{
		syncManager:       syncManager,
		registry:          registry,
		backend:           backend,
		listCache:         library.NewListCache(15 * time.Minute), // 15 min TTL
		config:            cfg,
		configPath:        configPath,
		version:           version,
		mux:               http.NewServeMux(),
		startTime:         time.Now(),
		wsHub:             wsHub,
		registeredDevices: make(map[string]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// Allow all origins for now (in production, check against allowed origins)
				return true
			},
		},
	}

	// Load initial registration state from config
	for deviceID := range cfg.Devices {
		s.registeredDevices[deviceID] = true
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
	// API endpoints
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/version", s.handleVersion)
	s.mux.HandleFunc("/api/sync/status", s.handleSyncStatus)
	s.mux.HandleFunc("/api/sync/history", s.handleSyncHistory)
	s.mux.HandleFunc("/api/stats", s.handleStats)

	// Library endpoints
	s.mux.HandleFunc("/api/library/lists/tree", s.handleGetListTree)
	s.mux.HandleFunc("/api/library/lists", s.handleGetLists)
	s.mux.HandleFunc("/api/library/lists/", s.handleListsRouter)

	// Device endpoints (all routes go through router)
	s.mux.HandleFunc("/api/devices", s.handleDevicesRouter)
	s.mux.HandleFunc("/api/devices/", s.handleDevicesRouter)

	// WebSocket endpoint
	s.mux.HandleFunc("/ws", s.handleWebSocket)

	// Metrics endpoint
	s.mux.Handle("/metrics", promhttp.Handler())

	// Static files (web UI) with SPA fallback
	webRoot, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create web filesystem")
	}
	s.mux.HandleFunc("/", s.spaHandler(webRoot))
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

	// Get offset from query parameter (default: 0)
	offset := 0
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		var parsedOffset int
		if _, err := fmt.Sscanf(offsetStr, "%d", &parsedOffset); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	// Use paginated API if offset is specified
	if r.URL.Query().Has("offset") {
		page := s.syncManager.GetHistoryPaginated(limit, offset)
		s.writeJSON(w, http.StatusOK, page)
		return
	}

	// Legacy behavior: return simple list without pagination metadata
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
	IsRegistered bool      `json:"is_registered"`
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

	// Parse filter parameters
	editionFilter := r.URL.Query().Get("edition")
	syncingFilter := r.URL.Query().Get("syncing")
	lastSeenAfterStr := r.URL.Query().Get("last_seen_after")

	// Parse last_seen_after timestamp
	var lastSeenAfter time.Time
	if lastSeenAfterStr != "" {
		parsed, err := time.Parse(time.RFC3339, lastSeenAfterStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid last_seen_after format (use RFC3339): %v", err), http.StatusBadRequest)
			return
		}
		lastSeenAfter = parsed
	}

	devices := s.registry.List()
	deviceInfos := make([]DeviceInfo, 0, len(devices))

	s.mu.RLock()
	defer s.mu.RUnlock()

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
			IsRegistered: s.registeredDevices[dev.Info.ID],
		}

		// Apply filters (AND logic - all must match)
		if editionFilter != "" && info.Edition != editionFilter {
			continue
		}

		if syncingFilter != "" {
			syncingWanted := syncingFilter == "true"
			if info.IsSyncing != syncingWanted {
				continue
			}
		}

		if !lastSeenAfter.IsZero() && info.LastSeen.Before(lastSeenAfter) {
			continue
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

// handleWebSocket handles WebSocket upgrade requests
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to upgrade WebSocket connection")
		return
	}

	client := ws.NewClient(s.wsHub, conn)
	s.wsHub.Register(client)
	client.Start()

	log.Info().
		Str("remote_addr", conn.RemoteAddr().String()).
		Msg("WebSocket client connected")
}

// Device registration request
type DeviceRegisterRequest struct {
	DeviceID string `json:"device_id"`
}

// handleDeviceRegister handles device registration requests
func (s *Server) handleDeviceRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DeviceRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.DeviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	// Verify device exists in registry
	dev, exists := s.registry.Get(req.DeviceID)
	if !exists {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	// Mark device as registered (in-memory)
	s.mu.Lock()
	s.registeredDevices[req.DeviceID] = true
	s.mu.Unlock()

	// Create device config entry if it doesn't exist
	if s.config.Devices == nil {
		s.config.Devices = make(map[string]*config.DeviceConfig)
	}

	if _, exists := s.config.Devices[req.DeviceID]; !exists {
		s.config.Devices[req.DeviceID] = &config.DeviceConfig{
			DeviceID:     req.DeviceID,
			FriendlyName: dev.Info.Name,
			LastSeen:     dev.LastSeen,
			Lists:        []config.SharedListConfig{},
		}
	}

	// Save config to disk
	if err := config.Save(s.config, s.configPath); err != nil {
		log.Error().Err(err).Msg("Failed to save config after device registration")
		// Don't fail the request - registration is still in memory
	}

	log.Info().
		Str("device_id", req.DeviceID).
		Str("device_name", dev.Info.Name).
		Msg("Device registered")

	// Broadcast device registered event
	s.wsHub.Broadcast(ws.EventDeviceRegistered, map[string]interface{}{
		"device_id":   req.DeviceID,
		"device_name": dev.Info.Name,
	})

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "registered",
		"message": "Device registered successfully",
	})
}

// handleDeviceUnregister handles device unregistration requests
func (s *Server) handleDeviceUnregister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DeviceRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.DeviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	// Get device info for logging
	dev, exists := s.registry.Get(req.DeviceID)
	deviceName := req.DeviceID
	if exists {
		deviceName = dev.Info.Name
	}

	// Mark device as unregistered (in-memory)
	s.mu.Lock()
	delete(s.registeredDevices, req.DeviceID)
	s.mu.Unlock()

	// Remove from config
	if s.config.Devices != nil {
		delete(s.config.Devices, req.DeviceID)

		// Save config to disk
		if err := config.Save(s.config, s.configPath); err != nil {
			log.Error().Err(err).Msg("Failed to save config after device unregistration")
			// Don't fail the request - unregistration is still in memory
		}
	}

	log.Info().
		Str("device_id", req.DeviceID).
		Str("device_name", deviceName).
		Msg("Device unregistered")

	// Broadcast device unregistered event
	s.wsHub.Broadcast(ws.EventDeviceUnregistered, map[string]interface{}{
		"device_id":   req.DeviceID,
		"device_name": deviceName,
	})

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "unregistered",
		"message": "Device unregistered successfully",
	})
}

// GetHub returns the WebSocket hub for broadcasting events
func (s *Server) GetHub() *ws.Hub {
	return s.wsHub
}

// DeviceConfigResponse provides device configuration including assigned lists
type DeviceConfigResponse struct {
	DeviceID     string                    `json:"device_id"`
	FriendlyName string                    `json:"friendly_name"`
	LastSeen     time.Time                 `json:"last_seen"`
	Lists        []config.SharedListConfig `json:"lists"`
	Settings     *csync.SharedListSettings `json:"default_settings,omitempty"`
}

// handleDeviceConfig returns device configuration (GET /api/devices/config/{deviceId})
func (s *Server) handleDeviceConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	deviceID := parsePathParam(r.URL.Path, "/api/devices/config/")
	if deviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	// Get device config
	deviceConfig, exists := s.config.Devices[deviceID]
	if !exists {
		http.Error(w, "Device configuration not found", http.StatusNotFound)
		return
	}

	response := DeviceConfigResponse{
		DeviceID:     deviceConfig.DeviceID,
		FriendlyName: deviceConfig.FriendlyName,
		LastSeen:     deviceConfig.LastSeen,
		Lists:        deviceConfig.Lists,
		Settings:     deviceConfig.DefaultSettings,
	}

	s.writeJSON(w, http.StatusOK, response)
}

// AddListRequest is the request body for adding a list to a device
type AddListRequest struct {
	ListID   string                    `json:"list_id"`
	ListName string                    `json:"list_name"`
	Enabled  bool                      `json:"enabled"`
	Settings *csync.SharedListSettings `json:"settings,omitempty"`
}

// handleDeviceLists handles adding/removing/updating lists from devices
// POST /api/devices/lists/{deviceId} - Add list to device
// PUT /api/devices/lists/{deviceId}/{listId} - Update list settings
// DELETE /api/devices/lists/{deviceId}/{listId} - Remove list from device
func (s *Server) handleDeviceLists(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleDeviceListAdd(w, r)
	case http.MethodPut:
		s.handleDeviceListUpdate(w, r)
	case http.MethodDelete:
		s.handleDeviceListRemove(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDeviceListAdd adds a smart list to a device's sync configuration
func (s *Server) handleDeviceListAdd(w http.ResponseWriter, r *http.Request) {
	deviceID := parsePathParam(r.URL.Path, "/api/devices/lists/")
	if deviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	var req AddListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ListID == "" {
		http.Error(w, "list_id is required", http.StatusBadRequest)
		return
	}

	// Verify list exists in library (search recursively through folders)
	list, err := s.backend.FindListByID(req.ListID)
	if err != nil {
		log.Error().Err(err).Str("list_id", req.ListID).Msg("Error looking up smart list")
		http.Error(w, "Error looking up smart list", http.StatusInternalServerError)
		return
	}
	if list == nil || !strings.Contains(list.Type, "SmartList") {
		log.Warn().
			Str("requested_list_id", req.ListID).
			Bool("found", list != nil).
			Str("type", func() string {
				if list != nil {
					return list.Type
				}
				return "n/a"
			}()).
			Msg("Smart list not found in library")
		http.Error(w, "Smart list not found in library", http.StatusNotFound)
		return
	}

	log.Debug().
		Str("list_id", list.ID).
		Str("list_name", list.Name).
		Msg("Found matching smart list")

	// Use the list name from the library if not provided
	if req.ListName == "" {
		req.ListName = list.Name
	}

	// Check if device is registered
	s.mu.RLock()
	isRegistered := s.registeredDevices[deviceID]
	s.mu.RUnlock()

	if !isRegistered {
		http.Error(w, "Device not registered. Please register the device first.", http.StatusNotFound)
		return
	}

	// Get or create device config
	if s.config.Devices == nil {
		s.config.Devices = make(map[string]*config.DeviceConfig)
	}

	deviceConfig, exists := s.config.Devices[deviceID]
	if !exists {
		// Device is registered but config entry missing - create it
		dev, devExists := s.registry.Get(deviceID)
		if !devExists {
			http.Error(w, "Device not found in registry", http.StatusNotFound)
			return
		}

		deviceConfig = &config.DeviceConfig{
			DeviceID:     deviceID,
			FriendlyName: dev.Info.Name,
			LastSeen:     dev.LastSeen,
			Lists:        []config.SharedListConfig{},
		}
		s.config.Devices[deviceID] = deviceConfig

		log.Info().
			Str("device_id", deviceID).
			Msg("Created missing device config entry during list assignment")
	}

	// Check if list is already added
	for _, listConfig := range deviceConfig.Lists {
		if listConfig.ListID == req.ListID {
			http.Error(w, "List already assigned to device", http.StatusConflict)
			return
		}
	}

	// Add list to device config
	newList := config.SharedListConfig{
		ListID:   req.ListID,
		ListName: req.ListName,
		Enabled:  req.Enabled,
		Settings: req.Settings,
	}

	deviceConfig.Lists = append(deviceConfig.Lists, newList)

	// Save config to disk
	if err := config.Save(s.config, s.configPath); err != nil {
		log.Error().Err(err).Msg("Failed to save config after adding list to device")
		// Don't fail the request - list is still in memory
	}

	log.Info().
		Str("device_id", deviceID).
		Str("list_id", req.ListID).
		Str("list_name", req.ListName).
		Msg("Smart list added to device")

	// Broadcast list added event
	s.wsHub.Broadcast(ws.EventDeviceUpdated, map[string]interface{}{
		"device_id": deviceID,
		"list_id":   req.ListID,
		"list_name": req.ListName,
		"action":    "list_added",
	})

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "List added to device successfully",
	})
}

// handleDeviceListRemove removes a smart list from a device's sync configuration
func (s *Server) handleDeviceListRemove(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/devices/lists/{deviceId}/{listId}
	path := strings.TrimPrefix(r.URL.Path, "/api/devices/lists/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, "device_id and list_id are required", http.StatusBadRequest)
		return
	}

	deviceID := parts[0]
	listID := parts[1]

	// Get device config
	deviceConfig, exists := s.config.Devices[deviceID]
	if !exists {
		http.Error(w, "Device configuration not found", http.StatusNotFound)
		return
	}

	// Find and remove the list
	listIndex := -1
	var removedListName string
	for i, listConfig := range deviceConfig.Lists {
		if listConfig.ListID == listID {
			listIndex = i
			removedListName = listConfig.ListName
			break
		}
	}

	if listIndex == -1 {
		http.Error(w, "List not found in device configuration", http.StatusNotFound)
		return
	}

	// Remove list from slice
	deviceConfig.Lists = append(deviceConfig.Lists[:listIndex], deviceConfig.Lists[listIndex+1:]...)

	// Save config to disk
	if err := config.Save(s.config, s.configPath); err != nil {
		log.Error().Err(err).Msg("Failed to save config after removing list from device")
		// Don't fail the request - list is still removed in memory
	}

	log.Info().
		Str("device_id", deviceID).
		Str("list_id", listID).
		Str("list_name", removedListName).
		Msg("Smart list removed from device")

	// Broadcast list removed event
	s.wsHub.Broadcast(ws.EventDeviceUpdated, map[string]interface{}{
		"device_id": deviceID,
		"list_id":   listID,
		"list_name": removedListName,
		"action":    "list_removed",
	})

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "List removed from device successfully",
	})
}

// UpdateListRequest is the request body for updating list settings
type UpdateListRequest struct {
	Enabled  *bool                     `json:"enabled,omitempty"`
	Settings *csync.SharedListSettings `json:"settings,omitempty"`
}

// handleDeviceListUpdate updates settings for a smart list assigned to a device
func (s *Server) handleDeviceListUpdate(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/devices/lists/{deviceId}/{listId}
	path := strings.TrimPrefix(r.URL.Path, "/api/devices/lists/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, "device_id and list_id are required", http.StatusBadRequest)
		return
	}

	deviceID := parts[0]
	listID := parts[1]

	var req UpdateListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get device config
	deviceConfig, exists := s.config.Devices[deviceID]
	if !exists {
		http.Error(w, "Device configuration not found", http.StatusNotFound)
		return
	}

	// Find the list
	listIndex := -1
	for i, listConfig := range deviceConfig.Lists {
		if listConfig.ListID == listID {
			listIndex = i
			break
		}
	}

	if listIndex == -1 {
		http.Error(w, "List not found in device configuration", http.StatusNotFound)
		return
	}

	// Update fields
	if req.Enabled != nil {
		deviceConfig.Lists[listIndex].Enabled = *req.Enabled
	}
	if req.Settings != nil {
		deviceConfig.Lists[listIndex].Settings = req.Settings
	}

	// Save config to disk
	if err := config.Save(s.config, s.configPath); err != nil {
		log.Error().Err(err).Msg("Failed to save config after updating list settings")
		// Don't fail the request - settings are still updated in memory
	}

	log.Info().
		Str("device_id", deviceID).
		Str("list_id", listID).
		Str("list_name", deviceConfig.Lists[listIndex].ListName).
		Msg("Smart list settings updated for device")

	// Broadcast list updated event
	s.wsHub.Broadcast(ws.EventDeviceUpdated, map[string]interface{}{
		"device_id": deviceID,
		"list_id":   listID,
		"list_name": deviceConfig.Lists[listIndex].ListName,
		"action":    "list_updated",
	})

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "List settings updated successfully",
	})
}

// spaHandler serves static files and falls back to index.html for client-side routing
func (s *Server) spaHandler(webRoot fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(webRoot))

	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Normalize path for file checking
		filePath := strings.TrimPrefix(path, "/")
		if filePath == "" {
			filePath = "index.html"
		}

		// Check if file exists
		f, err := webRoot.Open(filePath)
		if err == nil {
			// File exists - serve it normally
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// File doesn't exist - check if it's an API route or WebSocket
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/ws") || strings.HasPrefix(path, "/metrics") {
			// Let it 404 (API route doesn't exist)
			http.NotFound(w, r)
			return
		}

		// Not a file and not an API route - serve index.html for SPA routing
		// Read and serve index.html directly
		indexFile, err := webRoot.Open("index.html")
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer indexFile.Close()

		stat, err := indexFile.Stat()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "index.html", stat.ModTime(), indexFile.(io.ReadSeeker))
	}
}
