package storage

import (
	"path/filepath"
	"sync"
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

// TestSQLiteBackend_FallbackEvaluationReusesCachedLibrary is the
// regression test for comic-server-ea5: a matcher type that can't use the
// SQL fast path (here, Tags - matcher_sql.go doesn't translate it) used to
// rebuild the full book/list snapshot from SQL on every single evaluation.
// At real-library scale (hundreds of Tags-based lists) that turned a cold
// list-tree warm-up into a multi-hour stall. cachedLibrary() now builds
// that snapshot once and reuses it until something actually invalidates
// it - proven here behaviorally: a raw SQL edit that bypasses UpdateBook
// (so nothing tells the backend to invalidate its cache) must NOT be
// picked up by a second fallback-path evaluation, while a real UpdateBook
// call (which does invalidate) must be picked up immediately after.
func TestSQLiteBackend_FallbackEvaluationReusesCachedLibrary(t *testing.T) {
	dir := t.TempDir()
	xmlPath := filepath.Join(dir, "ComicDb.xml")
	dbPath := filepath.Join(dir, "test.db")

	lib := &library.ComicLibrary{
		ID: "test-library",
		Books: []library.ComicBook{
			{ID: "book-1", FilePath: "/comics/book1.cbz", Tags: "hero"},
			{ID: "book-2", FilePath: "/comics/book2.cbz", Tags: "villain"},
		},
		ComicLists: []library.ComicListItem{
			{
				ID: "list-1", Name: "Heroes", Type: "ComicSmartListItem", MatcherMode: "And",
				// "Tags" has no SQL translation (matcher_sql.go) - every
				// evaluation of this list hits the cachedLibrary fallback.
				Matchers: []library.ComicBookMatcher{
					{Type: "Tags", MatchOperator: "0", MatchValue: "hero"},
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

	first, err := backend.MatchBooks(list)
	if err != nil || len(first) != 1 || first[0].ID != "book-1" {
		t.Fatalf("first MatchBooks: expected [book-1], got %v (err=%v)", first, err)
	}

	// Bypass UpdateBook entirely - a raw SQL edit the backend has no way
	// to know about, simulating "something changed underneath the cache."
	// Tags live only in the book_tags join table (comic_book.Tags is
	// derived from it on read), so removing the row there is the real
	// "book-1 no longer has the hero tag" edit.
	if _, err := backend.db.Exec(`DELETE FROM book_tags WHERE book_id = 'book-1'`); err != nil {
		t.Fatalf("raw book_tags delete: %v", err)
	}

	second, err := backend.MatchBooks(list)
	if err != nil {
		t.Fatalf("second MatchBooks: %v", err)
	}
	if len(second) != 1 || second[0].ID != "book-1" {
		t.Fatalf("expected the cached snapshot to still be served (book-1 still matching) after an uninvalidating raw edit, got %v", second)
	}

	// A real write through the backend's own API DOES invalidate the
	// cache - the next evaluation must reflect it.
	book1, err := backend.GetBook("book-1")
	if err != nil || book1 == nil {
		t.Fatalf("GetBook(book-1): book=%+v err=%v", book1, err)
	}
	book1.Tags = ""
	if err := backend.UpdateBook(book1); err != nil {
		t.Fatalf("UpdateBook: %v", err)
	}

	third, err := backend.MatchBooks(list)
	if err != nil {
		t.Fatalf("third MatchBooks: %v", err)
	}
	if len(third) != 0 {
		t.Errorf("expected the cache to be invalidated by UpdateBook, got %v still matching", third)
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

// TestSQLiteBackend_ConcurrentFallbackEvaluationsAndCVDataAreRaceFree runs
// many concurrent fallback-path evaluations (comic-server-ea5's
// cachedLibrary) alongside concurrent SetCVData calls under `go test
// -race`: cachedLibrary hands each caller its own *library.ComicLibrary
// wrapper around a shared, read-only Books/ComicLists slice pair, with
// cvData set on that per-call wrapper alone - an earlier version of this
// fix mutated the SHARED cached struct's cvData field directly, which is
// exactly the kind of race this test would have caught.
func TestSQLiteBackend_ConcurrentFallbackEvaluationsAndCVDataAreRaceFree(t *testing.T) {
	dir := t.TempDir()
	xmlPath := filepath.Join(dir, "ComicDb.xml")
	dbPath := filepath.Join(dir, "test.db")

	lib := &library.ComicLibrary{
		ID: "test-library",
		Books: []library.ComicBook{
			{ID: "book-1", FilePath: "/comics/book1.cbz", Tags: "hero"},
		},
		ComicLists: []library.ComicListItem{
			{
				ID: "list-1", Name: "Heroes", Type: "ComicSmartListItem", MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "Tags", MatchOperator: "0", MatchValue: "hero"},
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

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := backend.MatchBooks(list); err != nil {
				t.Errorf("MatchBooks: %v", err)
			}
		}()
	}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			backend.SetCVData(map[string]*library.CVCompleteness{
				"book-1": {TotalIssues: i},
			})
		}(i)
	}
	wg.Wait()
}

// TestSQLiteBackend_IdListMembershipSurvivesImportAndReimport is the
// regression test for comic-server-254: a ComicIdListItem (an explicit,
// manually-curated set of book GUIDs - NOT matcher-based) used to have no
// persisted membership at all on the SQL backend, so GetBooksForList
// always returned empty regardless of what the XML said. Covers both a
// fresh import and a reimport (exercising updateList's delete-and-
// reinsert path, not just insertList's), plus that reordering/changing
// membership on an otherwise-identical list is actually detected as a
// change (computeListHash now hashes BookIds too - previously an XML edit
// that only changed an id list's members, with the same book_count,
// would never have been picked up as "changed" on reimport at all).
func TestSQLiteBackend_IdListMembershipSurvivesImportAndReimport(t *testing.T) {
	dir := t.TempDir()
	xmlPath := filepath.Join(dir, "ComicDb.xml")
	dbPath := filepath.Join(dir, "test.db")

	lib := &library.ComicLibrary{
		ID: "test-library",
		Books: []library.ComicBook{
			{ID: "book-1", FilePath: "/comics/book1.cbz", Title: "One"},
			{ID: "book-2", FilePath: "/comics/book2.cbz", Title: "Two"},
			{ID: "book-3", FilePath: "/comics/book3.cbz", Title: "Three"},
		},
		ComicLists: []library.ComicListItem{
			{
				ID:      "to-read",
				Name:    "To Read",
				Type:    "ComicIdListItem",
				BookIds: []string{"book-2", "book-1"}, // deliberately out of insertion order
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

	list, err := backend.FindListByID("to-read")
	if err != nil || list == nil {
		t.Fatalf("FindListByID: list=%+v err=%v", list, err)
	}
	if got := list.BookIds; len(got) != 2 || got[0] != "book-2" || got[1] != "book-1" {
		t.Fatalf("expected FindListByID's BookIds to be [book-2 book-1] (order preserved), got %v", got)
	}

	books, err := backend.GetBooksForList(list)
	if err != nil {
		t.Fatalf("GetBooksForList: %v", err)
	}
	if len(books) != 2 {
		t.Fatalf("expected 2 books for the id list, got %d: %v", len(books), books)
	}
	gotIDs := map[string]bool{books[0].ID: true, books[1].ID: true}
	if !gotIDs["book-1"] || !gotIDs["book-2"] {
		t.Errorf("expected book-1 and book-2, got %v", gotIDs)
	}

	// Reimport with changed membership (same book_count, so anything that
	// only compares BookCount rather than the actual member IDs would
	// miss this) - must be detected and applied.
	lib.ComicLists[0].BookIds = []string{"book-3", "book-1"}
	if err := library.SaveLibrary(xmlPath, lib); err != nil {
		t.Fatalf("SaveLibrary (update): %v", err)
	}
	if err := backend.Reload(); err != nil {
		t.Fatalf("Reload (second): %v", err)
	}

	list2, err := backend.FindListByID("to-read")
	if err != nil || list2 == nil {
		t.Fatalf("FindListByID (after reimport): list=%+v err=%v", list2, err)
	}
	books2, err := backend.GetBooksForList(list2)
	if err != nil {
		t.Fatalf("GetBooksForList (after reimport): %v", err)
	}
	if len(books2) != 2 {
		t.Fatalf("expected 2 books after reimport, got %d: %v", len(books2), books2)
	}
	gotIDs2 := map[string]bool{books2[0].ID: true, books2[1].ID: true}
	if !gotIDs2["book-1"] || !gotIDs2["book-3"] {
		t.Errorf("expected membership to update to book-1 and book-3 after reimport, got %v", gotIDs2)
	}
	if gotIDs2["book-2"] {
		t.Errorf("expected book-2 to no longer be a member after reimport, got %v", gotIDs2)
	}
}
