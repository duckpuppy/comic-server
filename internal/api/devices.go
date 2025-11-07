package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/duckpuppy/comic-server/internal/log"
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

	// Look up device in config (may include offline devices)
	// Note: s.config is not protected by mutex - config access is read-only after server start
	deviceConfig, inConfig := s.config.Devices[deviceID]

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
			if !found && s.library != nil {
				// Not in cache - evaluate and cache
				for i := range s.library.ComicLists {
					if s.library.ComicLists[i].ID == listConfig.ListID {
						matches, err := s.library.MatchBooks(&s.library.ComicLists[i])
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
						break
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

	// If no sub-path (no slash after device ID), treat as device detail
	// URL: /api/devices/:deviceId
	remainder := path[len("/api/devices/"):]
	if !strings.Contains(remainder, "/") {
		s.handleGetDeviceDetail(w, r)
		return
	}

	// If we have a sub-path but it's not recognized, check for sync-history
	// URL: /api/devices/:deviceId/sync-history (to be implemented in Task 3)
	if strings.HasSuffix(path, "/sync-history") {
		// TODO: Task 3 will implement this
		http.Error(w, "Not implemented yet", http.StatusNotImplemented)
		return
	}

	http.NotFound(w, r)
}
