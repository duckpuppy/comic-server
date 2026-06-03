package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// ListSummary represents a smart list with cached count
type ListSummary struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	MatcherMode  string `json:"matcher_mode"`
	BookCount    int    `json:"book_count"`
	MatcherCount int    `json:"matcher_count"`
}

// ListTreeNode represents a node in the list tree (folder or smart list)
type ListTreeNode struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Type         string         `json:"type"`
	IsFolder     bool           `json:"is_folder"`
	BookCount    int            `json:"book_count,omitempty"`
	MatcherCount int            `json:"matcher_count,omitempty"`
	MatcherMode  string         `json:"matcher_mode,omitempty"`
	Children     []ListTreeNode `json:"children,omitempty"`
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
			// For smart lists and id lists, get book count and matcher info
			count, found := s.listCache.GetCount(item.ID)
			if !found {
				matches, err := s.backend.GetBooksForList(item)
				if err != nil {
					count = 0
				} else {
					count = len(matches)
				}
				s.listCache.SetCount(item.ID, count)
			}

			node.BookCount = count
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
				count, found := s.listCache.GetCount(list.ID)
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
						count = 0
					} else {
						count = len(matches)
						log.Debug().
							Str("list_name", list.Name).
							Int("matched_books", count).
							Msg("List evaluation complete")
					}
					s.listCache.SetCount(list.ID, count)
				}

				lists = append(lists, ListSummary{
					ID:           list.ID,
					Name:         list.Name,
					Type:         list.Type,
					MatcherMode:  list.MatcherMode,
					BookCount:    count,
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

	// Check for sub-paths
	// /api/library/lists/:listId
	if !strings.Contains(path[len("/api/library/lists/"):], "/") {
		s.handleGetListDetail(w, r)
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

	// Get cached count
	count, found := s.listCache.GetCount(listID)
	if !found {
		matches, err := s.backend.GetBooksForList(targetList)
		if err != nil {
			log.Warn().Err(err).Str("list_id", listID).Msg("Failed to get books for list")
			count = 0
		} else {
			count = len(matches)
		}
		s.listCache.SetCount(listID, count)
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
