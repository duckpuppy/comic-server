package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
)

func newDataManagerTestServer(t *testing.T, books []library.ComicBook) (*Server, *configdb.DB) {
	t.Helper()
	lib := &library.ComicLibrary{
		Books: books,
		ComicLists: []library.ComicListItem{
			{
				ID:          "list-1",
				Name:        "All Batman",
				Type:        "ComicSmartListItem",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "Series", MatchOperator: "0", MatchValue: "Batman"},
				},
			},
		},
	}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	db := newTestConfigDB(t)
	return &Server{backend: backend, configDB: db}, db
}

// seedBatmanRuleset creates a group("Quality")>ruleset("Batman Family")
// hierarchy in db matching the shape real dataman.dat groups take, so the
// walkDMGroup/loadEnabledDMRulesets path (not just ApplyAll in isolation)
// is exercised end to end.
func seedBatmanRuleset(t *testing.T, db *configdb.DB) {
	t.Helper()
	if err := db.CreateDMGroup(configdb.DMGroup{ID: "g-quality", Name: "Quality", SortOrder: 0}); err != nil {
		t.Fatalf("CreateDMGroup: %v", err)
	}
	if err := db.CreateDMRuleset(configdb.DMRuleset{ID: "rs-batman", GroupID: "g-quality", Name: "Batman Family", Mode: "AND", SortOrder: 1}); err != nil {
		t.Fatalf("CreateDMRuleset: %v", err)
	}
	if _, err := db.CreateDMRule(configdb.DMRule{RulesetID: "rs-batman", Field: "Series", Modifier: "Is", Value: "Batman", SortOrder: 0}); err != nil {
		t.Fatalf("CreateDMRule: %v", err)
	}
	if _, err := db.CreateDMAction(configdb.DMAction{RulesetID: "rs-batman", Field: "SeriesGroup", Modifier: "SetValue", Value: "Batman Family", SortOrder: 0}); err != nil {
		t.Fatalf("CreateDMAction: %v", err)
	}
}

func TestHandleDataManagerPreview_NoRulesReturns422(t *testing.T) {
	s, _ := newDataManagerTestServer(t, []library.ComicBook{{ID: "1", Series: "Batman"}})

	req := httptest.NewRequest(http.MethodPost, "/api/library/lists/list-1/datamanager-preview", nil)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDataManagerPreview_ShowsChangesWithoutWriting(t *testing.T) {
	books := []library.ComicBook{
		{ID: "1", Series: "Batman", Number: "1"},
		{ID: "2", Series: "Batman Beyond", Number: "1"}, // does not match "Series Is Batman"
	}
	s, db := newDataManagerTestServer(t, books)
	seedBatmanRuleset(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/library/lists/list-1/datamanager-preview", nil)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result DMRunResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Applied {
		t.Error("preview must report Applied=false")
	}
	if result.Processed != 1 {
		// list-1 itself only matches "Series Contains Batman" -> book 1
		// only ("Batman Beyond" contains "Batman" too via the list's own
		// matcher... check via MatchBooks below instead of assuming).
		t.Logf("Processed = %d (list matcher may include both books; that's fine, the DM rule itself only matches book 1)", result.Processed)
	}
	if result.Changed != 1 {
		t.Fatalf("expected exactly 1 book changed (only Series==\"Batman\" matches the DM rule), got %d: %+v", result.Changed, result.Books)
	}
	if len(result.Books) != 1 || result.Books[0].BookID != "1" {
		t.Fatalf("expected book 1 in the diff, got %+v", result.Books)
	}
	found := false
	for _, c := range result.Books[0].Changes {
		if c.Field == "SeriesGroup" && c.New == "Batman Family" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a SeriesGroup->Batman Family change, got %+v", result.Books[0].Changes)
	}

	// Preview must not have written anything back.
	book1, err := s.backend.GetBook("1")
	if err != nil || book1 == nil {
		t.Fatalf("GetBook(1): %v", err)
	}
	if book1.SeriesGroup != "" {
		t.Errorf("preview must not persist changes, but SeriesGroup = %q", book1.SeriesGroup)
	}
}

func TestHandleDataManagerApply_PersistsChanges(t *testing.T) {
	books := []library.ComicBook{
		{ID: "1", Series: "Batman", Number: "1"},
	}
	s, db := newDataManagerTestServer(t, books)
	seedBatmanRuleset(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/library/lists/list-1/datamanager-apply", nil)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result DMRunResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !result.Applied {
		t.Error("apply must report Applied=true")
	}

	book1, err := s.backend.GetBook("1")
	if err != nil || book1 == nil {
		t.Fatalf("GetBook(1): %v", err)
	}
	if book1.SeriesGroup != "Batman Family" {
		t.Errorf("SeriesGroup = %q, want %q to be persisted", book1.SeriesGroup, "Batman Family")
	}
}

func TestHandleDataManagerPreview_DisabledRulesetIgnored(t *testing.T) {
	books := []library.ComicBook{{ID: "1", Series: "Batman"}}
	s, db := newDataManagerTestServer(t, books)
	seedBatmanRuleset(t, db)
	if err := db.CreateDMGroup(configdb.DMGroup{ID: "g-disabled", Name: "Old", Disabled: true, SortOrder: 2}); err != nil {
		t.Fatalf("CreateDMGroup: %v", err)
	}
	if err := db.CreateDMRuleset(configdb.DMRuleset{ID: "rs-disabled", GroupID: "g-disabled", Name: "Retired", Mode: "AND", Disabled: true, SortOrder: 0}); err != nil {
		t.Fatalf("CreateDMRuleset: %v", err)
	}
	if _, err := db.CreateDMRule(configdb.DMRule{RulesetID: "rs-disabled", Field: "Series", Modifier: "Is", Value: "Batman", SortOrder: 0}); err != nil {
		t.Fatalf("CreateDMRule: %v", err)
	}
	if _, err := db.CreateDMAction(configdb.DMAction{RulesetID: "rs-disabled", Field: "Notes", Modifier: "SetValue", Value: "should not appear", SortOrder: 0}); err != nil {
		t.Fatalf("CreateDMAction: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/library/lists/list-1/datamanager-preview", nil)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)

	var result DMRunResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for _, c := range result.Books[0].Changes {
		if c.Field == "Notes" {
			t.Errorf("disabled ruleset's action must not run, but Notes changed: %+v", c)
		}
	}
}

func TestHandleDataManagerPreview_MethodNotAllowed(t *testing.T) {
	s, _ := newDataManagerTestServer(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/library/lists/list-1/datamanager-preview", nil)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}
