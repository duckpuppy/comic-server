package storage

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/cbl"
	"github.com/duckpuppy/comic-server/internal/library"
)

func newTestDBWithBooks(t *testing.T, books []library.ComicBook) *DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	lib := &library.ComicLibrary{ID: "lib-1", Name: "Test Library", Books: books}
	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("seed library: %v", err)
	}
	return db
}

func TestImportCBL_MatchesAndCreatesList(t *testing.T) {
	batmanWithCV := library.ComicBook{ID: "book-1", FilePath: "/x/batman1.cbz", Series: "Batman", Number: "1", Volume: 1940, Year: 1940}
	batmanWithCV.CustomValuesStore = library.SetCustomValue(batmanWithCV.CustomValuesStore, "comicvine_issue", "105811")
	detectiveNoCV := library.ComicBook{ID: "book-2", FilePath: "/x/det27.cbz", Series: "Detective Comics", Number: "27", Volume: 1937, Year: 1939}

	db := newTestDBWithBooks(t, []library.ComicBook{batmanWithCV, detectiveNoCV})

	rl := &cbl.ReadingList{
		Name: "Test Reading Order",
		Books: []cbl.Book{
			{Series: "Batman", Number: "1", Volume: 1940, Year: 1940, Database: []cbl.Database{{Name: "cv", Issue: "105811"}}},
			{Series: "Detective Comics", Number: "27", Volume: 1937, Year: 1939},  // no <Database>, string-fallback path
			{Series: "Nonexistent Series", Number: "999", Volume: -1, Year: 1999}, // unmatched
		},
	}

	result, err := db.ImportCBL(rl, CBLImportSource{Source: "local_file"})
	if err != nil {
		t.Fatalf("ImportCBL: %v", err)
	}
	if result.MatchedCVID != 1 {
		t.Errorf("MatchedCVID = %d, want 1", result.MatchedCVID)
	}
	if result.MatchedOther != 1 {
		t.Errorf("MatchedOther = %d, want 1", result.MatchedOther)
	}
	if result.Unmatched != 1 {
		t.Errorf("Unmatched = %d, want 1", result.Unmatched)
	}
	if len(result.Entries) != 3 {
		t.Fatalf("len(Entries) = %d, want 3", len(result.Entries))
	}

	// The list itself was created with exactly the 2 matched books, in order.
	list, err := db.GetList(result.ListID)
	if err != nil || list == nil {
		t.Fatalf("GetList(%s): list=%v err=%v", result.ListID, list, err)
	}
	if list.Name != "Test Reading Order" {
		t.Errorf("list.Name = %q, want %q", list.Name, "Test Reading Order")
	}
	if list.BookCount != 2 {
		t.Errorf("list.BookCount = %d, want 2", list.BookCount)
	}
	if len(list.Items) != 2 || list.Items[0].ID != "book-1" || list.Items[1].ID != "book-2" {
		t.Fatalf("unexpected list.Items order/content: %+v", list.Items)
	}

	// Source provenance was recorded.
	var cblSource string
	if err := db.QueryRow("SELECT cbl_source FROM lists WHERE id = ?", result.ListID).Scan(&cblSource); err != nil {
		t.Fatalf("query cbl_source: %v", err)
	}
	if cblSource != "local_file" {
		t.Errorf("cbl_source = %q, want local_file", cblSource)
	}

	// All 3 entries (including the unmatched one) were recorded in
	// cbl_import_entries, so a later match-correction/wanted-list feature
	// has something to act on.
	var entryCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM cbl_import_entries WHERE list_id = ?", result.ListID).Scan(&entryCount); err != nil {
		t.Fatalf("query cbl_import_entries count: %v", err)
	}
	if entryCount != 3 {
		t.Errorf("cbl_import_entries count = %d, want 3", entryCount)
	}

	var unmatchedSeries string
	err = db.QueryRow("SELECT series FROM cbl_import_entries WHERE match_path = 'none' AND list_id = ?", result.ListID).Scan(&unmatchedSeries)
	if err != nil {
		t.Fatalf("query unmatched entry: %v", err)
	}
	if unmatchedSeries != "Nonexistent Series" {
		t.Errorf("unmatched entry series = %q, want Nonexistent Series", unmatchedSeries)
	}
}

func TestImportCBL_RejectsSmartList(t *testing.T) {
	db := newTestDBWithBooks(t, nil)
	// A CBL with real <Matchers> content - parsed via cbl.Parse to
	// exercise the real IsSmartList() detection, not a hand-set flag.
	rl, err := cbl.Parse(strings.NewReader(`<ReadingList><Name>x</Name><Matchers><ComicBookMatcher/></Matchers></ReadingList>`))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if _, err := db.ImportCBL(rl, CBLImportSource{}); err != ErrCBLIsSmartList {
		t.Errorf("err = %v, want ErrCBLIsSmartList", err)
	}
}
