package storage

import (
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestGetBook(t *testing.T) {
	// Create temp database
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// Import a library
	lib := &library.ComicLibrary{
		ID:   "test-library-id",
		Name: "Test Library",
		Books: []library.ComicBook{
			{
				ID:                "book-1",
				FilePath:          "/path/to/book1.cbz",
				Title:             "Test Book 1",
				Series:            "Test Series",
				Publisher:         "Test Publisher",
				Year:              2020,
				Rating:            4.5,
				Tags:              "action, superhero",
				CustomValuesStore: ",SeriesGroup=X-Men,ReadingOrder=5",
			},
		},
	}

	_, err = db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	// Get book
	book, err := db.GetBook("book-1")
	if err != nil {
		t.Fatalf("get book: %v", err)
	}
	if book == nil {
		t.Fatal("book not found")
	}

	// Verify fields
	if book.Title != "Test Book 1" {
		t.Errorf("expected title 'Test Book 1', got '%s'", book.Title)
	}
	if book.Series != "Test Series" {
		t.Errorf("expected series 'Test Series', got '%s'", book.Series)
	}
	if book.Rating != 4.5 {
		t.Errorf("expected rating 4.5, got %f", book.Rating)
	}

	// Verify tags were loaded
	if book.Tags == "" {
		t.Error("tags not loaded")
	}

	// Verify custom values were loaded
	if book.CustomValuesStore == "" {
		t.Error("custom values not loaded")
	}
}

// TestUpdateBookFields_ClearsTagsToEmpty verifies that reverse-syncing a
// book with Tags == "" (device cleared all tags) actually removes the
// existing book_tags rows, matching XMLBackend's behavior of overwriting
// the whole book in place. See comic-server-dfs: the previous
// `if book.Tags != ""` guard silently skipped the delete, leaving stale
// tags behind.
func TestUpdateBookFields_ClearsTagsToEmpty(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	lib := &library.ComicLibrary{
		ID: "test-library-id",
		Books: []library.ComicBook{
			{ID: "book-1", FilePath: "/path/to/book1.cbz", Tags: "action, superhero"},
		},
	}
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("import: %v", err)
	}

	book, err := db.GetBook("book-1")
	if err != nil || book == nil {
		t.Fatalf("get book: book=%+v err=%v", book, err)
	}
	if book.Tags == "" {
		t.Fatal("expected tags to be loaded before clearing")
	}

	book.Tags = ""
	if err := db.UpdateBookFields(book); err != nil {
		t.Fatalf("UpdateBookFields: %v", err)
	}

	cleared, err := db.GetBook("book-1")
	if err != nil || cleared == nil {
		t.Fatalf("get book after clear: book=%+v err=%v", cleared, err)
	}
	if cleared.Tags != "" {
		t.Errorf("expected tags to be cleared, got %q", cleared.Tags)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM book_tags WHERE book_id = ?", "book-1").Scan(&count); err != nil {
		t.Fatalf("count book_tags: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 book_tags rows after clearing, got %d", count)
	}
}

// TestUpdateBookFields_PersistsFieldsPreviouslyDropped is the regression
// test for comic-server-4vq: UpdateBookFields used to only write 11 of the
// books table's 70+ columns, so scan-info (ScanInformation) and
// CBZ-convert (FilePath, PageCount) silently no-op'd on the SQLite
// backend. Covers those three specifically, plus a broader sample of
// fields that were equally dropped before the fix, to catch a future
// regression that only re-narrows the column list rather than fully
// reverting it.
func TestUpdateBookFields_PersistsFieldsPreviouslyDropped(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	lib := &library.ComicLibrary{
		ID: "test-library-id",
		Books: []library.ComicBook{
			{ID: "book-1", FilePath: "/comics/original.cbr", Series: "Original Series"},
		},
	}
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("import: %v", err)
	}

	book, err := db.GetBook("book-1")
	if err != nil || book == nil {
		t.Fatalf("get book: book=%+v err=%v", book, err)
	}

	// The three fields that directly triggered finding this bug (scan-info
	// sets ScanInformation; CBZ-convert sets FilePath and PageCount) plus a
	// spread of other previously-dropped fields (publisher, a credit
	// field, a physical-property field, a custom value).
	book.ScanInformation = "Scanner:TestGroup"
	book.FilePath = "/comics/converted.cbz"
	book.PageCount = 42
	book.Publisher = "Test Publisher"
	book.Writer = "Test Writer"
	book.BlackAndWhite = "Yes"
	book.CustomValuesStore = ",comicvine_volume=12345"

	if err := db.UpdateBookFields(book); err != nil {
		t.Fatalf("UpdateBookFields: %v", err)
	}

	got, err := db.GetBook("book-1")
	if err != nil || got == nil {
		t.Fatalf("get book after update: book=%+v err=%v", got, err)
	}

	if got.ScanInformation != "Scanner:TestGroup" {
		t.Errorf("ScanInformation = %q, want %q", got.ScanInformation, "Scanner:TestGroup")
	}
	if got.FilePath != "/comics/converted.cbz" {
		t.Errorf("FilePath = %q, want %q", got.FilePath, "/comics/converted.cbz")
	}
	if got.PageCount != 42 {
		t.Errorf("PageCount = %d, want 42", got.PageCount)
	}
	if got.Publisher != "Test Publisher" {
		t.Errorf("Publisher = %q, want %q", got.Publisher, "Test Publisher")
	}
	if got.Writer != "Test Writer" {
		t.Errorf("Writer = %q, want %q", got.Writer, "Test Writer")
	}
	if got.BlackAndWhite != "Yes" {
		t.Errorf("BlackAndWhite = %q, want %q", got.BlackAndWhite, "Yes")
	}
	if got.CustomValuesStore != ",comicvine_volume=12345" {
		t.Errorf("CustomValuesStore = %q, want %q", got.CustomValuesStore, ",comicvine_volume=12345")
	}
}

func TestUpdateBookFields_CustomValuesReplaceNotAccumulate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	lib := &library.ComicLibrary{
		ID: "test-library-id",
		Books: []library.ComicBook{
			{ID: "book-1", FilePath: "/comics/book.cbz", CustomValuesStore: ",key1=value1"},
		},
	}
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("import: %v", err)
	}

	book, err := db.GetBook("book-1")
	if err != nil || book == nil {
		t.Fatalf("get book: book=%+v err=%v", book, err)
	}

	book.CustomValuesStore = ",key2=value2"
	if err := db.UpdateBookFields(book); err != nil {
		t.Fatalf("UpdateBookFields: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM book_custom_values WHERE book_id = ?", "book-1").Scan(&count); err != nil {
		t.Fatalf("count book_custom_values: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 custom value row after replace, got %d (old key1 should be gone, not accumulated)", count)
	}

	got, err := db.GetBook("book-1")
	if err != nil || got == nil {
		t.Fatalf("get book after update: book=%+v err=%v", got, err)
	}
	if got.CustomValuesStore != ",key2=value2" {
		t.Errorf("CustomValuesStore = %q, want %q", got.CustomValuesStore, ",key2=value2")
	}
}

func TestGetBookNotFound(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	book, err := db.GetBook("nonexistent")
	if err != nil {
		t.Fatalf("get book: %v", err)
	}
	if book != nil {
		t.Error("expected nil for nonexistent book")
	}
}

func TestGetAllBooks(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// Import library with multiple books
	lib := &library.ComicLibrary{
		ID:   "test-library-id",
		Name: "Test Library",
		Books: []library.ComicBook{
			{ID: "book-1", FilePath: "/path/1.cbz", Title: "Book 1"},
			{ID: "book-2", FilePath: "/path/2.cbz", Title: "Book 2"},
			{ID: "book-3", FilePath: "/path/3.cbz", Title: "Book 3"},
		},
	}

	_, err = db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	// Get all books
	books, err := db.GetAllBooks()
	if err != nil {
		t.Fatalf("get all books: %v", err)
	}

	if len(books) != 3 {
		t.Errorf("expected 3 books, got %d", len(books))
	}
}

func TestGetBookCount(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// Import library
	lib := &library.ComicLibrary{
		ID:   "test-library-id",
		Name: "Test Library",
		Books: []library.ComicBook{
			{ID: "book-1", FilePath: "/path/1.cbz"},
			{ID: "book-2", FilePath: "/path/2.cbz"},
		},
	}

	_, err = db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	count, err := db.GetBookCount()
	if err != nil {
		t.Fatalf("get book count: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestGetList(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// Import library with list
	lib := &library.ComicLibrary{
		ID:   "test-library-id",
		Name: "Test Library",
		ComicLists: []library.ComicListItem{
			{
				ID:          "list-1",
				Name:        "Test List",
				Type:        "ComicSmartListItem",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{
						Type:          "ComicBookSeriesMatcher",
						MatchOperator: "1",
						MatchValue:    "Batman",
					},
				},
			},
		},
	}

	_, err = db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	// Get list
	list, err := db.GetList("list-1")
	if err != nil {
		t.Fatalf("get list: %v", err)
	}
	if list == nil {
		t.Fatal("list not found")
	}

	if list.Name != "Test List" {
		t.Errorf("expected name 'Test List', got '%s'", list.Name)
	}
	if len(list.Matchers) != 1 {
		t.Errorf("expected 1 matcher, got %d", len(list.Matchers))
	}
}

func TestGetAllLists(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// Import library with nested lists
	lib := &library.ComicLibrary{
		ID:   "test-library-id",
		Name: "Test Library",
		ComicLists: []library.ComicListItem{
			{
				ID:   "folder-1",
				Name: "Folder",
				Type: "ComicListItemFolder",
				ChildItems: []library.ComicListItem{
					{ID: "list-1", Name: "Child List", Type: "ComicSmartListItem"},
				},
			},
			{
				ID:   "list-2",
				Name: "Root List",
				Type: "ComicSmartListItem",
			},
		},
	}

	_, err = db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	// Get all lists
	lists, err := db.GetAllLists()
	if err != nil {
		t.Fatalf("get all lists: %v", err)
	}

	// Should return 2 root items
	if len(lists) != 2 {
		t.Errorf("expected 2 root lists, got %d", len(lists))
	}

	// Find folder and verify it has child
	for _, list := range lists {
		if list.ID == "folder-1" {
			if len(list.ChildItems) != 1 {
				t.Errorf("expected folder to have 1 child, got %d", len(list.ChildItems))
			}
		}
	}
}
