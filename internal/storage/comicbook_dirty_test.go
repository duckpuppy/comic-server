package storage

import (
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestComicBookIsDirty_SurvivesImportAndUpdate(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	lib := &library.ComicLibrary{Books: []library.ComicBook{
		{ID: "dirty", FilePath: "/c/a.cbz", Series: "A", ComicBookIsDirty: true},
		{ID: "clean", FilePath: "/c/b.cbz", Series: "B"},
	}}
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}

	for id, want := range map[string]bool{"dirty": true, "clean": false} {
		b, err := db.GetBook(id)
		if err != nil {
			t.Fatalf("GetBook(%s): %v", id, err)
		}
		if b.ComicBookIsDirty != want {
			t.Errorf("%s: ComicBookIsDirty = %v after import, want %v", id, b.ComicBookIsDirty, want)
		}
	}

	// Re-importing unchanged data must be a no-op (hash stable).
	stats, err := db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.BooksUpdated != 0 || stats.BooksAdded != 0 || stats.BooksUnchanged != 2 {
		t.Errorf("re-import of identical data changed rows: %+v", stats)
	}

	// Flipping the flag in the source is detected as a change and persisted.
	lib.Books[1].ComicBookIsDirty = true
	lib.Books[0].ComicBookIsDirty = false
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]bool{"dirty": false, "clean": true} {
		b, _ := db.GetBook(id)
		if b.ComicBookIsDirty != want {
			t.Errorf("%s: ComicBookIsDirty = %v after change, want %v", id, b.ComicBookIsDirty, want)
		}
	}
}

func TestMigrateV9ToV10_AddsComicBookIsDirtyColumn(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	lib := &library.ComicLibrary{Books: []library.ComicBook{{ID: "b1", FilePath: "/c/a.cbz", Series: "A"}}}
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatal(err)
	}
	// Rewind to a v9-shaped database: no comic_book_is_dirty column.
	if _, err := db.Exec("ALTER TABLE books DROP COLUMN comic_book_is_dirty"); err != nil {
		t.Fatalf("drop column: %v", err)
	}
	if _, err := db.Exec("PRAGMA user_version = 9"); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db, err = Open(dbPath)
	if err != nil {
		t.Fatalf("reopen (migrate) failed: %v", err)
	}
	defer db.Close()

	has, err := db.hasColumn("books", "comic_book_is_dirty")
	if err != nil || !has {
		t.Fatalf("comic_book_is_dirty column missing after migration (has=%v err=%v)", has, err)
	}
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != schemaVersion {
		t.Errorf("user_version = %d (err %v), want %d", version, err, schemaVersion)
	}
	b, err := db.GetBook("b1")
	if err != nil {
		t.Fatalf("existing row unreadable after migration: %v", err)
	}
	if b.Series != "A" || b.ComicBookIsDirty {
		t.Errorf("existing row changed: %+v", b)
	}
}
