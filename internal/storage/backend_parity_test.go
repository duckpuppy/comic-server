package storage

import (
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

// This file runs the same behavioral assertions against both library.Backend
// implementations (XMLBackend and SQLiteBackend) using an identical fixture,
// to validate SQLiteBackend's read/write semantics and catch future
// backend-parity regressions (see comic-server-znn).
//
// Known intentional/accidental differences between the two backends that
// this suite does NOT assert are identical (see the dedicated tests at the
// bottom of this file for what actually happens on each backend):
//   - CreateList with a duplicate ID: both return an error, but the message
//     differs (XML pre-checks; SQLite relies on the PK constraint).
//   - UpdateList/DeleteList/MoveList with a nonexistent ID: XMLBackend
//     returns an error; SQLiteBackend's UPDATE/DELETE affects zero rows and
//     returns nil. This is a real behavioral gap (SQLite silently no-ops),
//     not a deliberate design choice - tracked here rather than silently
//     assumed away.
// BaseListId scoping (comic-server-hha) used to diverge here too - fixed by
// including ComicLists in the temporary library MatchBooks/GetBooksForList
// build, so it's covered by the shared suite (TestBackend_MatchBooks) now.

func fixtureLibrary() *library.ComicLibrary {
	return &library.ComicLibrary{
		ID:   "fixture-library",
		Name: "Fixture Library",
		Books: []library.ComicBook{
			{ID: "book-1", FilePath: "/comics/batman-1.cbz", Title: "Batman #1", Series: "Batman", Publisher: "DC Comics", Year: 2020, Rating: 4},
			{ID: "book-2", FilePath: "/comics/batman-2.cbz", Title: "Batman #2", Series: "Batman", Publisher: "DC Comics", Year: 2019},
			{ID: "book-3", FilePath: "/comics/superman-1.cbz", Title: "Superman #1", Series: "Superman", Publisher: "DC Comics", Year: 2020},
			{ID: "book-4", FilePath: "/comics/spiderman-1.cbz", Title: "Spider-Man #1", Series: "Spider-Man", Publisher: "Marvel Comics", Year: 2021},
		},
		ComicLists: []library.ComicListItem{
			{
				Type:        "ComicSmartListItem",
				ID:          "list-smart-batman",
				Name:        "Recent Batman",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "Series", MatchOperator: "0", MatchValue: "Batman"},
					{Type: "Year", MatchOperator: "1", MatchValue: "2019"}, // Greater than 2019
				},
			},
			{
				Type: "ComicReadingList",
				ID:   "list-reading-1",
				Name: "My Reading",
				Items: []library.ComicReadingListItem{
					{ID: "book-3"},
					{ID: "book-4"},
				},
			},
			{
				Type: "ComicListItemFolder",
				ID:   "list-folder-1",
				Name: "Folder",
				ChildItems: []library.ComicListItem{
					{
						Type:        "ComicSmartListItem",
						ID:          "list-smart-marvel",
						Name:        "Marvel",
						MatcherMode: "And",
						Matchers: []library.ComicBookMatcher{
							{Type: "Publisher", MatchOperator: "0", MatchValue: "Marvel Comics"},
						},
					},
				},
			},
		},
	}
}

// newXMLBackendFixture writes the fixture library to a temp XML file and
// opens it as an XMLBackend (no LibraryCache - flushInterval=0).
func newXMLBackendFixture(t *testing.T) library.Backend {
	t.Helper()
	xmlPath := filepath.Join(t.TempDir(), "ComicDb.xml")
	if err := library.SaveLibrary(xmlPath, fixtureLibrary()); err != nil {
		t.Fatalf("SaveLibrary: %v", err)
	}
	backend, err := library.NewXMLBackend(xmlPath, 0)
	if err != nil {
		t.Fatalf("NewXMLBackend: %v", err)
	}
	t.Cleanup(func() { backend.Close() })
	return backend
}

// newSQLiteBackendFixture imports the fixture library into a fresh SQLite
// database and opens it as a SQLiteBackend.
func newSQLiteBackendFixture(t *testing.T) library.Backend {
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

// backendFixtures maps a backend name to its fixture constructor, so every
// shared test below runs once per backend with t.Run(name, ...).
func backendFixtures() map[string]func(t *testing.T) library.Backend {
	return map[string]func(t *testing.T) library.Backend{
		"XML":    newXMLBackendFixture,
		"SQLite": newSQLiteBackendFixture,
	}
}

func TestBackend_GetBook(t *testing.T) {
	for name, newBackend := range backendFixtures() {
		t.Run(name, func(t *testing.T) {
			backend := newBackend(t)

			book, err := backend.GetBook("book-1")
			if err != nil {
				t.Fatalf("GetBook: %v", err)
			}
			if book == nil {
				t.Fatal("expected book-1 to exist")
			}
			if book.Title != "Batman #1" || book.Series != "Batman" || book.Year != 2020 {
				t.Errorf("unexpected book-1: %+v", book)
			}

			missing, err := backend.GetBook("does-not-exist")
			if err != nil {
				t.Fatalf("GetBook(missing): %v", err)
			}
			if missing != nil {
				t.Errorf("expected nil for missing book, got %+v", missing)
			}
		})
	}
}

func TestBackend_GetAllBooks(t *testing.T) {
	for name, newBackend := range backendFixtures() {
		t.Run(name, func(t *testing.T) {
			backend := newBackend(t)

			books, err := backend.GetAllBooks()
			if err != nil {
				t.Fatalf("GetAllBooks: %v", err)
			}
			if len(books) != 4 {
				t.Fatalf("expected 4 books, got %d", len(books))
			}
			if backend.BookCount() != 4 {
				t.Errorf("BookCount() = %d, want 4", backend.BookCount())
			}
		})
	}
}

func TestBackend_FindListByID(t *testing.T) {
	for name, newBackend := range backendFixtures() {
		t.Run(name, func(t *testing.T) {
			backend := newBackend(t)

			top, err := backend.FindListByID("list-smart-batman")
			if err != nil {
				t.Fatalf("FindListByID(top-level): %v", err)
			}
			if top == nil || top.Name != "Recent Batman" {
				t.Fatalf("expected top-level list, got %+v", top)
			}

			nested, err := backend.FindListByID("list-smart-marvel")
			if err != nil {
				t.Fatalf("FindListByID(nested): %v", err)
			}
			if nested == nil || nested.Name != "Marvel" {
				t.Fatalf("expected nested list to be found recursively, got %+v", nested)
			}

			missing, err := backend.FindListByID("does-not-exist")
			if err != nil {
				t.Fatalf("FindListByID(missing): %v", err)
			}
			if missing != nil {
				t.Errorf("expected nil for missing list, got %+v", missing)
			}
		})
	}
}

func TestBackend_FindList(t *testing.T) {
	for name, newBackend := range backendFixtures() {
		t.Run(name, func(t *testing.T) {
			backend := newBackend(t)

			found, err := backend.FindList("Recent Batman")
			if err != nil {
				t.Fatalf("FindList: %v", err)
			}
			if found == nil || found.ID != "list-smart-batman" {
				t.Fatalf("expected to find 'Recent Batman', got %+v", found)
			}
		})
	}
}

func TestBackend_GetAllLists(t *testing.T) {
	for name, newBackend := range backendFixtures() {
		t.Run(name, func(t *testing.T) {
			backend := newBackend(t)

			lists, err := backend.GetAllLists()
			if err != nil {
				t.Fatalf("GetAllLists: %v", err)
			}
			if len(lists) != 3 {
				t.Fatalf("expected 3 root-level lists, got %d: %+v", len(lists), lists)
			}

			var folder *library.ComicListItem
			for i := range lists {
				if lists[i].ID == "list-folder-1" {
					folder = &lists[i]
				}
			}
			if folder == nil {
				t.Fatal("expected to find list-folder-1 among root lists")
			}
			if len(folder.ChildItems) != 1 || folder.ChildItems[0].ID != "list-smart-marvel" {
				t.Errorf("expected folder to contain list-smart-marvel, got %+v", folder.ChildItems)
			}
		})
	}
}

func TestBackend_MatchBooks(t *testing.T) {
	for name, newBackend := range backendFixtures() {
		t.Run(name, func(t *testing.T) {
			backend := newBackend(t)

			list, err := backend.FindListByID("list-smart-batman")
			if err != nil || list == nil {
				t.Fatalf("FindListByID: list=%+v err=%v", list, err)
			}

			books, err := backend.MatchBooks(list)
			if err != nil {
				t.Fatalf("MatchBooks: %v", err)
			}
			if len(books) != 1 || books[0].ID != "book-1" {
				t.Fatalf("expected exactly [book-1], got %+v", books)
			}

			nested, err := backend.FindListByID("list-smart-marvel")
			if err != nil || nested == nil {
				t.Fatalf("FindListByID(nested): list=%+v err=%v", nested, err)
			}
			marvelBooks, err := backend.MatchBooks(nested)
			if err != nil {
				t.Fatalf("MatchBooks(nested): %v", err)
			}
			if len(marvelBooks) != 1 || marvelBooks[0].ID != "book-4" {
				t.Fatalf("expected exactly [book-4], got %+v", marvelBooks)
			}

			// BaseListId scoping (comic-server-hha): a smart list scoped to
			// "My Reading" (book-3, book-4) filtered further to Superman
			// should resolve the base list and match only book-3, not the
			// unscoped Superman book-3 match against the whole library (which
			// would be the same result here, so also scope to "Marvel" to
			// prove the base list is actually being resolved and not just
			// falling back to the full library).
			scoped := &library.ComicListItem{
				Type:        "ComicSmartListItem",
				ID:          "list-scoped",
				Name:        "Scoped",
				MatcherMode: "And",
				BaseListId:  "list-reading-1",
				Matchers: []library.ComicBookMatcher{
					{Type: "Publisher", MatchOperator: "0", MatchValue: "DC Comics"},
				},
			}
			scopedBooks, err := backend.MatchBooks(scoped)
			if err != nil {
				t.Fatalf("MatchBooks(scoped): %v", err)
			}
			if len(scopedBooks) != 1 || scopedBooks[0].ID != "book-3" {
				t.Fatalf("expected BaseListId scoping to resolve 'My Reading' (book-3, book-4) and match only book-3 (DC Comics), got %+v", scopedBooks)
			}
		})
	}
}

func TestBackend_GetBooksForList(t *testing.T) {
	for name, newBackend := range backendFixtures() {
		t.Run(name, func(t *testing.T) {
			backend := newBackend(t)

			readingList, err := backend.FindListByID("list-reading-1")
			if err != nil || readingList == nil {
				t.Fatalf("FindListByID: list=%+v err=%v", readingList, err)
			}

			books, err := backend.GetBooksForList(readingList)
			if err != nil {
				t.Fatalf("GetBooksForList: %v", err)
			}
			if len(books) != 2 {
				t.Fatalf("expected 2 books in reading list, got %d: %+v", len(books), books)
			}
			gotIDs := map[string]bool{books[0].ID: true, books[1].ID: true}
			if !gotIDs["book-3"] || !gotIDs["book-4"] {
				t.Errorf("expected book-3 and book-4, got %+v", books)
			}
		})
	}
}

func TestBackend_UpdateBook(t *testing.T) {
	for name, newBackend := range backendFixtures() {
		t.Run(name, func(t *testing.T) {
			backend := newBackend(t)

			book, err := backend.GetBook("book-2")
			if err != nil || book == nil {
				t.Fatalf("GetBook: book=%+v err=%v", book, err)
			}
			book.Rating = 5
			book.Checked = true

			if err := backend.UpdateBook(book); err != nil {
				t.Fatalf("UpdateBook: %v", err)
			}
			if err := backend.Flush(); err != nil {
				t.Fatalf("Flush: %v", err)
			}

			updated, err := backend.GetBook("book-2")
			if err != nil || updated == nil {
				t.Fatalf("GetBook after update: book=%+v err=%v", updated, err)
			}
			if updated.Rating != 5 || !updated.Checked {
				t.Errorf("expected updated Rating=5, Checked=true, got %+v", updated)
			}

			if backend.BookCount() != 4 {
				t.Errorf("BookCount() changed after UpdateBook: got %d, want 4", backend.BookCount())
			}
		})
	}
}

func TestBackend_UpdateBooks(t *testing.T) {
	for name, newBackend := range backendFixtures() {
		t.Run(name, func(t *testing.T) {
			backend := newBackend(t)

			book3, err := backend.GetBook("book-3")
			if err != nil || book3 == nil {
				t.Fatalf("GetBook(book-3): book=%+v err=%v", book3, err)
			}
			book4, err := backend.GetBook("book-4")
			if err != nil || book4 == nil {
				t.Fatalf("GetBook(book-4): book=%+v err=%v", book4, err)
			}
			book3.Rating = 3
			book4.Rating = 2

			if err := backend.UpdateBooks([]*library.ComicBook{book3, book4}); err != nil {
				t.Fatalf("UpdateBooks: %v", err)
			}
			if err := backend.Flush(); err != nil {
				t.Fatalf("Flush: %v", err)
			}

			got3, _ := backend.GetBook("book-3")
			got4, _ := backend.GetBook("book-4")
			if got3.Rating != 3 {
				t.Errorf("book-3 Rating = %v, want 3", got3.Rating)
			}
			if got4.Rating != 2 {
				t.Errorf("book-4 Rating = %v, want 2", got4.Rating)
			}
		})
	}
}

func TestBackend_Metadata(t *testing.T) {
	for name, newBackend := range backendFixtures() {
		t.Run(name, func(t *testing.T) {
			backend := newBackend(t)

			if got := backend.LibraryID(); got != "fixture-library" {
				t.Errorf("LibraryID() = %q, want %q", got, "fixture-library")
			}
			if got := backend.LibraryName(); got != "Fixture Library" {
				t.Errorf("LibraryName() = %q, want %q", got, "Fixture Library")
			}
			if !backend.CanPersist() {
				t.Error("expected CanPersist() to be true when a storage path is configured")
			}
		})
	}
}

func TestBackend_CreateUpdateDeleteList(t *testing.T) {
	for name, newBackend := range backendFixtures() {
		t.Run(name, func(t *testing.T) {
			backend := newBackend(t)

			newList := &library.ComicListItem{
				Type:        "ComicSmartListItem",
				ID:          "list-new",
				Name:        "New List",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "Series", MatchOperator: "0", MatchValue: "Superman"},
				},
			}
			if err := backend.CreateList(newList); err != nil {
				t.Fatalf("CreateList: %v", err)
			}

			found, err := backend.FindListByID("list-new")
			if err != nil || found == nil {
				t.Fatalf("FindListByID after create: list=%+v err=%v", found, err)
			}
			books, err := backend.MatchBooks(found)
			if err != nil || len(books) != 1 || books[0].ID != "book-3" {
				t.Fatalf("expected new list to match [book-3], got %+v (err=%v)", books, err)
			}

			found.Name = "Renamed List"
			if err := backend.UpdateList(found); err != nil {
				t.Fatalf("UpdateList: %v", err)
			}
			renamed, err := backend.FindListByID("list-new")
			if err != nil || renamed == nil || renamed.Name != "Renamed List" {
				t.Fatalf("expected renamed list, got %+v (err=%v)", renamed, err)
			}

			if err := backend.DeleteList("list-new"); err != nil {
				t.Fatalf("DeleteList: %v", err)
			}
			gone, err := backend.FindListByID("list-new")
			if err != nil {
				t.Fatalf("FindListByID after delete: %v", err)
			}
			if gone != nil {
				t.Errorf("expected list to be gone after DeleteList, got %+v", gone)
			}
		})
	}
}

func TestBackend_MoveList(t *testing.T) {
	for name, newBackend := range backendFixtures() {
		t.Run(name, func(t *testing.T) {
			backend := newBackend(t)

			if err := backend.MoveList("list-smart-batman", "list-folder-1"); err != nil {
				t.Fatalf("MoveList: %v", err)
			}

			lists, err := backend.GetAllLists()
			if err != nil {
				t.Fatalf("GetAllLists: %v", err)
			}
			for _, l := range lists {
				if l.ID == "list-smart-batman" {
					t.Fatalf("expected list-smart-batman to no longer be a root list after MoveList")
				}
			}

			var folder *library.ComicListItem
			for i := range lists {
				if lists[i].ID == "list-folder-1" {
					folder = &lists[i]
				}
			}
			if folder == nil {
				t.Fatal("expected to find list-folder-1 among root lists")
			}
			found := false
			for _, child := range folder.ChildItems {
				if child.ID == "list-smart-batman" {
					found = true
				}
			}
			if !found {
				t.Errorf("expected list-smart-batman to be a child of list-folder-1, got children %+v", folder.ChildItems)
			}
		})
	}
}

// --- Documented behavioral differences (not part of the shared suite) ---

// cvDataSetter is implemented by both XMLBackend and SQLiteBackend but is
// deliberately not part of the library.Backend interface (it's an
// enrichment side-channel set by the ComicVine sync orchestrator, not core
// library data - see cmd/server.go's updateCVData and comic-server-22c).
type cvDataSetter interface {
	SetCVData(map[string]*library.CVCompleteness)
}

// TestBackend_CVDataMatchers verifies SQLiteBackend now supports CV smart
// list matchers (CVSeriesComplete/CVMissingCount/CVPercentOwned) the same
// way XMLBackend does, via SetCVData - closing comic-server-22c.
func TestBackend_CVDataMatchers(t *testing.T) {
	for name, newBackend := range backendFixtures() {
		t.Run(name, func(t *testing.T) {
			backend := newBackend(t)

			setter, ok := backend.(cvDataSetter)
			if !ok {
				t.Fatalf("%s backend does not implement SetCVData", name)
			}
			setter.SetCVData(map[string]*library.CVCompleteness{
				"book-1": {TotalIssues: 10, OwnedIssues: 10, MissingCount: 0, PercentOwned: 100, IsComplete: "Yes"},
				"book-2": {TotalIssues: 10, OwnedIssues: 6, MissingCount: 4, PercentOwned: 60, IsComplete: "No"},
			})

			list := &library.ComicListItem{
				Type:        "ComicSmartListItem",
				ID:          "list-cv-complete",
				Name:        "Complete Series",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "ComicServerCVSeriesCompleteMatcher", MatchOperator: "0"}, // Yes
				},
			}

			books, err := backend.MatchBooks(list)
			if err != nil {
				t.Fatalf("MatchBooks: %v", err)
			}
			if len(books) != 1 || books[0].ID != "book-1" {
				t.Fatalf("expected CV data to be attached and match [book-1] as complete, got %+v", books)
			}
		})
	}
}

// TestBackend_UpdateList_MissingID documents that the two backends diverge
// on updating a list ID that doesn't exist: XMLBackend returns an error,
// SQLiteBackend's UPDATE affects zero rows and returns nil.
func TestBackend_UpdateList_MissingID(t *testing.T) {
	xmlBackend := newXMLBackendFixture(t)
	if err := xmlBackend.UpdateList(&library.ComicListItem{ID: "does-not-exist", Name: "X"}); err == nil {
		t.Error("expected XMLBackend.UpdateList to error for a nonexistent list ID")
	}

	sqliteBackend := newSQLiteBackendFixture(t)
	if err := sqliteBackend.UpdateList(&library.ComicListItem{ID: "does-not-exist", Name: "X"}); err != nil {
		t.Errorf("SQLiteBackend.UpdateList unexpectedly errored for a nonexistent list ID (behavior may have changed - update the divergence comment if so): %v", err)
	}
}
