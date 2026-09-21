package storage

import (
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/cbl"
	"github.com/duckpuppy/comic-server/internal/library"
)

func TestGetUnresolvedCBLImportEntries_OnlyReturnsUnmatchedAndUnresolved(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	batman := library.ComicBook{ID: "book-1", FilePath: "/x/batman1.cbz", Series: "Batman", Number: "1", Volume: 1940, Year: 1940}
	if _, err := db.Import(&library.ComicLibrary{ID: "lib", Books: []library.ComicBook{batman}}, ImportOptions{}); err != nil {
		t.Fatalf("seed library: %v", err)
	}

	rl := &cbl.ReadingList{
		Name: "Test",
		Books: []cbl.Book{
			{Series: "Batman", Number: "1", Volume: 1940, Year: 1940},
			{Series: "Missing One", Number: "1", Volume: -1, Year: 2000},
			{Series: "Missing Two", Number: "1", Volume: -1, Year: 2001, Database: []cbl.Database{{Name: "cv", Issue: "99999"}}},
		},
	}
	result, err := db.ImportCBL(rl, CBLImportSource{Source: "local_file"})
	if err != nil {
		t.Fatalf("ImportCBL: %v", err)
	}
	if result.Unmatched != 2 {
		t.Fatalf("expected 2 unmatched, got %d", result.Unmatched)
	}

	unresolved, err := db.GetUnresolvedCBLImportEntries(result.ListID)
	if err != nil {
		t.Fatalf("GetUnresolvedCBLImportEntries: %v", err)
	}
	if len(unresolved) != 2 {
		t.Fatalf("expected 2 unresolved entries, got %d: %+v", len(unresolved), unresolved)
	}
	// Ordered by position - "Missing One" (pos 1) before "Missing Two" (pos 2).
	if unresolved[0].Series != "Missing One" || unresolved[1].Series != "Missing Two" {
		t.Errorf("unexpected order/content: %+v", unresolved)
	}
	if unresolved[1].CVIssueID != 99999 {
		t.Errorf("expected CVIssueID 99999 to carry through, got %d", unresolved[1].CVIssueID)
	}

	// Mark one resolved, confirm it drops out of the unresolved set.
	if err := db.MarkCBLImportEntryWanted(unresolved[0].ID, "wanted-book-1"); err != nil {
		t.Fatalf("MarkCBLImportEntryWanted: %v", err)
	}
	unresolved2, err := db.GetUnresolvedCBLImportEntries(result.ListID)
	if err != nil {
		t.Fatalf("GetUnresolvedCBLImportEntries (after mark): %v", err)
	}
	if len(unresolved2) != 1 || unresolved2[0].Series != "Missing Two" {
		t.Errorf("expected only 'Missing Two' left unresolved, got %+v", unresolved2)
	}
}
