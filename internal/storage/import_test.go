package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestImportBasic(t *testing.T) {
	// Create temp database
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// Create a simple library
	lib := &library.ComicLibrary{
		ID:   "test-library-id",
		Name: "Test Library",
		Books: []library.ComicBook{
			{
				ID:        "book-1",
				FilePath:  "/path/to/book1.cbz",
				Title:     "Test Book 1",
				Series:    "Test Series",
				Publisher: "Test Publisher",
				Year:      2020,
				Rating:    4.5,
			},
			{
				ID:        "book-2",
				FilePath:  "/path/to/book2.cbz",
				Title:     "Test Book 2",
				Series:    "Test Series",
				Publisher: "Test Publisher",
				Year:      2021,
			},
		},
	}

	// Import
	stats, err := db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	// Verify stats
	if stats.BooksAdded != 2 {
		t.Errorf("expected 2 books added, got %d", stats.BooksAdded)
	}
	if stats.BooksUpdated != 0 {
		t.Errorf("expected 0 books updated, got %d", stats.BooksUpdated)
	}
	if stats.BooksDeleted != 0 {
		t.Errorf("expected 0 books deleted, got %d", stats.BooksDeleted)
	}

	// Verify books exist in database
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM books").Scan(&count)
	if err != nil {
		t.Fatalf("count books: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 books in database, got %d", count)
	}
}

func TestImportIdempotency(t *testing.T) {
	// Create temp database
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// Create a library
	lib := &library.ComicLibrary{
		ID:   "test-library-id",
		Name: "Test Library",
		Books: []library.ComicBook{
			{
				ID:        "book-1",
				FilePath:  "/path/to/book1.cbz",
				Title:     "Test Book 1",
				Series:    "Test Series",
				Publisher: "Test Publisher",
				Year:      2020,
			},
		},
	}

	// First import
	stats1, err := db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatalf("first import: %v", err)
	}

	if stats1.BooksAdded != 1 {
		t.Errorf("first import: expected 1 book added, got %d", stats1.BooksAdded)
	}

	// Second import - same data
	stats2, err := db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatalf("second import: %v", err)
	}

	// Should have no changes
	if stats2.BooksAdded != 0 {
		t.Errorf("second import: expected 0 books added, got %d", stats2.BooksAdded)
	}
	if stats2.BooksUpdated != 0 {
		t.Errorf("second import: expected 0 books updated, got %d", stats2.BooksUpdated)
	}
	if stats2.BooksDeleted != 0 {
		t.Errorf("second import: expected 0 books deleted, got %d", stats2.BooksDeleted)
	}
	if stats2.BooksUnchanged != 1 {
		t.Errorf("second import: expected 1 book unchanged, got %d", stats2.BooksUnchanged)
	}
}

func TestImportDetectsChanges(t *testing.T) {
	// Create temp database
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// Create initial library
	lib := &library.ComicLibrary{
		ID:   "test-library-id",
		Name: "Test Library",
		Books: []library.ComicBook{
			{
				ID:       "book-1",
				FilePath: "/path/to/book1.cbz",
				Title:    "Original Title",
				Year:     2020,
			},
		},
	}

	// First import
	_, err = db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Modify book
	lib.Books[0].Title = "Updated Title"
	lib.Books[0].Rating = 5.0

	// Second import
	stats, err := db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatalf("second import: %v", err)
	}

	// Should detect the update
	if stats.BooksUpdated != 1 {
		t.Errorf("expected 1 book updated, got %d", stats.BooksUpdated)
	}
	if stats.BooksUnchanged != 0 {
		t.Errorf("expected 0 books unchanged, got %d", stats.BooksUnchanged)
	}
}

func TestImportDeletesRemovedBooks(t *testing.T) {
	// Create temp database
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// Create library with 2 books
	lib := &library.ComicLibrary{
		ID:   "test-library-id",
		Name: "Test Library",
		Books: []library.ComicBook{
			{
				ID:       "book-1",
				FilePath: "/path/to/book1.cbz",
				Title:    "Book 1",
			},
			{
				ID:       "book-2",
				FilePath: "/path/to/book2.cbz",
				Title:    "Book 2",
			},
		},
	}

	// First import
	_, err = db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Remove book-2
	lib.Books = lib.Books[:1]

	// Second import
	stats, err := db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatalf("second import: %v", err)
	}

	// Should delete book-2
	if stats.BooksDeleted != 1 {
		t.Errorf("expected 1 book deleted, got %d", stats.BooksDeleted)
	}
	if stats.BooksUnchanged != 1 {
		t.Errorf("expected 1 book unchanged, got %d", stats.BooksUnchanged)
	}

	// comic-server-b53: removed books are soft-deleted, not gone - the raw
	// row count stays 2, but GetBookCount/GetAllBooks (which filter
	// deleted_at) should only see 1.
	var rawCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM books").Scan(&rawCount); err != nil {
		t.Fatalf("count books: %v", err)
	}
	if rawCount != 2 {
		t.Errorf("expected 2 raw rows in database (book-2 soft-deleted, not removed), got %d", rawCount)
	}

	visibleCount, err := db.GetBookCount()
	if err != nil {
		t.Fatalf("GetBookCount: %v", err)
	}
	if visibleCount != 1 {
		t.Errorf("expected 1 visible book, got %d", visibleCount)
	}

	var deletedAt sql.NullString
	if err := db.QueryRow("SELECT deleted_at FROM books WHERE id = ?", "book-2").Scan(&deletedAt); err != nil {
		t.Fatalf("query deleted_at: %v", err)
	}
	if !deletedAt.Valid || deletedAt.String == "" {
		t.Error("expected book-2's deleted_at to be set")
	}

	if book, err := db.GetBook("book-2"); err != nil || book != nil {
		t.Errorf("expected GetBook(book-2) to return nil (soft-deleted), got book=%+v err=%v", book, err)
	}
}

// TestImportRevivesReappearingBook covers comic-server-b53's revival
// requirement: a book soft-deleted by one import must come back to life
// (deleted_at cleared, visible again) if its ID reappears in a later one -
// not stay stuck invisibly deleted, and not collide with insertBook's
// INSERT (which would hit the still-present PRIMARY KEY).
func TestImportRevivesReappearingBook(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	lib := &library.ComicLibrary{
		ID: "test-library-id",
		Books: []library.ComicBook{
			{ID: "book-1", FilePath: "/path/to/book1.cbz", Title: "Book 1"},
		},
	}
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Remove it.
	emptyLib := &library.ComicLibrary{ID: "test-library-id"}
	if _, err := db.Import(emptyLib, ImportOptions{}); err != nil {
		t.Fatalf("second import (removal): %v", err)
	}
	if book, err := db.GetBook("book-1"); err != nil || book != nil {
		t.Fatalf("expected book-1 to be soft-deleted after removal, got book=%+v err=%v", book, err)
	}

	// Bring it back with the same ID (e.g. the user undid a delete in
	// ComicRack, or it was a transient export glitch).
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("third import (revival): %v", err)
	}

	revived, err := db.GetBook("book-1")
	if err != nil || revived == nil {
		t.Fatalf("expected book-1 to be revived, got book=%+v err=%v", revived, err)
	}
	if revived.Title != "Book 1" {
		t.Errorf("revived book has wrong content: %+v", revived)
	}

	var deletedAt sql.NullString
	if err := db.QueryRow("SELECT deleted_at FROM books WHERE id = ?", "book-1").Scan(&deletedAt); err != nil {
		t.Fatalf("query deleted_at: %v", err)
	}
	if deletedAt.Valid {
		t.Errorf("expected deleted_at to be cleared after revival, got %q", deletedAt.String)
	}
}

// TestImportRevivesUnchangedReappearingBook is the trickier case: the
// revived book's content is IDENTICAL to what it was when deleted (same
// hash), so the whole-row hash comparison alone would call it
// "Unchanged" and never clear deleted_at - the deleted flag has to be
// checked independently of the hash comparison.
func TestImportRevivesUnchangedReappearingBook(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	lib := &library.ComicLibrary{
		ID: "test-library-id",
		Books: []library.ComicBook{
			{ID: "book-1", FilePath: "/path/to/book1.cbz", Title: "Book 1"},
		},
	}
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("first import: %v", err)
	}
	if _, err := db.Import(&library.ComicLibrary{ID: "test-library-id"}, ImportOptions{}); err != nil {
		t.Fatalf("second import (removal): %v", err)
	}

	// Reimport with byte-for-byte identical content to what was deleted.
	stats, err := db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatalf("third import (revival, unchanged content): %v", err)
	}
	if stats.BooksUpdated != 1 {
		t.Errorf("expected the revival to count as an update, got BooksUpdated=%d BooksUnchanged=%d", stats.BooksUpdated, stats.BooksUnchanged)
	}

	revived, err := db.GetBook("book-1")
	if err != nil || revived == nil {
		t.Fatalf("expected book-1 to be revived even with unchanged content, got book=%+v err=%v", revived, err)
	}
}

func TestImportCustomValues(t *testing.T) {
	// Create temp database
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// Create library with custom values
	lib := &library.ComicLibrary{
		ID:   "test-library-id",
		Name: "Test Library",
		Books: []library.ComicBook{
			{
				ID:                "book-1",
				FilePath:          "/path/to/book1.cbz",
				Title:             "Book 1",
				CustomValuesStore: ",SeriesGroup=X-Men,ReadingOrder=5",
			},
		},
	}

	// Import
	_, err = db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	// Verify custom values
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM book_custom_values WHERE book_id = ?", "book-1").Scan(&count)
	if err != nil {
		t.Fatalf("count custom values: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 custom values, got %d", count)
	}

	// Verify specific value
	var value string
	err = db.QueryRow("SELECT value FROM book_custom_values WHERE book_id = ? AND key = ?", "book-1", "SeriesGroup").Scan(&value)
	if err != nil {
		t.Fatalf("get custom value: %v", err)
	}
	if value != "X-Men" {
		t.Errorf("expected SeriesGroup=X-Men, got %s", value)
	}
}

func TestImportTags(t *testing.T) {
	// Create temp database
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// Create library with tags
	lib := &library.ComicLibrary{
		ID:   "test-library-id",
		Name: "Test Library",
		Books: []library.ComicBook{
			{
				ID:       "book-1",
				FilePath: "/path/to/book1.cbz",
				Title:    "Book 1",
				Tags:     "superhero, action, classic",
			},
		},
	}

	// Import
	_, err = db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	// Verify tags
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM book_tags WHERE book_id = ?", "book-1").Scan(&count)
	if err != nil {
		t.Fatalf("count tags: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 tags, got %d", count)
	}

	// Verify specific tag
	err = db.QueryRow("SELECT COUNT(*) FROM book_tags WHERE book_id = ? AND tag = ?", "book-1", "superhero").Scan(&count)
	if err != nil {
		t.Fatalf("check tag: %v", err)
	}
	if count != 1 {
		t.Errorf("expected tag 'superhero' to exist")
	}
}

func TestImportLists(t *testing.T) {
	// Create temp database
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// Create library with smart list
	lib := &library.ComicLibrary{
		ID:   "test-library-id",
		Name: "Test Library",
		Books: []library.ComicBook{
			{
				ID:       "book-1",
				FilePath: "/path/to/book1.cbz",
				Title:    "Book 1",
				Series:   "Batman",
			},
		},
		ComicLists: []library.ComicListItem{
			{
				ID:          "list-1",
				Name:        "Batman Series",
				Type:        "ComicSmartListItem",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{
						Type:          "ComicBookSeriesMatcher",
						MatchOperator: "1", // Contains
						MatchValue:    "Batman",
					},
				},
			},
		},
	}

	// Import
	stats, err := db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	// Verify list stats
	if stats.ListsAdded != 1 {
		t.Errorf("expected 1 list added, got %d", stats.ListsAdded)
	}

	// Verify list exists
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM lists").Scan(&count)
	if err != nil {
		t.Fatalf("count lists: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 list in database, got %d", count)
	}

	// Verify list data
	var name, listType string
	err = db.QueryRow("SELECT name, type FROM lists WHERE id = ?", "list-1").Scan(&name, &listType)
	if err != nil {
		t.Fatalf("get list: %v", err)
	}
	if name != "Batman Series" {
		t.Errorf("expected list name 'Batman Series', got '%s'", name)
	}
	if listType != "ComicSmartListItem" {
		t.Errorf("expected list type 'ComicSmartListItem', got '%s'", listType)
	}
}

// TestImportSoftDeletesAndRevivesLists mirrors the book-level soft-delete
// and revival tests (comic-server-b53) for lists.
func TestImportSoftDeletesAndRevivesLists(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	lib := &library.ComicLibrary{
		ID: "test-library-id",
		ComicLists: []library.ComicListItem{
			{ID: "list-1", Name: "Batman Series", Type: "ComicSmartListItem", MatcherMode: "And"},
		},
	}
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Remove it.
	if _, err := db.Import(&library.ComicLibrary{ID: "test-library-id"}, ImportOptions{}); err != nil {
		t.Fatalf("second import (removal): %v", err)
	}

	var rawCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM lists").Scan(&rawCount); err != nil {
		t.Fatalf("count lists: %v", err)
	}
	if rawCount != 1 {
		t.Errorf("expected 1 raw row (soft-deleted, not removed), got %d", rawCount)
	}
	if list, err := db.GetList("list-1"); err != nil || list != nil {
		t.Errorf("expected GetList(list-1) to return nil (soft-deleted), got list=%+v err=%v", list, err)
	}
	allLists, err := db.GetAllLists()
	if err != nil {
		t.Fatalf("GetAllLists: %v", err)
	}
	if len(allLists) != 0 {
		t.Errorf("expected 0 visible lists, got %d", len(allLists))
	}

	// Revive it.
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("third import (revival): %v", err)
	}
	revived, err := db.GetList("list-1")
	if err != nil || revived == nil {
		t.Fatalf("expected list-1 to be revived, got list=%+v err=%v", revived, err)
	}
	if revived.Name != "Batman Series" {
		t.Errorf("revived list has wrong content: %+v", revived)
	}
}

func TestImportNestedLists(t *testing.T) {
	// Create temp database
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// Create library with nested folder structure
	lib := &library.ComicLibrary{
		ID:   "test-library-id",
		Name: "Test Library",
		ComicLists: []library.ComicListItem{
			{
				ID:   "folder-1",
				Name: "DC Comics",
				Type: "ComicListItemFolder",
				ChildItems: []library.ComicListItem{
					{
						ID:          "list-1",
						Name:        "Batman",
						Type:        "ComicSmartListItem",
						MatcherMode: "And",
					},
					{
						ID:          "list-2",
						Name:        "Superman",
						Type:        "ComicSmartListItem",
						MatcherMode: "And",
					},
				},
			},
		},
	}

	// Import
	stats, err := db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	// Should add folder + 2 lists
	if stats.ListsAdded != 3 {
		t.Errorf("expected 3 lists added, got %d", stats.ListsAdded)
	}

	// Verify parent_id for nested lists
	var parentID string
	err = db.QueryRow("SELECT parent_id FROM lists WHERE id = ?", "list-1").Scan(&parentID)
	if err != nil {
		t.Fatalf("get parent_id: %v", err)
	}
	if parentID != "folder-1" {
		t.Errorf("expected parent_id 'folder-1', got '%s'", parentID)
	}
}

func TestImportDryRun(t *testing.T) {
	// Create temp database
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// Create a library
	lib := &library.ComicLibrary{
		ID:   "test-library-id",
		Name: "Test Library",
		Books: []library.ComicBook{
			{
				ID:       "book-1",
				FilePath: "/path/to/book1.cbz",
				Title:    "Book 1",
			},
		},
	}

	// Import with dry-run
	stats, err := db.Import(lib, ImportOptions{DryRun: true})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	// Stats should show what would happen
	if stats.BooksAdded != 1 {
		t.Errorf("expected 1 book would be added, got %d", stats.BooksAdded)
	}

	// But database should be empty
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM books").Scan(&count)
	if err != nil {
		t.Fatalf("count books: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 books in database (dry run), got %d", count)
	}
}

func TestImportWithTestLibrary(t *testing.T) {
	// Test with the actual test library if it exists
	testLibPath := "../../testdata/library/ComicDb.xml"
	if _, err := os.Stat(testLibPath); os.IsNotExist(err) {
		t.Skip("Test library not found at", testLibPath)
	}

	// Create temp database
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// Load test library
	lib, err := library.LoadLibrary(testLibPath)
	if err != nil {
		t.Fatalf("load library: %v", err)
	}

	// First import
	stats1, err := db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatalf("first import: %v", err)
	}

	t.Logf("First import: %d books added, %d lists added in %s",
		stats1.BooksAdded, stats1.ListsAdded, stats1.Duration)

	// Second import - should be idempotent
	stats2, err := db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatalf("second import: %v", err)
	}

	// Verify idempotency
	if stats2.BooksAdded != 0 {
		t.Errorf("second import should add 0 books, added %d", stats2.BooksAdded)
	}
	if stats2.BooksUpdated != 0 {
		t.Errorf("second import should update 0 books, updated %d", stats2.BooksUpdated)
	}
	if stats2.BooksDeleted != 0 {
		t.Errorf("second import should delete 0 books, deleted %d", stats2.BooksDeleted)
	}
	if stats2.BooksUnchanged != stats1.BooksAdded {
		t.Errorf("second import should have %d unchanged books, got %d", stats1.BooksAdded, stats2.BooksUnchanged)
	}

	t.Logf("Second import: %d unchanged in %s", stats2.BooksUnchanged, stats2.Duration)
}
