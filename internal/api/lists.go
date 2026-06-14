package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

func newUUID() string {
	return uuid.New().String()
}

// ListSummary represents a smart list with cached count
type ListSummary struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	MatcherMode  string `json:"matcher_mode"`
	BookCount    int    `json:"book_count"`
	UnreadCount  int    `json:"unread_count"`
	MatcherCount int    `json:"matcher_count"`
}

// ListTreeNode represents a node in the list tree (folder or smart list)
type ListTreeNode struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Type         string         `json:"type"`
	IsFolder     bool           `json:"is_folder"`
	BookCount    int            `json:"book_count,omitempty"`
	UnreadCount  int            `json:"unread_count,omitempty"`
	MatcherCount int            `json:"matcher_count,omitempty"`
	MatcherMode  string         `json:"matcher_mode,omitempty"`
	Children     []ListTreeNode `json:"children,omitempty"`
}

// countUnread returns the number of unread books in a slice.
func countUnread(books []*library.ComicBook) int {
	n := 0
	for _, b := range books {
		if b.IsUnread() {
			n++
		}
	}
	return n
}

// buildListTree recursively builds a tree structure from ComicListItems
func (s *Server) buildListTree(items []library.ComicListItem) []ListTreeNode {
	nodes := make([]ListTreeNode, 0)

	for i := range items {
		item := &items[i]

		// Check if this is a folder
		isFolder := strings.Contains(item.Type, "Folder")

		node := ListTreeNode{
			ID:       item.ID,
			Name:     item.Name,
			Type:     item.Type,
			IsFolder: isFolder,
		}

		if isFolder {
			// Recursively build children for folders
			node.Children = s.buildListTree(item.ChildItems)
		} else if strings.Contains(item.Type, "SmartList") || strings.Contains(item.Type, "IdListItem") {
			count, unread, found := s.listCache.GetCounts(item.ID)
			if !found {
				matches, err := s.backend.GetBooksForList(item)
				if err != nil {
					count, unread = 0, 0
				} else {
					count = len(matches)
					unread = countUnread(matches)
				}
				s.listCache.SetCounts(item.ID, count, unread)
			}

			node.BookCount = count
			node.UnreadCount = unread
			node.MatcherCount = len(item.Matchers)
			node.MatcherMode = item.MatcherMode
		}

		nodes = append(nodes, node)
	}

	return nodes
}

// handleGetListTree returns the nested tree structure of lists
func (s *Server) handleGetListTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.backend == nil {
		log.Error().Msg("Library backend not loaded")
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	allLists, err := s.backend.GetAllLists()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get lists from backend")
		http.Error(w, "Failed to get lists", http.StatusInternalServerError)
		return
	}
	tree := s.buildListTree(allLists)

	response := map[string]interface{}{
		"tree": tree,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleGetLists returns all smart lists with cached counts
func (s *Server) handleGetLists(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// fall through to existing logic
	case http.MethodPost:
		s.handleCreateList(w, r)
		return
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.backend == nil {
		log.Error().Msg("Library backend not loaded")
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	allLists, err := s.backend.GetAllLists()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get lists from backend")
		http.Error(w, "Failed to get lists", http.StatusInternalServerError)
		return
	}

	lists := make([]ListSummary, 0)

	// Recursively collect smart lists from all folders
	var collectSmartLists func(items []library.ComicListItem)
	collectSmartLists = func(items []library.ComicListItem) {
		for _, list := range items {
			// Debug: Log all list types to help diagnose issue
			log.Debug().
				Str("list_name", list.Name).
				Str("list_type", list.Type).
				Int("matcher_count", len(list.Matchers)).
				Int("child_count", len(list.ChildItems)).
				Msg("Checking list")

			// Include smart lists and id lists (not folders or reading lists)
			isSmartOrId := strings.Contains(list.Type, "SmartList") || strings.Contains(list.Type, "IdListItem")
			if isSmartOrId {
				count, unread, found := s.listCache.GetCounts(list.ID)
				if !found {
					log.Debug().
						Str("list_name", list.Name).
						Int("total_books", s.backend.BookCount()).
						Int("matcher_count", len(list.Matchers)).
						Msg("Evaluating list")

					matches, err := s.backend.GetBooksForList(&list)
					if err != nil {
						log.Warn().Err(err).
							Str("list_id", list.ID).
							Str("list_name", list.Name).
							Msg("Failed to get books for list")
						count, unread = 0, 0
					} else {
						count = len(matches)
						unread = countUnread(matches)
						log.Debug().
							Str("list_name", list.Name).
							Int("matched_books", count).
							Int("unread_books", unread).
							Msg("List evaluation complete")
					}
					s.listCache.SetCounts(list.ID, count, unread)
				}

				lists = append(lists, ListSummary{
					ID:           list.ID,
					Name:         list.Name,
					Type:         list.Type,
					MatcherMode:  list.MatcherMode,
					BookCount:    count,
					UnreadCount:  unread,
					MatcherCount: len(list.Matchers),
				})
			}

			// Recursively process child items (folders)
			if len(list.ChildItems) > 0 {
				collectSmartLists(list.ChildItems)
			}
		}
	}

	// Start recursion from top-level lists
	collectSmartLists(allLists)

	response := map[string]interface{}{
		"lists": lists,
		"total": len(lists),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleListsRouter routes requests to list detail, preview, or devices endpoints
func (s *Server) handleListsRouter(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	suffix := path[len("/api/library/lists/"):]

	// Reserved sub-paths (before generic :listId handler)
	if suffix == "schema" {
		s.handleGetListSchema(w, r)
		return
	}

	// /api/library/lists/:listId  (no further slash)
	if !strings.Contains(suffix, "/") {
		switch r.Method {
		case http.MethodGet:
			s.handleGetListDetail(w, r)
		case http.MethodPut:
			s.handleUpdateList(w, r)
		case http.MethodDelete:
			s.handleDeleteList(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	// /api/library/lists/:listId/preview
	if strings.HasSuffix(path, "/preview") {
		s.handleGetListPreview(w, r)
		return
	}

	// /api/library/lists/:listId/devices
	if strings.HasSuffix(path, "/devices") {
		s.handleGetListDevices(w, r)
		return
	}

	http.NotFound(w, r)
}

// ListDetail represents full details of a smart list
type ListDetail struct {
	ID                   string                `json:"id"`
	Name                 string                `json:"name"`
	Type                 string                `json:"type"`
	MatcherMode          string                `json:"matcher_mode"`
	MatcherModeFormatted string                `json:"matcher_mode_formatted"`
	BookCount            int                   `json:"book_count"`
	UnreadCount          int                   `json:"unread_count"`
	Matchers             []library.MatcherInfo `json:"matchers"`
}

// handleGetListDetail returns details for a specific list
func (s *Server) handleGetListDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	// Extract list ID from URL path
	// URL: /api/library/lists/:listId
	listID := parsePathParam(r.URL.Path, "/api/library/lists/")

	// Find the list (searches recursively through folders)
	targetList, err := s.backend.FindListByID(listID)
	if err != nil {
		log.Error().Err(err).Str("list_id", listID).Msg("Error looking up list")
		http.Error(w, "Error looking up list", http.StatusInternalServerError)
		return
	}
	if targetList == nil {
		http.Error(w, "List not found", http.StatusNotFound)
		return
	}

	// Get cached counts
	count, unread, found := s.listCache.GetCounts(listID)
	if !found {
		matches, err := s.backend.GetBooksForList(targetList)
		if err != nil {
			log.Warn().Err(err).Str("list_id", listID).Msg("Failed to get books for list")
			count, unread = 0, 0
		} else {
			count = len(matches)
			unread = countUnread(matches)
		}
		s.listCache.SetCounts(listID, count, unread)
	}

	// Format matchers
	matchers := make([]library.MatcherInfo, len(targetList.Matchers))
	for i, m := range targetList.Matchers {
		matchers[i] = library.GetMatcherInfo(m)
	}

	detail := ListDetail{
		ID:                   targetList.ID,
		Name:                 targetList.Name,
		Type:                 targetList.Type,
		MatcherMode:          targetList.MatcherMode,
		MatcherModeFormatted: library.FormatMatcherMode(targetList.MatcherMode),
		BookCount:            count,
		UnreadCount:          unread,
		Matchers:             matchers,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}

// ComicPreview represents a comic for preview display
type ComicPreview struct {
	ID        string `json:"id"`
	Series    string `json:"series"`
	Number    string `json:"number"`
	Title     string `json:"title"`
	Volume    int    `json:"volume"`
	Publisher string `json:"publisher"`
	Year      int    `json:"year"`
}

// handleGetListPreview returns a preview of comics matching the list
func (s *Server) handleGetListPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	// Extract list ID from URL
	path := r.URL.Path
	listID := path[len("/api/library/lists/"):]
	if idx := strings.Index(listID, "/"); idx != -1 {
		listID = listID[:idx]
	}

	// Parse query parameters
	query := r.URL.Query()
	limit := 20 // Default
	if l := query.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
			if limit > 100 {
				limit = 100 // Max limit
			}
		}
	}

	offset := 0
	if o := query.Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	// Find the list (searches recursively through folders)
	targetList, err := s.backend.FindListByID(listID)
	if err != nil {
		log.Error().Err(err).Str("list_id", listID).Msg("Error looking up list")
		http.Error(w, "Error looking up list", http.StatusInternalServerError)
		return
	}
	if targetList == nil {
		http.Error(w, "List not found", http.StatusNotFound)
		return
	}

	// Evaluate list
	matches, err := s.backend.GetBooksForList(targetList)
	if err != nil {
		log.Error().Err(err).Str("list_id", listID).Msg("Failed to get books for list")
		http.Error(w, "Failed to evaluate list", http.StatusInternalServerError)
		return
	}

	total := len(matches)

	// Apply pagination
	start := offset
	if start > total {
		start = total
	}

	end := start + limit
	if end > total {
		end = total
	}

	// Build preview
	previews := make([]ComicPreview, 0, end-start)
	for i := start; i < end; i++ {
		comic := matches[i]
		previews = append(previews, ComicPreview{
			ID:        comic.ID,
			Series:    comic.Series,
			Number:    comic.Number,
			Title:     comic.Title,
			Volume:    comic.Volume,
			Publisher: comic.Publisher,
			Year:      comic.Year,
		})
	}

	response := map[string]interface{}{
		"comics": previews,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// DeviceAssignment represents a device assigned to a list
type DeviceAssignment struct {
	DeviceID     string `json:"device_id"`
	FriendlyName string `json:"friendly_name"`
	Enabled      bool   `json:"enabled"`
}

// handleGetListDevices returns devices assigned to a specific list
func (s *Server) handleGetListDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract list ID
	path := r.URL.Path
	listID := path[len("/api/library/lists/"):]
	if idx := strings.Index(listID, "/"); idx != -1 {
		listID = listID[:idx]
	}

	assignments := []DeviceAssignment{}

	// Scan all devices for this list
	// Note: Using the existing server mutex pattern from api.go
	s.mu.RLock()
	if s.config != nil && s.config.Devices != nil {
		for _, device := range s.config.Devices {
			for _, list := range device.Lists {
				if list.ListID == listID {
					assignments = append(assignments, DeviceAssignment{
						DeviceID:     device.DeviceID,
						FriendlyName: device.FriendlyName,
						Enabled:      list.Enabled,
					})
					break
				}
			}
		}
	}
	s.mu.RUnlock()

	response := map[string]interface{}{
		"devices": assignments,
		"total":   len(assignments),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ListWriteRequest is the body for create/update smart list endpoints.
type ListWriteRequest struct {
	Name        string                     `json:"name"`
	Type        string                     `json:"type"`
	MatcherMode string                     `json:"matcher_mode"`
	Matchers    []library.ComicBookMatcher `json:"matchers"`
	BaseListId  string                     `json:"base_list_id,omitempty"`
	Description string                     `json:"description,omitempty"`
	Favorite    bool                       `json:"favorite,omitempty"`
}

// handleCreateList creates a new smart list. POST /api/library/lists
func (s *Server) handleCreateList(w http.ResponseWriter, r *http.Request) {
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	if !s.backend.CanPersist() {
		http.Error(w, "Library is read-only", http.StatusForbidden)
		return
	}

	var req ListWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.Type == "" {
		req.Type = "ComicSmartListItem"
	}
	if req.MatcherMode == "" {
		req.MatcherMode = "And"
	}

	list := &library.ComicListItem{
		ID:          "{" + newUUID() + "}",
		Name:        req.Name,
		Type:        req.Type,
		MatcherMode: req.MatcherMode,
		Matchers:    req.Matchers,
		BaseListId:  req.BaseListId,
		Description: req.Description,
		Favorite:    req.Favorite,
	}

	if err := s.backend.CreateList(list); err != nil {
		log.Error().Err(err).Msg("Failed to create list")
		http.Error(w, "Failed to create list", http.StatusInternalServerError)
		return
	}

	log.Info().Str("id", list.ID).Str("name", list.Name).Msg("Smart list created")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(list)
}

// handleUpdateList replaces an existing smart list. PUT /api/library/lists/:id
func (s *Server) handleUpdateList(w http.ResponseWriter, r *http.Request) {
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

	var req ListWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	existing, err := s.backend.FindListByID(listID)
	if err != nil || existing == nil {
		http.Error(w, "List not found", http.StatusNotFound)
		return
	}

	existing.Name = req.Name
	existing.MatcherMode = req.MatcherMode
	existing.Matchers = req.Matchers
	existing.BaseListId = req.BaseListId
	existing.Description = req.Description
	existing.Favorite = req.Favorite

	if err := s.backend.UpdateList(existing); err != nil {
		log.Error().Err(err).Str("id", listID).Msg("Failed to update list")
		http.Error(w, "Failed to update list", http.StatusInternalServerError)
		return
	}

	s.listCache.Invalidate(listID)

	log.Info().Str("id", listID).Str("name", existing.Name).Msg("Smart list updated")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(existing)
}

// handleDeleteList removes a smart list. DELETE /api/library/lists/:id
func (s *Server) handleDeleteList(w http.ResponseWriter, r *http.Request) {
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

	if err := s.backend.DeleteList(listID); err != nil {
		log.Error().Err(err).Str("id", listID).Msg("Failed to delete list")
		http.Error(w, "List not found", http.StatusNotFound)
		return
	}

	s.listCache.Invalidate(listID)

	log.Info().Str("id", listID).Msg("Smart list deleted")
	w.WriteHeader(http.StatusNoContent)
}
