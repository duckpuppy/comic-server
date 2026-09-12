package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/libraryorganizer"
	"github.com/google/uuid"
)

// LOSchema is the static metadata GET /api/library/organize-profiles/schema
// returns for the profile editor UI (comic-server-7ecr) - exclude-rule
// fields/operators are exported directly from internal/libraryorganizer
// so this list can never drift from what ShouldMove actually accepts.
type LOSchema struct {
	ExcludeFields    []string `json:"excludeFields"`
	ExcludeOperators []string `json:"excludeOperators"`
}

func (s *Server) handleLOSchema(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeJSON(w, http.StatusOK, LOSchema{
		ExcludeFields:    libraryorganizer.ExcludeRuleFields,
		ExcludeOperators: libraryorganizer.ExcludeRuleOperators,
	})
}

// LOExcludeRuleWire is the wire shape of one lo_exclude_rules row.
type LOExcludeRuleWire struct {
	ID        int64  `json:"id,omitempty"`
	Field     string `json:"field"`
	Operator  string `json:"operator"`
	Value     string `json:"value"`
	SortOrder int    `json:"sort_order"`
}

// LOProfileWire is the wire shape of one profile - only the fields
// comic-server-7ecr's first-slice editor exposes (core path-building
// fields + exclude rules). Every other configdb.LOProfile field either
// isn't read by Plan/Apply at all (comic-server-b2al tracks that cleanup)
// or is deferred (Months/IllegalCharacters lookup tables, comic-server-kt4w)
// - both keep their current values on update since this DTO never carries
// them, see loProfileFromWire's merge-onto-existing behavior.
type LOProfileWire struct {
	ID                    string              `json:"id,omitempty"`
	Name                  string              `json:"name"`
	BaseFolder            string              `json:"base_folder"`
	FolderTemplate        string              `json:"folder_template"`
	FileTemplate          string              `json:"file_template"`
	CopyMode              bool                `json:"copy_mode"`
	ReplaceMultipleSpaces bool                `json:"replace_multiple_spaces"`
	EmptyFolder           string              `json:"empty_folder"`
	FilelessFormat        string              `json:"fileless_format"`
	ExcludeMode           string              `json:"exclude_mode"`
	ExcludeOperator       string              `json:"exclude_operator"`
	ExcludeRules          []LOExcludeRuleWire `json:"exclude_rules"`
}

func loExcludeRuleToWire(r configdb.LOExcludeRule) LOExcludeRuleWire {
	return LOExcludeRuleWire{ID: r.ID, Field: r.Field, Operator: r.Operator, Value: r.Value, SortOrder: r.SortOrder}
}

func (s *Server) loadLOProfileWire(p configdb.LOProfile) (LOProfileWire, error) {
	rules, err := s.configDB.ListLOExcludeRules(p.ID)
	if err != nil {
		return LOProfileWire{}, fmt.Errorf("list exclude rules: %w", err)
	}
	wireRules := make([]LOExcludeRuleWire, len(rules))
	for i, r := range rules {
		wireRules[i] = loExcludeRuleToWire(r)
	}
	return LOProfileWire{
		ID:                    p.ID,
		Name:                  p.Name,
		BaseFolder:            p.BaseFolder,
		FolderTemplate:        p.FolderTemplate,
		FileTemplate:          p.FileTemplate,
		CopyMode:              p.CopyMode,
		ReplaceMultipleSpaces: p.ReplaceMultipleSpaces,
		EmptyFolder:           p.EmptyFolder,
		FilelessFormat:        p.FilelessFormat,
		ExcludeMode:           p.ExcludeMode,
		ExcludeOperator:       p.ExcludeOperator,
		ExcludeRules:          wireRules,
	}, nil
}

// mergeLOProfileWire applies req onto existing (a profile already loaded
// from config.db, or a zero-value one for create) - fields this DTO
// doesn't carry (UseFolder, UseFileName, the dead fields comic-server-b2al
// tracks, Months/IllegalCharacters) are left exactly as they were, never
// reset to a Go zero value by an edit through this editor.
func mergeLOProfileWire(existing configdb.LOProfile, req LOProfileWire) configdb.LOProfile {
	existing.Name = req.Name
	existing.BaseFolder = req.BaseFolder
	existing.FolderTemplate = req.FolderTemplate
	existing.FileTemplate = req.FileTemplate
	existing.CopyMode = req.CopyMode
	existing.ReplaceMultipleSpaces = req.ReplaceMultipleSpaces
	existing.EmptyFolder = req.EmptyFolder
	existing.FilelessFormat = req.FilelessFormat
	existing.ExcludeMode = req.ExcludeMode
	existing.ExcludeOperator = req.ExcludeOperator
	return existing
}

// handleLOProfilesRouter dispatches every /api/library/organize-profiles/
// path this first-slice editor needs (comic-server-7ecr). Registered
// alongside the existing exact-match GET /api/library/organize-profiles
// (the profile-picker summary list, handleListLOProfiles) which this
// leaves untouched - POST on that same exact path is handled here too
// (see api.go's registration) so create doesn't need its own separate
// top-level route.
func (s *Server) handleLOProfilesRouter(w http.ResponseWriter, r *http.Request) {
	if s.configDB == nil {
		http.Error(w, "Config database not available", http.StatusServiceUnavailable)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/library/organize-profiles/")
	segments := strings.Split(strings.Trim(path, "/"), "/")

	switch {
	case len(segments) == 1 && segments[0] == "schema":
		s.handleLOSchema(w, r)
		return
	case len(segments) == 1 && segments[0] != "":
		s.handleLOProfileItem(w, r, segments[0])
		return
	case len(segments) == 2 && segments[1] == "rules":
		s.handleLORulesCollection(w, r, segments[0])
		return
	}

	http.NotFound(w, r)
}

// handleLOProfilesCollection serves POST (create) on
// /api/library/organize-profiles - GET on that same path stays
// handleListLOProfiles, unchanged.
func (s *Server) handleLOProfilesCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.handleListLOProfiles(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.configDB == nil {
		http.Error(w, "Config database not available", http.StatusServiceUnavailable)
		return
	}

	var req LOProfileWire
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	p := mergeLOProfileWire(configdb.LOProfile{
		ID:   uuid.New().String(),
		Mode: "Move",
	}, req)
	if err := s.configDB.CreateLOProfile(p); err != nil {
		http.Error(w, "Failed to create profile: "+err.Error(), http.StatusInternalServerError)
		return
	}
	wire, err := s.loadLOProfileWire(p)
	if err != nil {
		http.Error(w, "Failed to load created profile: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, http.StatusCreated, wire)
}

// handleLOProfileItem serves GET/PUT/DELETE on
// /api/library/organize-profiles/:id.
func (s *Server) handleLOProfileItem(w http.ResponseWriter, r *http.Request, id string) {
	existing, err := s.configDB.GetLOProfile(id)
	if err != nil {
		http.Error(w, "Failed to load profile: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		wire, err := s.loadLOProfileWire(*existing)
		if err != nil {
			http.Error(w, "Failed to load profile: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s.writeJSON(w, http.StatusOK, wire)

	case http.MethodPut:
		var req LOProfileWire
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		updated := mergeLOProfileWire(*existing, req)
		if err := s.configDB.UpdateLOProfile(updated); err != nil {
			http.Error(w, "Failed to update profile: "+err.Error(), http.StatusInternalServerError)
			return
		}
		wire, err := s.loadLOProfileWire(updated)
		if err != nil {
			http.Error(w, "Failed to load updated profile: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s.writeJSON(w, http.StatusOK, wire)

	case http.MethodDelete:
		if err := s.configDB.DeleteLOProfile(id); err != nil {
			http.Error(w, "Failed to delete profile: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleLORulesCollection serves POST (create) on
// /api/library/organize-profiles/:id/rules.
func (s *Server) handleLORulesCollection(w http.ResponseWriter, r *http.Request, profileID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	profile, err := s.configDB.GetLOProfile(profileID)
	if err != nil {
		http.Error(w, "Failed to load profile: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if profile == nil {
		http.NotFound(w, r)
		return
	}

	var req LOExcludeRuleWire
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Field) == "" || strings.TrimSpace(req.Operator) == "" {
		http.Error(w, "field and operator are required", http.StatusBadRequest)
		return
	}
	id, err := s.configDB.CreateLOExcludeRule(configdb.LOExcludeRule{
		ProfileID: profileID, Field: req.Field, Operator: req.Operator, Value: req.Value, SortOrder: req.SortOrder,
	})
	if err != nil {
		http.Error(w, "Failed to create exclude rule: "+err.Error(), http.StatusInternalServerError)
		return
	}
	req.ID = id
	s.writeJSON(w, http.StatusCreated, req)
}

// handleLORuleItem serves PUT/DELETE on /api/library/organize-rules/:id.
func (s *Server) handleLORuleItem(w http.ResponseWriter, r *http.Request) {
	if s.configDB == nil {
		http.Error(w, "Config database not available", http.StatusServiceUnavailable)
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/api/library/organize-rules/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid rule id", http.StatusBadRequest)
		return
	}
	existing, err := s.configDB.GetLOExcludeRule(id)
	if err != nil {
		http.Error(w, "Failed to load exclude rule: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodPut:
		var req LOExcludeRuleWire
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Field) == "" || strings.TrimSpace(req.Operator) == "" {
			http.Error(w, "field and operator are required", http.StatusBadRequest)
			return
		}
		existing.Field, existing.Operator, existing.Value, existing.SortOrder = req.Field, req.Operator, req.Value, req.SortOrder
		if err := s.configDB.UpdateLOExcludeRule(*existing); err != nil {
			http.Error(w, "Failed to update exclude rule: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s.writeJSON(w, http.StatusOK, loExcludeRuleToWire(*existing))

	case http.MethodDelete:
		if err := s.configDB.DeleteLOExcludeRule(id); err != nil {
			http.Error(w, "Failed to delete exclude rule: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
