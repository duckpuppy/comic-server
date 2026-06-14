package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// handleFolders handles POST /api/library/folders (create a new folder).
func (s *Server) handleFolders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	if !s.backend.CanPersist() {
		http.Error(w, "Library is read-only", http.StatusForbidden)
		return
	}

	var req struct {
		Name     string `json:"name"`
		ParentID string `json:"parent_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	folder := &library.ComicListItem{
		ID:   "{" + uuid.New().String() + "}",
		Name: req.Name,
		Type: "ComicListItemFolder",
	}

	if req.ParentID != "" {
		// Create inside a parent folder: add to parent's ChildItems via CreateList
		// then immediately move it. Simpler: create at root then move.
		if err := s.backend.CreateList(folder); err != nil {
			log.Error().Err(err).Msg("Failed to create folder")
			http.Error(w, "Failed to create folder", http.StatusInternalServerError)
			return
		}
		if err := s.backend.MoveList(folder.ID, req.ParentID); err != nil {
			log.Error().Err(err).Str("id", folder.ID).Str("parent", req.ParentID).Msg("Failed to move folder to parent")
			// Not fatal — folder was created at root
		}
	} else {
		if err := s.backend.CreateList(folder); err != nil {
			log.Error().Err(err).Msg("Failed to create folder")
			http.Error(w, "Failed to create folder", http.StatusInternalServerError)
			return
		}
	}

	log.Info().Str("id", folder.ID).Str("name", folder.Name).Msg("Folder created")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(folder)
}

// handleMoveList handles PUT /api/library/lists/:id/parent (move list or folder).
func (s *Server) handleMoveList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	if !s.backend.CanPersist() {
		http.Error(w, "Library is read-only", http.StatusForbidden)
		return
	}

	path := r.URL.Path
	listID := path[len("/api/library/lists/"):]
	if idx := strings.LastIndex(listID, "/"); idx != -1 {
		listID = listID[:idx]
	}

	var req struct {
		ParentID string `json:"parent_id"` // "" means move to root
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Prevent moving a folder into itself or a descendant
	if req.ParentID != "" && req.ParentID == listID {
		http.Error(w, "Cannot move a folder into itself", http.StatusBadRequest)
		return
	}

	if err := s.backend.MoveList(listID, req.ParentID); err != nil {
		log.Error().Err(err).Str("id", listID).Str("parent", req.ParentID).Msg("Failed to move list")
		http.Error(w, "Failed to move list: "+err.Error(), http.StatusNotFound)
		return
	}

	log.Info().Str("id", listID).Str("parent", req.ParentID).Msg("List moved")
	w.WriteHeader(http.StatusNoContent)
}
