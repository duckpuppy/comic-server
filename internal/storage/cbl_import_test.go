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

// TestImportCBL_RecordsAmbiguousCandidates is the persistence half of
// comic-server-a2hz (match-correction UI): when the string-fallback path
// ties between two real candidates, ImportCBL must record the full
// candidate set (not just the arbitrary FirstOrDefault pick) so the UI
// can offer the others.
func TestImportCBL_RecordsAmbiguousCandidates(t *testing.T) {
	dup1 := library.ComicBook{ID: "book-1", FilePath: "/x/dup1.cbz", Series: "Weird Duplicates", Number: "1", Volume: -1, Year: 2000}
	dup2 := library.ComicBook{ID: "book-2", FilePath: "/x/dup2.cbz", Series: "Weird Duplicates", Number: "1", Volume: -1, Year: 2000}
	db := newTestDBWithBooks(t, []library.ComicBook{dup1, dup2})

	rl := &cbl.ReadingList{
		Name:  "Ambiguous",
		Books: []cbl.Book{{Series: "Weird Duplicates", Number: "1", Volume: -1, Year: 2000}},
	}
	result, err := db.ImportCBL(rl, CBLImportSource{Source: "local_file"})
	if err != nil {
		t.Fatalf("ImportCBL: %v", err)
	}

	entries, err := db.GetCBLImportEntries(result.ListID)
	if err != nil {
		t.Fatalf("GetCBLImportEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Path != cbl.MatchSeriesNumber {
		t.Fatalf("Path = %v, want MatchSeriesNumber", e.Path)
	}
	if e.BookID != "book-1" {
		t.Errorf("BookID = %q, want book-1 (FirstOrDefault pick)", e.BookID)
	}
	gotCandidates := append([]string(nil), e.CandidateBookIDs...)
	wantCandidates := []string{"book-1", "book-2"}
	if len(gotCandidates) != len(wantCandidates) {
		t.Fatalf("CandidateBookIDs = %v, want %v", gotCandidates, wantCandidates)
	}
	for i := range wantCandidates {
		if gotCandidates[i] != wantCandidates[i] {
			t.Errorf("CandidateBookIDs[%d] = %q, want %q", i, gotCandidates[i], wantCandidates[i])
		}
	}
}

// TestCorrectCBLImportEntry_RepointsToDifferentBook covers the primary
// match-correction flow (spec §2d): the string-fallback path guessed
// book-1 out of a tie with book-2, and the user re-points the entry to
// the actually-correct book-2.
func TestCorrectCBLImportEntry_RepointsToDifferentBook(t *testing.T) {
	dup1 := library.ComicBook{ID: "book-1", FilePath: "/x/dup1.cbz", Series: "Weird Duplicates", Number: "1", Volume: -1, Year: 2000}
	dup2 := library.ComicBook{ID: "book-2", FilePath: "/x/dup2.cbz", Series: "Weird Duplicates", Number: "1", Volume: -1, Year: 2000}
	db := newTestDBWithBooks(t, []library.ComicBook{dup1, dup2})

	rl := &cbl.ReadingList{
		Name:  "Ambiguous",
		Books: []cbl.Book{{Series: "Weird Duplicates", Number: "1", Volume: -1, Year: 2000}},
	}
	result, err := db.ImportCBL(rl, CBLImportSource{})
	if err != nil {
		t.Fatalf("ImportCBL: %v", err)
	}
	entries, err := db.GetCBLImportEntries(result.ListID)
	if err != nil || len(entries) != 1 {
		t.Fatalf("GetCBLImportEntries: entries=%v err=%v", entries, err)
	}
	entryID := entries[0].ID

	corrected, err := db.CorrectCBLImportEntry(entryID, "book-2")
	if err != nil {
		t.Fatalf("CorrectCBLImportEntry: %v", err)
	}
	if corrected.BookID != "book-2" {
		t.Errorf("corrected.BookID = %q, want book-2", corrected.BookID)
	}
	if corrected.Path != cbl.MatchManual {
		t.Errorf("corrected.Path = %v, want MatchManual", corrected.Path)
	}

	// reading_list_items reflects the swap: book-1 out, book-2 in.
	var memberIDs []string
	rows, err := db.Query(`SELECT book_id FROM reading_list_items WHERE list_id = ? ORDER BY book_id`, result.ListID)
	if err != nil {
		t.Fatalf("query reading_list_items: %v", err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		memberIDs = append(memberIDs, id)
	}
	rows.Close()
	if len(memberIDs) != 1 || memberIDs[0] != "book-2" {
		t.Errorf("reading_list_items = %v, want [book-2]", memberIDs)
	}

	// book_count still reflects the one-book list, not a phantom +1.
	list, err := db.GetList(result.ListID)
	if err != nil || list == nil {
		t.Fatalf("GetList: list=%v err=%v", list, err)
	}
	if list.BookCount != 1 {
		t.Errorf("BookCount = %d, want 1", list.BookCount)
	}
}

// TestCorrectCBLImportEntry_Unmatch covers dropping a wrong match
// entirely (spec §2d "unmatch an entry", feeding into the wanted-list
// flow per §2e/comic-server-sx2d).
func TestCorrectCBLImportEntry_Unmatch(t *testing.T) {
	batman := library.ComicBook{ID: "book-1", FilePath: "/x/batman1.cbz", Series: "Batman", Number: "1", Volume: 1940, Year: 1940}
	db := newTestDBWithBooks(t, []library.ComicBook{batman})

	rl := &cbl.ReadingList{
		Name:  "Solo",
		Books: []cbl.Book{{Series: "Batman", Number: "1", Volume: 1940, Year: 1940}},
	}
	result, err := db.ImportCBL(rl, CBLImportSource{})
	if err != nil {
		t.Fatalf("ImportCBL: %v", err)
	}
	entries, err := db.GetCBLImportEntries(result.ListID)
	if err != nil || len(entries) != 1 {
		t.Fatalf("GetCBLImportEntries: entries=%v err=%v", entries, err)
	}

	corrected, err := db.CorrectCBLImportEntry(entries[0].ID, "")
	if err != nil {
		t.Fatalf("CorrectCBLImportEntry: %v", err)
	}
	if corrected.BookID != "" {
		t.Errorf("corrected.BookID = %q, want empty (unmatched)", corrected.BookID)
	}
	if corrected.Path != cbl.MatchNone {
		t.Errorf("corrected.Path = %v, want MatchNone", corrected.Path)
	}

	list, err := db.GetList(result.ListID)
	if err != nil || list == nil {
		t.Fatalf("GetList: list=%v err=%v", list, err)
	}
	if list.BookCount != 0 {
		t.Errorf("BookCount = %d, want 0 after unmatching the only entry", list.BookCount)
	}

	// GetUnresolvedCBLImportEntries now sees it - the wanted-list flow's
	// entry point (comic-server-sx2d).
	unresolved, err := db.GetUnresolvedCBLImportEntries(result.ListID)
	if err != nil {
		t.Fatalf("GetUnresolvedCBLImportEntries: %v", err)
	}
	if len(unresolved) != 1 {
		t.Errorf("len(unresolved) = %d, want 1", len(unresolved))
	}
}

func TestCorrectCBLImportEntry_UnknownEntry(t *testing.T) {
	db := newTestDBWithBooks(t, nil)
	if _, err := db.CorrectCBLImportEntry("does-not-exist", "book-1"); err != ErrCBLImportEntryNotFound {
		t.Errorf("err = %v, want ErrCBLImportEntryNotFound", err)
	}
}

func TestCorrectCBLImportEntry_UnknownBook(t *testing.T) {
	batman := library.ComicBook{ID: "book-1", FilePath: "/x/batman1.cbz", Series: "Batman", Number: "1", Volume: 1940, Year: 1940}
	db := newTestDBWithBooks(t, []library.ComicBook{batman})
	rl := &cbl.ReadingList{
		Name:  "Solo",
		Books: []cbl.Book{{Series: "Batman", Number: "1", Volume: 1940, Year: 1940}},
	}
	result, err := db.ImportCBL(rl, CBLImportSource{})
	if err != nil {
		t.Fatalf("ImportCBL: %v", err)
	}
	entries, err := db.GetCBLImportEntries(result.ListID)
	if err != nil || len(entries) != 1 {
		t.Fatalf("GetCBLImportEntries: entries=%v err=%v", entries, err)
	}

	if _, err := db.CorrectCBLImportEntry(entries[0].ID, "does-not-exist"); err != ErrCBLCandidateBookNotFound {
		t.Errorf("err = %v, want ErrCBLCandidateBookNotFound", err)
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
