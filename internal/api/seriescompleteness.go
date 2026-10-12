package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/google/uuid"
)

// Missing-issues view (comic-server-buhs): per ComicVine volume, which
// issues the library owns and which it does not, with a way to put missing
// ones on the Wanted list.

const volumeSummaryTTL = 2 * time.Minute

func (s *Server) invalidateVolumeSummaries() {
	s.volSummaryMu.Lock()
	s.volSummaryEntries = nil
	s.volSummaryMu.Unlock()
}

// volumeSummaries computes (or returns the recent) per-volume rollup. It
// scans the library and the ComicVine cache, which is cheap per call but not
// worth repeating for every keystroke of the page's filter box.
func (s *Server) volumeSummaries() ([]comicvine.VolumeSummary, error) {
	s.volSummaryMu.Lock()
	defer s.volSummaryMu.Unlock()
	if s.volSummaryEntries != nil && time.Since(s.volSummaryAt) < volumeSummaryTTL {
		return s.volSummaryEntries, nil
	}
	books, err := s.backend.GetAllBooks()
	if err != nil {
		return nil, err
	}
	entries, err := comicvine.BuildVolumeSummaries(books, s.cvCache)
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []comicvine.VolumeSummary{}
	}
	s.volSummaryEntries, s.volSummaryAt = entries, time.Now()
	return entries, nil
}

// handleSeriesCompleteness serves the volume list.
// GET /api/library/series-completeness?q=&only_missing=1&limit=&offset=
func (s *Server) handleSeriesCompleteness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	if s.cvCache == nil {
		http.Error(w, "ComicVine enrichment is not configured", http.StatusServiceUnavailable)
		return
	}
	all, err := s.volumeSummaries()
	if err != nil {
		http.Error(w, "Failed to compute completeness", http.StatusInternalServerError)
		return
	}

	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	onlyMissing := r.URL.Query().Get("only_missing") != "0"
	var filtered []comicvine.VolumeSummary
	for _, v := range all {
		if onlyMissing && v.Missing == 0 {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(v.Name), q) {
			continue
		}
		filtered = append(filtered, v)
	}

	limit, offset := parseLimitOffset(r, 50, 200)
	total := len(filtered)
	start := min(offset, total)
	end := min(start+limit, total)
	page := filtered[start:end]
	if page == nil {
		page = []comicvine.VolumeSummary{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"volumes": page, "total": total, "limit": limit, "offset": offset, "has_more": end < total,
	})
}

// handleSeriesCompletenessDetail serves one volume (GET) and the add-to-
// Wanted action (POST .../wanted).
// GET  /api/library/series-completeness/:volumeId
// POST /api/library/series-completeness/:volumeId/wanted
func (s *Server) handleSeriesCompletenessDetail(w http.ResponseWriter, r *http.Request) {
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	if s.cvCache == nil {
		http.Error(w, "ComicVine enrichment is not configured", http.StatusServiceUnavailable)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/library/series-completeness/")
	idPart, action, _ := strings.Cut(rest, "/")
	volumeID, err := strconv.Atoi(idPart)
	if err != nil || volumeID <= 0 {
		http.NotFound(w, r)
		return
	}

	switch {
	case action == "" && r.Method == http.MethodGet:
		books, err := s.backend.GetAllBooks()
		if err != nil {
			http.Error(w, "Failed to load books", http.StatusInternalServerError)
			return
		}
		sc, err := comicvine.BuildSeriesCompleteness(books, s.cvCache, volumeID)
		if err != nil {
			http.Error(w, "Failed to compute completeness", http.StatusInternalServerError)
			return
		}
		if sc == nil {
			http.Error(w, "That volume's issues are not synced from ComicVine yet", http.StatusNotFound)
			return
		}
		s.writeJSON(w, http.StatusOK, sc)
	case action == "wanted" && r.Method == http.MethodPost:
		s.addMissingToWanted(w, r, volumeID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// MissingToWantedRequest is the body of the add-to-Wanted POST.
type MissingToWantedRequest struct {
	IssueIDs []int `json:"issue_ids"`
}

// MissingToWantedResult is its response.
type MissingToWantedResult struct {
	Added   int      `json:"added"`
	Skipped int      `json:"skipped"` // owned, already wanted, or unknown
	Errors  []string `json:"errors,omitempty"`
}

// addMissingToWanted creates a Wanted placeholder (a book with no file) for
// each requested issue the library neither owns nor already wants. The
// placeholder carries the issue's ComicVine ids so linking a file later
// keeps it attached to the right issue; completeness ignores placeholders
// as owned.
func (s *Server) addMissingToWanted(w http.ResponseWriter, r *http.Request, volumeID int) {
	var req MissingToWantedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.IssueIDs) == 0 {
		http.Error(w, "issue_ids is required", http.StatusBadRequest)
		return
	}
	books, err := s.backend.GetAllBooks()
	if err != nil {
		http.Error(w, "Failed to load books", http.StatusInternalServerError)
		return
	}
	sc, err := comicvine.BuildSeriesCompleteness(books, s.cvCache, volumeID)
	if err != nil || sc == nil {
		http.Error(w, "That volume's issues are not synced from ComicVine yet", http.StatusNotFound)
		return
	}
	byID := map[int]comicvine.SeriesIssue{}
	for _, iss := range sc.Issues {
		byID[iss.IssueCVID] = iss
	}

	var res MissingToWantedResult
	seen := map[int]bool{}
	for _, id := range req.IssueIDs {
		iss, ok := byID[id]
		if seen[id] || !ok || iss.Owned || iss.Wanted {
			res.Skipped++
			continue
		}
		seen[id] = true

		book := library.ComicBook{
			ID:        "{" + uuid.New().String() + "}",
			Series:    sc.Name,
			Number:    iss.Number,
			Title:     iss.Name,
			Publisher: sc.Publisher,
		}
		if len(iss.CoverDate) >= 4 {
			book.Year, _ = strconv.Atoi(iss.CoverDate[:4])
		}
		book.CustomValuesStore = library.SetCustomValue(book.CustomValuesStore, "comicvine_volume", strconv.Itoa(volumeID))
		book.CustomValuesStore = library.SetCustomValue(book.CustomValuesStore, "comicvine_issue", strconv.Itoa(id))

		if err := s.backend.CreateBook(&book); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("issue %d: %v", id, err))
			continue
		}
		s.backend.MarkDirty(book.ID)
		audit.Record(s.configDB, configdb.AuditRow{
			BookID: book.ID, Action: audit.ActionWantedFromMissing, Series: book.Series, Number: book.Number,
			Changes: fmt.Sprintf(`[{"field":"record","old":"","new":"wanted (CV issue %d)"}]`, id),
		})
		res.Added++
	}
	if res.Added > 0 {
		s.InvalidateWorkflowCache()
		s.invalidateVolumeSummaries()
	}
	s.writeJSON(w, http.StatusOK, res)
}
