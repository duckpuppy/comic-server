package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newDMRulesTestServer(t *testing.T) *Server {
	t.Helper()
	db := newTestConfigDB(t)
	return &Server{configDB: db}
}

func TestHandleDataManagerSchema(t *testing.T) {
	s := newDMRulesTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/datamanager/schema", nil)
	w := httptest.NewRecorder()
	s.handleDataManagerSchema(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var schema DMRulesSchema
	if err := json.NewDecoder(w.Body).Decode(&schema); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(schema.Fields) == 0 {
		t.Error("expected at least one field in schema")
	}
	if len(schema.RuleModifiers["string"]) == 0 {
		t.Error("expected string rule modifiers")
	}
	if len(schema.ActionModifiers) == 0 {
		t.Error("expected action modifiers")
	}
	// Series should be a writable string field.
	found := false
	for _, f := range schema.Fields {
		if f.Name == "Series" {
			found = true
			if f.Kind != "string" || !f.Writable {
				t.Errorf("Series field = %+v, want Kind=string Writable=true", f)
			}
		}
	}
	if !found {
		t.Error("expected Series field in schema")
	}
}

// TestDataManagerRulesEditor_FullLifecycle exercises the whole first-slice
// editor surface (comic-server-tj6o) end to end: create a ruleset, add a
// rule and an action, edit both, list, then delete everything - mirroring
// the acceptance criteria in the bd issue ("create, edit, and delete a
// Data Manager rule... entirely through the web UI").
func TestDataManagerRulesEditor_FullLifecycle(t *testing.T) {
	s := newDMRulesTestServer(t)

	// Create ruleset.
	createBody, _ := json.Marshal(DMRulesetWire{Name: "My Ruleset", Mode: "And"})
	req := httptest.NewRequest(http.MethodPost, "/api/datamanager/rulesets", bytes.NewReader(createBody))
	w := httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create ruleset: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var created DMRulesetWire
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.ID == "" || created.Name != "My Ruleset" {
		t.Fatalf("created ruleset = %+v, want ID set, Name=My Ruleset", created)
	}

	// Add a rule.
	ruleBody, _ := json.Marshal(DMRuleWire{Field: "Series", Modifier: "Is", Value: "Batman"})
	req = httptest.NewRequest(http.MethodPost, "/api/datamanager/rulesets/"+created.ID+"/rules", bytes.NewReader(ruleBody))
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create rule: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var rule DMRuleWire
	if err := json.NewDecoder(w.Body).Decode(&rule); err != nil {
		t.Fatalf("decode rule: %v", err)
	}
	if rule.ID == 0 {
		t.Fatal("expected non-zero rule ID")
	}

	// Add an action.
	actionBody, _ := json.Marshal(DMRuleWire{Field: "SeriesGroup", Modifier: "SetValue", Value: "Batman Family"})
	req = httptest.NewRequest(http.MethodPost, "/api/datamanager/rulesets/"+created.ID+"/actions", bytes.NewReader(actionBody))
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create action: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var action DMRuleWire
	if err := json.NewDecoder(w.Body).Decode(&action); err != nil {
		t.Fatalf("decode action: %v", err)
	}

	// GET the ruleset back - should have both the rule and action inlined.
	req = httptest.NewRequest(http.MethodGet, "/api/datamanager/rulesets/"+created.ID, nil)
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get ruleset: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var full DMRulesetWire
	if err := json.NewDecoder(w.Body).Decode(&full); err != nil {
		t.Fatalf("decode full: %v", err)
	}
	if len(full.Rules) != 1 || len(full.Actions) != 1 {
		t.Fatalf("full ruleset = %+v, want 1 rule and 1 action", full)
	}

	// Edit the rule.
	editRuleBody, _ := json.Marshal(DMRuleWire{Field: "Series", Modifier: "Is", Value: "Superman"})
	req = httptest.NewRequest(http.MethodPut, "/api/datamanager/rules/1", bytes.NewReader(editRuleBody))
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("edit rule: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Edit the ruleset itself (rename, disable).
	editRulesetBody, _ := json.Marshal(DMRulesetWire{Name: "Renamed", Mode: "Or", Disabled: true})
	req = httptest.NewRequest(http.MethodPut, "/api/datamanager/rulesets/"+created.ID, bytes.NewReader(editRulesetBody))
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("edit ruleset: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var editedRS DMRulesetWire
	if err := json.NewDecoder(w.Body).Decode(&editedRS); err != nil {
		t.Fatalf("decode edited ruleset: %v", err)
	}
	if editedRS.Name != "Renamed" || editedRS.Mode != "Or" || !editedRS.Disabled {
		t.Fatalf("edited ruleset = %+v, want Name=Renamed Mode=Or Disabled=true", editedRS)
	}

	// Delete the action, then the rule, then the ruleset.
	req = httptest.NewRequest(http.MethodDelete, "/api/datamanager/actions/1", nil)
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete action: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/datamanager/rules/1", nil)
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete rule: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/datamanager/rulesets/"+created.ID, nil)
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete ruleset: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/datamanager/rulesets/"+created.ID, nil)
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get deleted ruleset: expected 404, got %d", w.Code)
	}
}

func TestHandleDMRulesetsCollection_ListEmpty(t *testing.T) {
	s := newDMRulesTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/datamanager/rulesets", nil)
	w := httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result struct {
		Rulesets []DMRulesetWire `json:"rulesets"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Rulesets) != 0 {
		t.Errorf("expected 0 rulesets, got %d", len(result.Rulesets))
	}
}

func TestHandleDMRulesetsCollection_CreateRejectsEmptyName(t *testing.T) {
	s := newDMRulesTestServer(t)

	body, _ := json.Marshal(DMRulesetWire{Name: "   "})
	req := httptest.NewRequest(http.MethodPost, "/api/datamanager/rulesets", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty name, got %d: %s", w.Code, w.Body.String())
	}
}

// TestDataManagerGroups_TreeLifecycle exercises the nested-group-folders
// piece of comic-server-vkpq: create a folder, create a subfolder and a
// ruleset inside it, fetch the tree and confirm the shape, move the
// ruleset back to root, rename and delete the folder.
func TestDataManagerGroups_TreeLifecycle(t *testing.T) {
	s := newDMRulesTestServer(t)

	// Create a top-level folder.
	createBody, _ := json.Marshal(map[string]string{"name": "Quality"})
	req := httptest.NewRequest(http.MethodPost, "/api/datamanager/groups", bytes.NewReader(createBody))
	w := httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create group: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var group DMTreeNode
	if err := json.NewDecoder(w.Body).Decode(&group); err != nil {
		t.Fatalf("decode group: %v", err)
	}
	if group.ID == "" || !group.IsFolder {
		t.Fatalf("created group = %+v, want ID set and IsFolder=true", group)
	}

	// Create a ruleset inside that folder.
	rsBody, _ := json.Marshal(DMRulesetWire{Name: "In Folder", Mode: "And", GroupID: group.ID})
	req = httptest.NewRequest(http.MethodPost, "/api/datamanager/rulesets", bytes.NewReader(rsBody))
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create ruleset in folder: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var rs DMRulesetWire
	if err := json.NewDecoder(w.Body).Decode(&rs); err != nil {
		t.Fatalf("decode ruleset: %v", err)
	}
	if rs.GroupID != group.ID {
		t.Fatalf("created ruleset GroupID = %q, want %q", rs.GroupID, group.ID)
	}

	// Fetch the tree - should have one top-level folder containing one ruleset.
	req = httptest.NewRequest(http.MethodGet, "/api/datamanager/tree", nil)
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get tree: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var treeResp struct {
		Tree []DMTreeNode `json:"tree"`
	}
	if err := json.NewDecoder(w.Body).Decode(&treeResp); err != nil {
		t.Fatalf("decode tree: %v", err)
	}
	if len(treeResp.Tree) != 1 || !treeResp.Tree[0].IsFolder || treeResp.Tree[0].ID != group.ID {
		t.Fatalf("tree = %+v, want one folder node %q", treeResp.Tree, group.ID)
	}
	if len(treeResp.Tree[0].Children) != 1 || treeResp.Tree[0].Children[0].IsFolder || treeResp.Tree[0].Children[0].ID != rs.ID {
		t.Fatalf("folder children = %+v, want one ruleset node %q", treeResp.Tree[0].Children, rs.ID)
	}
	if treeResp.Tree[0].Children[0].Ruleset == nil || treeResp.Tree[0].Children[0].Ruleset.Name != "In Folder" {
		t.Fatalf("ruleset node's inlined Ruleset = %+v, want Name=In Folder", treeResp.Tree[0].Children[0].Ruleset)
	}

	// Move the ruleset back to root.
	moveBody, _ := json.Marshal(map[string]string{"parent_id": ""})
	req = httptest.NewRequest(http.MethodPut, "/api/datamanager/rulesets/"+rs.ID+"/parent", bytes.NewReader(moveBody))
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("move ruleset to root: expected 204, got %d: %s", w.Code, w.Body.String())
	}
	moved, err := s.configDB.GetDMRuleset(rs.ID)
	if err != nil || moved == nil || moved.GroupID != "" {
		t.Fatalf("GetDMRuleset after move = %+v err=%v, want GroupID=\"\"", moved, err)
	}

	// Rename the folder.
	renameBody, _ := json.Marshal(map[string]any{"name": "Renamed Folder", "disabled": true})
	req = httptest.NewRequest(http.MethodPut, "/api/datamanager/groups/"+group.ID, bytes.NewReader(renameBody))
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("rename group: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	renamedGroup, err := s.configDB.GetDMGroup(group.ID)
	if err != nil || renamedGroup == nil || renamedGroup.Name != "Renamed Folder" || !renamedGroup.Disabled {
		t.Fatalf("GetDMGroup after rename = %+v err=%v, want Name=Renamed Folder Disabled=true", renamedGroup, err)
	}

	// Delete the folder.
	req = httptest.NewRequest(http.MethodDelete, "/api/datamanager/groups/"+group.ID, nil)
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete group: expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if gone, err := s.configDB.GetDMGroup(group.ID); err != nil || gone != nil {
		t.Errorf("expected group gone after delete, got %+v err=%v", gone, err)
	}
}

// TestDataManagerGroups_SortOrderRoundTrips covers comic-server-vkpq's
// drag-reorder piece: PUT on a group or ruleset must carry sort_order
// through (both to persist a drag-reorder, and to NOT silently reset it
// on an unrelated edit like a rename), and GET .../tree must reflect it.
func TestDataManagerGroups_SortOrderRoundTrips(t *testing.T) {
	s := newDMRulesTestServer(t)

	createBody, _ := json.Marshal(map[string]string{"name": "Quality"})
	req := httptest.NewRequest(http.MethodPost, "/api/datamanager/groups", bytes.NewReader(createBody))
	w := httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	var group DMTreeNode
	json.NewDecoder(w.Body).Decode(&group)

	rsBody, _ := json.Marshal(DMRulesetWire{Name: "Batman", Mode: "And"})
	req = httptest.NewRequest(http.MethodPost, "/api/datamanager/rulesets", bytes.NewReader(rsBody))
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	var rs DMRulesetWire
	json.NewDecoder(w.Body).Decode(&rs)

	// Reorder the group to sort_order=5 via PUT (the drag-reorder path).
	reorderBody, _ := json.Marshal(map[string]any{"name": group.Name, "disabled": false, "sort_order": 5})
	req = httptest.NewRequest(http.MethodPut, "/api/datamanager/groups/"+group.ID, bytes.NewReader(reorderBody))
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("reorder group: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	reordered, err := s.configDB.GetDMGroup(group.ID)
	if err != nil || reordered == nil || reordered.SortOrder != 5 {
		t.Fatalf("GetDMGroup after reorder = %+v err=%v, want SortOrder=5", reordered, err)
	}

	// Reorder the ruleset to sort_order=3.
	rsReorderBody, _ := json.Marshal(DMRulesetWire{Name: rs.Name, Mode: rs.Mode, Disabled: rs.Disabled, SortOrder: 3})
	req = httptest.NewRequest(http.MethodPut, "/api/datamanager/rulesets/"+rs.ID, bytes.NewReader(rsReorderBody))
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("reorder ruleset: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	reorderedRS, err := s.configDB.GetDMRuleset(rs.ID)
	if err != nil || reorderedRS == nil || reorderedRS.SortOrder != 3 {
		t.Fatalf("GetDMRuleset after reorder = %+v err=%v, want SortOrder=3", reorderedRS, err)
	}

	// GET .../tree must expose the new sort_order on both node types.
	req = httptest.NewRequest(http.MethodGet, "/api/datamanager/tree", nil)
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	var treeResp struct {
		Tree []DMTreeNode `json:"tree"`
	}
	json.NewDecoder(w.Body).Decode(&treeResp)
	var gotGroup, gotRuleset *DMTreeNode
	for i := range treeResp.Tree {
		if treeResp.Tree[i].ID == group.ID {
			gotGroup = &treeResp.Tree[i]
		}
		if treeResp.Tree[i].ID == rs.ID {
			gotRuleset = &treeResp.Tree[i]
		}
	}
	if gotGroup == nil || gotGroup.SortOrder != 5 {
		t.Errorf("tree group node = %+v, want SortOrder=5", gotGroup)
	}
	if gotRuleset == nil || gotRuleset.SortOrder != 3 {
		t.Errorf("tree ruleset node = %+v, want SortOrder=3", gotRuleset)
	}

	// A rename that DOES carry sort_order forward must not reset it back to 0.
	renameBody, _ := json.Marshal(map[string]any{"name": "Renamed", "disabled": false, "sort_order": gotGroup.SortOrder})
	req = httptest.NewRequest(http.MethodPut, "/api/datamanager/groups/"+group.ID, bytes.NewReader(renameBody))
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	afterRename, err := s.configDB.GetDMGroup(group.ID)
	if err != nil || afterRename == nil || afterRename.SortOrder != 5 {
		t.Fatalf("GetDMGroup after rename = %+v err=%v, want SortOrder still 5", afterRename, err)
	}
}

// TestDataManagerRules_SortOrderRoundTrips covers the rule/action half of
// drag-reorder - PUT already applied SortOrder before this change, this
// just confirms it still does and that GET .../tree reflects it via the
// inlined ruleset wire.
func TestDataManagerRules_SortOrderRoundTrips(t *testing.T) {
	s := newDMRulesTestServer(t)

	rsBody, _ := json.Marshal(DMRulesetWire{Name: "Batman", Mode: "And"})
	req := httptest.NewRequest(http.MethodPost, "/api/datamanager/rulesets", bytes.NewReader(rsBody))
	w := httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	var rs DMRulesetWire
	json.NewDecoder(w.Body).Decode(&rs)

	ruleBody, _ := json.Marshal(DMRuleWire{Field: "Series", Modifier: "Is", Value: "Batman"})
	req = httptest.NewRequest(http.MethodPost, "/api/datamanager/rulesets/"+rs.ID+"/rules", bytes.NewReader(ruleBody))
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	var rule DMRuleWire
	json.NewDecoder(w.Body).Decode(&rule)

	reorderBody, _ := json.Marshal(DMRuleWire{Field: rule.Field, Modifier: rule.Modifier, Value: rule.Value, SortOrder: 7})
	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/datamanager/rules/%d", rule.ID), bytes.NewReader(reorderBody))
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("reorder rule: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/datamanager/tree", nil)
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	var treeResp struct {
		Tree []DMTreeNode `json:"tree"`
	}
	json.NewDecoder(w.Body).Decode(&treeResp)
	if len(treeResp.Tree) != 1 || treeResp.Tree[0].Ruleset == nil || len(treeResp.Tree[0].Ruleset.Rules) != 1 {
		t.Fatalf("tree = %+v, want one ruleset with one rule", treeResp.Tree)
	}
	if got := treeResp.Tree[0].Ruleset.Rules[0].SortOrder; got != 7 {
		t.Errorf("rule sort_order in tree = %d, want 7", got)
	}
}

func TestHandleDMGroupParent_RejectsSelfMove(t *testing.T) {
	s := newDMRulesTestServer(t)

	createBody, _ := json.Marshal(map[string]string{"name": "A"})
	req := httptest.NewRequest(http.MethodPost, "/api/datamanager/groups", bytes.NewReader(createBody))
	w := httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	var group DMTreeNode
	json.NewDecoder(w.Body).Decode(&group)

	moveBody, _ := json.Marshal(map[string]string{"parent_id": group.ID})
	req = httptest.NewRequest(http.MethodPut, "/api/datamanager/groups/"+group.ID+"/parent", bytes.NewReader(moveBody))
	w = httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 moving group into itself, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDataManagerRulesRouter_ConfigDBUnavailable(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/api/datamanager/rulesets", nil)
	w := httptest.NewRecorder()
	s.handleDataManagerRulesRouter(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}
