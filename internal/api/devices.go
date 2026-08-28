package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/duckpuppy/comic-server/internal/device"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/syncstate"
)

// DeviceDetail represents full device information
type DeviceDetail struct {
	// Device registry info
	ID           string `json:"id"`
	IP           string `json:"ip"`
	Name         string `json:"name"`
	Model        string `json:"model"`
	Manufacturer string `json:"manufacturer"`
	Edition      string `json:"edition"`
	LastSeen     string `json:"last_seen"`

	// Config info
	FriendlyName string             `json:"friendly_name"`
	Lists        []AssignedListInfo `json:"lists"`

	// Current status
	IsSyncing bool `json:"is_syncing"`
}

// AssignedListInfo represents a smart list assigned to a device
type AssignedListInfo struct {
	ListID    string `json:"list_id"`
	ListName  string `json:"list_name"`
	Enabled   bool   `json:"enabled"`
	BookCount int    `json:"book_count"`
}

// handleGetDeviceDetail returns detailed info for a specific device
func (s *Server) handleGetDeviceDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract device ID from URL path
	// URL: /api/devices/:deviceId
	deviceID := r.URL.Path[len("/api/devices/"):]
	if idx := strings.Index(deviceID, "/"); idx != -1 {
		deviceID = deviceID[:idx]
	}

	// Validate device ID is not empty
	if deviceID == "" {
		http.Error(w, "Device ID is required", http.StatusBadRequest)
		return
	}

	// Look up device in registry (online devices)
	var detail DeviceDetail
	device, exists := s.registry.Get(deviceID)

	// Look up device in config.db (may include offline devices)
	deviceConfig, err := s.configDB.GetDevice(deviceID)
	if err != nil {
		log.Error().Err(err).Str("device_id", deviceID).Msg("Failed to look up device config")
		http.Error(w, "Failed to look up device config", http.StatusInternalServerError)
		return
	}
	inConfig := deviceConfig != nil

	// Device must be in registry OR config
	if !exists && !inConfig {
		http.Error(w, "Device not found in registry or configuration", http.StatusNotFound)
		return
	}

	// Populate basic info from registry (if available)
	if exists {
		detail.ID = device.Info.ID
		detail.IP = device.IPAddress
		detail.Name = device.Info.Name
		detail.Model = device.Info.Model
		detail.Manufacturer = device.Info.Manufacturer
		detail.Edition = string(device.Info.Edition)
		detail.LastSeen = device.LastSeen.Format("2006-01-02T15:04:05Z07:00")
	} else {
		// Offline device - only have config info
		detail.ID = deviceID
		detail.Name = deviceConfig.FriendlyName
	}

	// Populate config info
	if inConfig {
		detail.FriendlyName = deviceConfig.FriendlyName

		// Build assigned lists with book counts
		detail.Lists = make([]AssignedListInfo, 0, len(deviceConfig.Lists))
		for _, listConfig := range deviceConfig.Lists {
			// Get book count from cache
			count, found := s.listCache.GetCount(listConfig.ListID)
			if !found && s.backend != nil {
				// Not in cache - evaluate and cache
				targetList, err := s.backend.FindListByID(listConfig.ListID)
				if err != nil {
					log.Error().
						Err(err).
						Str("list_id", listConfig.ListID).
						Msg("Failed to find smart list")
				} else if targetList != nil {
					matches, err := s.backend.MatchBooks(targetList)
					if err == nil {
						count = len(matches)
						s.listCache.SetCount(listConfig.ListID, count)
					} else {
						log.Error().
							Err(err).
							Str("list_id", listConfig.ListID).
							Str("list_name", listConfig.ListName).
							Str("device_id", deviceID).
							Msg("Failed to evaluate smart list for book count")
					}
				}
			}

			detail.Lists = append(detail.Lists, AssignedListInfo{
				ListID:    listConfig.ListID,
				ListName:  listConfig.ListName,
				Enabled:   listConfig.Enabled,
				BookCount: count,
			})
		}
	}

	// Check if currently syncing
	detail.IsSyncing = s.syncManager.IsDeviceSyncing(deviceID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}

// handleDevicesRouter routes /api/devices/* requests to appropriate handlers
func (s *Server) handleDevicesRouter(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Exact match for base endpoint: /api/devices or /api/devices/
	if path == "/api/devices" || path == "/api/devices/" {
		s.handleDevices(w, r)
		return
	}

	// Handle exact matches for specific endpoints
	if path == "/api/devices/register" {
		s.handleDeviceRegister(w, r)
		return
	}

	if path == "/api/devices/unregister" {
		s.handleDeviceUnregister(w, r)
		return
	}

	// Handle sub-routes with trailing slash
	if strings.HasPrefix(path, "/api/devices/config/") {
		s.handleDeviceConfig(w, r)
		return
	}

	if strings.HasPrefix(path, "/api/devices/lists/") {
		s.handleDeviceLists(w, r)
		return
	}

	// If no sub-path (no slash after device ID), handle device detail/update
	// URL: /api/devices/:deviceId
	remainder := path[len("/api/devices/"):]
	if !strings.Contains(remainder, "/") {
		if r.Method == http.MethodGet {
			s.handleGetDeviceDetail(w, r)
		} else if r.Method == http.MethodPatch {
			s.handleUpdateDevice(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	// If we have a sub-path but it's not recognized, check for sync-history
	// URL: /api/devices/:deviceId/sync-history
	if strings.HasSuffix(path, "/sync-history") {
		s.handleGetDeviceSyncHistory(w, r)
		return
	}

	// URL: /api/devices/:deviceId/sync - manually trigger a sync
	// (comic-server-yfp). Checked before the generic "/sync-history"
	// substring could ever be mistaken for it: HasSuffix on "/sync" only
	// matches a path actually ending in "/sync", never
	// ".../sync-history".
	if strings.HasSuffix(path, "/sync") {
		s.handleTriggerSync(w, r)
		return
	}

	// URL: /api/devices/:deviceId/lists/:listId/preview - the only
	// /api/devices/:deviceId/lists... route left on this shape.
	// Add/remove/update-settings for a device's lists all live on
	// /api/devices/lists/:deviceId[/:listId] instead (handled above via
	// handleDeviceLists) - comic-server-3ek consolidated what used to be
	// two independent route trees for the same data onto that one.
	if strings.HasSuffix(path, "/preview") && strings.Contains(remainder, "/lists/") {
		s.handlePreviewListBooks(w, r)
		return
	}

	http.NotFound(w, r)
}

// handleGetDeviceSyncHistory returns paginated sync history for a device
func (s *Server) handleGetDeviceSyncHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract device ID from URL
	path := r.URL.Path
	deviceID := path[len("/api/devices/"):]
	if idx := strings.Index(deviceID, "/"); idx != -1 {
		deviceID = deviceID[:idx]
	}

	// Parse query parameters
	query := r.URL.Query()
	limit := 10 // Default
	if l := query.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
			if limit > 50 {
				http.Error(w, "Limit cannot exceed 50", http.StatusBadRequest)
				return
			}
		}
	}

	offset := 0
	if o := query.Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	// Get filtered history
	history, metadata := s.syncManager.GetHistoryForDevice(deviceID, limit, offset)

	response := map[string]interface{}{
		"history":     history,
		"total":       metadata.Total,
		"limit":       metadata.Limit,
		"offset":      metadata.Offset,
		"has_more":    metadata.HasMore,
		"next_offset": metadata.NextOffset,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleTriggerSync manually starts a sync for a currently-connected
// device (comic-server-yfp), instead of waiting for auto-sync or the
// device's own sync button. The sync itself runs in the background - this
// only reports whether it was started, via s.triggerSync (wired by
// SetSyncTrigger; see its doc comment for why the API package can't call
// into it directly).
func (s *Server) handleTriggerSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// URL: /api/devices/:deviceId/sync
	path := r.URL.Path
	deviceID := strings.TrimSuffix(path[len("/api/devices/"):], "/sync")
	if deviceID == "" {
		http.Error(w, "Device ID is required", http.StatusBadRequest)
		return
	}

	if s.triggerSync == nil {
		http.Error(w, "Manual sync trigger not available", http.StatusServiceUnavailable)
		return
	}

	if err := s.triggerSync(deviceID); err != nil {
		if errors.Is(err, device.ErrNotConnected) {
			http.Error(w, "Device is not currently connected", http.StatusNotFound)
			return
		}
		var alreadySyncing *syncstate.DeviceAlreadySyncingError
		if errors.As(err, &alreadySyncing) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		log.Error().Err(err).Str("device_id", deviceID).Msg("Failed to trigger manual sync")
		http.Error(w, "Failed to trigger sync", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status":    "sync started",
		"device_id": deviceID,
	})
}
