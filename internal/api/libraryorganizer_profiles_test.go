package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func newLOProfilesTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{configDB: newTestConfigDB(t)}
}

func TestHandleLOSchema(t *testing.T) {
	s := newLOProfilesTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/library/organize-profiles/schema", nil)
	w := httptest.NewRecorder()
	s.handleLOProfilesRouter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var schema LOSchema
	if err := json.NewDecoder(w.Body).Decode(&schema); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(schema.ExcludeFields) == 0 || len(schema.ExcludeOperators) == 0 {
		t.Errorf("expected non-empty schema, got %+v", schema)
	}
}

// TestLOProfileEditor_FullLifecycle mirrors comic-server-tj6o's own
// full-lifecycle test shape: create a profile, add an exclude rule, edit
// both, fetch, delete, confirm gone - matching comic-server-7ecr's
// acceptance criteria (create/edit/delete a profile entirely through the
// web UI, no CLI import involved).
func TestLOProfileEditor_FullLifecycle(t *testing.T) {
	s := newLOProfilesTestServer(t)

	createBody, _ := json.Marshal(LOProfileWire{
		Name:            "My Profile",
		BaseFolder:      "/comics",
		FolderTemplate:  `{<publisher>}\{<series>}`,
		FileTemplate:    `{<series>} #{<number2>}`,
		CopyMode:        true,
		ExcludeMode:     "Do not",
		ExcludeOperator: "Any",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/library/organize-profiles", bytes.NewReader(createBody))
	w := httptest.NewRecorder()
	s.handleLOProfilesCollection(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create profile: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var created LOProfileWire
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.ID == "" || created.Name != "My Profile" || !created.CopyMode {
		t.Fatalf("created profile = %+v, want ID set, Name=My Profile, CopyMode=true", created)
	}

	// Add an exclude rule.
	ruleBody, _ := json.Marshal(LOExcludeRuleWire{Field: "Tags", Operator: "contains", Value: "Archive"})
	req = httptest.NewRequest(http.MethodPost, "/api/library/organize-profiles/"+created.ID+"/rules", bytes.NewReader(ruleBody))
	w = httptest.NewRecorder()
	s.handleLOProfilesRouter(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create rule: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var rule LOExcludeRuleWire
	if err := json.NewDecoder(w.Body).Decode(&rule); err != nil {
		t.Fatalf("decode rule: %v", err)
	}
	if rule.ID == 0 {
		t.Fatal("expected non-zero rule ID")
	}

	// GET the profile back - should have the rule inlined.
	req = httptest.NewRequest(http.MethodGet, "/api/library/organize-profiles/"+created.ID, nil)
	w = httptest.NewRecorder()
	s.handleLOProfilesRouter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get profile: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var full LOProfileWire
	if err := json.NewDecoder(w.Body).Decode(&full); err != nil {
		t.Fatalf("decode full: %v", err)
	}
	if len(full.ExcludeRules) != 1 || full.ExcludeRules[0].Field != "Tags" {
		t.Fatalf("full profile = %+v, want 1 exclude rule on Tags", full)
	}

	// Edit the rule.
	editRuleBody, _ := json.Marshal(LOExcludeRuleWire{Field: "File Path", Operator: "contains", Value: "0Day"})
	req = httptest.NewRequest(http.MethodPut, "/api/library/organize-rules/"+strconv.FormatInt(rule.ID, 10), bytes.NewReader(editRuleBody))
	w = httptest.NewRecorder()
	s.handleLORuleItem(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("edit rule: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Edit the profile itself.
	editProfileBody, _ := json.Marshal(LOProfileWire{Name: "Renamed", BaseFolder: "/other", CopyMode: false})
	req = httptest.NewRequest(http.MethodPut, "/api/library/organize-profiles/"+created.ID, bytes.NewReader(editProfileBody))
	w = httptest.NewRecorder()
	s.handleLOProfilesRouter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("edit profile: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var edited LOProfileWire
	if err := json.NewDecoder(w.Body).Decode(&edited); err != nil {
		t.Fatalf("decode edited: %v", err)
	}
	if edited.Name != "Renamed" || edited.BaseFolder != "/other" || edited.CopyMode {
		t.Fatalf("edited profile = %+v, want Name=Renamed BaseFolder=/other CopyMode=false", edited)
	}

	// Delete the rule, then the profile.
	req = httptest.NewRequest(http.MethodDelete, "/api/library/organize-rules/"+strconv.FormatInt(rule.ID, 10), nil)
	w = httptest.NewRecorder()
	s.handleLORuleItem(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete rule: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/library/organize-profiles/"+created.ID, nil)
	w = httptest.NewRecorder()
	s.handleLOProfilesRouter(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete profile: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/library/organize-profiles/"+created.ID, nil)
	w = httptest.NewRecorder()
	s.handleLOProfilesRouter(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get deleted profile: expected 404, got %d", w.Code)
	}
}

// TestLOProfileEditor_MonthsAndIllegalCharactersRoundTrip covers
// comic-server-kt4w: create with Months/IllegalCharacters, confirm they
// come back on GET, edit them via PUT, confirm the new values persist
// (not stale/default ones), and confirm an update that omits them clears
// the collection (matches ExcludedEmptyFolders/FailedFields' own
// clear-on-omit behavior).
func TestLOProfileEditor_MonthsAndIllegalCharactersRoundTrip(t *testing.T) {
	s := newLOProfilesTestServer(t)

	createBody, _ := json.Marshal(LOProfileWire{
		Name: "My Profile",
		Months: []LOPairWire{
			{Name: "1", Value: "January"},
			{Name: "13", Value: "Spring"},
		},
		IllegalCharacters: []LOPairWire{
			{Name: `"`, Value: "'"},
			{Name: "/", Value: ""},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/library/organize-profiles", bytes.NewReader(createBody))
	w := httptest.NewRecorder()
	s.handleLOProfilesCollection(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create profile: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var created LOProfileWire
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if len(created.Months) != 2 || created.Months[0].Value != "January" {
		t.Fatalf("created.Months = %+v, want 2 entries incl. 1=January", created.Months)
	}
	if len(created.IllegalCharacters) != 2 || created.IllegalCharacters[1].Name != "/" || created.IllegalCharacters[1].Value != "" {
		t.Fatalf("created.IllegalCharacters = %+v, want 2 entries incl. /=\"\"", created.IllegalCharacters)
	}

	// GET should reflect the same persisted values, not defaults.
	req = httptest.NewRequest(http.MethodGet, "/api/library/organize-profiles/"+created.ID, nil)
	w = httptest.NewRecorder()
	s.handleLOProfilesRouter(w, req)
	var fetched LOProfileWire
	if err := json.NewDecoder(w.Body).Decode(&fetched); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if len(fetched.Months) != 2 || len(fetched.IllegalCharacters) != 2 {
		t.Fatalf("fetched profile = %+v, want Months/IllegalCharacters still populated", fetched)
	}

	// Edit: change a value and add an entry.
	fetched.Months[0].Value = "Jan"
	fetched.IllegalCharacters = append(fetched.IllegalCharacters, LOPairWire{Name: "*", Value: ""})
	editBody, _ := json.Marshal(fetched)
	req = httptest.NewRequest(http.MethodPut, "/api/library/organize-profiles/"+created.ID, bytes.NewReader(editBody))
	w = httptest.NewRecorder()
	s.handleLOProfilesRouter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("edit profile: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var edited LOProfileWire
	if err := json.NewDecoder(w.Body).Decode(&edited); err != nil {
		t.Fatalf("decode edited: %v", err)
	}
	if len(edited.Months) != 2 || edited.Months[0].Value != "Jan" {
		t.Fatalf("edited.Months = %+v, want 1=Jan", edited.Months)
	}
	if len(edited.IllegalCharacters) != 3 {
		t.Fatalf("edited.IllegalCharacters = %+v, want 3 entries", edited.IllegalCharacters)
	}

	// Reload from scratch (fresh GET) to confirm persistence, not just the
	// PUT response echoing the request back.
	req = httptest.NewRequest(http.MethodGet, "/api/library/organize-profiles/"+created.ID, nil)
	w = httptest.NewRecorder()
	s.handleLOProfilesRouter(w, req)
	var reloaded LOProfileWire
	if err := json.NewDecoder(w.Body).Decode(&reloaded); err != nil {
		t.Fatalf("decode reload: %v", err)
	}
	if len(reloaded.Months) != 2 || reloaded.Months[0].Value != "Jan" || len(reloaded.IllegalCharacters) != 3 {
		t.Fatalf("reloaded profile = %+v, want persisted edits", reloaded)
	}

	// An update that omits Months/IllegalCharacters entirely clears them,
	// matching the ordinary form-submission semantics ExcludedEmptyFolders/
	// FailedFields already use.
	clearBody, _ := json.Marshal(LOProfileWire{Name: "My Profile"})
	req = httptest.NewRequest(http.MethodPut, "/api/library/organize-profiles/"+created.ID, bytes.NewReader(clearBody))
	w = httptest.NewRecorder()
	s.handleLOProfilesRouter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("clear profile: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var cleared LOProfileWire
	if err := json.NewDecoder(w.Body).Decode(&cleared); err != nil {
		t.Fatalf("decode cleared: %v", err)
	}
	if len(cleared.Months) != 0 || len(cleared.IllegalCharacters) != 0 {
		t.Fatalf("cleared profile = %+v, want empty Months/IllegalCharacters", cleared)
	}
}

func TestHandleLOProfilesCollection_CreateRejectsEmptyName(t *testing.T) {
	s := newLOProfilesTestServer(t)

	body, _ := json.Marshal(LOProfileWire{Name: "  "})
	req := httptest.NewRequest(http.MethodPost, "/api/library/organize-profiles", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleLOProfilesCollection(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty name, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleLOProfilesCollection_GetStillListsSummaries(t *testing.T) {
	s := newLOProfilesTestServer(t)

	createBody, _ := json.Marshal(LOProfileWire{Name: "Profile A"})
	req := httptest.NewRequest(http.MethodPost, "/api/library/organize-profiles", bytes.NewReader(createBody))
	w := httptest.NewRecorder()
	s.handleLOProfilesCollection(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/library/organize-profiles", nil)
	w = httptest.NewRecorder()
	s.handleLOProfilesCollection(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result struct {
		Profiles []LOProfileSummary `json:"profiles"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Profiles) != 1 || result.Profiles[0].Name != "Profile A" {
		t.Fatalf("profiles = %+v, want [Profile A]", result.Profiles)
	}
}
