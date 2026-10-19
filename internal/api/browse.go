package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/duckpuppy/comic-server/internal/attention"
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
	// ShowAll explicitly requests every book in the library, bypassing
	// the matcher list entirely - comic-server-w7ig's "Show all books"
	// toggle. Deliberately a separate field from "zero matchers", not an
	// alternate meaning for it: zero matchers still means zero results
	// (comic-server-haz's safety rule, unchanged), so running a bulk
	// action over the whole library is always this one explicit,
	// visible opt-in rather than an accidental default.
	ShowAll bool `json:"show_all,omitempty"`

	// Query is an optional quick-search narrowing the matched books
	// further (comic-server-mzkf): plain words plus #N issue number,
	// (YYYY) year and VN volume tokens - see library.ParseSearchQuery.
	// It never widens: with no matchers and no show_all the result is
	// still empty, so bulk actions scoped to a Browse filter act on
	// exactly what the preview shows.
	Query string `json:"query,omitempty"`

	// Sort orders the preview ("" keeps library order): series, number,
	// title, publisher, year, volume, pages. Books with an unset value
	// (no page count, no year, ...) always sort last, in both directions.
	Sort     string `json:"sort,omitempty"`
	SortDesc bool   `json:"sort_desc,omitempty"`

	// Facets narrows further by publisher/format/stage/flag (comic-server-j6lk).
	Facets BrowseFacets `json:"facets,omitempty"`

	// GroupBy shows stacks instead of books (tracker #11): series, publisher,
	// year or format. GroupKey opens one stack (a key from an earlier answer)
	// and the response is that stack's books, paged like any other. Sort
	// "count" orders the stacks by size.
	GroupBy  string `json:"group_by,omitempty"`
	GroupKey string `json:"group_key,omitempty"`
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

	base, err := s.resolveBrowseBase(req)
	if err != nil {
		log.Error().Err(err).Msg("Failed to evaluate ad-hoc browse filter")
		http.Error(w, "Failed to evaluate filter", http.StatusInternalServerError)
		return
	}
	// The flag index costs a pass over the workflow cache; only build it
	// when a flag facet is set or the rail's counts are being sent.
	var flags map[string][]attention.Flag
	withFacets := offset == 0
	if withFacets || len(req.Facets.Flags) > 0 {
		flags = s.bookFlagIndex()
	}
	matches := applyFacets(base, req.Facets, flags)

	if browseGroupBys[req.GroupBy] {
		if req.GroupKey != "" {
			matches = filterToStack(matches, req.GroupBy, req.GroupKey)
			if req.Sort == "" {
				req.Sort = "number" // inside a stack, issue order is the useful one
			}
		} else {
			stacks := buildStacks(matches, req.GroupBy, req.Sort, req.SortDesc)
			total := len(stacks)
			start := min(offset, total)
			end := min(start+limit, total)
			resp := map[string]any{
				"stacks":      stacks[start:end],
				"total":       total,
				"total_books": len(matches),
				"limit":       limit,
				"offset":      offset,
				"has_more":    end < total,
			}
			if withFacets {
				resp["facets"] = countFacets(base, flags)
			}
			s.writeJSON(w, http.StatusOK, resp)
			return
		}
	}

	sortBrowseResults(matches, req.Sort, req.SortDesc)

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
			PageCount: comic.PageCount,
			Unread:    comic.IsUnread(),
			Kind:      bookKind(comic),
		})
	}

	resp := map[string]any{
		"comics":   previews,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"has_more": end < total,
	}
	// The rail's option counts come with the first page only; later pages
	// (Load More) do not change them.
	if withFacets {
		resp["facets"] = countFacets(base, flags)
	}
	s.writeJSON(w, http.StatusOK, resp)
}

// resolveBrowseCandidates resolves a BrowseRequest into the actual books
// in scope - shared by the preview table (handleBrowse) and the Browse-
// scoped Data Manager actions (comic-server-w7ig), since both need
// exactly the same "what does this ad-hoc filter currently mean" answer.
// ShowAll takes priority over the matcher list entirely (an explicit,
// separate opt-in - see BrowseRequest.ShowAll's doc comment); otherwise
// zero matchers still means zero results (comic-server-haz).
func (s *Server) resolveBrowseCandidates(req BrowseRequest) ([]*library.ComicBook, error) {
	books, err := s.resolveBrowseBase(req)
	if err != nil {
		return books, err
	}
	if !req.Facets.empty() {
		var flags map[string][]attention.Flag
		if len(req.Facets.Flags) > 0 {
			flags = s.bookFlagIndex()
		}
		books = applyFacets(books, req.Facets, flags)
	}
	// An open stack is part of what the page shows, so bulk edit and Data
	// Manager scopes must act on just that stack.
	if browseGroupBys[req.GroupBy] && req.GroupKey != "" {
		books = filterToStack(books, req.GroupBy, req.GroupKey)
	}
	return books, nil
}

// resolveBrowseBase is resolveBrowseCandidates before facets: matchers (or
// show_all) narrowed by the quick search. The rail counts over this set.
func (s *Server) resolveBrowseBase(req BrowseRequest) ([]*library.ComicBook, error) {
	books, err := s.resolveBrowseMatchers(req)
	if err != nil || strings.TrimSpace(req.Query) == "" {
		return books, err
	}
	sq := library.ParseSearchQuery(req.Query)
	if sq.IsEmpty() {
		return books, nil
	}
	filtered := books[:0:0]
	for _, b := range books {
		if sq.Match(b) {
			filtered = append(filtered, b)
		}
	}
	return filtered, nil
}

// resolveBrowseMatchers applies ShowAll / the matcher list, before any
// quick-search narrowing.
func (s *Server) resolveBrowseMatchers(req BrowseRequest) ([]*library.ComicBook, error) {
	if req.ShowAll {
		allBooks, err := s.backend.GetAllBooks()
		if err != nil {
			return nil, err
		}
		books := make([]*library.ComicBook, len(allBooks))
		for i := range allBooks {
			books[i] = &allBooks[i]
		}
		return books, nil
	}
	if len(req.Matchers) == 0 {
		return nil, nil
	}
	list := &library.ComicListItem{
		Type:        "ComicSmartListItem",
		MatcherMode: req.MatcherMode,
		Matchers:    req.Matchers,
	}
	return s.backend.GetBooksForList(list)
}

// sortBrowseResults orders books in place. Unset values sort last in both
// directions so unscraped books (no page count, no year) never dominate
// either end of the list. The sort is stable, so equal keys keep library
// order.
func sortBrowseResults(books []*library.ComicBook, field string, desc bool) {
	type key struct {
		num   float64
		str   string
		isNum bool
		unset bool
	}
	keyOf := func(b *library.ComicBook) key {
		switch field {
		case "series":
			return key{str: strings.ToLower(b.Series), unset: b.Series == ""}
		case "title":
			return key{str: strings.ToLower(b.Title), unset: b.Title == ""}
		case "publisher":
			return key{str: strings.ToLower(b.Publisher), unset: b.Publisher == ""}
		case "year":
			return key{num: float64(b.Year), isNum: true, unset: b.Year <= 0}
		case "volume":
			return key{num: float64(b.Volume), isNum: true, unset: b.Volume <= 0}
		case "pages":
			return key{num: float64(b.PageCount), isNum: true, unset: b.PageCount <= 0}
		case "number":
			n, err := strconv.ParseFloat(strings.TrimLeft(strings.TrimSpace(b.Number), "#"), 64)
			if err != nil {
				return key{str: strings.ToLower(b.Number), unset: b.Number == ""}
			}
			return key{num: n, isNum: true}
		}
		return key{unset: true}
	}
	switch field {
	case "series", "title", "publisher", "year", "volume", "pages", "number":
	default:
		return
	}
	sort.SliceStable(books, func(i, j int) bool {
		a, b := keyOf(books[i]), keyOf(books[j])
		if a.unset != b.unset {
			return b.unset // unset last regardless of direction
		}
		if a.unset {
			return false
		}
		// Mixed numeric/non-numeric issue numbers: numeric first.
		if a.isNum != b.isNum {
			return a.isNum
		}
		var less, greater bool
		if a.isNum {
			less, greater = a.num < b.num, a.num > b.num
		} else {
			less, greater = a.str < b.str, a.str > b.str
		}
		if desc {
			return greater
		}
		return less
	})
}
