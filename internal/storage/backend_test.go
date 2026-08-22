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
