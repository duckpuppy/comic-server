package api

import (
	"encoding/json"
	"net/http"
	"strings"
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

	// Look up device in registry
	var detail DeviceDetail
	device, exists := s.registry.Get(deviceID)

	// Also check config
	s.mu.RLock()
	deviceConfig, inConfig := s.config.Devices[deviceID]
	s.mu.RUnlock()

	// Device must be in registry OR config
	if !exists && !inConfig {
		http.Error(w, "Device not found", http.StatusNotFound)
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
