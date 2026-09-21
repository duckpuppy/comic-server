package storage

import (
	"testing"

	"github.com/duckpuppy/comic-server/internal/cbl"
	"github.com/duckpuppy/comic-server/internal/library"
)

func TestReimportCBL_FullReplace(t *testing.T) {
	batman := library.ComicBook{ID: "book-1", FilePath: "/x/batman1.cbz", Series: "Batman", Number: "1", Volume: 1940, Year: 1940}
	detective := library.ComicBook{ID: "book-2", FilePath: "/x/det27.cbz", Series: "Detective Comics", Number: "27", Volume: 1937, Year: 1939}
	superman := library.ComicBook{ID: "book-3", FilePath: "/x/superman1.cbz", Series: "Superman", Number: "1", Volume: 1939, Year: 1939}

	db := newTestDBWithBooks(t, []library.ComicBook{batman, detective, superman})

	initial := &cbl.ReadingList{
		Name: "Reading Order",
		Books: []cbl.Book{
			{Series: "Batman", Number: "1", Volume: 1940, Year: 1940},
			{Series: "Detective Comics", Number: "27", Volume: 1937, Year: 1939},
		},
	}
	result, err := db.ImportCBL(initial, CBLImportSource{Source: "git:https://example/repo:list.cbl", SourceRef: "sha1"})
	if err != nil {
		t.Fatalf("ImportCBL: %v", err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("initial import: len(Entries) = %d, want 2", len(result.Entries))
	}

	// Upstream file changed: Batman dropped, Superman added, Detective kept.
	updated := &cbl.ReadingList{
		Name: "Reading Order (renamed upstream, ignored - see below)",
		Books: []cbl.Book{
			{Series: "Detective Comics", Number: "27", Volume: 1937, Year: 1939},
			{Series: "Superman", Number: "1", Volume: 1939, Year: 1939},
		},
	}
	reimportResult, err := db.ReimportCBL(result.ListID, updated, CBLImportSource{Source: "git:https://example/repo:list.cbl", SourceRef: "sha2"})
	if err != nil {
		t.Fatalf("ReimportCBL: %v", err)
	}
	if reimportResult.MatchedOther != 2 {
		t.Errorf("MatchedOther = %d, want 2", reimportResult.MatchedOther)
	}

	list, err := db.GetList(result.ListID)
	if err != nil || list == nil {
		t.Fatalf("GetList: list=%v err=%v", list, err)
	}
	// Name is untouched by reimport - list metadata belongs to the user,
	// only membership is reimport-owned (comic-server-zw0o policy).
	if list.Name != "Reading Order" {
		t.Errorf("list.Name = %q, want unchanged %q", list.Name, "Reading Order")
	}
	if len(list.Items) != 2 || list.Items[0].ID != "book-2" || list.Items[1].ID != "book-3" {
		t.Fatalf("unexpected list.Items after reimport: %+v", list.Items)
	}
	if list.BookCount != 2 {
		t.Errorf("list.BookCount = %d, want 2", list.BookCount)
	}

	// cbl_import_entries was fully replaced, not appended to.
	var entryCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM cbl_import_entries WHERE list_id = ?", result.ListID).Scan(&entryCount); err != nil {
		t.Fatalf("query cbl_import_entries count: %v", err)
	}
	if entryCount != 2 {
		t.Errorf("cbl_import_entries count = %d, want 2 (old entries should be replaced, not appended)", entryCount)
	}

	// Provenance was updated to the new source ref.
	source, err := db.GetCBLSource(result.ListID)
	if err != nil || source == nil {
		t.Fatalf("GetCBLSource: source=%v err=%v", source, err)
	}
	if source.SourceRef != "sha2" {
		t.Errorf("SourceRef = %q, want sha2", source.SourceRef)
	}
}

func TestReimportCBL_RejectsNonCBLList(t *testing.T) {
	db := newTestDBWithBooks(t, nil)
	list := &library.ComicListItem{Type: "ComicReadingList", ID: "plain-list", Name: "Not from CBL"}
	if err := db.InsertList(list); err != nil {
		t.Fatalf("InsertList: %v", err)
	}

	_, err := db.ReimportCBL("plain-list", &cbl.ReadingList{Name: "x"}, CBLImportSource{})
	if err != ErrListNotCBLImported {
		t.Errorf("ReimportCBL on non-CBL list: err = %v, want ErrListNotCBLImported", err)
	}
}

func TestListCBLImportedLists(t *testing.T) {
	db := newTestDBWithBooks(t, nil)

	rl := &cbl.ReadingList{Name: "L1"}
	r1, err := db.ImportCBL(rl, CBLImportSource{Source: "git:https://example/repo:a.cbl", SourceRef: "sha1"})
	if err != nil {
		t.Fatalf("ImportCBL: %v", err)
	}
	plain := &library.ComicListItem{Type: "ComicReadingList", ID: "plain-list", Name: "Hand-made list"}
	if err := db.InsertList(plain); err != nil {
		t.Fatalf("InsertList: %v", err)
	}

	lists, err := db.ListCBLImportedLists()
	if err != nil {
		t.Fatalf("ListCBLImportedLists: %v", err)
	}
	if len(lists) != 1 {
		t.Fatalf("len(lists) = %d, want 1", len(lists))
	}
	if lists[0].ListID != r1.ListID || lists[0].SourceRef != "sha1" {
		t.Errorf("unexpected entry: %+v", lists[0])
	}
}
