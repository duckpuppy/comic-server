package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func postLock(t *testing.T, s *Server, id, field string, locked bool) (int, []string) {
	t.Helper()
	body, _ := json.Marshal(LockFieldRequest{Field: field, Locked: locked})
	w := httptest.NewRecorder()
	s.handleLockField(w, httptest.NewRequest(http.MethodPost, "/api/library/books/"+id+"/lock-field", bytes.NewReader(body)))
	var out struct {
		Locked []string `json:"locked_fields"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out.Locked
}

func TestLockField_SetClearAndValidate(t *testing.T) {
	s := newAuditTestServer(t, library.ComicBook{ID: "1", Series: "S", FilePath: "a.cbz"})

	if code, got := postLock(t, s, "1", "Series", true); code != http.StatusOK || !slices.Equal(got, []string{"Series"}) {
		t.Fatalf("lock Series: %d %v", code, got)
	}
	if _, got := postLock(t, s, "1", "custom:tag", true); !slices.Equal(got, []string{"Series", "custom:tag"}) {
		t.Fatalf("two locks: %v", got)
	}
	// Locking twice changes nothing.
	if _, got := postLock(t, s, "1", "Series", true); len(got) != 2 {
		t.Fatalf("double lock: %v", got)
	}
	for _, bad := range []string{"FilePath", "custom:comic_server_x", "Nonsense", "custom:", "A+B"} {
		if code, _ := postLock(t, s, "1", bad, true); code != http.StatusBadRequest {
			t.Errorf("lock %q status = %d, want 400", bad, code)
		}
	}
	if _, got := postLock(t, s, "1", "Series", false); !slices.Equal(got, []string{"custom:tag"}) {
		t.Fatalf("unlock: %v", got)
	}
	if _, got := postLock(t, s, "1", "custom:tag", false); len(got) != 0 {
		t.Fatalf("unlock last: %v", got)
	}
	if cur, _ := s.backend.GetBook("1"); cur.CustomValuesStore != "" {
		t.Errorf("empty lock list should leave no value, store=%q", cur.CustomValuesStore)
	}
	if code, _ := postLock(t, s, "nope", "Series", true); code != http.StatusNotFound {
		t.Errorf("missing book status = %d, want 404", code)
	}
}

func TestAuditUndo_SkipsLockedFieldsEvenWithForce(t *testing.T) {
	s := newAuditTestServer(t, library.ComicBook{ID: "1", Series: "A", Year: 2000, Rating: 3, FilePath: "a.cbz"})
	entry := editBook(t, s, "1", func(b *library.ComicBook) { b.Series, b.Year, b.Rating = "B", 2010, 4 })
	postLock(t, s, "1", "Year", true)

	_, prev, _ := postUndo(t, s, UndoRequest{EntryID: entry})
	status := map[string]string{}
	for _, it := range prev.Items {
		status[it.Field] = it.Status
	}
	if status["Year"] != undoLocked || status["Series"] != undoOK || status["Rating"] != undoOK {
		t.Fatalf("statuses = %v", status)
	}

	// All-or-nothing apply (no fields list) and force both leave Year alone.
	if code, _, body := postUndo(t, s, UndoRequest{EntryID: entry, Apply: true, Force: true}); code != http.StatusOK {
		t.Fatalf("apply: %d %s", code, body)
	}
	cur, _ := s.backend.GetBook("1")
	if cur.Series != "A" || cur.Rating != 3 || cur.Year != 2010 {
		t.Errorf("series=%q rating=%v year=%d; want A, 3, 2010 (Year locked)", cur.Series, cur.Rating, cur.Year)
	}

	// Unlock: Year is undoable again.
	postLock(t, s, "1", "Year", false)
	if code, _, body := postUndo(t, s, UndoRequest{EntryID: entry, Apply: true}); code != http.StatusOK {
		t.Fatalf("second apply: %d %s", code, body)
	}
	if cur, _ := s.backend.GetBook("1"); cur.Year != 2000 {
		t.Errorf("year = %d after unlock, want 2000", cur.Year)
	}
}

func TestAuditUndo_OnlyLockedLeftIsNothingToUndo(t *testing.T) {
	s := newAuditTestServer(t, library.ComicBook{ID: "1", Series: "A", FilePath: "a.cbz"})
	entry := editBook(t, s, "1", func(b *library.ComicBook) { b.Series = "B" })
	postLock(t, s, "1", "Series", true)
	if code, _, _ := postUndo(t, s, UndoRequest{EntryID: entry, Apply: true}); code != http.StatusConflict {
		t.Errorf("status = %d, want 409", code)
	}
}
