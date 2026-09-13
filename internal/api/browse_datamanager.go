package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/duckpuppy/comic-server/internal/datamanager"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// BrowseDataManagerRequest is the POST body for the Browse-scoped Data
// Manager preview/apply endpoints (comic-server-w7ig) - the same ad-hoc
// filter shape BrowseRequest already carries, plus (for apply) the same
// selective book_ids/fields shape DMApplyRequest already carries.
type BrowseDataManagerRequest struct {
	MatcherMode string                     `json:"matcher_mode"`
	Matchers    []library.ComicBookMatcher `json:"matchers"`
	ShowAll     bool                       `json:"show_all,omitempty"`
	BookIDs     []string                   `json:"book_ids,omitempty"`
	Fields      []DMFieldSelector          `json:"fields,omitempty"`
}

// handleBrowseDataManagerPreview and handleBrowseDataManagerApply replace
// the old standalone whole-library Data Manager tab (comic-server-w7ig,
// resolving comic-server-af0): Browse's current ad-hoc filter (or the
// explicit "Show all books" opt-in) IS the whole-library case with an
// empty filter, so this is the same background-job machinery
// handleDataManagerJobStart already runs, just scoped to
// resolveBrowseCandidates' answer instead of always the whole library.
// Both share the existing GET /api/library/datamanager-job status
// endpoint and single s.dmJob slot - Browse is now simply a different
// front end onto the same one job, not a second one.
// POST /api/library/browse/datamanager-preview
// POST /api/library/browse/datamanager-apply
func (s *Server) handleBrowseDataManagerPreview(w http.ResponseWriter, r *http.Request) {
	s.handleBrowseDataManagerJobStart(w, r, false)
}

func (s *Server) handleBrowseDataManagerApply(w http.ResponseWriter, r *http.Request) {
	s.handleBrowseDataManagerJobStart(w, r, true)
}

func (s *Server) handleBrowseDataManagerJobStart(w http.ResponseWriter, r *http.Request, apply bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	if s.configDB == nil {
		http.Error(w, "Configuration database not available", http.StatusServiceUnavailable)
		return
	}

	rulesets, err := datamanager.LoadRulesets(s.configDB)
	if err != nil {
		log.Error().Err(err).Msg("Failed to load Data Manager rules")
		http.Error(w, "Failed to load Data Manager rules", http.StatusInternalServerError)
		return
	}
	if len(rulesets) == 0 {
		http.Error(w, "No Data Manager rules configured - import a dataman.dat file first", http.StatusUnprocessableEntity)
		return
	}

	var req BrowseDataManagerRequest
	if r.Body != nil {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if len(data) > 0 {
			if err := json.Unmarshal(data, &req); err != nil {
				http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
				return
			}
		}
	}
	if req.MatcherMode == "" {
		req.MatcherMode = "And"
	}

	// Same selective-apply shortcut handleDataManagerJobStart uses: a
	// specific set of book_ids/fields only needs those books re-evaluated,
	// not the whole ad-hoc filter re-run (comic-server-c9v).
	var books []*library.ComicBook
	if apply && (len(req.BookIDs) > 0 || len(req.Fields) > 0) {
		books, err = s.fetchDMSelectedBooks(req.BookIDs, req.Fields)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to load selected books: %v", err), http.StatusInternalServerError)
			return
		}
	} else {
		books, err = s.resolveBrowseCandidates(BrowseRequest{
			MatcherMode: req.MatcherMode,
			Matchers:    req.Matchers,
			ShowAll:     req.ShowAll,
		})
		if err != nil {
			log.Error().Err(err).Msg("Failed to evaluate ad-hoc browse filter for Data Manager")
			http.Error(w, "Failed to evaluate filter", http.StatusInternalServerError)
			return
		}
	}

	s.dmJobMu.Lock()
	if s.dmJob != nil && s.dmJob.Status == "running" {
		s.dmJobMu.Unlock()
		http.Error(w, "A Data Manager run is already in progress", http.StatusConflict)
		return
	}
	job := &DMJobStatus{
		JobID:     fmt.Sprintf("dm-%d", time.Now().UnixNano()),
		Apply:     apply,
		Status:    "running",
		Total:     len(books),
		StartedAt: time.Now(),
	}
	s.dmJob = job
	s.dmJobMu.Unlock()

	go s.runDataManagerJob(job, books, rulesets, apply, req.BookIDs, req.Fields)

	s.writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.JobID})
}
