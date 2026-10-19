package api

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/storage"
)

// newAuditSQLiteServer is newAuditTestServer on the SQLite backend, which is
// what production uses.
func newAuditSQLiteServer(t *testing.T, book library.ComicBook) *Server {
	t.Helper()
	sb, err := storage.NewSQLiteBackend(filepath.Join(t.TempDir(), "lib.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sb.Close() })
	if _, err := sb.DB().Import(&library.ComicLibrary{ID: "lib", Books: []library.ComicBook{book}}, storage.ImportOptions{}); err != nil {
		t.Fatal(err)
	}
	db, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &Server{backend: sb, configDB: db, config: &config.Config{}, listCache: library.NewListCache(0)}
}

func TestAuditUndo_SQLiteBackend(t *testing.T) {
	s := newAuditSQLiteServer(t, library.ComicBook{ID: "1", Series: "Old", Year: 2000, Rating: 3,
		CustomValuesStore: ",keep=1", FilePath: "a.cbz"})
	entry := editBook(t, s, "1", func(b *library.ComicBook) {
		b.Series, b.Year, b.Rating = "New", 2010, 4.5
		b.CustomValuesStore = library.SetCustomValue(b.CustomValuesStore, "tag", "x")
	})
	if cur, _ := s.backend.GetBook("1"); cur.Series != "New" {
		t.Fatalf("edit did not stick on SQLite: %+v", cur)
	}

	code, prev, body := postUndo(t, s, UndoRequest{EntryID: entry})
	if code != http.StatusOK || prev.Applied {
		t.Fatalf("preview: %d %s", code, body)
	}
	for _, it := range prev.Items {
		if it.Status != undoOK {
			t.Errorf("%s = %q, want ok (%s)", it.Field, it.Status, it.Reason)
		}
	}

	code, res, body := postUndo(t, s, UndoRequest{EntryID: entry, Apply: true})
	if code != http.StatusOK || !res.Applied {
		t.Fatalf("apply: %d %s", code, body)
	}
	cur, err := s.backend.GetBook("1")
	if err != nil || cur == nil {
		t.Fatalf("GetBook: %v", err)
	}
	if cur.Series != "Old" || cur.Year != 2000 || cur.Rating != 3 {
		t.Errorf("not restored on SQLite: series=%q year=%d rating=%v", cur.Series, cur.Year, cur.Rating)
	}
	if _, ok := library.GetCustomValue(cur.CustomValuesStore, "tag"); ok {
		t.Errorf("custom value tag should be removed again, store=%q", cur.CustomValuesStore)
	}
	if v, _ := library.GetCustomValue(cur.CustomValuesStore, "keep"); v != "1" {
		t.Errorf("unrelated custom value lost, store=%q", cur.CustomValuesStore)
	}

	// Second preview: everything is already back, nothing left to restore.
	code, _, body = postUndo(t, s, UndoRequest{EntryID: entry, Apply: true})
	if code != http.StatusConflict {
		t.Errorf("second apply status = %d, want 409 (nothing restorable): %s", code, body)
	}
}
