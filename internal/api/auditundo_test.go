package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
)

func postUndo(t *testing.T, s *Server, req UndoRequest) (int, UndoResponse, string) {
	t.Helper()
	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	s.handleAuditUndo(w, httptest.NewRequest(http.MethodPost, "/api/library/audit/undo", bytes.NewReader(body)))
	var out UndoResponse
	if w.Code == http.StatusOK {
		_ = json.Unmarshal(w.Body.Bytes(), &out)
	}
	return w.Code, out, w.Body.String()
}

// editBook changes the book through the audited backend and returns the
// newest audit entry id.
func editBook(t *testing.T, s *Server, id string, mutate func(*library.ComicBook)) int64 {
	t.Helper()
	b := s.audited(audit.ActionEdit)
	book, _ := b.GetBook(id)
	mutate(book)
	if err := b.UpdateBook(book); err != nil {
		t.Fatal(err)
	}
	rows, _, _ := s.configDB.ListAudit(configdb.AuditFilter{BookID: id, Limit: 1})
	return rows[0].ID
}

func TestAuditUndo_PreviewApplyAndAudit(t *testing.T) {
	s := newAuditTestServer(t, library.ComicBook{ID: "1", Series: "Old", Year: 2000, Rating: 3, Checked: false,
		CustomValuesStore: ",keep=1", FilePath: "a.cbz"})
	entry := editBook(t, s, "1", func(b *library.ComicBook) {
		b.Series, b.Year, b.Rating, b.Checked = "New", 2010, 4.5, true
		b.CustomValuesStore = library.SetCustomValue(b.CustomValuesStore, "tag", "x")
		b.Pages = []library.ComicPageInfo{{Image: 0}}
		b.ComicInfoIsDirty = true
	})

	// Preview writes nothing.
	code, prev, body := postUndo(t, s, UndoRequest{EntryID: entry})
	if code != http.StatusOK || prev.Applied {
		t.Fatalf("preview: %d %s", code, body)
	}
	status := map[string]string{}
	for _, it := range prev.Items {
		status[it.Field] = it.Status
	}
	for _, f := range []string{"Series", "Year", "Rating", "Checked", "custom:tag"} {
		if status[f] != undoOK {
			t.Errorf("%s status = %q, want ok", f, status[f])
		}
	}
	for _, f := range []string{"Pages", "ComicInfoIsDirty"} {
		if status[f] != undoUnsupported {
			t.Errorf("%s status = %q, want unsupported", f, status[f])
		}
	}
	if cur, _ := s.backend.GetBook("1"); cur.Series != "New" {
		t.Fatal("preview changed the book")
	}

	code, res, body := postUndo(t, s, UndoRequest{EntryID: entry, Apply: true})
	if code != http.StatusOK || !res.Applied {
		t.Fatalf("apply: %d %s", code, body)
	}
	cur, _ := s.backend.GetBook("1")
	if cur.Series != "Old" || cur.Year != 2000 || cur.Rating != 3 || cur.Checked {
		t.Errorf("not restored: %+v", cur)
	}
	if _, ok := library.GetCustomValue(cur.CustomValuesStore, "tag"); ok {
		t.Error("custom value not removed")
	}
	if v, _ := library.GetCustomValue(cur.CustomValuesStore, "keep"); v != "1" {
		t.Error("unrelated custom value lost")
	}
	if len(cur.Pages) != 1 {
		t.Error("unsupported Pages field was touched")
	}
	rows, _, _ := s.configDB.ListAudit(configdb.AuditFilter{Action: audit.ActionUndo})
	if len(rows) != 1 {
		t.Errorf("undo audit rows = %d, want 1", len(rows))
	}

	// Second undo: everything now equals old, nothing to do.
	if code, _, _ := postUndo(t, s, UndoRequest{EntryID: entry, Apply: true}); code != http.StatusConflict {
		t.Errorf("repeat undo = %d, want 409", code)
	}
}

func TestAuditUndo_ConflictNeedsForce(t *testing.T) {
	s := newAuditTestServer(t, library.ComicBook{ID: "1", Series: "A", Publisher: "P"})
	entry := editBook(t, s, "1", func(b *library.ComicBook) { b.Series, b.Publisher = "B", "Q" })
	editBook(t, s, "1", func(b *library.ComicBook) { b.Series = "C" })

	_, prev, _ := postUndo(t, s, UndoRequest{EntryID: entry})
	status := map[string]string{}
	for _, it := range prev.Items {
		status[it.Field] = it.Status
	}
	if status["Series"] != undoConflict || status["Publisher"] != undoOK {
		t.Fatalf("status = %v", status)
	}

	if code, _, _ := postUndo(t, s, UndoRequest{EntryID: entry, Apply: true}); code != http.StatusOK {
		t.Fatalf("apply without force: %d", code)
	}
	cur, _ := s.backend.GetBook("1")
	if cur.Series != "C" || cur.Publisher != "P" {
		t.Errorf("without force: Series=%q Publisher=%q", cur.Series, cur.Publisher)
	}
	if code, _, _ := postUndo(t, s, UndoRequest{EntryID: entry, Apply: true, Force: true, Fields: []string{"Series"}}); code != http.StatusOK {
		t.Fatalf("apply with force: %d", code)
	}
	if cur, _ := s.backend.GetBook("1"); cur.Series != "A" {
		t.Errorf("forced Series = %q", cur.Series)
	}
}

func TestAuditUndo_RefusesBookkeepingAndMissing(t *testing.T) {
	s := newAuditTestServer(t, library.ComicBook{ID: "1", Series: "A"})
	entry := editBook(t, s, "1", func(b *library.ComicBook) {
		b.CustomValuesStore = library.SetCustomValue(b.CustomValuesStore, "comic_server_written_hash", "abc")
	})
	if code, _, _ := postUndo(t, s, UndoRequest{EntryID: entry, Apply: true}); code != http.StatusConflict {
		t.Errorf("bookkeeping undo = %d, want 409", code)
	}
	if code, _, _ := postUndo(t, s, UndoRequest{EntryID: 9999}); code != http.StatusNotFound {
		t.Errorf("missing entry = %d, want 404", code)
	}
	if code, _, _ := postUndo(t, s, UndoRequest{}); code != http.StatusBadRequest {
		t.Errorf("no entry id = %d, want 400", code)
	}
}
