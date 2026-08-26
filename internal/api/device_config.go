package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/sync"
)

// UpdateDeviceRequest represents a request to update device configuration
type UpdateDeviceRequest struct {
	FriendlyName    *string                  `json:"friendly_name,omitempty"`
	DefaultSettings *sync.SharedListSettings `json:"default_settings,omitempty"`
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

// handleUpdateDevice handles PATCH /api/devices/:deviceId - the only
// device-level (as opposed to list-level) write endpoint, so it kept its
// own file and path shape rather than moving to the
// /api/devices/lists/:deviceId tree consolidated onto in comic-server-3ek
// (that tree has no device-level PATCH equivalent).
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
	deviceConfig, err := s.configDB.GetDevice(deviceID)
	if err != nil {
		log.Error().Err(err).Str("device_id", deviceID).Msg("Failed to look up device config")
		http.Error(w, "Failed to look up device config", http.StatusInternalServerError)
		return
	}

	friendlyName := ""
	var lastSeen time.Time
	defaultSettings := (*sync.SharedListSettings)(nil)
	if deviceConfig != nil {
		friendlyName = deviceConfig.FriendlyName
		lastSeen = deviceConfig.LastSeen
		defaultSettings = deviceConfig.DefaultSettings
	}

	// Apply the requested changes on top of whatever's already there
	if req.FriendlyName != nil {
		friendlyName = *req.FriendlyName
	}
	if req.DefaultSettings != nil {
		defaultSettings = req.DefaultSettings
	}

	if err := s.configDB.UpsertDevice(deviceID, friendlyName, lastSeen, defaultSettings); err != nil {
		log.Error().Err(err).Str("device_id", deviceID).Msg("Failed to save device config")
		http.Error(w, "Failed to save configuration", http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("device_id", deviceID).
		Str("friendly_name", friendlyName).
		Msg("Device configuration updated")

	// Return success
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Device configuration updated successfully",
	})
}

// handlePreviewListBooks handles POST /api/devices/:deviceId/lists/:listId/preview
func (s *Server) handlePreviewListBooks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	_, listID := extractDeviceAndListID(r.URL.Path)
	if listID == "" {
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
