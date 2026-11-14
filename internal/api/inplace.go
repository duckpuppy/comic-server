package api

import (
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/libraryorganizer"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

// inPlaceSaveBatch is how many books one save writes while advancing the
// books that are already in place. A failed save stops the run with the
// earlier batches kept.
const inPlaceSaveBatch = 500

// InPlacePreview is the answer of GET .../in-place-preview: how many To Move
// books already sit exactly where the profile would put them.
type InPlacePreview struct {
	Profile  string   `json:"profile"`
	ToMove   int      `json:"to_move"`
	InPlace  int      `json:"in_place"`
	NeedMove int      `json:"need_move"` // a real file move is still needed
	Other    int      `json:"other"`     // excluded, failed to plan, or in a collision
	Examples []string `json:"examples,omitempty"`
}

// InPlaceResult is the answer of POST .../in-place-apply.
type InPlaceResult struct {
	Profile  string   `json:"profile"`
	InPlace  int      `json:"in_place"`
	Advanced int      `json:"advanced"`
	Errors   []string `json:"errors,omitempty"`
}

// inPlaceGuard stops two advance runs at once.
type inPlaceGuard struct{ running atomic.Bool }

// inPlaceProfile returns the id and name of the profile the Workflow page
// uses for target paths (the first one), or the one named by id.
func (s *Server) inPlaceProfile(id string) (string, string, string, int) {
	if s.configDB == nil {
		return "", "", "Config database not available", http.StatusServiceUnavailable
	}
	profiles, err := s.configDB.ListLOProfiles()
	if err != nil {
		return "", "", "Failed to load Library Organizer profiles", http.StatusInternalServerError
	}
	if len(profiles) == 0 {
		return "", "", "No Library Organizer profile is set up", http.StatusConflict
	}
	for _, p := range profiles {
		if id == "" && p.ID == profiles[0].ID || p.ID == id {
			return p.ID, p.Name, "", 0
		}
	}
	return "", "", "Profile not found", http.StatusNotFound
}

// planInPlace plans every To Move book and returns the books that already
// sit at their planned path, plus the counts.
func (s *Server) planInPlace(profileID string) ([]libraryorganizer.PlannedMove, map[string]*library.ComicBook, InPlacePreview, string, int) {
	var preview InPlacePreview
	id, name, errMsg, status := s.inPlaceProfile(profileID)
	if errMsg != "" {
		return nil, nil, preview, errMsg, status
	}
	preview.Profile = name
	opts, _, errMsg, status := s.loadLOPlanOptions(id)
	if errMsg != "" {
		return nil, nil, preview, errMsg, status
	}
	books, err := s.booksAtStage(workflow.StageToMove)
	if err != nil {
		return nil, nil, preview, fmt.Sprintf("Failed to load books: %v", err), http.StatusInternalServerError
	}
	opts.FileExists = s.libraryOrganizerFileExists

	moves := libraryorganizer.Plan(books, opts)
	byID := make(map[string]*library.ComicBook, len(books))
	for _, b := range books {
		byID[b.ID] = b
	}
	var inPlace []libraryorganizer.PlannedMove
	preview.ToMove = len(books)
	for _, m := range moves {
		switch {
		case m.Skipped || m.Failed || m.Collision:
			preview.Other++
		case m.OldRawPath != "" && m.NewResolvedPath == m.OldResolvedPath:
			inPlace = append(inPlace, m)
		default:
			preview.NeedMove++
		}
	}
	preview.InPlace = len(inPlace)
	for i := 0; i < len(inPlace) && i < 3; i++ {
		preview.Examples = append(preview.Examples, inPlace[i].OldRawPath)
	}
	return inPlace, byID, preview, "", 0
}

// handleInPlacePreview serves GET /api/library/workflow/in-place-preview. It
// reads no file contents and changes nothing.
func (s *Server) handleInPlacePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	_, _, preview, errMsg, status := s.planInPlace(r.URL.Query().Get("profile"))
	if errMsg != "" {
		http.Error(w, errMsg, status)
		return
	}
	s.writeJSON(w, http.StatusOK, preview)
}

// handleInPlaceApply serves POST /api/library/workflow/in-place-apply. It
// moves every To Move book that already sits at its planned path on to
// Organized. No file is moved, copied or trashed: only the stage changes.
// Books that need a real move are left at To Move for the Organize page.
func (s *Server) handleInPlaceApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	if !s.inPlace.running.CompareAndSwap(false, true) {
		http.Error(w, "This is already running", http.StatusConflict)
		return
	}
	defer s.inPlace.running.Store(false)

	inPlace, byID, preview, errMsg, status := s.planInPlace(r.URL.Query().Get("profile"))
	if errMsg != "" {
		http.Error(w, errMsg, status)
		return
	}

	result := InPlaceResult{Profile: preview.Profile, InPlace: len(inPlace)}
	progress, endActivity := s.beginActivity("to_move", len(inPlace))
	defer endActivity()
	rulesets := s.loadWorkflowRulesets()

	for start := 0; start < len(inPlace); start += inPlaceSaveBatch {
		progress(start)
		end := min(start+inPlaceSaveBatch, len(inPlace))
		// Apply with only in-place moves takes its no-op branch: it touches no
		// file (no trash needed) and advances the book past To Move.
		outcomes := libraryorganizer.Apply(inPlace[start:end], libraryorganizer.ApplyOptions{
			Books:    byID,
			Rulesets: rulesets,
		})
		var batch []*library.ComicBook
		for _, o := range outcomes {
			switch {
			case o.NoOp:
				batch = append(batch, byID[o.BookID])
			case o.Failed:
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", o.BookID, o.FailReason))
			}
		}
		if len(batch) == 0 {
			continue
		}
		if err := s.audited(audit.ActionOrganize).UpdateBooks(batch); err != nil {
			log.Error().Err(err).Msg("Failed to save in-place advance; stopping")
			result.Errors = append(result.Errors, fmt.Sprintf("saving failed, run stopped: %v", err))
			break
		}
		result.Advanced += len(batch)
	}

	if result.Advanced > 0 {
		s.InvalidateWorkflowCache()
	}
	s.writeJSON(w, http.StatusOK, result)
}
