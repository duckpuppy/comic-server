package audit

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

func newBackend(t *testing.T, books ...library.ComicBook) (library.Backend, *configdb.DB) {
	t.Helper()
	db, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return library.NewXMLBackendFromLibrary(&library.ComicLibrary{Books: books}, "", nil), db
}

func TestDiff(t *testing.T) {
	before := &library.ComicBook{ID: "1", Series: "A", Year: 2000, Rating: 3, CustomValuesStore: ",x=1,y=2"}
	after := &library.ComicBook{ID: "1", Series: "B", Year: 2000, Rating: 3, CustomValuesStore: ",x=1,y=3,z=9",
		Pages: []library.ComicPageInfo{{Image: 0}}}
	got := map[string]Change{}
	for _, c := range Diff(before, after) {
		got[c.Field] = c
	}
	want := map[string][2]string{"Series": {"A", "B"}, "custom:y": {"2", "3"}, "custom:z": {"", "9"}, "Pages": {"0 items", "1 items"}}
	if len(got) != len(want) {
		t.Fatalf("changes = %+v", got)
	}
	for f, w := range want {
		if got[f].Old != w[0] || got[f].New != w[1] {
			t.Errorf("%s = %+v, want %v", f, got[f], w)
		}
	}
}

func TestDiff_StageKeyIsNotAFieldChange(t *testing.T) {
	a := &library.ComicBook{ID: "1", FilePath: "a.cbz"}
	b := &library.ComicBook{ID: "1", FilePath: "a.cbz"}
	workflow.SetStage(b, workflow.StageScrape)
	if d := Diff(a, b); len(d) != 0 {
		t.Errorf("stage change leaked into field diff: %+v", d)
	}
}

func TestBackend_LogsDiffAndStageOnUpdate(t *testing.T) {
	book := library.ComicBook{ID: "1", Series: "Old", Number: "1", FilePath: "a.cbz"}
	workflow.SetStage(&book, workflow.StageScrape)
	inner, db := newBackend(t, book)
	b := Wrap(inner, ActionScanInfo, db)

	// In-place edit of the book returned by the wrapper must still diff.
	cur, err := b.GetBook("1")
	if err != nil || cur == nil {
		t.Fatal(err)
	}
	cur.Series = "New"
	workflow.SetStage(cur, workflow.StageScanInfo)
	if err := b.UpdateBook(cur); err != nil {
		t.Fatal(err)
	}

	rows, total, err := db.ListAudit(configdb.AuditFilter{BookID: "1"})
	if err != nil || total != 1 {
		t.Fatalf("rows=%d err=%v", total, err)
	}
	r := rows[0]
	if r.Action != ActionScanInfo || r.StageBefore != "scrape" || r.StageAfter != "scan_info" {
		t.Errorf("row = %+v", r)
	}
	if r.Changes == "" || r.Changes == "[]" {
		t.Errorf("expected field changes, got %q", r.Changes)
	}
}

func TestBackend_NoChangeLogsNothing(t *testing.T) {
	inner, db := newBackend(t, library.ComicBook{ID: "1", Series: "S"})
	b := Wrap(inner, ActionScrape, db)
	cur, _ := b.GetBook("1")
	if err := b.UpdateBook(cur); err != nil {
		t.Fatal(err)
	}
	if _, total, _ := db.ListAudit(configdb.AuditFilter{}); total != 0 {
		t.Errorf("logged %d rows for a no-op update", total)
	}
}

type failingBackend struct{ library.Backend }

func (failingBackend) UpdateBooks([]*library.ComicBook) error { return errors.New("boom") }

func TestBackend_FailedUpdateLogsNothing(t *testing.T) {
	inner, db := newBackend(t, library.ComicBook{ID: "1", Series: "S"})
	b := Wrap(failingBackend{inner}, ActionScrape, db)
	cur, _ := b.GetBook("1")
	cur.Series = "T"
	if err := b.UpdateBooks([]*library.ComicBook{cur}); err == nil {
		t.Fatal("expected error")
	}
	if _, total, _ := db.ListAudit(configdb.AuditFilter{}); total != 0 {
		t.Errorf("logged %d rows for a failed update", total)
	}
}

func TestWrap_NilSinkReturnsBackendUnchanged(t *testing.T) {
	inner, _ := newBackend(t)
	if Wrap(inner, ActionScrape, nil) != inner {
		t.Error("nil sink should not wrap")
	}
}

func TestBackend_DeleteBookIsLogged(t *testing.T) {
	inner, db := newBackend(t, library.ComicBook{ID: "1", Series: "S", Number: "4", FilePath: "a.cbz"})
	b := Wrap(inner, ActionDuplicateTrash, db)
	if err := b.DeleteBook("1"); err != nil {
		t.Fatal(err)
	}
	rows, total, _ := db.ListAudit(configdb.AuditFilter{BookID: "1"})
	if total != 1 || rows[0].Action != ActionDuplicateTrash || rows[0].Series != "S" {
		t.Fatalf("rows = %+v", rows)
	}
	if got, _ := inner.GetBook("1"); got != nil {
		t.Error("book still present")
	}
}
