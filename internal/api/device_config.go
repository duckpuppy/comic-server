package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/sync"
)

// UpdateDeviceRequest represents a request to update device configuration
type UpdateDeviceRequest struct {
	FriendlyName    *string                  `json:"friendly_name,omitempty"`
	DefaultSettings *sync.SharedListSettings `json:"default_settings,omitempty"`
}

// AddDeviceListRequest represents a request to add a list to a device
type AddDeviceListRequest struct {
	ListID   string                   `json:"list_id"`
	ListName string                   `json:"list_name"`
	Settings *sync.SharedListSettings `json:"settings,omitempty"`
}

// UpdateListSettingsRequest represents a request to update list settings
type UpdateListSettingsRequest struct {
	Enabled  *bool                    `json:"enabled,omitempty"`
	Settings *sync.SharedListSettings `json:"settings,omitempty"`
}

// PreviewRequest represents a request to preview books for given settings
type PreviewRequest struct {
	Settings *sync.SharedListSettings `json:"settings,omitempty"`
}

// PreviewResponse represents the response for a preview request
type PreviewResponse struct {
	BookCount int      `json:"book_count"`
	TotalSize int64    `json:"total_size_bytes"`
	Sample    []string `json:"sample_titles"` // First 10 book titles
}

// handleUpdateDevice handles PATCH /api/devices/:deviceId
func (s *Server) handleUpdateDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	deviceID := extractDeviceID(r.URL.Path)
	if deviceID == "" {
		http.Error(w, "Device ID is required", http.StatusBadRequest)
		return
	}

	// Parse request body
	var req UpdateDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Get device config (create if doesn't exist)
	s.configMu.Lock()
	defer s.configMu.Unlock()

	deviceConfig, exists := s.config.Devices[deviceID]
	if !exists {
		// Create new device config
		deviceConfig = &config.DeviceConfig{
			DeviceID: deviceID,
			Lists:    []config.SharedListConfig{},
		}
		s.config.Devices[deviceID] = deviceConfig
	}

	// Update fields if provided
	if req.FriendlyName != nil {
		deviceConfig.FriendlyName = *req.FriendlyName
	}
	if req.DefaultSettings != nil {
		deviceConfig.DefaultSettings = req.DefaultSettings
	}

	// Save config
	if err := config.Save(s.config, s.configPath); err != nil {
		log.Error().Err(err).Msg("Failed to save config after device update")
		http.Error(w, "Failed to save configuration", http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("device_id", deviceID).
		Str("friendly_name", deviceConfig.FriendlyName).
		Msg("Device configuration updated")

	// Return success
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Device configuration updated successfully",
	})
}

// handleAddListToDevice handles POST /api/devices/:deviceId/lists
func (s *Server) handleAddListToDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	deviceID := extractDeviceID(r.URL.Path)
	if deviceID == "" {
		http.Error(w, "Device ID is required", http.StatusBadRequest)
		return
	}

	// Parse request body
	var req AddDeviceListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.ListID == "" || req.ListName == "" {
		http.Error(w, "list_id and list_name are required", http.StatusBadRequest)
		return
	}

	// Verify list exists in library
	list, err := s.backend.FindListByID(req.ListID)
	if err != nil || list == nil {
		http.Error(w, "List not found in library", http.StatusNotFound)
		return
	}

	// Get device config (create if doesn't exist)
	s.configMu.Lock()
	defer s.configMu.Unlock()

	deviceConfig, exists := s.config.Devices[deviceID]
	if !exists {
		deviceConfig = &config.DeviceConfig{
			DeviceID: deviceID,
			Lists:    []config.SharedListConfig{},
		}
		s.config.Devices[deviceID] = deviceConfig
	}

	// Add list to device
	if err := deviceConfig.AddList(req.ListID, req.ListName, req.Settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Save config
	if err := config.Save(s.config, s.configPath); err != nil {
		log.Error().Err(err).Msg("Failed to save config after adding list")
		http.Error(w, "Failed to save configuration", http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("device_id", deviceID).
		Str("list_id", req.ListID).
		Str("list_name", req.ListName).
		Msg("List added to device")

	// Return success
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "List added to device successfully",
	})
}

// handleRemoveListFromDevice handles DELETE /api/devices/:deviceId/lists/:listId
func (s *Server) handleRemoveListFromDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	deviceID, listID := extractDeviceAndListID(r.URL.Path)
	if deviceID == "" || listID == "" {
		http.Error(w, "Device ID and List ID are required", http.StatusBadRequest)
		return
	}

	// Get device config
	s.configMu.Lock()
	defer s.configMu.Unlock()

	deviceConfig, exists := s.config.Devices[deviceID]
	if !exists {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	// Remove list from device
	if err := deviceConfig.RemoveList(listID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Save config
	if err := config.Save(s.config, s.configPath); err != nil {
		log.Error().Err(err).Msg("Failed to save config after removing list")
		http.Error(w, "Failed to save configuration", http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("device_id", deviceID).
		Str("list_id", listID).
		Msg("List removed from device")

	// Return success
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "List removed from device successfully",
	})
}

// handleUpdateListSettings handles PATCH /api/devices/:deviceId/lists/:listId
func (s *Server) handleUpdateListSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	deviceID, listID := extractDeviceAndListID(r.URL.Path)
	if deviceID == "" || listID == "" {
		http.Error(w, "Device ID and List ID are required", http.StatusBadRequest)
		return
	}

	// Parse request body
	var req UpdateListSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Get device config
	s.configMu.Lock()
	defer s.configMu.Unlock()

	deviceConfig, exists := s.config.Devices[deviceID]
	if !exists {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	// Get list config
	listConfig := deviceConfig.GetList(listID)
	if listConfig == nil {
		http.Error(w, "List not found on device", http.StatusNotFound)
		return
	}

	// Update enabled state if provided
	if req.Enabled != nil {
		if *req.Enabled {
			if err := deviceConfig.EnableList(listID); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		} else {
			if err := deviceConfig.DisableList(listID); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
	}

	// Update settings if provided
	if req.Settings != nil {
		if err := deviceConfig.UpdateListSettings(listID, req.Settings); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Save config
	if err := config.Save(s.config, s.configPath); err != nil {
		log.Error().Err(err).Msg("Failed to save config after updating list settings")
		http.Error(w, "Failed to save configuration", http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("device_id", deviceID).
		Str("list_id", listID).
		Msg("List settings updated")

	// Return success
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "List settings updated successfully",
	})
}

// handlePreviewListBooks handles POST /api/devices/:deviceId/lists/:listId/preview
func (s *Server) handlePreviewListBooks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	deviceID, listID := extractDeviceAndListID(r.URL.Path)
	if deviceID == "" || listID == "" {
		http.Error(w, "Device ID and List ID are required", http.StatusBadRequest)
		return
	}

	// Parse request body (settings to preview)
	var req PreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Get the list from library
	list, err := s.backend.FindListByID(listID)
	if err != nil || list == nil {
		http.Error(w, "List not found in library", http.StatusNotFound)
		return
	}

	// Match books from the list
	books, err := s.backend.MatchBooks(list)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to match books: %v", err), http.StatusInternalServerError)
		return
	}

	// Apply settings if provided
	settings := req.Settings
	if settings == nil {
		settings = sync.DefaultSettings()
	}

	filteredBooks, err := sync.ApplySettingsWithResolver(books, settings, s.resolveBookFilePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to apply settings: %v", err), http.StatusInternalServerError)
		return
	}

	// Calculate total size
	// TODO: Implement size calculation by reading file sizes
	// For now, we'll return 0 as this requires file system access
	var totalSize int64

	// Get sample titles (first 10)
	sampleTitles := make([]string, 0, 10)
	for i := 0; i < len(filteredBooks) && i < 10; i++ {
		book := filteredBooks[i]
		title := book.Title
		if title == "" {
			title = fmt.Sprintf("%s #%s", book.Series, book.Number)
		}
		sampleTitles = append(sampleTitles, title)
	}

	// Return preview
	response := PreviewResponse{
		BookCount: len(filteredBooks),
		TotalSize: totalSize,
		Sample:    sampleTitles,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Helper functions

// extractDeviceID extracts device ID from URL path like /api/devices/:deviceId
func extractDeviceID(path string) string {
	// Remove /api/devices/ prefix
	path = strings.TrimPrefix(path, "/api/devices/")

	// Find first / to isolate device ID
	if idx := strings.Index(path, "/"); idx != -1 {
		return path[:idx]
	}
	return path
}

// extractDeviceAndListID extracts device ID and list ID from URL path
// like /api/devices/:deviceId/lists/:listId
func extractDeviceAndListID(path string) (deviceID, listID string) {
	// Remove /api/devices/ prefix
	path = strings.TrimPrefix(path, "/api/devices/")

	// Split by /
	parts := strings.Split(path, "/")
	if len(parts) < 3 {
		return "", ""
	}

	// parts[0] = deviceID
	// parts[1] = "lists"
	// parts[2] = listID (may have /preview suffix)
	deviceID = parts[0]
	listID = parts[2]

	// Remove /preview suffix if present
	listID = strings.TrimSuffix(listID, "/preview")

	return deviceID, listID
}
