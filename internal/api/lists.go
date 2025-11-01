package api

import (
	"encoding/json"
	"net/http"

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
