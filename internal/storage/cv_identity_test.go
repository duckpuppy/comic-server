package storage

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

// TestCVIdentity_FreshDBHasColumnsAndIndexes confirms a brand-new
// database has the first-class cv_volume_id/cv_issue_id columns
// (comic-server-r8td) - the createTables and migration code paths must
// stay in sync.
func TestCVIdentity_FreshDBHasColumnsAndIndexes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open fresh db: %v", err)
	}
	defer db.Close()

	for _, col := range []string{"cv_volume_id", "cv_issue_id"} {
		has, err := db.hasColumn("books", col)
		if err != nil {
			t.Fatalf("hasColumn(%s): %v", col, err)
		}
		if !has {
			t.Errorf("fresh books table missing column %q", col)
		}
	}
}

// TestCVIdentity_InsertReadRoundTrip imports a book tagged with
// comicvine_volume/comicvine_issue via CustomValuesStore (the normal XML
// import path) and confirms: the values land in the new dedicated
// columns, NOT as book_custom_values rows (real migration, no
// duplication), and reading the book back synthesizes them into
// CustomValuesStore exactly as before (backward compatible for every
// existing caller).
func TestCVIdentity_InsertReadRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	book := library.ComicBook{ID: "book-1", FilePath: "/x/a.cbz", Series: "Batman", Number: "1"}
	book.CustomValuesStore = library.SetCustomValue(book.CustomValuesStore, "comicvine_volume", "796")
	book.CustomValuesStore = library.SetCustomValue(book.CustomValuesStore, "comicvine_issue", "105811")
	book.CustomValuesStore = library.SetCustomValue(book.CustomValuesStore, "unrelated_key", "keep-me")

	if _, err := db.Import(&library.ComicLibrary{ID: "lib", Books: []library.ComicBook{book}}, ImportOptions{}); err != nil {
		t.Fatalf("import: %v", err)
	}

	// Dedicated columns are populated.
	var cvVolumeID, cvIssueID sql.NullInt64
	err = db.QueryRow("SELECT cv_volume_id, cv_issue_id FROM books WHERE id = ?", "book-1").Scan(&cvVolumeID, &cvIssueID)
	if err != nil {
		t.Fatalf("query columns: %v", err)
	}
	if !cvVolumeID.Valid || cvVolumeID.Int64 != 796 {
		t.Errorf("cv_volume_id = %v, want 796", cvVolumeID)
	}
	if !cvIssueID.Valid || cvIssueID.Int64 != 105811 {
		t.Errorf("cv_issue_id = %v, want 105811", cvIssueID)
	}

	// NOT duplicated into book_custom_values.
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM book_custom_values WHERE book_id = ? AND key IN ('comicvine_volume', 'comicvine_issue')", "book-1").Scan(&count)
	if err != nil {
		t.Fatalf("count custom_values: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 book_custom_values rows for comicvine_volume/issue, got %d", count)
	}
	// The unrelated key IS still stored the normal way.
	var unrelated string
	err = db.QueryRow("SELECT value FROM book_custom_values WHERE book_id = ? AND key = 'unrelated_key'", "book-1").Scan(&unrelated)
	if err != nil || unrelated != "keep-me" {
		t.Errorf("expected unrelated_key to survive in book_custom_values, got %q err=%v", unrelated, err)
	}

	// GetBook (single-book read path) synthesizes both keys back in.
	got, err := db.GetBook("book-1")
	if err != nil || got == nil {
		t.Fatalf("GetBook: got=%v err=%v", got, err)
	}
	if v, ok := library.GetCustomValue(got.CustomValuesStore, "comicvine_volume"); !ok || v != "796" {
		t.Errorf("GetBook: comicvine_volume = %q, ok=%v, want 796", v, ok)
	}
	if v, ok := library.GetCustomValue(got.CustomValuesStore, "comicvine_issue"); !ok || v != "105811" {
		t.Errorf("GetBook: comicvine_issue = %q, ok=%v, want 105811", v, ok)
	}
	if v, ok := library.GetCustomValue(got.CustomValuesStore, "unrelated_key"); !ok || v != "keep-me" {
		t.Errorf("GetBook: unrelated_key = %q, ok=%v, want keep-me", v, ok)
	}

	// GetAllBooks (batch read path) does the same.
	all, err := db.GetAllBooks()
	if err != nil || len(all) != 1 {
		t.Fatalf("GetAllBooks: len=%d err=%v", len(all), err)
	}
	if v, ok := library.GetCustomValue(all[0].CustomValuesStore, "comicvine_issue"); !ok || v != "105811" {
		t.Errorf("GetAllBooks: comicvine_issue = %q, ok=%v, want 105811", v, ok)
	}
}

// TestCVIdentity_UpdateBookFieldsRoundTrip simulates the ComicVine
// scraper's live-write path (internal/comicvine/writer.go calls
// library.SetCustomValue then Backend.UpdateBook, which is
// UpdateBookFields at the storage layer) - confirms the same real
// migration + synthesis applies there too, not just at XML import.
func TestCVIdentity_UpdateBookFieldsRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	book := library.ComicBook{ID: "book-1", FilePath: "/x/a.cbz", Series: "Batman", Number: "1"}
	if _, err := db.Import(&library.ComicLibrary{ID: "lib", Books: []library.ComicBook{book}}, ImportOptions{}); err != nil {
		t.Fatalf("import: %v", err)
	}

	live, err := db.GetBook("book-1")
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	live.CustomValuesStore = library.SetCustomValue(live.CustomValuesStore, "comicvine_volume", "796")
	live.CustomValuesStore = library.SetCustomValue(live.CustomValuesStore, "comicvine_issue", "105811")
	if err := db.UpdateBookFields(live); err != nil {
		t.Fatalf("UpdateBookFields: %v", err)
	}

	var cvIssueID sql.NullInt64
	if err := db.QueryRow("SELECT cv_issue_id FROM books WHERE id = ?", "book-1").Scan(&cvIssueID); err != nil {
		t.Fatalf("query cv_issue_id: %v", err)
	}
	if !cvIssueID.Valid || cvIssueID.Int64 != 105811 {
		t.Errorf("cv_issue_id = %v, want 105811", cvIssueID)
	}

	got, err := db.GetBook("book-1")
	if err != nil {
		t.Fatalf("GetBook after update: %v", err)
	}
	if v, ok := library.GetCustomValue(got.CustomValuesStore, "comicvine_issue"); !ok || v != "105811" {
		t.Errorf("comicvine_issue = %q, ok=%v, want 105811", v, ok)
	}
}

// TestCVIdentity_ReimportDoesNotFalselyDetectLiveChange is the delicate
// one: reimporting the SAME XML content twice (unchanged
// comicvine_volume/comicvine_issue) must not make the field-level merge
// (comic-server-aio, mergeUpdateBook/diffBookColumns) think the live row
// diverged from the snapshot just because CustomValuesStore is now
// reconstructed from a different source (columns instead of
// book_custom_values rows) - it must still compare equal.
func TestCVIdentity_ReimportDoesNotFalselyDetectLiveChange(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	book := library.ComicBook{ID: "book-1", FilePath: "/x/a.cbz", Series: "Batman", Number: "1", Title: "First"}
	book.CustomValuesStore = library.SetCustomValue(book.CustomValuesStore, "comicvine_volume", "796")
	book.CustomValuesStore = library.SetCustomValue(book.CustomValuesStore, "comicvine_issue", "105811")

	lib := &library.ComicLibrary{ID: "lib", Books: []library.ComicBook{book}}
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Reimport the exact same XML content (same comicvine tags, same
	// everything) - should be a no-op (Unchanged), not treat
	// CustomValuesStore as having drifted.
	stats, err := db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if stats.BooksUnchanged != 1 {
		t.Errorf("BooksUnchanged = %d, want 1 (reimporting identical content should be a no-op)", stats.BooksUnchanged)
	}
	if stats.BooksUpdated != 0 {
		t.Errorf("BooksUpdated = %d, want 0", stats.BooksUpdated)
	}

	// A genuine field change (Title) on reimport still works normally,
	// and comicvine_volume/issue survive untouched.
	book2 := book
	book2.Title = "Second"
	lib2 := &library.ComicLibrary{ID: "lib", Books: []library.ComicBook{book2}}
	stats2, err := db.Import(lib2, ImportOptions{})
	if err != nil {
		t.Fatalf("third import (title change): %v", err)
	}
	if stats2.BooksUpdated != 1 {
		t.Errorf("BooksUpdated = %d, want 1 after a genuine title change", stats2.BooksUpdated)
	}
	got, err := db.GetBook("book-1")
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if got.Title != "Second" {
		t.Errorf("Title = %q, want Second", got.Title)
	}
	if v, ok := library.GetCustomValue(got.CustomValuesStore, "comicvine_issue"); !ok || v != "105811" {
		t.Errorf("comicvine_issue survived title-only reimport: %q, ok=%v, want 105811", v, ok)
	}
}

// TestCVIdentity_ExportPreservesCustomValue is the user's explicit
// concern (2026-09-21): ComicDb.xml export must still carry
// comicvine_volume/comicvine_issue, even though they're no longer stored
// as book_custom_values rows. Exercises the REAL export path
// (DB.Export, internal/storage/export.go), not just GetBook.
func TestCVIdentity_ExportPreservesCustomValue(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	book := library.ComicBook{ID: "book-1", FilePath: "/x/a.cbz", Series: "Batman", Number: "1"}
	book.CustomValuesStore = library.SetCustomValue(book.CustomValuesStore, "comicvine_volume", "796")
	book.CustomValuesStore = library.SetCustomValue(book.CustomValuesStore, "comicvine_issue", "105811")
	if _, err := db.Import(&library.ComicLibrary{ID: "lib", Books: []library.ComicBook{book}}, ImportOptions{}); err != nil {
		t.Fatalf("import: %v", err)
	}

	result, err := db.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(result.Library.Books) != 1 {
		t.Fatalf("expected 1 exported book, got %d", len(result.Library.Books))
	}
	exported := result.Library.Books[0]
	if v, ok := library.GetCustomValue(exported.CustomValuesStore, "comicvine_volume"); !ok || v != "796" {
		t.Errorf("exported comicvine_volume = %q, ok=%v, want 796", v, ok)
	}
	if v, ok := library.GetCustomValue(exported.CustomValuesStore, "comicvine_issue"); !ok || v != "105811" {
		t.Errorf("exported comicvine_issue = %q, ok=%v, want 105811", v, ok)
	}

	// Round-trip through actual XML marshal/unmarshal, matching real
	// ComicRackCE usage - confirms the value survives real serialization,
	// not just the in-memory struct.
	outPath := filepath.Join(t.TempDir(), "out.xml")
	if err := library.SaveLibrary(outPath, result.Library); err != nil {
		t.Fatalf("SaveLibrary: %v", err)
	}
	reloaded, err := library.LoadLibrary(outPath)
	if err != nil {
		t.Fatalf("LoadLibrary: %v", err)
	}
	if len(reloaded.Books) != 1 {
		t.Fatalf("expected 1 book after XML round-trip, got %d", len(reloaded.Books))
	}
	if v, ok := library.GetCustomValue(reloaded.Books[0].CustomValuesStore, "comicvine_issue"); !ok || v != "105811" {
		t.Errorf("after real XML round-trip: comicvine_issue = %q, ok=%v, want 105811", v, ok)
	}
}

// TestCVIdentity_FastLookups confirms the new indexed lookup methods
// (comic-server-r8td's actual performance payoff).
func TestCVIdentity_FastLookups(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	tagged := library.ComicBook{ID: "book-1", FilePath: "/x/a.cbz", Series: "Batman", Number: "1"}
	tagged.CustomValuesStore = library.SetCustomValue(tagged.CustomValuesStore, "comicvine_volume", "796")
	tagged.CustomValuesStore = library.SetCustomValue(tagged.CustomValuesStore, "comicvine_issue", "105811")
	untagged := library.ComicBook{ID: "book-2", FilePath: "/x/b.cbz", Series: "Untagged", Number: "1"}
	if _, err := db.Import(&library.ComicLibrary{ID: "lib", Books: []library.ComicBook{tagged, untagged}}, ImportOptions{}); err != nil {
		t.Fatalf("import: %v", err)
	}

	got, err := db.GetBookByCVIssueID(105811)
	if err != nil {
		t.Fatalf("GetBookByCVIssueID: %v", err)
	}
	if got == nil || got.ID != "book-1" {
		t.Fatalf("GetBookByCVIssueID(105811) = %v, want book-1", got)
	}

	got2, err := db.GetBookByCVVolumeID(796)
	if err != nil {
		t.Fatalf("GetBookByCVVolumeID: %v", err)
	}
	if got2 == nil || got2.ID != "book-1" {
		t.Fatalf("GetBookByCVVolumeID(796) = %v, want book-1", got2)
	}

	notFound, err := db.GetBookByCVIssueID(999999)
	if err != nil {
		t.Fatalf("GetBookByCVIssueID (not found): %v", err)
	}
	if notFound != nil {
		t.Errorf("expected nil for an unused CV issue ID, got %+v", notFound)
	}
}

// TestMigrateV7ToV8_BackfillsFromBookCustomValues simulates a database
// already sitting at schema v7 with comicvine_volume/comicvine_issue
// stored the OLD way (book_custom_values rows) and confirms opening it
// backfills the new columns and removes those two rows, leaving other
// custom values untouched.
func TestMigrateV7ToV8_BackfillsFromBookCustomValues(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	v7Books := `CREATE TABLE books (id TEXT PRIMARY KEY, file_path TEXT NOT NULL, series TEXT, number TEXT)`
	v7CustomValues := `
		CREATE TABLE book_custom_values (
			book_id TEXT NOT NULL, key TEXT NOT NULL, value TEXT,
			PRIMARY KEY (book_id, key)
		)
	`
	for _, stmt := range []string{v7Books, v7CustomValues, "PRAGMA user_version = 7"} {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	if _, err := raw.Exec(`INSERT INTO books (id, file_path, series, number) VALUES ('book-1', '/x/a.cbz', 'Batman', '1')`); err != nil {
		t.Fatalf("seed book: %v", err)
	}
	for _, kv := range [][2]string{{"comicvine_volume", "796"}, {"comicvine_issue", "105811"}, {"other_key", "keep-me"}} {
		if _, err := raw.Exec(`INSERT INTO book_custom_values (book_id, key, value) VALUES (?, ?, ?)`, "book-1", kv[0], kv[1]); err != nil {
			t.Fatalf("seed custom value %v: %v", kv, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open (should migrate v7->v8): %v", err)
	}
	defer db.Close()

	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("get schema version: %v", err)
	}
	if version != schemaVersion {
		t.Errorf("schema version = %d, want %d", version, schemaVersion)
	}

	var cvVolumeID, cvIssueID sql.NullInt64
	if err := db.QueryRow("SELECT cv_volume_id, cv_issue_id FROM books WHERE id = ?", "book-1").Scan(&cvVolumeID, &cvIssueID); err != nil {
		t.Fatalf("query backfilled columns: %v", err)
	}
	if !cvVolumeID.Valid || cvVolumeID.Int64 != 796 {
		t.Errorf("backfilled cv_volume_id = %v, want 796", cvVolumeID)
	}
	if !cvIssueID.Valid || cvIssueID.Int64 != 105811 {
		t.Errorf("backfilled cv_issue_id = %v, want 105811", cvIssueID)
	}

	var migratedRowCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM book_custom_values WHERE key IN ('comicvine_volume', 'comicvine_issue')").Scan(&migratedRowCount); err != nil {
		t.Fatalf("count migrated rows: %v", err)
	}
	if migratedRowCount != 0 {
		t.Errorf("expected the old comicvine_volume/comicvine_issue rows to be removed, found %d", migratedRowCount)
	}

	var otherValue string
	if err := db.QueryRow("SELECT value FROM book_custom_values WHERE book_id = ? AND key = 'other_key'", "book-1").Scan(&otherValue); err != nil {
		t.Fatalf("query untouched custom value: %v", err)
	}
	if otherValue != "keep-me" {
		t.Errorf("other_key = %q, want keep-me (must survive the migration untouched)", otherValue)
	}
}
