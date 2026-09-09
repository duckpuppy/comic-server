package api

import (
	"encoding/json"
	"net/http"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// BrowseRequest is the POST /api/library/browse body - an ad-hoc,
// never-saved smart-list definition (comic-server-joj): the same matcher
// engine every saved smart list already uses (internal/library/
// smartlist.go), evaluated on the fly without first creating a named
// list. Matchers use the exact wire shape library.ComicBookMatcher
// already speaks with the JS client (Type/Not/MatchOperator/MatchValue/
// MatchValue2/MatcherMode/Matchers) - it carries no json tags of its own,
// so Go's default field-name marshaling is what both sides already use
// (see handleGetListRaw, comic-server-58d).
type BrowseRequest struct {
	MatcherMode string                     `json:"matcher_mode"`
	Matchers    []library.ComicBookMatcher `json:"matchers"`
}

// handleBrowse evaluates an ad-hoc matcher set against the WHOLE library
// and returns a paginated preview - nothing is created or saved. Zero
// matchers means "0 matches", never "match everything", same deliberate
// safety choice handleGetListPreview already makes for a saved smart list
// with no conditions yet (comic-server-haz) - reused here rather than
// diverging, since an ad-hoc filter with nothing picked yet is the exact
// same "not configured to mean anything" state.
// POST /api/library/browse
func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	var req BrowseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.MatcherMode == "" {
		req.MatcherMode = "And"
	}

	limit, offset := parseLimitOffset(r, 20, 100)

	var matches []*library.ComicBook
	if len(req.Matchers) > 0 {
		list := &library.ComicListItem{
			Type:        "ComicSmartListItem",
			MatcherMode: req.MatcherMode,
			Matchers:    req.Matchers,
		}
		var err error
		matches, err = s.backend.GetBooksForList(list)
		if err != nil {
			log.Error().Err(err).Msg("Failed to evaluate ad-hoc browse filter")
			http.Error(w, "Failed to evaluate filter", http.StatusInternalServerError)
			return
		}
	}

	total := len(matches)
	start := min(offset, total)
	end := min(start+limit, total)

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
			Unread:    comic.IsUnread(),
		})
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"comics":   previews,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"has_more": end < total,
	})
}
