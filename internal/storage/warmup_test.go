package storage

import (
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

// newSQLiteBackendFixtureConcrete is like newSQLiteBackendFixture but
// returns the concrete *SQLiteBackend type, needed to reach the
// WarmUp/IsWarm/NotReadyLists methods that aren't part of library.Backend.
func newSQLiteBackendFixtureConcrete(t *testing.T) *SQLiteBackend {
	t.Helper()
	dir := t.TempDir()
	xmlPath := filepath.Join(dir, "ComicDb.xml")
	dbPath := filepath.Join(dir, "test.db")
	if err := library.SaveLibrary(xmlPath, fixtureLibrary()); err != nil {
		t.Fatalf("SaveLibrary: %v", err)
	}
	backend, err := NewSQLiteBackend(dbPath, xmlPath)
	if err != nil {
		t.Fatalf("NewSQLiteBackend: %v", err)
	}
	if err := backend.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	t.Cleanup(func() { backend.Close() })
	return backend
}

// TestSQLiteBackend_WarmUp exercises the comic-server-jrn readiness
// tracking end to end through a real Reload() (per the comic-server-hha
// lesson: hand-building lists in memory instead of round-tripping them
// through Import can hide bugs that only show up on the real path).
//
// The fixture's "list-reading-1" (a ComicReadingList) always needs the
// shared full-library snapshot (readingLists/id-lists never use the SQL
// fast path), while "list-smart-batman" (Series+Year matchers) always
// translates to a scoped SQL query and so never needs it - this gives one
// list that should report not-ready while cold and one that never does.
func TestSQLiteBackend_WarmUp(t *testing.T) {
	backend := newSQLiteBackendFixtureConcrete(t)

	readingList, err := backend.FindListByID("list-reading-1")
	if err != nil || readingList == nil {
		t.Fatalf("FindListByID(list-reading-1): %v, %v", readingList, err)
	}
	smartList, err := backend.FindListByID("list-smart-batman")
	if err != nil || smartList == nil {
		t.Fatalf("FindListByID(list-smart-batman): %v, %v", smartList, err)
	}

	if backend.IsWarm() {
		t.Fatal("expected backend to be cold immediately after the initial Reload")
	}

	notReady := backend.NotReadyLists([]*library.ComicListItem{readingList, smartList})
	if len(notReady) != 1 || notReady[0] != "list-reading-1" {
		t.Fatalf("expected only list-reading-1 to be not-ready while cold, got %v", notReady)
	}

	if err := backend.WarmUp(); err != nil {
		t.Fatalf("WarmUp: %v", err)
	}
	if !backend.IsWarm() {
		t.Fatal("expected backend to be warm after WarmUp")
	}
	if notReady := backend.NotReadyLists([]*library.ComicListItem{readingList, smartList}); len(notReady) != 0 {
		t.Fatalf("expected no not-ready lists once warm, got %v", notReady)
	}

	// A subsequent Reload must mark the snapshot stale again - readiness
	// isn't a one-time latch.
	if err := backend.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if backend.IsWarm() {
		t.Fatal("expected backend to be cold again immediately after a second Reload")
	}
	notReady = backend.NotReadyLists([]*library.ComicListItem{readingList, smartList})
	if len(notReady) != 1 || notReady[0] != "list-reading-1" {
		t.Fatalf("expected list-reading-1 not-ready again after reload, got %v", notReady)
	}
}

// TestSQLiteBackend_NotReadyLists_EmptyWhenNoSnapshotNeeded confirms a
// device whose assigned lists are all SQL-fast-path smart lists is never
// blocked, even while the backend is cold - the readiness gate should only
// affect lists that would actually pay the slow evaluation cost.
func TestSQLiteBackend_NotReadyLists_EmptyWhenNoSnapshotNeeded(t *testing.T) {
	backend := newSQLiteBackendFixtureConcrete(t)

	smartList, err := backend.FindListByID("list-smart-batman")
	if err != nil || smartList == nil {
		t.Fatalf("FindListByID(list-smart-batman): %v, %v", smartList, err)
	}

	if backend.IsWarm() {
		t.Fatal("expected backend to be cold immediately after Reload")
	}
	if notReady := backend.NotReadyLists([]*library.ComicListItem{smartList}); len(notReady) != 0 {
		t.Fatalf("expected no not-ready lists for an SQL-fast-path-only list, got %v", notReady)
	}
}
