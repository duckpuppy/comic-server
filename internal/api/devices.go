package api

import (
	"encoding/json"
	"net/http"
	"strconv"
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

	// Handle list management endpoints
	// URL: /api/devices/:deviceId/lists
	// URL: /api/devices/:deviceId/lists/:listId
	// URL: /api/devices/:deviceId/lists/:listId/preview
	if strings.Contains(remainder, "/lists") {
		// Check if this is a preview request
		if strings.HasSuffix(path, "/preview") {
			s.handlePreviewListBooks(w, r)
			return
		}

		// Extract device ID and check if list ID is present
		parts := strings.Split(remainder, "/")
		// parts[0] = deviceID, parts[1] = "lists", parts[2] = listID (if present)

		if len(parts) == 2 {
			// /api/devices/:deviceId/lists - add list to device
			if r.Method == http.MethodPost {
				s.handleAddListToDevice(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}

		if len(parts) >= 3 {
			// /api/devices/:deviceId/lists/:listId
			if r.Method == http.MethodDelete {
				s.handleRemoveListFromDevice(w, r)
			} else if r.Method == http.MethodPatch {
				s.handleUpdateListSettings(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
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
