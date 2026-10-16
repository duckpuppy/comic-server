package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
)

func postBulkEdit(t *testing.T, s *Server, body string) (int, BulkEditResponse, string) {
	t.Helper()
	w := httptest.NewRecorder()
	s.handleBulkEdit(w, httptest.NewRequest(http.MethodPost, "/api/library/browse/bulk-edit", strings.NewReader(body)))
	var resp BulkEditResponse
	if w.Code == http.StatusOK {
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
	}
	return w.Code, resp, w.Body.String()
}

func TestMergeListItems(t *testing.T) {
	cases := []struct{ cur, add, remove, want string }{
		{"Action, Drama", "Comedy", "", "Action, Drama, Comedy"},
		{"Action, Drama", "action, Horror", "", "Action, Drama, Horror"}, // case-insensitive dedupe
		{"Action, Drama", "", "drama", "Action"},
		{"Action", "", "Action", ""},
		{"", "A, B", "", "A, B"},
		{"A, B", "C", "A", "B, C"},
	}
	for _, c := range cases {
		if got := mergeListItems(c.cur, splitItems(c.add), splitItems(c.remove)); got != c.want {
			t.Errorf("merge(%q,+%q,-%q) = %q, want %q", c.cur, c.add, c.remove, got, c.want)
		}
	}
}

func TestBookEdit_ListFieldRules(t *testing.T) {
	s := newDuplicatesTestServer(t, t.TempDir(), []library.ComicBook{{ID: "b1", Series: "S", Tags: "x, y", FilePath: "a.cbz"}})
	if code, _, body := postBookEdit(t, s, "b1", `{"list_add":{"Series":"z"}}`); code != http.StatusBadRequest {
		t.Errorf("non-list field: %d %s", code, body)
	}
	if code, _, body := postBookEdit(t, s, "b1", `{"fields":{"Tags":"q"},"list_add":{"Tags":"z"}}`); code != http.StatusBadRequest {
		t.Errorf("replace + add on one field: %d %s", code, body)
	}
	code, resp, body := postBookEdit(t, s, "b1", `{"list_add":{"Tags":"z"},"list_remove":{"Tags":"x"}}`)
	if code != http.StatusOK || strings.Join(resp.Changed, ",") != "Tags" {
		t.Fatalf("add/remove: %d %v %s", code, resp.Changed, body)
	}
	if b, _ := s.backend.GetBook("b1"); b.Tags != "y, z" {
		t.Errorf("Tags = %q", b.Tags)
	}
}

func bulkEditBooks() []library.ComicBook {
	return []library.ComicBook{
		{ID: "b1", Series: "Batman", Number: "1", Publisher: "DC", Tags: "a", FilePath: "1.cbz"},
		{ID: "b2", Series: "Batman", Number: "2", Publisher: "DC Comics", Tags: "a, b", FilePath: "2.cbz"},
		{ID: "b3", Series: "Superman", Number: "1", Publisher: "DC", FilePath: "3.cbz"},
	}
}

func TestBulkEdit_PreviewWritesNothing(t *testing.T) {
	s := newDuplicatesTestServer(t, t.TempDir(), bulkEditBooks())
	code, resp, body := postBulkEdit(t, s, `{"book_ids":["b1","b2","b3"],"edit":{"fields":{"Publisher":"DC"}}}`)
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	if resp.Applied || resp.Total != 3 || resp.Changed != 1 || resp.Unchanged != 2 || resp.Fields["Publisher"] != 1 {
		t.Errorf("resp = %+v", resp)
	}
	if b, _ := s.backend.GetBook("b2"); b.Publisher != "DC Comics" {
		t.Errorf("preview wrote: Publisher = %q", b.Publisher)
	}
	rows, _, _ := s.configDB.ListAudit(configdb.AuditFilter{Limit: 10})
	if len(rows) != 0 {
		t.Errorf("preview was audited: %d rows", len(rows))
	}
}

func TestBulkEdit_ApplyByIDsAudits(t *testing.T) {
	s := newDuplicatesTestServer(t, t.TempDir(), bulkEditBooks())
	code, resp, body := postBulkEdit(t, s, `{"book_ids":["b1","b2"],"expected_count":2,"apply":true,
		"edit":{"fields":{"Publisher":"DC Comics","Year":1940},"list_add":{"Tags":"c"},"list_remove":{"Tags":"a"}}}`)
	if code != http.StatusOK || !resp.Applied || resp.Changed != 2 {
		t.Fatalf("status %d resp %+v: %s", code, resp, body)
	}
	b1, _ := s.backend.GetBook("b1")
	b2, _ := s.backend.GetBook("b2")
	b3, _ := s.backend.GetBook("b3")
	if b1.Publisher != "DC Comics" || b1.Year != 1940 || b1.Tags != "c" || b2.Tags != "b, c" || b3.Year != 0 {
		t.Errorf("books = %+v / %+v / %+v", b1, b2, b3)
	}
	for _, id := range []string{"b1", "b2"} {
		rows, _, _ := s.configDB.ListAudit(configdb.AuditFilter{BookID: id, Limit: 10})
		if len(rows) == 0 || rows[0].Action != "edit" {
			t.Errorf("book %s audit rows = %+v", id, rows)
		}
	}
}

func TestBulkEdit_ApplyByBrowseFilterAndExpectedCount(t *testing.T) {
	s := newDuplicatesTestServer(t, t.TempDir(), bulkEditBooks())
	filter := `"browse":{"show_all":true,"query":"batman"},`
	if code, _, body := postBulkEdit(t, s, `{`+filter+`"expected_count":3,"apply":true,"edit":{"fields":{"Genre":"Hero"}}}`); code != http.StatusConflict {
		t.Fatalf("wrong expected_count: %d %s", code, body)
	}
	code, resp, body := postBulkEdit(t, s, `{`+filter+`"expected_count":2,"apply":true,"edit":{"fields":{"Genre":"Hero"}}}`)
	if code != http.StatusOK || resp.Changed != 2 {
		t.Fatalf("apply: %d %+v %s", code, resp, body)
	}
	if b, _ := s.backend.GetBook("b3"); b.Genre != "" {
		t.Errorf("Superman was outside the filter but got Genre %q", b.Genre)
	}
}

func TestBulkEdit_BadValueWritesNothingAndEmptyScopeRefused(t *testing.T) {
	s := newDuplicatesTestServer(t, t.TempDir(), bulkEditBooks())
	code, _, _ := postBulkEdit(t, s, `{"book_ids":["b1","b2"],"expected_count":2,"apply":true,"edit":{"fields":{"Rating":9}}}`)
	if code != http.StatusBadRequest {
		t.Errorf("bad rating: %d", code)
	}
	if b, _ := s.backend.GetBook("b1"); b.Rating != 0 {
		t.Errorf("Rating written despite error: %v", b.Rating)
	}
	// No matchers and no show_all means no books, never "everything".
	if code, _, _ := postBulkEdit(t, s, `{"browse":{},"edit":{"fields":{"Genre":"x"}}}`); code != http.StatusBadRequest {
		t.Errorf("empty scope: %d", code)
	}
	if code, _, _ := postBulkEdit(t, s, `{"book_ids":["b1"],"edit":{}}`); code != http.StatusBadRequest {
		t.Errorf("empty edit: %d", code)
	}
}

func TestBulkEdit_MarkRead(t *testing.T) {
	s := newDuplicatesTestServer(t, t.TempDir(), bulkEditBooks())
	code, resp, body := postBulkEdit(t, s, `{"book_ids":["b1","b2"],"expected_count":2,"apply":true,"edit":{"read":"read"}}`)
	if code != http.StatusOK || resp.Changed != 2 || resp.Fields["Read"] != 2 {
		t.Fatalf("%d %+v %s", code, resp, body)
	}
	if b, _ := s.backend.GetBook("b1"); b.IsUnread() {
		t.Error("b1 still unread")
	}
}
