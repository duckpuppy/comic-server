package storage

import (
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

// TestExportRoundTrip imports the real testdata fixture into a scratch
// SQLite database, exports it back out, and verifies the reconstructed
// library.ComicLibrary contains the expected books, lists, and matchers.
// The fixture doesn't contain any comic-server-only CV matchers, so it
// doesn't exercise the drop-and-warn path (see
// TestExportDropsCVOnlyMatchers for that).
func TestExportRoundTrip(t *testing.T) {
	lib, err := library.LoadLibrary("../../testdata/library/ComicDb.xml")
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("import: %v", err)
	}

	result, err := db.Export()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings for fixture without CV matchers, got: %v", result.Warnings)
	}

	if len(result.Library.Books) != len(lib.Books) {
		t.Errorf("expected %d books, got %d", len(lib.Books), len(result.Library.Books))
	}

	origListCount := countAllLists(lib.ComicLists)
	exportedListCount := countAllLists(result.Library.ComicLists)
	if origListCount != exportedListCount {
		t.Errorf("expected %d total lists, got %d", origListCount, exportedListCount)
	}

	// Find "All Comics" (a smart list with a matcher in the fixture) and
	// confirm its matcher round-tripped.
	var allComics *library.ComicListItem
	for i := range result.Library.ComicLists {
		if result.Library.ComicLists[i].Name == "All Comics" {
			allComics = &result.Library.ComicLists[i]
		}
	}
	if allComics == nil {
		t.Fatal("expected to find 'All Comics' smart list in export")
	}
	if len(allComics.Matchers) == 0 {
		t.Fatal("expected 'All Comics' list to have matchers")
	}
	if allComics.Matchers[0].Type != "ComicBookYearMatcher" {
		t.Errorf("expected ComicBookYearMatcher, got %s", allComics.Matchers[0].Type)
	}

	// Confirm the exported library actually marshals into valid,
	// ComicRackCE-compatible XML with real xsi:type attributes.
	outPath := filepath.Join(t.TempDir(), "out.xml")
	if err := library.SaveLibrary(outPath, result.Library); err != nil {
		t.Fatalf("save exported library: %v", err)
	}

	reloaded, err := library.LoadLibrary(outPath)
	if err != nil {
		t.Fatalf("reload exported library: %v", err)
	}
	if len(reloaded.Books) != len(lib.Books) {
		t.Errorf("reloaded export: expected %d books, got %d", len(lib.Books), len(reloaded.Books))
	}
}

// TestExportDropsCVOnlyMatchers verifies that a smart list containing a
// comic-server-only CV matcher (alongside a real ComicRack-compatible
// matcher) has just the CV matcher dropped on export, keeps its other
// matcher, and reports a warning - since the real testdata fixture
// doesn't happen to contain any CV matchers to exercise this path.
func TestExportDropsCVOnlyMatchers(t *testing.T) {
	lib := &library.ComicLibrary{
		ID:   "cv-test-library",
		Name: "CV Test Library",
		Books: []library.ComicBook{
			{ID: "book-1", FilePath: "/comics/book1.cbz", Series: "Test Series"},
		},
		ComicLists: []library.ComicListItem{
			{
				Type:        "ComicSmartListItem",
				ID:          "list-1",
				Name:        "Mixed Matchers",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "ComicBookSeriesMatcher", MatchOperator: "0", MatchValue: "Test Series"},
					{Type: "ComicServerCVSeriesCompleteMatcher", MatchOperator: "0"},
				},
			},
		},
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if _, err := db.Import(lib, ImportOptions{}); err != nil {
		t.Fatalf("import: %v", err)
	}

	result, err := db.Export()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	if len(result.Warnings) != 1 {
		t.Fatalf("expected exactly 1 warning, got %d: %v", len(result.Warnings), result.Warnings)
	}

	var mixed *library.ComicListItem
	for i := range result.Library.ComicLists {
		if result.Library.ComicLists[i].Name == "Mixed Matchers" {
			mixed = &result.Library.ComicLists[i]
		}
	}
	if mixed == nil {
		t.Fatal("expected to find 'Mixed Matchers' list in export")
	}

	if len(mixed.Matchers) != 1 {
		t.Fatalf("expected 1 matcher remaining after drop, got %d", len(mixed.Matchers))
	}
	if mixed.Matchers[0].Type != "ComicBookSeriesMatcher" {
		t.Errorf("expected surviving matcher to be ComicBookSeriesMatcher, got %s", mixed.Matchers[0].Type)
	}

	for _, m := range mixed.Matchers {
		if library.IsCVOnlyMatcherType(m.Type) {
			t.Errorf("CV-only matcher %s should have been dropped", m.Type)
		}
	}
}

func countAllLists(lists []library.ComicListItem) int {
	count := len(lists)
	for _, l := range lists {
		count += countAllLists(l.ChildItems)
	}
	return count
}
