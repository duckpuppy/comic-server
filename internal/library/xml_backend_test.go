package library

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestXMLLibraryFile writes a minimal library XML file to a temp
// directory and returns its path.
func newTestXMLLibraryFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ComicDb.xml")
	lib := &ComicLibrary{
		ID: "test-library",
		Books: []ComicBook{
			{ID: "book-1", Series: "Batman", Title: "Original Title"},
		},
	}
	if err := SaveLibrary(path, lib); err != nil {
		t.Fatal(err)
	}
	return path
}

// ageFile backdates path's mtime so a later write can be detected even when
// the rewritten content would be byte-identical (SaveLibrary is
// deterministic, so a content diff can't distinguish "didn't write" from
// "wrote the same bytes").
func ageFile(t *testing.T, path string) time.Time {
	t.Helper()
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	return old
}

func TestXMLBackend_FlushIsNoOpWhenClean(t *testing.T) {
	path := newTestXMLLibraryFile(t)
	old := ageFile(t, path)

	backend, err := NewXMLBackend(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.Flush(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.ModTime().After(old) {
		t.Error("Flush() rewrote the file with no changes made")
	}
}

func TestXMLBackend_CloseIsNoOpWhenClean(t *testing.T) {
	path := newTestXMLLibraryFile(t)
	old := ageFile(t, path)

	backend, err := NewXMLBackend(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.Close(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.ModTime().After(old) {
		t.Error("Close() rewrote the file with no changes made (this is the --dry-run bug: comic-server-ns7)")
	}
}

func TestXMLBackend_FlushWritesAfterUpdateBook(t *testing.T) {
	path := newTestXMLLibraryFile(t)

	backend, err := NewXMLBackend(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.UpdateBook(&ComicBook{ID: "book-1", Series: "Batman", Title: "New Title"}); err != nil {
		t.Fatal(err)
	}
	if err := backend.Flush(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := LoadLibrary(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Books) != 1 || reloaded.Books[0].Title != "New Title" {
		t.Errorf("expected persisted title update, got %+v", reloaded.Books)
	}
}

func TestXMLBackend_CloseWritesAfterUpdateBook(t *testing.T) {
	path := newTestXMLLibraryFile(t)

	backend, err := NewXMLBackend(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.UpdateBook(&ComicBook{ID: "book-1", Series: "Batman", Title: "New Title"}); err != nil {
		t.Fatal(err)
	}
	if err := backend.Close(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := LoadLibrary(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Books) != 1 || reloaded.Books[0].Title != "New Title" {
		t.Errorf("expected persisted title update, got %+v", reloaded.Books)
	}
}

func TestXMLBackend_MarkDirtyTriggersFlush(t *testing.T) {
	path := newTestXMLLibraryFile(t)

	backend, err := NewXMLBackend(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	old := ageFile(t, path)

	backend.MarkDirty("book-1")
	if err := backend.Flush(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().After(old) {
		t.Error("expected MarkDirty to cause the next Flush() to write, but mtime is unchanged")
	}
}

func TestXMLBackend_FlushClearsDirtyFlag(t *testing.T) {
	path := newTestXMLLibraryFile(t)

	backend, err := NewXMLBackend(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.UpdateBook(&ComicBook{ID: "book-1", Series: "Batman", Title: "New Title"}); err != nil {
		t.Fatal(err)
	}
	if err := backend.Flush(); err != nil {
		t.Fatal(err)
	}

	afterFirstFlush, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Second flush with no further changes should be a no-op.
	if err := backend.Flush(); err != nil {
		t.Fatal(err)
	}
	afterSecondFlush, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterFirstFlush) != string(afterSecondFlush) {
		t.Error("second Flush() rewrote the file even though nothing changed since the first flush")
	}
}

func TestXMLBackend_UpdateBooksMarksDirty(t *testing.T) {
	path := newTestXMLLibraryFile(t)

	backend, err := NewXMLBackend(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.UpdateBooks([]*ComicBook{{ID: "book-1", Series: "Batman", Title: "Batch Title"}}); err != nil {
		t.Fatal(err)
	}
	if err := backend.Flush(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := LoadLibrary(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Books) != 1 || reloaded.Books[0].Title != "Batch Title" {
		t.Errorf("expected persisted title update, got %+v", reloaded.Books)
	}
}

func TestXMLBackend_EmptyLibraryPathFlushIsNoOp(t *testing.T) {
	lib := &ComicLibrary{Books: []ComicBook{{ID: "book-1"}}}
	backend := NewXMLBackendFromLibrary(lib, "", nil)

	if err := backend.UpdateBook(&ComicBook{ID: "book-1", Title: "New Title"}); err != nil {
		t.Fatal(err)
	}
	// Must not attempt to write to an empty path.
	if err := backend.Flush(); err != nil {
		t.Fatalf("Flush() with empty libraryPath should be a no-op, got error: %v", err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close() with empty libraryPath should be a no-op, got error: %v", err)
	}
}
