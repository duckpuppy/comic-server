package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/log"
	ws "github.com/duckpuppy/comic-server/internal/websocket"
)

// deviceIDRe is what a device ID may look like in a re-pair request. IDs
// appear in URLs, so no slash or space.
var deviceIDRe = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// RepairRequest is the body of POST /api/devices/:id/repair.
type RepairRequest struct {
	NewDeviceID string `json:"new_device_id"`
}

// RepairResponse tells the page what moved.
type RepairResponse struct {
	OldDeviceID string `json:"old_device_id"`
	NewDeviceID string `json:"new_device_id"`
	Name        string `json:"name"`
	ListCount   int    `json:"list_count"`
}

// handleRepairDevice handles POST /api/devices/:deviceId/repair (tracker
// #53): move a registered device's lists, settings and history to a new
// device ID, for an app that made a new Sync ID. See configdb.RepairDevice
// for what moves and when the new ID may already be registered. Re-pairing
// back from the new ID to the old one undoes it.
func (s *Server) handleRepairDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// URL: /api/devices/:deviceId/repair
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/devices/")
	oldID := strings.TrimSuffix(trimmed, "/repair")

	var req RepairRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	newID := strings.TrimSpace(req.NewDeviceID)
	if !deviceIDRe.MatchString(newID) {
		http.Error(w, "new_device_id is required: letters, digits, . _ : - only, up to 128 characters", http.StatusBadRequest)
		return
	}

	// A running sync holds the old ID in memory and on the device; do not
	// move it from under that sync.
	if s.syncManager.IsDeviceSyncing(oldID) || s.syncManager.IsDeviceSyncing(newID) {
		http.Error(w, "A sync is running for one of these devices. Wait for it to finish, then re-pair.", http.StatusConflict)
		return
	}

	existing, err := s.configDB.GetDevice(oldID)
	if err != nil {
		log.Error().Err(err).Str("device_id", oldID).Msg("Failed to look up device for re-pair")
		http.Error(w, "Failed to look up the device", http.StatusInternalServerError)
		return
	}

	if err := s.configDB.RepairDevice(oldID, newID); err != nil {
		switch {
		case errors.Is(err, configdb.ErrRepairNotRegistered):
			http.Error(w, "Device not found", http.StatusNotFound)
		case errors.Is(err, configdb.ErrRepairSameDevice):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, configdb.ErrRepairTargetInUse):
			http.Error(w, "The new device ID already has lists or sync history. Unregister it first if you want to replace it.", http.StatusConflict)
		default:
			log.Error().Err(err).Str("old_device_id", oldID).Str("new_device_id", newID).Msg("Re-pair failed")
			http.Error(w, "Re-pair failed; nothing was changed", http.StatusInternalServerError)
		}
		return
	}

	// config.db is the truth now; bring the in-memory state along.
	s.mu.Lock()
	delete(s.registeredDevices, oldID)
	s.registeredDevices[newID] = true
	s.mu.Unlock()
	s.registry.Remove(oldID)
	s.syncManager.RenameDevice(oldID, newID)

	name, lists := oldID, 0
	if existing != nil {
		lists = len(existing.Lists)
		if existing.FriendlyName != "" {
			name = existing.FriendlyName
		}
	}
	log.Info().
		Str("old_device_id", oldID).
		Str("new_device_id", newID).
		Str("device_name", name).
		Int("lists", lists).
		Msg("Device re-paired")

	s.wsHub.Broadcast(ws.EventDeviceUnregistered, map[string]interface{}{"device_id": oldID, "device_name": name})
	s.wsHub.Broadcast(ws.EventDeviceRegistered, map[string]interface{}{"device_id": newID, "device_name": name})

	s.writeJSON(w, http.StatusOK, RepairResponse{OldDeviceID: oldID, NewDeviceID: newID, Name: name, ListCount: lists})
}
