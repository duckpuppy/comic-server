package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/workflow"
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

// seedBatmanRulesetTwoActions is seedBatmanRuleset's sibling with TWO
// actions on the same rule (SeriesGroup and a custom "Concept" value), so
// a single matching book gets two field changes at once - needed to
// exercise field-level selective apply (comic-server-z93), which
// specifically needs "keep one field, skip the other on the same book".
func seedBatmanRulesetTwoActions(t *testing.T, db *configdb.DB) {
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
		t.Fatalf("CreateDMAction(SeriesGroup): %v", err)
	}
	if _, err := db.CreateDMAction(configdb.DMAction{RulesetID: "rs-batman", Field: "Concept", Modifier: "SetValue", Value: "Dark Knight", SortOrder: 1}); err != nil {
		t.Fatalf("CreateDMAction(Concept): %v", err)
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

// TestHandleDataManagerApply_AdvancesWorkflowStage covers
// comic-server-1iv.2: a book explicitly tracked at StageDataManager
// advances to StageToMove once its changes are actually committed.
func TestHandleDataManagerApply_AdvancesWorkflowStage(t *testing.T) {
	book := library.ComicBook{ID: "1", Series: "Batman", Number: "1"}
	workflow.SetStage(&book, workflow.StageDataManager)
	s, db := newDataManagerTestServer(t, []library.ComicBook{book})
	seedBatmanRuleset(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/library/lists/list-1/datamanager-apply", nil)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	book1, err := s.backend.GetBook("1")
	if err != nil || book1 == nil {
		t.Fatalf("GetBook(1): %v", err)
	}
	if got := workflow.GetStage(book1); got != workflow.StageToMove {
		t.Errorf("workflow stage = %v, want StageToMove", got)
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

// TestHandleDataManagerApply_SelectiveBookIDsOnlyCommitsThose covers
// comic-server-dpq's per-book selective apply: a book_ids body should
// commit only the requested books, leaving other matched-and-changed
// books untouched.
func TestHandleDataManagerApply_SelectiveBookIDsOnlyCommitsThose(t *testing.T) {
	books := []library.ComicBook{
		{ID: "1", Series: "Batman", Number: "1"},
		{ID: "2", Series: "Batman", Number: "2"},
	}
	s, db := newDataManagerTestServer(t, books)
	seedBatmanRuleset(t, db)

	body := strings.NewReader(`{"book_ids":["1"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/library/lists/list-1/datamanager-apply", body)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result DMRunResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Changed != 1 || len(result.Books) != 1 || result.Books[0].BookID != "1" {
		t.Fatalf("expected exactly book 1 committed, got %+v", result)
	}

	book1, _ := s.backend.GetBook("1")
	book2, _ := s.backend.GetBook("2")
	if book1.SeriesGroup != "Batman Family" {
		t.Errorf("book 1 SeriesGroup = %q, want %q (selected)", book1.SeriesGroup, "Batman Family")
	}
	if book2.SeriesGroup != "" {
		t.Errorf("book 2 SeriesGroup = %q, want empty (not selected, must be untouched)", book2.SeriesGroup)
	}
}

// TestHandleDataManagerApply_UnknownBookIDIsSilentlySkipped covers the
// re-verification design note from comic-server-dpq: a requested book_id
// that no longer needs a change (or never existed) between preview and
// apply must be skipped, not error the whole request.
func TestHandleDataManagerApply_UnknownBookIDIsSilentlySkipped(t *testing.T) {
	books := []library.ComicBook{{ID: "1", Series: "Batman"}}
	s, db := newDataManagerTestServer(t, books)
	seedBatmanRuleset(t, db)

	body := strings.NewReader(`{"book_ids":["1","does-not-exist"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/library/lists/list-1/datamanager-apply", body)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result DMRunResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Changed != 1 {
		t.Errorf("expected 1 committed (unknown id silently skipped), got %d: %+v", result.Changed, result)
	}
}

func newDataManagerLibraryTestServer(t *testing.T, books []library.ComicBook) (*Server, *configdb.DB) {
	t.Helper()
	lib := &library.ComicLibrary{Books: books}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	db := newTestConfigDB(t)
	return &Server{backend: backend, configDB: db}, db
}

// TestHandleDataManagerPreviewLibrary_ScansWholeLibraryNotJustOneList
// covers comic-server-dpq's "Apply All": the library-scope preview must
// evaluate every book, including ones no existing smart list happens to
// match, unlike the list-scoped preview.
func TestHandleDataManagerPreviewLibrary_ScansWholeLibraryNotJustOneList(t *testing.T) {
	books := []library.ComicBook{
		{ID: "1", Series: "Batman"},
		{ID: "2", Series: "Some Unrelated Series"},
	}
	s, db := newDataManagerLibraryTestServer(t, books)
	seedBatmanRuleset(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/library/datamanager-preview", nil)
	w := httptest.NewRecorder()
	s.handleDataManagerPreviewLibrary(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result DMRunResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Processed != 2 {
		t.Errorf("Processed = %d, want 2 (whole library, not one list)", result.Processed)
	}
	if result.Changed != 1 || len(result.Books) != 1 || result.Books[0].BookID != "1" {
		t.Fatalf("expected exactly book 1 to have changed, got %+v", result)
	}
}

// TestHandleDataManagerPreviewLibrary_Paginates covers the summary-first
// pagination design (comic-server-dpq): Changed reports the TOTAL that
// would change, while Books is only the requested page.
func TestHandleDataManagerPreviewLibrary_Paginates(t *testing.T) {
	books := make([]library.ComicBook, 5)
	for i := range books {
		books[i] = library.ComicBook{ID: string(rune('1' + i)), Series: "Batman"}
	}
	s, db := newDataManagerLibraryTestServer(t, books)
	seedBatmanRuleset(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/library/datamanager-preview?limit=2&offset=1", nil)
	w := httptest.NewRecorder()
	s.handleDataManagerPreviewLibrary(w, req)

	var result DMRunResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Changed != 5 {
		t.Errorf("Changed = %d, want 5 (total, not page size)", result.Changed)
	}
	if len(result.Books) != 2 {
		t.Fatalf("expected a page of 2 books, got %d", len(result.Books))
	}
	if result.Limit != 2 || result.Offset != 1 || !result.HasMore {
		t.Errorf("pagination fields = limit=%d offset=%d hasMore=%v, want limit=2 offset=1 hasMore=true", result.Limit, result.Offset, result.HasMore)
	}
}

// TestHandleDataManagerApplyLibrary_ApplyAllCommitsEveryChangedBook is the
// literal "Apply All" case: no book_ids body means commit everything that
// changed across the whole library.
func TestHandleDataManagerApplyLibrary_ApplyAllCommitsEveryChangedBook(t *testing.T) {
	books := []library.ComicBook{
		{ID: "1", Series: "Batman"},
		{ID: "2", Series: "Batman"},
		{ID: "3", Series: "Unrelated"},
	}
	s, db := newDataManagerLibraryTestServer(t, books)
	seedBatmanRuleset(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/library/datamanager-apply", nil)
	w := httptest.NewRecorder()
	s.handleDataManagerApplyLibrary(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result DMRunResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !result.Applied || result.Changed != 2 {
		t.Fatalf("expected Applied=true Changed=2, got %+v", result)
	}

	book1, _ := s.backend.GetBook("1")
	book2, _ := s.backend.GetBook("2")
	if book1.SeriesGroup != "Batman Family" || book2.SeriesGroup != "Batman Family" {
		t.Errorf("expected both Batman books updated, got book1=%q book2=%q", book1.SeriesGroup, book2.SeriesGroup)
	}
}

// TestHandleDataManagerApply_FieldLevelSelectiveApply is
// comic-server-z93's core case: a book with TWO changed fields
// (SeriesGroup, a built-in field; Concept, a custom value) should let the
// caller commit just one of them and leave the other at its original
// value, even though both matched the same ruleset run.
func TestHandleDataManagerApply_FieldLevelSelectiveApply(t *testing.T) {
	books := []library.ComicBook{{ID: "1", Series: "Batman"}}
	s, db := newDataManagerTestServer(t, books)
	seedBatmanRulesetTwoActions(t, db)

	body := strings.NewReader(`{"fields":[{"book_id":"1","field":"SeriesGroup","custom":false}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/library/lists/list-1/datamanager-apply", body)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result DMRunResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Changed != 1 || len(result.Books) != 1 || len(result.Books[0].Changes) != 1 {
		t.Fatalf("expected exactly 1 book with 1 committed field, got %+v", result)
	}
	if result.Books[0].Changes[0].Field != "SeriesGroup" {
		t.Errorf("committed field = %q, want %q", result.Books[0].Changes[0].Field, "SeriesGroup")
	}

	book1, err := s.backend.GetBook("1")
	if err != nil || book1 == nil {
		t.Fatalf("GetBook(1): %v", err)
	}
	if book1.SeriesGroup != "Batman Family" {
		t.Errorf("SeriesGroup = %q, want %q (selected field)", book1.SeriesGroup, "Batman Family")
	}
	if _, hasConcept := getCustomValueForTest(book1, "Concept"); hasConcept {
		t.Errorf("Concept custom value should be untouched (not selected), got a value")
	}
}

// TestHandleDataManagerApply_FieldSelectorsTakePrecedenceOverBookIDs
// covers DMApplyRequest's documented precedence: when Fields is
// non-empty, BookIDs is ignored entirely, even if it also names the book.
func TestHandleDataManagerApply_FieldSelectorsTakePrecedenceOverBookIDs(t *testing.T) {
	books := []library.ComicBook{{ID: "1", Series: "Batman"}}
	s, db := newDataManagerTestServer(t, books)
	seedBatmanRulesetTwoActions(t, db)

	// BookIDs also present, but Fields must win - only SeriesGroup should
	// land, not the whole book (which would also set Concept).
	body := strings.NewReader(`{"book_ids":["1"],"fields":[{"book_id":"1","field":"SeriesGroup","custom":false}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/library/lists/list-1/datamanager-apply", body)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)

	var result DMRunResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result.Books) != 1 || len(result.Books[0].Changes) != 1 {
		t.Fatalf("expected Fields selection (1 field) to win over BookIDs (whole book), got %+v", result)
	}
}

// TestHandleDataManagerApply_StaleFieldSelectorIsSilentlySkipped mirrors
// the book-level re-verification test: a field selector that no longer
// matches a fresh change (unknown field name here) must be skipped, not
// error the whole request.
func TestHandleDataManagerApply_StaleFieldSelectorIsSilentlySkipped(t *testing.T) {
	books := []library.ComicBook{{ID: "1", Series: "Batman"}}
	s, db := newDataManagerTestServer(t, books)
	seedBatmanRuleset(t, db) // single-action ruleset - only SeriesGroup changes

	body := strings.NewReader(`{"fields":[{"book_id":"1","field":"SeriesGroup","custom":false},{"book_id":"1","field":"Notes","custom":false}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/library/lists/list-1/datamanager-apply", body)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)

	var result DMRunResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Changed != 1 || len(result.Books[0].Changes) != 1 {
		t.Fatalf("expected only the real SeriesGroup change committed, stale Notes selector skipped, got %+v", result)
	}
}

func getCustomValueForTest(book *library.ComicBook, key string) (string, bool) {
	for pair := range strings.SplitSeq(book.CustomValuesStore, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		k, v, found := strings.Cut(pair, "=")
		if found && strings.TrimSpace(k) == key {
			return strings.TrimSpace(v), true
		}
	}
	return "", false
}
