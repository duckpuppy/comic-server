package storage

import (
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestSQLiteBackend_ReloadPicksUpExternalChanges(t *testing.T) {
	dir := t.TempDir()
	xmlPath := filepath.Join(dir, "ComicDb.xml")
	dbPath := filepath.Join(dir, "test.db")

	if err := library.SaveLibrary(xmlPath, &library.ComicLibrary{
		ID: "test-library",
		Books: []library.ComicBook{
			{ID: "book-1", FilePath: "/comics/book1.cbz", Title: "Book One"},
		},
	}); err != nil {
		t.Fatalf("SaveLibrary: %v", err)
	}

	backend, err := NewSQLiteBackend(dbPath, xmlPath)
	if err != nil {
		t.Fatalf("NewSQLiteBackend: %v", err)
	}
	defer backend.Close()

	if got := backend.BookCount(); got != 0 {
		t.Fatalf("expected 0 books before any import, got %d", got)
	}

	if err := backend.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := backend.BookCount(); got != 1 {
		t.Fatalf("expected 1 book after first reload, got %d", got)
	}

	// Simulate an external edit (e.g. ComicRack adding a book) and confirm
	// a second Reload picks it up without recreating the backend.
	if err := library.SaveLibrary(xmlPath, &library.ComicLibrary{
		ID: "test-library",
		Books: []library.ComicBook{
			{ID: "book-1", FilePath: "/comics/book1.cbz", Title: "Book One"},
			{ID: "book-2", FilePath: "/comics/book2.cbz", Title: "Book Two"},
		},
	}); err != nil {
		t.Fatalf("SaveLibrary (update): %v", err)
	}

	if err := backend.Reload(); err != nil {
		t.Fatalf("Reload (second): %v", err)
	}
	if got := backend.BookCount(); got != 2 {
		t.Errorf("expected 2 books after second reload, got %d", got)
	}

	book, err := backend.GetBook("book-2")
	if err != nil || book == nil || book.Title != "Book Two" {
		t.Errorf("expected book-2 to be present after reload, got %+v (err=%v)", book, err)
	}
}

// TestSQLiteBackend_SoftDeletedBookInvisibleToSmartLists is the
// backend-level regression test for comic-server-b53: a book removed from
// the XML and soft-deleted must not appear in smart list evaluation
// (MatchBooks/GetBooksForList), not just in the raw table.
func TestSQLiteBackend_SoftDeletedBookInvisibleToSmartLists(t *testing.T) {
	dir := t.TempDir()
	xmlPath := filepath.Join(dir, "ComicDb.xml")
	dbPath := filepath.Join(dir, "test.db")

	lib := &library.ComicLibrary{
		ID: "test-library",
		Books: []library.ComicBook{
			{ID: "book-1", FilePath: "/comics/batman1.cbz", Series: "Batman"},
			{ID: "book-2", FilePath: "/comics/batman2.cbz", Series: "Batman"},
		},
		ComicLists: []library.ComicListItem{
			{
				ID: "list-1", Name: "Batman", Type: "ComicSmartListItem", MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "ComicBookSeriesMatcher", MatchOperator: "1", MatchValue: "Batman"},
				},
			},
		},
	}
	if err := library.SaveLibrary(xmlPath, lib); err != nil {
		t.Fatalf("SaveLibrary: %v", err)
	}

	backend, err := NewSQLiteBackend(dbPath, xmlPath)
	if err != nil {
		t.Fatalf("NewSQLiteBackend: %v", err)
	}
	defer backend.Close()
	if err := backend.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	list, err := backend.FindListByID("list-1")
	if err != nil || list == nil {
		t.Fatalf("FindListByID: list=%+v err=%v", list, err)
	}
	before, err := backend.MatchBooks(list)
	if err != nil || len(before) != 2 {
		t.Fatalf("expected 2 matches before removal, got %d (err=%v)", len(before), err)
	}

	// Remove book-2 from the XML and reimport - it should soft-delete.
	lib.Books = lib.Books[:1]
	if err := library.SaveLibrary(xmlPath, lib); err != nil {
		t.Fatalf("SaveLibrary (removal): %v", err)
	}
	if err := backend.Reload(); err != nil {
		t.Fatalf("Reload (after removal): %v", err)
	}

	after, err := backend.MatchBooks(list)
	if err != nil {
		t.Fatalf("MatchBooks after removal: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("expected 1 match after removal, got %d", len(after))
	}
	if after[0].ID != "book-1" {
		t.Errorf("expected the remaining match to be book-1, got %s", after[0].ID)
	}

	// GetBooksForList goes through a different in-memory snapshot path
	// (tempLibraryLocked, not the matcher-SQL-translation shortcut) -
	// cover it too, since queryBooks' filter needs to hold for both.
	viaGetBooksForList, err := backend.GetBooksForList(list)
	if err != nil || len(viaGetBooksForList) != 1 {
		t.Errorf("GetBooksForList: expected 1 match, got %d (err=%v)", len(viaGetBooksForList), err)
	}
}

func TestSQLiteBackend_ReloadErrorsWithoutXMLPath(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	backend, err := NewSQLiteBackend(dbPath, "")
	if err != nil {
		t.Fatalf("NewSQLiteBackend: %v", err)
	}
	defer backend.Close()

	if err := backend.Reload(); err == nil {
		t.Error("expected Reload to error when no XML source path was configured")
	}
}
