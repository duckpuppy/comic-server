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

func TestXMLBackend_Reload_PicksUpExternalChange(t *testing.T) {
	path := newTestXMLLibraryFile(t)

	backend, err := NewXMLBackend(path, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate ComicRack (or another process) rewriting the file externally.
	externalLib := &ComicLibrary{
		ID: "test-library",
		Books: []ComicBook{
			{ID: "book-1", Series: "Batman", Title: "Externally Updated Title"},
			{ID: "book-2", Series: "Robin", Title: "New Book"},
		},
	}
	if err := SaveLibrary(path, externalLib); err != nil {
		t.Fatal(err)
	}

	if err := backend.Reload(); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	if backend.BookCount() != 2 {
		t.Fatalf("expected 2 books after reload, got %d", backend.BookCount())
	}
	book, err := backend.GetBook("book-1")
	if err != nil || book == nil || book.Title != "Externally Updated Title" {
		t.Errorf("expected reloaded book-1 title, got %+v (err=%v)", book, err)
	}
}

func TestXMLBackend_Reload_FlushesPendingChangesFirst(t *testing.T) {
	path := newTestXMLLibraryFile(t)

	backend, err := NewXMLBackend(path, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Dirty an in-memory change comic-server hasn't flushed yet.
	if err := backend.UpdateBook(&ComicBook{ID: "book-1", Series: "Batman", Title: "Our Pending Edit"}); err != nil {
		t.Fatal(err)
	}

	if err := backend.Reload(); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	// The pending edit must have been flushed to disk before the reload
	// read it back, not silently discarded.
	book, err := backend.GetBook("book-1")
	if err != nil || book == nil || book.Title != "Our Pending Edit" {
		t.Errorf("expected pending edit to survive reload, got %+v (err=%v)", book, err)
	}
}

func TestXMLBackend_Reload_WithLibraryCache(t *testing.T) {
	path := newTestXMLLibraryFile(t)

	backend, err := NewXMLBackend(path, time.Hour) // flushInterval > 0 configures a LibraryCache
	if err != nil {
		t.Fatal(err)
	}

	externalLib := &ComicLibrary{
		ID: "test-library",
		Books: []ComicBook{
			{ID: "book-1", Series: "Batman", Title: "Cache Path External Title"},
		},
	}
	if err := SaveLibrary(path, externalLib); err != nil {
		t.Fatal(err)
	}

	if err := backend.Reload(); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	book, err := backend.GetBook("book-1")
	if err != nil || book == nil || book.Title != "Cache Path External Title" {
		t.Errorf("expected reloaded title via cache path, got %+v (err=%v)", book, err)
	}

	// The cache must have been repointed to the new library too, or a
	// subsequent auto-flush would silently overwrite the just-reloaded
	// external change with the stale pre-reload object.
	if backend.Cache().GetLibrary() != backend.Library() {
		t.Error("expected LibraryCache to be repointed to the reloaded library")
	}
}

func TestXMLBackend_Reload_NoLibraryPathErrors(t *testing.T) {
	lib := &ComicLibrary{Books: []ComicBook{{ID: "book-1"}}}
	backend := NewXMLBackendFromLibrary(lib, "", nil)

	if err := backend.Reload(); err == nil {
		t.Error("expected Reload() to error when no library path is configured")
	}
}

func TestXMLBackend_LastWriteTime_UpdatesOnFlush(t *testing.T) {
	path := newTestXMLLibraryFile(t)
	backend, err := NewXMLBackend(path, 0)
	if err != nil {
		t.Fatal(err)
	}

	before := backend.LastWriteTime()

	if err := backend.UpdateBook(&ComicBook{ID: "book-1", Series: "Batman", Title: "New"}); err != nil {
		t.Fatal(err)
	}
	if err := backend.Flush(); err != nil {
		t.Fatal(err)
	}

	if !backend.LastWriteTime().After(before) {
		t.Errorf("expected LastWriteTime to advance after Flush, before=%v after=%v", before, backend.LastWriteTime())
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
