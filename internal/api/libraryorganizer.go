package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/libraryorganizer"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

// LOPlannedMove is the JSON shape for one PlannedMove - libraryorganizer.
// PlannedMove itself isn't JSON-tagged, so both preview and apply
// responses go through this.
type LOPlannedMove struct {
	BookID          string `json:"book_id"`
	Series          string `json:"series"`
	Number          string `json:"number"`
	Title           string `json:"title"`
	OldRawPath      string `json:"old_path"`
	NewRawPath      string `json:"new_path"`
	Skipped         bool   `json:"skipped"`
	Failed          bool   `json:"failed,omitempty"`
	FailReason      string `json:"fail_reason,omitempty"`
	Collision       bool   `json:"collision,omitempty"`
	CollisionReason string `json:"collision_reason,omitempty"`
}

// LOOrganizePreviewResult is the response for GET .../organize-preview.
type LOOrganizePreviewResult struct {
	Moves []LOPlannedMove `json:"moves"`
}

// LOOrganizeApplyResult is the response for POST .../organize-apply.
type LOOrganizeApplyResult struct {
	Processed int      `json:"processed"`
	Applied   int      `json:"applied"`
	NoOp      int      `json:"no_op"`
	Skipped   int      `json:"skipped"`
	Failed    int      `json:"failed"`
	Errors    []string `json:"errors,omitempty"`
}

func toLOPlannedMove(m libraryorganizer.PlannedMove) LOPlannedMove {
	return LOPlannedMove{
		BookID: m.BookID, Series: m.Series, Number: m.Number, Title: m.Title,
		OldRawPath: m.OldRawPath, NewRawPath: m.NewRawPath,
		Skipped: m.Skipped, Failed: m.Failed, FailReason: m.FailReason,
		Collision: m.Collision, CollisionReason: m.CollisionReason,
	}
}

// handleOrganizePreview computes (without writing anything) every book
// currently at workflow.StageToMove's planned destination under the
// profile named by the "profile" query parameter - comic-server-3bz.5's
// preview half of "Preview then Apply, same as Data Manager" (the safety
// model settled with the user for this feature, since it's comic-server's
// first feature that moves/renames the user's own existing files).
// GET /api/library/workflow/organize-preview?profile=<id>
func (s *Server) handleOrganizePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	opts, _, errMsg, status := s.loadLOPlanOptions(r.URL.Query().Get("profile"))
	if errMsg != "" {
		http.Error(w, errMsg, status)
		return
	}

	books, err := s.booksAtStage(workflow.StageToMove)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load books: %v", err), http.StatusInternalServerError)
		return
	}
	opts.FileExists = s.libraryOrganizerFileExists

	moves := libraryorganizer.Plan(books, opts)
	result := LOOrganizePreviewResult{Moves: make([]LOPlannedMove, len(moves))}
	for i, m := range moves {
		result.Moves[i] = toLOPlannedMove(m)
	}
	s.writeJSON(w, http.StatusOK, result)
}

// handleOrganizeApply re-runs the same plan handleOrganizePreview computed
// and, this time, actually moves/copies every approved book via
// libraryorganizer.Apply - comic-server's first feature that writes to the
// user's own existing directory structure, so every real move goes
// through internal/trash (see internal/libraryorganizer/apply.go). The
// caller is expected to have shown the preview and gotten explicit user
// approval first; this endpoint itself re-plans rather than trusting a
// client-submitted move list, so nothing can drift between preview and
// apply except the book set itself moving forward (e.g. a book converted
// out of StageToMove by another action in between).
// POST /api/library/workflow/organize-apply?profile=<id>
func (s *Server) handleOrganizeApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	tr, err := s.newTrashFromConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	opts, mode, errMsg, status := s.loadLOPlanOptions(r.URL.Query().Get("profile"))
	if errMsg != "" {
		http.Error(w, errMsg, status)
		return
	}

	// An optional JSON body {"book_ids": [...]} scopes the apply to a
	// user-approved subset of the preview (mirrors Data Manager's
	// selective apply, at book granularity rather than field granularity
	// since a Library Organizer move is a single all-or-nothing per-book
	// operation). No body, or an absent/null book_ids, applies every move
	// Plan itself approves - the same as the preview showed.
	var body struct {
		BookIDs []string `json:"book_ids"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err.Error() != "EOF" {
			http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
			return
		}
	}

	books, err := s.booksAtStage(workflow.StageToMove)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load books: %v", err), http.StatusInternalServerError)
		return
	}
	opts.FileExists = s.libraryOrganizerFileExists

	moves := libraryorganizer.Plan(books, opts)
	if body.BookIDs != nil {
		selected := make(map[string]bool, len(body.BookIDs))
		for _, id := range body.BookIDs {
			selected[id] = true
		}
		for i := range moves {
			if !selected[moves[i].BookID] {
				moves[i].Skipped = true
			}
		}
	}

	byID := make(map[string]*library.ComicBook, len(books))
	for _, b := range books {
		byID[b.ID] = b
	}

	outcomes := libraryorganizer.Apply(moves, libraryorganizer.ApplyOptions{
		Mode:     mode,
		Trash:    tr,
		Books:    byID,
		Rulesets: s.loadWorkflowRulesets(),
	})

	result := LOOrganizeApplyResult{Processed: len(outcomes)}
	var toUpdate []*library.ComicBook
	for i, o := range outcomes {
		switch {
		case o.Applied:
			result.Applied++
			toUpdate = append(toUpdate, byID[o.BookID])
		case o.NoOp:
			result.NoOp++
			toUpdate = append(toUpdate, byID[o.BookID])
		case o.Failed:
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", o.BookID, o.FailReason))
			log.Error().Str("book_id", o.BookID).Str("reason", o.FailReason).Msg("library-organizer apply failed for book")
		case o.Skipped:
			result.Skipped++
		}
		_ = i
	}

	if len(toUpdate) > 0 {
		if err := s.backend.UpdateBooks(toUpdate); err != nil {
			log.Error().Err(err).Msg("Failed to save library-organizer updates")
			result.Errors = append(result.Errors, err.Error())
		}
		s.InvalidateWorkflowCache()
	}

	s.writeJSON(w, http.StatusOK, result)
}

// libraryOrganizerFileExists backs Plan's collision detection against the
// real filesystem, resolving through the same path translation Plan
// already applied to compute the resolved path it's passed here.
func (s *Server) libraryOrganizerFileExists(resolvedPath string) bool {
	_, err := os.Stat(resolvedPath)
	return err == nil
}

// loadLOPlanOptions loads profileID's stored settings from configDB and
// builds the libraryorganizer.PlanOptions Plan needs (minus FileExists,
// which the caller wires separately). Returns a non-empty errMsg/status
// when profileID is missing, unknown, or configDB isn't available -
// callers should http.Error with those and stop.
func (s *Server) loadLOPlanOptions(profileID string) (libraryorganizer.PlanOptions, libraryorganizer.Mode, string, int) {
	if profileID == "" {
		return libraryorganizer.PlanOptions{}, 0, "profile query parameter is required", http.StatusBadRequest
	}
	if s.configDB == nil {
		return libraryorganizer.PlanOptions{}, 0, "Config database not available", http.StatusServiceUnavailable
	}

	p, err := s.configDB.GetLOProfile(profileID)
	if err != nil {
		return libraryorganizer.PlanOptions{}, 0, fmt.Sprintf("Failed to load profile: %v", err), http.StatusInternalServerError
	}
	if p == nil {
		return libraryorganizer.PlanOptions{}, 0, "Profile not found", http.StatusNotFound
	}

	months, err := s.loLoadMonths(p.ID)
	if err != nil {
		return libraryorganizer.PlanOptions{}, 0, fmt.Sprintf("Failed to load profile months: %v", err), http.StatusInternalServerError
	}
	illegal, err := s.loLoadIllegalCharacters(p.ID)
	if err != nil {
		return libraryorganizer.PlanOptions{}, 0, fmt.Sprintf("Failed to load profile illegal characters: %v", err), http.StatusInternalServerError
	}
	excludeRules, err := s.configDB.ListLOExcludeRules(p.ID)
	if err != nil {
		return libraryorganizer.PlanOptions{}, 0, fmt.Sprintf("Failed to load profile exclude rules: %v", err), http.StatusInternalServerError
	}

	rules := make([]libraryorganizer.ExcludeRule, len(excludeRules))
	for i, r := range excludeRules {
		rules[i] = libraryorganizer.ExcludeRule{Field: r.Field, Operator: r.Operator, Value: r.Value}
	}

	opts := libraryorganizer.PlanOptions{
		Profile: libraryorganizer.Profile{
			Months:                months,
			IllegalCharacters:     illegal,
			ReplaceMultipleSpaces: p.ReplaceMultipleSpaces,
			EmptyFolder:           p.EmptyFolder,
			FilelessFormat:        p.FilelessFormat,
		},
		Exclude: libraryorganizer.ExcludeConfig{
			Rules:           rules,
			ExcludeMode:     p.ExcludeMode,
			ExcludeOperator: p.ExcludeOperator,
		},
		BaseFolder:     p.BaseFolder,
		FolderTemplate: p.FolderTemplate,
		FileTemplate:   p.FileTemplate,
		ResolvePath:    s.resolveBookFilePath,
	}

	mode := libraryorganizer.ModeMove
	if p.CopyMode {
		mode = libraryorganizer.ModeCopy
	}

	return opts, mode, "", 0
}

// loLoadMonths loads a profile's "months" item collection into a
// libraryorganizer.Months map (item Name = month number "1".."12", Value =
// display name), falling back to libraryorganizer.DefaultMonths when the
// profile has no stored items for this category (e.g. a profile created
// without ever touching this collection).
func (s *Server) loLoadMonths(profileID string) (libraryorganizer.Months, error) {
	items, err := s.configDB.ListLOProfileItems(profileID, "months")
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return libraryorganizer.DefaultMonths, nil
	}
	months := make(libraryorganizer.Months, len(items))
	for _, item := range items {
		var n int
		if _, err := fmt.Sscanf(item.Name, "%d", &n); err != nil {
			continue
		}
		months[n] = item.Value
	}
	return months, nil
}

// loLoadIllegalCharacters mirrors loLoadMonths for the "illegal_characters"
// category (item Name = the illegal character, Value = its replacement).
func (s *Server) loLoadIllegalCharacters(profileID string) (libraryorganizer.IllegalCharacters, error) {
	items, err := s.configDB.ListLOProfileItems(profileID, "illegal_characters")
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return libraryorganizer.DefaultIllegalCharacters, nil
	}
	illegal := make(libraryorganizer.IllegalCharacters, len(items))
	for _, item := range items {
		illegal[item.Name] = item.Value
	}
	return illegal, nil
}

// LOProfileSummary is the JSON shape for one profile in the picker list -
// just enough for a UI to display and select a profile, not every
// persistence-layer field configdb.LOProfile carries.
type LOProfileSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	CopyMode bool   `json:"copy_mode"`
}

// handleListLOProfiles returns every configured Library Organizer profile,
// for a UI profile picker (comic-server-3bz.6).
// GET /api/library/organize-profiles
func (s *Server) handleListLOProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.configDB == nil {
		http.Error(w, "Config database not available", http.StatusServiceUnavailable)
		return
	}
	profiles, err := s.configDB.ListLOProfiles()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list profiles: %v", err), http.StatusInternalServerError)
		return
	}
	summaries := make([]LOProfileSummary, len(profiles))
	for i, p := range profiles {
		summaries[i] = LOProfileSummary{ID: p.ID, Name: p.Name, CopyMode: p.CopyMode}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"profiles": summaries})
}
