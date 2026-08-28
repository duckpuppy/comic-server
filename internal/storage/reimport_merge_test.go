package storage

import (
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestDiffBookColumns_OnlyChangedFieldsReported(t *testing.T) {
	old := &library.ComicBook{ID: "1", Title: "Old Title", Series: "Batman", Writer: "Jane Doe"}
	newBook := &library.ComicBook{ID: "1", Title: "New Title", Series: "Batman", Writer: "Jane Doe"}

	// live == old for every field, so nothing's in genuine conflict.
	changes := diffBookColumns(old, old, newBook)
	if len(changes) != 1 {
		t.Fatalf("expected 1 changed column, got %d: %+v", len(changes), changes)
	}
	if changes[0].column != "title" || changes[0].value != "New Title" {
		t.Errorf("unexpected change: %+v", changes[0])
	}
}

func TestDiffBookColumns_IdenticalBooksProduceNoChanges(t *testing.T) {
	book := &library.ComicBook{ID: "1", Title: "Same", Series: "Batman"}
	changes := diffBookColumns(book, book, book)
	if len(changes) != 0 {
		t.Errorf("expected no changes for identical books, got %+v", changes)
	}
}

func TestDiffBookColumns_PagesComparedByContent(t *testing.T) {
	old := &library.ComicBook{ID: "1", Pages: []library.ComicPageInfo{{Image: 0, Type: "Story"}}}
	newBook := &library.ComicBook{ID: "1", Pages: []library.ComicPageInfo{{Image: 0, Type: "FrontCover"}}}

	changes := diffBookColumns(old, old, newBook)
	found := false
	for _, c := range changes {
		if c.column == "pages" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a pages change, got %+v", changes)
	}
}

// TestDiffBookColumns_LiveEditWinsOverConflictingXMLChange is the unit-
// level check for comic-server-cfi: when the XML changed a field AND
// comic-server's live value has independently diverged from the old
// snapshot for that same field, the field is left alone (comic-server's
// live value wins) rather than being overwritten by the new XML value.
func TestDiffBookColumns_LiveEditWinsOverConflictingXMLChange(t *testing.T) {
	old := &library.ComicBook{ID: "1", Notes: "original notes"}
	live := &library.ComicBook{ID: "1", Notes: "locally edited notes"}
	newBook := &library.ComicBook{ID: "1", Notes: "notes from ComicRack"}

	changes := diffBookColumns(old, live, newBook)
	for _, c := range changes {
		if c.column == "notes" {
			t.Fatalf("expected no change for notes (live value should win a genuine conflict), got %+v", c)
		}
	}
}

// TestImportMerge_PreservesLocalEditWhenXMLChangesOtherField is the exact
// regression scenario from comic-server-aio: a comic-server feature (here
// standing in for CBZ-convert, which sets FilePath) edits a book locally,
// then an unrelated field changes in the XML and gets reimported. Before
// this fix, the whole-row hash comparison would have overwritten
// EVERYTHING from the stale XML on that reimport, silently reverting the
// local FilePath change.
func TestImportMerge_PreservesLocalEditWhenXMLChangesOtherField(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	lib := &library.ComicLibrary{
		ID: "test-library-id",
		Books: []library.ComicBook{
			{ID: "book-1", FilePath: `G:\Comics\book.cbr`, Title: "Original Title", Series: "Batman"},
		},
	}
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Simulate CBZ-convert: a local write to FilePath (and PageCount) via
	// UpdateBookFields, independent of any XML reimport.
	book, err := db.GetBook("book-1")
	if err != nil || book == nil {
		t.Fatalf("get book: book=%+v err=%v", book, err)
	}
	book.FilePath = `G:\Comics\book.cbz`
	book.PageCount = 22
	if err := db.UpdateBookFields(book); err != nil {
		t.Fatalf("UpdateBookFields (simulated CBZ-convert): %v", err)
	}

	// ComicRack Desktop edits something UNRELATED (the title) and saves -
	// the XML's FilePath/PageCount are still the OLD values, since
	// ComicRack never knew about the conversion.
	lib.Books[0].Title = "Updated Title"
	stats, err := db.Import(lib, ImportOptions{})
	if err != nil {
		t.Fatalf("second import (unrelated XML change): %v", err)
	}
	if stats.BooksUpdated != 1 {
		t.Errorf("expected 1 book updated, got %d", stats.BooksUpdated)
	}

	after, err := db.GetBook("book-1")
	if err != nil || after == nil {
		t.Fatalf("get book after reimport: book=%+v err=%v", after, err)
	}

	// The genuinely-changed XML field must be applied.
	if after.Title != "Updated Title" {
		t.Errorf("Title = %q, want %q (the real XML change should still apply)", after.Title, "Updated Title")
	}
	// The locally-set fields must survive, NOT revert to the stale XML.
	if after.FilePath != `G:\Comics\book.cbz` {
		t.Errorf("FilePath = %q, want %q (local CBZ-convert edit should survive an unrelated reimport)", after.FilePath, `G:\Comics\book.cbz`)
	}
	if after.PageCount != 22 {
		t.Errorf("PageCount = %d, want 22 (local CBZ-convert edit should survive an unrelated reimport)", after.PageCount)
	}
}

// TestImportMerge_LiveEditWinsOverConflictingXMLChange covers the real
// conflict case (both sides touched the same field): comic-server's live
// value wins, per comic-server-cfi's flip of aio's original "XML wins"
// rule - now that ComicRack Desktop's XML is a periodic, essentially-
// frozen import source rather than a live co-author, comic-server's own
// value (e.g. reading progress reverse-synced from a device) is the more
// authoritative one when both sides have genuinely diverged.
func TestImportMerge_LiveEditWinsOverConflictingXMLChange(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	lib := &library.ComicLibrary{
		ID:    "test-library-id",
		Books: []library.ComicBook{{ID: "book-1", FilePath: "/comics/book.cbz", Notes: "original notes"}},
	}
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("first import: %v", err)
	}

	book, err := db.GetBook("book-1")
	if err != nil || book == nil {
		t.Fatalf("get book: book=%+v err=%v", book, err)
	}
	book.Notes = "locally edited notes"
	if err := db.UpdateBookFields(book); err != nil {
		t.Fatalf("UpdateBookFields: %v", err)
	}

	// ComicRack ALSO changes Notes in its own XML.
	lib.Books[0].Notes = "notes from ComicRack"
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("second import (conflicting field): %v", err)
	}

	after, err := db.GetBook("book-1")
	if err != nil || after == nil {
		t.Fatalf("get book after reimport: book=%+v err=%v", after, err)
	}
	if after.Notes != "locally edited notes" {
		t.Errorf("Notes = %q, want %q (comic-server's live value should win a genuine conflict)", after.Notes, "locally edited notes")
	}
}

// TestImportMerge_TagsAndCustomValuesOnlyReplacedWhenSourceChanged ensures
// the merge's tags/custom-values handling (outside diffBookColumns, since
// they live in join tables) is equally conditional - not always
// delete+reinsert on every merge-triggering reimport regardless of
// whether they actually changed.
func TestImportMerge_TagsAndCustomValuesOnlyReplacedWhenSourceChanged(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	lib := &library.ComicLibrary{
		ID: "test-library-id",
		Books: []library.ComicBook{
			{ID: "book-1", FilePath: "/comics/book.cbz", Tags: "action, superhero", CustomValuesStore: ",comicvine_volume=1"},
		},
	}
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// A local write to a comic-server-owned field (ScanInformation) that
	// leaves Tags/CustomValuesStore in the *retrieved* book struct
	// unchanged, then a genuinely unrelated XML field change.
	book, err := db.GetBook("book-1")
	if err != nil || book == nil {
		t.Fatalf("get book: book=%+v err=%v", book, err)
	}
	book.ScanInformation = "Scanner:Test"
	if err := db.UpdateBookFields(book); err != nil {
		t.Fatalf("UpdateBookFields: %v", err)
	}

	lib.Books[0].Title = "New Title"
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("second import: %v", err)
	}

	after, err := db.GetBook("book-1")
	if err != nil || after == nil {
		t.Fatalf("get book after reimport: book=%+v err=%v", after, err)
	}
	if after.Tags != "action, superhero" {
		t.Errorf("Tags = %q, want unchanged %q", after.Tags, "action, superhero")
	}
	if after.CustomValuesStore != ",comicvine_volume=1" {
		t.Errorf("CustomValuesStore = %q, want unchanged %q", after.CustomValuesStore, ",comicvine_volume=1")
	}
	if after.ScanInformation != "Scanner:Test" {
		t.Errorf("ScanInformation = %q, want the local edit to survive", after.ScanInformation)
	}
}

// TestImportMerge_MissingSnapshotFallsBackToFullOverwrite covers the
// migration bootstrap case documented on migrateV3ToV4: a book with no
// prior xml_snapshot (e.g. migrated from pre-v4) degrades to a full
// overwrite on its next changed reimport, rather than erroring or
// crashing on a nil/empty snapshot.
func TestImportMerge_MissingSnapshotFallsBackToFullOverwrite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	lib := &library.ComicLibrary{
		ID:    "test-library-id",
		Books: []library.ComicBook{{ID: "book-1", FilePath: "/comics/book.cbz", Title: "Original"}},
	}
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Simulate a pre-v4 row: clear its snapshot directly.
	if _, err := db.Exec("UPDATE books SET xml_snapshot = NULL WHERE id = ?", "book-1"); err != nil {
		t.Fatalf("clear snapshot: %v", err)
	}

	lib.Books[0].Title = "Changed"
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("second import (no prior snapshot): %v", err)
	}

	after, err := db.GetBook("book-1")
	if err != nil || after == nil {
		t.Fatalf("get book: book=%+v err=%v", after, err)
	}
	if after.Title != "Changed" {
		t.Errorf("Title = %q, want %q", after.Title, "Changed")
	}
}

// TestImportMerge_ReadingProgressSurvivesConflictingXMLReimport is
// comic-server-cfi's acceptance scenario: a device reverse-syncs a new
// CurrentPage into comic-server's live DB, then a reimport arrives where
// the XML's CurrentPage ALSO changed - to a third, different value (not
// the original snapshot value, not the live value) - simulating a stale
// ComicRack Desktop XML disagreeing with comic-server's own up-to-date
// reading state. The live (reverse-synced) value must survive.
func TestImportMerge_ReadingProgressSurvivesConflictingXMLReimport(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	lib := &library.ComicLibrary{
		ID: "test-library-id",
		Books: []library.ComicBook{
			{ID: "book-1", FilePath: "/comics/book.cbz", CurrentPage: 1, LastPageRead: 1},
		},
	}
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Device reverse-sync: user read further, comic-server's live DB is
	// updated directly (not via a reimport).
	book, err := db.GetBook("book-1")
	if err != nil || book == nil {
		t.Fatalf("get book: book=%+v err=%v", book, err)
	}
	book.CurrentPage = 15
	book.LastPageRead = 15
	if err := db.UpdateBookFields(book); err != nil {
		t.Fatalf("UpdateBookFields (simulated reverse-sync): %v", err)
	}

	// The stale ComicRack Desktop XML also has a CurrentPage change - to a
	// THIRD value, matching neither the original snapshot (1) nor the
	// live reverse-synced value (15).
	lib.Books[0].CurrentPage = 3
	lib.Books[0].LastPageRead = 3
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("second import (conflicting reading progress): %v", err)
	}

	after, err := db.GetBook("book-1")
	if err != nil || after == nil {
		t.Fatalf("get book after reimport: book=%+v err=%v", after, err)
	}
	if after.CurrentPage != 15 {
		t.Errorf("CurrentPage = %d, want 15 (comic-server's reverse-synced reading progress should survive a conflicting XML reimport)", after.CurrentPage)
	}
	if after.LastPageRead != 15 {
		t.Errorf("LastPageRead = %d, want 15 (comic-server's reverse-synced reading progress should survive a conflicting XML reimport)", after.LastPageRead)
	}
}
