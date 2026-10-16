package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

func postBookEdit(t *testing.T, s *Server, id, body string) (int, BookEditResponse, string) {
	t.Helper()
	w := httptest.NewRecorder()
	s.handleBooksRouter(w, httptest.NewRequest(http.MethodPost, "/api/library/books/"+id+"/edit", strings.NewReader(body)))
	var resp BookEditResponse
	if w.Code == http.StatusOK {
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
	}
	return w.Code, resp, w.Body.String()
}

func TestBookEdit_WritesOnlySentFieldsAndAudits(t *testing.T) {
	books := []library.ComicBook{
		{ID: "b1", Series: "Batmn", Number: "1", Publisher: "DC", Summary: "keep", FilePath: "a.cbz"},
	}
	s := newDuplicatesTestServer(t, t.TempDir(), books)

	code, resp, body := postBookEdit(t, s, "b1", `{"fields":{"Series":"  Batman ","Year":2011,"Rating":4.5,"Checked":true,"Publisher":"DC"}}`)
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	if want := []string{"Checked", "Rating", "Series", "Year"}; strings.Join(resp.Changed, ",") != strings.Join(want, ",") {
		t.Errorf("changed = %v, want %v", resp.Changed, want)
	}
	b, _ := s.backend.GetBook("b1")
	if b.Series != "Batman" || b.Year != 2011 || b.Rating != 4.5 || !b.Checked || b.Summary != "keep" || b.Number != "1" {
		t.Errorf("book = %+v", b)
	}
	if !b.ComicInfoIsDirty || !b.ComicBookIsDirty {
		t.Error("dirty flags not set after a ComicInfo field changed")
	}
	rows, _, _ := s.configDB.ListAudit(configdb.AuditFilter{BookID: "b1", Limit: 10})
	if len(rows) != 1 || rows[0].Action != "edit" {
		t.Errorf("audit rows = %+v", rows)
	}

	// Nothing changed: no write, no audit row.
	code, resp, _ = postBookEdit(t, s, "b1", `{"fields":{"Series":"Batman"}}`)
	if code != http.StatusOK || len(resp.Changed) != 0 {
		t.Errorf("no-op edit: %d %v", code, resp.Changed)
	}
	rows, _, _ = s.configDB.ListAudit(configdb.AuditFilter{BookID: "b1", Limit: 10})
	if len(rows) != 1 {
		t.Errorf("no-op edit was audited: %d rows", len(rows))
	}

	// Clearing a field.
	if code, _, body := postBookEdit(t, s, "b1", `{"fields":{"Year":""}}`); code != http.StatusOK {
		t.Fatalf("clear: %d %s", code, body)
	}
	if b, _ := s.backend.GetBook("b1"); b.Year != 0 {
		t.Errorf("Year not cleared: %d", b.Year)
	}
}

func TestBookEdit_Rejects(t *testing.T) {
	s := newDuplicatesTestServer(t, t.TempDir(), []library.ComicBook{{ID: "b1", Series: "X", FilePath: "a.cbz"}})
	for _, body := range []string{
		`{"fields":{"FilePath":"/etc/passwd"}}`,
		`{"fields":{"Rating":6}}`,
		`{"fields":{"Month":13}}`,
		`{"fields":{"Year":"soon"}}`,
		`{"fields":{"Manga":"Maybe"}}`,
		`{"fields":{"Series":["a"]}}`,
		`{"read":"halfway"}`,
		`not json`,
	} {
		if code, _, _ := postBookEdit(t, s, "b1", body); code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", body, code)
		}
	}
	if b, _ := s.backend.GetBook("b1"); b.Series != "X" || b.ComicInfoIsDirty {
		t.Errorf("rejected edit changed the book: %+v", b)
	}
	if code, _, _ := postBookEdit(t, s, "nope", `{"fields":{"Series":"Y"}}`); code != http.StatusNotFound {
		t.Errorf("unknown book: %d", code)
	}
}

func TestApplyBookEdit_ReadUnread(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	b := &library.ComicBook{ID: "b", PageCount: 20, FilePath: "a.cbz"}
	changed, err := applyBookEdit(b, BookEditRequest{Read: "read"}, now)
	if err != nil || len(changed) != 1 || changed[0] != "Read" {
		t.Fatalf("read: %v %v", changed, err)
	}
	if b.IsUnread() || b.LastPageRead != 19 || b.OpenCount != 1 || !b.OpenedTime.Time.Equal(now) {
		t.Errorf("after read: %+v", b)
	}
	if b.ComicInfoIsDirty {
		t.Error("read state is not ComicInfo data")
	}
	if changed, _ := applyBookEdit(b, BookEditRequest{Read: "read"}, now); len(changed) != 0 {
		t.Errorf("read twice changed %v", changed)
	}
	if changed, _ := applyBookEdit(b, BookEditRequest{Read: "unread"}, now); len(changed) != 1 || !b.IsUnread() || b.OpenCount != 0 {
		t.Errorf("unread: %v %+v", changed, b)
	}
}

func TestApplyBookEdit_StageAndDirtyRules(t *testing.T) {
	filed := func() *library.ComicBook {
		b := &library.ComicBook{ID: "b", Series: "S", FilePath: "a.cbz"}
		workflow.SetStage(b, workflow.StageOrganized)
		return b
	}

	// A personal field neither re-files the book nor touches ComicInfo.
	b := filed()
	if _, err := applyBookEdit(b, BookEditRequest{Fields: map[string]any{"BookLocation": "Box 3"}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if workflow.GetStage(b) != workflow.StageOrganized || b.ComicInfoIsDirty || !b.ComicBookIsDirty {
		t.Errorf("BookLocation: stage %v ci %v cb %v", workflow.GetStage(b), b.ComicInfoIsDirty, b.ComicBookIsDirty)
	}

	// Rating goes into ComicInfo.xml but not into the folder layout.
	b = filed()
	_, _ = applyBookEdit(b, BookEditRequest{Fields: map[string]any{"Rating": 3}}, time.Now())
	if workflow.GetStage(b) != workflow.StageOrganized || !b.ComicInfoIsDirty {
		t.Errorf("Rating: stage %v ci %v", workflow.GetStage(b), b.ComicInfoIsDirty)
	}

	// Series can change where the book is filed.
	b = filed()
	_, _ = applyBookEdit(b, BookEditRequest{Fields: map[string]any{"Series": "T"}}, time.Now())
	if workflow.GetStage(b) != workflow.StageToMove {
		t.Errorf("Series: stage %v, want ToMove", workflow.GetStage(b))
	}

	// A book without a file has nothing to mark dirty.
	b = &library.ComicBook{ID: "w", Series: "S"}
	_, _ = applyBookEdit(b, BookEditRequest{Fields: map[string]any{"Series": "T"}}, time.Now())
	if b.ComicInfoIsDirty || b.ComicBookIsDirty {
		t.Error("fileless book marked dirty")
	}

	// Unknown is stored as empty, like the library does.
	b = filed()
	_, _ = applyBookEdit(b, BookEditRequest{Fields: map[string]any{"Manga": "Unknown", "BlackAndWhite": "Yes"}}, time.Now())
	if b.Manga != "" || b.BlackAndWhite != "Yes" {
		t.Errorf("enums: %q %q", b.Manga, b.BlackAndWhite)
	}
}
