package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/datamanager"
	"github.com/google/uuid"
)

// DMFieldInfo describes one field the rule editor can build a
// condition/action against.
type DMFieldInfo struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"` // string | numeric | date | yesno | language
	Writable bool   `json:"writable"`
}

// DMModifierInfo describes one rule/action modifier choice.
type DMModifierInfo struct {
	Value     string `json:"value"`
	Label     string `json:"label"`
	Params    int    `json:"params"`              // how many "||"-joined value parts this modifier needs (1 or 2)
	ValueHint string `json:"valueHint,omitempty"` // placeholder text for freeform multi-value modifiers
}

// DMRulesSchema is the static metadata GET /api/datamanager/schema returns
// for the rule editor UI (comic-server-tj6o) - field list plus, per field
// kind, the modifiers TranslateRule/ApplyAction actually understand. Kept
// in this package rather than internal/datamanager itself so the wire
// shape (JSON tags, UI label text) stays a web-API concern, matching
// list_schema.go's own separation for smart lists.
type DMRulesSchema struct {
	Fields                   []DMFieldInfo               `json:"fields"`
	RuleModifiers            map[string][]DMModifierInfo `json:"ruleModifiers"`            // keyed by field kind
	ActionModifiers          []DMModifierInfo            `json:"actionModifiers"`          // same list regardless of field kind
	CustomFieldRuleModifiers []DMModifierInfo            `json:"customFieldRuleModifiers"` // for fields not in the Fields list (custom values)
}

var dmRulesSchemaCache *DMRulesSchema

func getDMRulesSchema() *DMRulesSchema {
	if dmRulesSchemaCache != nil {
		return dmRulesSchemaCache
	}

	notted := func(base []DMModifierInfo) []DMModifierInfo {
		out := make([]DMModifierInfo, 0, len(base)*2)
		for _, m := range base {
			out = append(out, m)
			out = append(out, DMModifierInfo{Value: "Not" + m.Value, Label: "Not " + strings.ToLower(m.Label), Params: m.Params, ValueHint: m.ValueHint})
		}
		return out
	}

	stringBase := []DMModifierInfo{
		{Value: "Is", Label: "Is", Params: 1},
		{Value: "Contains", Label: "Contains", Params: 1},
		{Value: "StartsWith", Label: "Starts with", Params: 1},
		{Value: "Regex", Label: "Matches regex", Params: 1},
		{Value: "ContainsAnyOf", Label: "Contains any of", Params: 1, ValueHint: "value1||value2||..."},
		{Value: "ContainsAllOf", Label: "Contains all of", Params: 1, ValueHint: "value1||value2||..."},
		{Value: "IsAnyOf", Label: "Is any of", Params: 1, ValueHint: "value1||value2||..."},
		{Value: "StartsWithAnyOf", Label: "Starts with any of", Params: 1, ValueHint: "value1||value2||..."},
	}
	numericBase := []DMModifierInfo{
		{Value: "Is", Label: "Is", Params: 1},
		{Value: "Greater", Label: "Greater than", Params: 1},
		{Value: "Less", Label: "Less than", Params: 1},
		{Value: "GreaterEq", Label: "Greater than or equal to", Params: 1},
		{Value: "LessEq", Label: "Less than or equal to", Params: 1},
		{Value: "Range", Label: "In range", Params: 2},
		{Value: "IsAnyOf", Label: "Is any of", Params: 1, ValueHint: "value1||value2||..."},
	}
	dateBase := []DMModifierInfo{
		{Value: "Is", Label: "Is", Params: 1},
		{Value: "Greater", Label: "After", Params: 1},
		{Value: "Less", Label: "Before", Params: 1},
		{Value: "GreaterEq", Label: "On or after", Params: 1},
		{Value: "LessEq", Label: "On or before", Params: 1},
		{Value: "Range", Label: "In range", Params: 2},
		{Value: "IsInLastDays", Label: "In the last N days", Params: 1},
	}
	languageBase := []DMModifierInfo{
		{Value: "Is", Label: "Is", Params: 1},
		{Value: "IsAnyOf", Label: "Is any of", Params: 1, ValueHint: "value1||value2||..."},
	}
	customBase := []DMModifierInfo{
		{Value: "Is", Label: "Is", Params: 1},
		{Value: "Contains", Label: "Contains", Params: 1},
		{Value: "StartsWith", Label: "Starts with", Params: 1},
		{Value: "Regex", Label: "Matches regex", Params: 1},
		{Value: "ContainsAnyOf", Label: "Contains any of", Params: 1, ValueHint: "value1||value2||..."},
		{Value: "ContainsAllOf", Label: "Contains all of", Params: 1, ValueHint: "value1||value2||..."},
	}
	yesNoModifiers := []DMModifierInfo{
		{Value: "Is", Label: "Is", Params: 1},
		{Value: "Not", Label: "Is not", Params: 1},
	}

	fields := make([]DMFieldInfo, 0, len(datamanager.BuiltinFieldNames()))
	for _, name := range datamanager.BuiltinFieldNames() {
		def, _ := datamanager.LookupField(name)
		fields = append(fields, DMFieldInfo{
			Name:     name,
			Kind:     dmFieldKindName(def.Kind),
			Writable: def.Writable,
		})
	}

	dmRulesSchemaCache = &DMRulesSchema{
		Fields: fields,
		RuleModifiers: map[string][]DMModifierInfo{
			"string":   notted(stringBase),
			"numeric":  notted(numericBase),
			"date":     notted(dateBase),
			"yesno":    yesNoModifiers,
			"language": notted(languageBase),
		},
		CustomFieldRuleModifiers: notted(customBase),
		ActionModifiers: []DMModifierInfo{
			{Value: "SetValue", Label: "Set value", Params: 1},
			{Value: "Add", Label: "Add", Params: 1},
			{Value: "Remove", Label: "Remove", Params: 1},
			{Value: "RemoveLeading", Label: "Remove leading", Params: 1},
			{Value: "Replace", Label: "Replace (old||new)", Params: 2},
			{Value: "RegexReplace", Label: "Regex replace (pattern||replacement)", Params: 2},
			{Value: "RegExVarReplace", Label: "Regex capture into named fields (pattern)", Params: 1},
			{Value: "RegExVarAppend", Label: "Regex capture, appending into named fields (pattern)", Params: 1},
		},
	}
	return dmRulesSchemaCache
}

// dmFieldKindName maps internal/datamanager's FieldKind to the schema
// grouping the rule editor keys ruleModifiers by - String and MultiValue
// share the same modifier set (see translate.go's translateStringRule),
// as do Bool/YesNo/Manga (translateBoolRule), so they collapse to one
// schema kind each.
func dmFieldKindName(k datamanager.FieldKind) string {
	switch k {
	case datamanager.KindString, datamanager.KindMultiValue:
		return "string"
	case datamanager.KindNumeric, datamanager.KindPseudoNumeric:
		return "numeric"
	case datamanager.KindDate:
		return "date"
	case datamanager.KindBool, datamanager.KindYesNo, datamanager.KindManga:
		return "yesno"
	case datamanager.KindLanguage:
		return "language"
	default:
		return "string"
	}
}

// handleDataManagerSchema serves GET /api/datamanager/schema.
func (s *Server) handleDataManagerSchema(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeJSON(w, http.StatusOK, getDMRulesSchema())
}

// DMRuleWire is the wire shape of one dm_rules/dm_actions row, shared by
// rules and actions since both are field/modifier/value triples.
type DMRuleWire struct {
	ID        int64  `json:"id,omitempty"`
	Field     string `json:"field"`
	Modifier  string `json:"modifier"`
	Value     string `json:"value"`
	SortOrder int    `json:"sort_order"`
}

// DMRulesetWire is the wire shape of one ruleset, with its rules/actions
// inlined - the editor always reads/writes a whole ruleset's conditions
// and actions together, never rule-list and action-list independently.
type DMRulesetWire struct {
	ID        string       `json:"id,omitempty"`
	Name      string       `json:"name"`
	Mode      string       `json:"mode"`
	Disabled  bool         `json:"disabled"`
	SortOrder int          `json:"sort_order"`
	Rules     []DMRuleWire `json:"rules"`
	Actions   []DMRuleWire `json:"actions"`
}

// handleDataManagerRulesRouter dispatches every /api/datamanager/ path this
// first-slice editor needs (comic-server-tj6o) - nested groups/reordering
// are explicitly out of scope here (comic-server-vkpq tracks that follow-
// up), so every ruleset this editor creates is top-level (GroupID "").
func (s *Server) handleDataManagerRulesRouter(w http.ResponseWriter, r *http.Request) {
	if s.configDB == nil {
		http.Error(w, "Config database not available", http.StatusServiceUnavailable)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/datamanager/")
	segments := strings.Split(strings.Trim(path, "/"), "/")

	switch {
	case len(segments) == 1 && segments[0] == "rulesets":
		s.handleDMRulesetsCollection(w, r)
		return
	case len(segments) == 2 && segments[0] == "rulesets":
		s.handleDMRulesetItem(w, r, segments[1])
		return
	case len(segments) == 3 && segments[0] == "rulesets" && segments[2] == "rules":
		s.handleDMRulesCollection(w, r, segments[1])
		return
	case len(segments) == 3 && segments[0] == "rulesets" && segments[2] == "actions":
		s.handleDMActionsCollection(w, r, segments[1])
		return
	case len(segments) == 2 && segments[0] == "rules":
		s.handleDMRuleItem(w, r, segments[1])
		return
	case len(segments) == 2 && segments[0] == "actions":
		s.handleDMActionItem(w, r, segments[1])
		return
	}

	http.NotFound(w, r)
}

// handleDMRulesetsCollection serves GET (list) and POST (create) on
// /api/datamanager/rulesets.
func (s *Server) handleDMRulesetsCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rulesets, err := s.configDB.ListDMRulesets("")
		if err != nil {
			http.Error(w, "Failed to list rulesets: "+err.Error(), http.StatusInternalServerError)
			return
		}
		out := make([]DMRulesetWire, 0, len(rulesets))
		for _, rs := range rulesets {
			wire, err := s.loadDMRulesetWire(rs)
			if err != nil {
				http.Error(w, "Failed to load ruleset: "+err.Error(), http.StatusInternalServerError)
				return
			}
			out = append(out, wire)
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"rulesets": out})

	case http.MethodPost:
		var req DMRulesetWire
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		id := newDMID()
		rec := configdb.DMRuleset{ID: id, Name: req.Name, Mode: normalizeDMMode(req.Mode), Disabled: req.Disabled}
		if err := s.configDB.CreateDMRuleset(rec); err != nil {
			http.Error(w, "Failed to create ruleset: "+err.Error(), http.StatusInternalServerError)
			return
		}
		wire, err := s.loadDMRulesetWire(rec)
		if err != nil {
			http.Error(w, "Failed to load created ruleset: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s.writeJSON(w, http.StatusCreated, wire)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDMRulesetItem serves GET/PUT/DELETE on /api/datamanager/rulesets/:id.
func (s *Server) handleDMRulesetItem(w http.ResponseWriter, r *http.Request, id string) {
	existing, err := s.configDB.GetDMRuleset(id)
	if err != nil {
		http.Error(w, "Failed to load ruleset: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		wire, err := s.loadDMRulesetWire(*existing)
		if err != nil {
			http.Error(w, "Failed to load ruleset: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s.writeJSON(w, http.StatusOK, wire)

	case http.MethodPut:
		var req DMRulesetWire
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		existing.Name = req.Name
		existing.Mode = normalizeDMMode(req.Mode)
		existing.Disabled = req.Disabled
		if err := s.configDB.UpdateDMRuleset(*existing); err != nil {
			http.Error(w, "Failed to update ruleset: "+err.Error(), http.StatusInternalServerError)
			return
		}
		wire, err := s.loadDMRulesetWire(*existing)
		if err != nil {
			http.Error(w, "Failed to load updated ruleset: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s.writeJSON(w, http.StatusOK, wire)

	case http.MethodDelete:
		if err := s.configDB.DeleteDMRuleset(id); err != nil {
			http.Error(w, "Failed to delete ruleset: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDMRulesCollection serves POST (create) on
// /api/datamanager/rulesets/:id/rules.
func (s *Server) handleDMRulesCollection(w http.ResponseWriter, r *http.Request, rulesetID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ruleset, err := s.configDB.GetDMRuleset(rulesetID)
	if err != nil {
		http.Error(w, "Failed to load ruleset: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if ruleset == nil {
		http.NotFound(w, r)
		return
	}

	var req DMRuleWire
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Field) == "" || strings.TrimSpace(req.Modifier) == "" {
		http.Error(w, "field and modifier are required", http.StatusBadRequest)
		return
	}
	id, err := s.configDB.CreateDMRule(configdb.DMRule{RulesetID: rulesetID, Field: req.Field, Modifier: req.Modifier, Value: req.Value, SortOrder: req.SortOrder})
	if err != nil {
		http.Error(w, "Failed to create rule: "+err.Error(), http.StatusInternalServerError)
		return
	}
	req.ID = id
	s.writeJSON(w, http.StatusCreated, req)
}

// handleDMRuleItem serves PUT/DELETE on /api/datamanager/rules/:id.
func (s *Server) handleDMRuleItem(w http.ResponseWriter, r *http.Request, idStr string) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid rule id", http.StatusBadRequest)
		return
	}
	existing, err := s.configDB.GetDMRule(id)
	if err != nil {
		http.Error(w, "Failed to load rule: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodPut:
		var req DMRuleWire
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Field) == "" || strings.TrimSpace(req.Modifier) == "" {
			http.Error(w, "field and modifier are required", http.StatusBadRequest)
			return
		}
		existing.Field, existing.Modifier, existing.Value, existing.SortOrder = req.Field, req.Modifier, req.Value, req.SortOrder
		if err := s.configDB.UpdateDMRule(*existing); err != nil {
			http.Error(w, "Failed to update rule: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s.writeJSON(w, http.StatusOK, dmRuleToWire(*existing))

	case http.MethodDelete:
		if err := s.configDB.DeleteDMRule(id); err != nil {
			http.Error(w, "Failed to delete rule: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDMActionsCollection serves POST (create) on
// /api/datamanager/rulesets/:id/actions.
func (s *Server) handleDMActionsCollection(w http.ResponseWriter, r *http.Request, rulesetID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ruleset, err := s.configDB.GetDMRuleset(rulesetID)
	if err != nil {
		http.Error(w, "Failed to load ruleset: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if ruleset == nil {
		http.NotFound(w, r)
		return
	}

	var req DMRuleWire
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Field) == "" || strings.TrimSpace(req.Modifier) == "" {
		http.Error(w, "field and modifier are required", http.StatusBadRequest)
		return
	}
	id, err := s.configDB.CreateDMAction(configdb.DMAction{RulesetID: rulesetID, Field: req.Field, Modifier: req.Modifier, Value: req.Value, SortOrder: req.SortOrder})
	if err != nil {
		http.Error(w, "Failed to create action: "+err.Error(), http.StatusInternalServerError)
		return
	}
	req.ID = id
	s.writeJSON(w, http.StatusCreated, req)
}

// handleDMActionItem serves PUT/DELETE on /api/datamanager/actions/:id.
func (s *Server) handleDMActionItem(w http.ResponseWriter, r *http.Request, idStr string) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid action id", http.StatusBadRequest)
		return
	}
	existing, err := s.configDB.GetDMAction(id)
	if err != nil {
		http.Error(w, "Failed to load action: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodPut:
		var req DMRuleWire
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Field) == "" || strings.TrimSpace(req.Modifier) == "" {
			http.Error(w, "field and modifier are required", http.StatusBadRequest)
			return
		}
		existing.Field, existing.Modifier, existing.Value, existing.SortOrder = req.Field, req.Modifier, req.Value, req.SortOrder
		if err := s.configDB.UpdateDMAction(*existing); err != nil {
			http.Error(w, "Failed to update action: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s.writeJSON(w, http.StatusOK, dmActionToWire(*existing))

	case http.MethodDelete:
		if err := s.configDB.DeleteDMAction(id); err != nil {
			http.Error(w, "Failed to delete action: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) loadDMRulesetWire(rec configdb.DMRuleset) (DMRulesetWire, error) {
	dbRules, err := s.configDB.ListDMRules(rec.ID)
	if err != nil {
		return DMRulesetWire{}, fmt.Errorf("list rules: %w", err)
	}
	dbActions, err := s.configDB.ListDMActions(rec.ID)
	if err != nil {
		return DMRulesetWire{}, fmt.Errorf("list actions: %w", err)
	}
	rules := make([]DMRuleWire, len(dbRules))
	for i, dr := range dbRules {
		rules[i] = dmRuleToWire(dr)
	}
	actions := make([]DMRuleWire, len(dbActions))
	for i, da := range dbActions {
		actions[i] = dmActionToWire(da)
	}
	return DMRulesetWire{
		ID:        rec.ID,
		Name:      rec.Name,
		Mode:      rec.Mode,
		Disabled:  rec.Disabled,
		SortOrder: rec.SortOrder,
		Rules:     rules,
		Actions:   actions,
	}, nil
}

func dmRuleToWire(r configdb.DMRule) DMRuleWire {
	return DMRuleWire{ID: r.ID, Field: r.Field, Modifier: r.Modifier, Value: r.Value, SortOrder: r.SortOrder}
}

func dmActionToWire(a configdb.DMAction) DMRuleWire {
	return DMRuleWire{ID: a.ID, Field: a.Field, Modifier: a.Modifier, Value: a.Value, SortOrder: a.SortOrder}
}

func normalizeDMMode(mode string) string {
	if strings.EqualFold(mode, "Or") {
		return "Or"
	}
	return "And"
}

// newDMID generates a caller-supplied ID for a new group/ruleset - see
// configdb.CreateDMGroup's doc comment for why these are caller-supplied
// rather than autoincrement. Matches lists.go's own uuid.New() convention
// for smart-list IDs.
func newDMID() string {
	return uuid.New().String()
}
