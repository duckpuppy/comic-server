package api

import (
	"encoding/json"
	"net/http"

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

// handleGetLists returns all smart lists with cached counts
func (s *Server) handleGetLists(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.library == nil {
		log.Error().Msg("Library not loaded")
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	lists := make([]ListSummary, 0, len(s.library.ComicLists))

	for _, list := range s.library.ComicLists {
		// Only include smart lists
		if list.Type != "ComicSmartListItem" {
			continue
		}

		// Get cached count, or calculate if not cached
		count, found := s.listCache.GetCount(list.ID)
		if !found {
			// Evaluate list to get count
			matches, err := s.library.MatchBooks(&list)
			if err != nil {
				log.Warn().Err(err).Str("list_id", list.ID).Msg("Failed to match books for list")
				count = 0
			} else {
				count = len(matches)
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

	response := map[string]interface{}{
		"lists": lists,
		"total": len(lists),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ListDetail represents full details of a smart list
type ListDetail struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Type                 string   `json:"type"`
	MatcherMode          string   `json:"matcher_mode"`
	MatcherModeFormatted string   `json:"matcher_mode_formatted"`
	BookCount            int      `json:"book_count"`
	Matchers             []string `json:"matchers"`
}

// handleGetListDetail returns details for a specific list
func (s *Server) handleGetListDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.library == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	// Extract list ID from URL path
	// URL: /api/library/lists/:listId
	listID := parsePathParam(r.URL.Path, "/api/library/lists/")

	// Find the list
	var targetList *library.ComicListItem
	for i := range s.library.ComicLists {
		if s.library.ComicLists[i].ID == listID {
			targetList = &s.library.ComicLists[i]
			break
		}
	}

	if targetList == nil {
		http.Error(w, "List not found", http.StatusNotFound)
		return
	}

	// Get cached count
	count, found := s.listCache.GetCount(listID)
	if !found {
		matches, err := s.library.MatchBooks(targetList)
		if err != nil {
			log.Warn().Err(err).Str("list_id", listID).Msg("Failed to match books for list")
			count = 0
		} else {
			count = len(matches)
		}
		s.listCache.SetCount(listID, count)
	}

	// Format matchers
	matchers := make([]string, len(targetList.Matchers))
	for i, m := range targetList.Matchers {
		matchers[i] = library.FormatMatcher(m)
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
