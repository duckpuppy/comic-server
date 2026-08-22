package storage

import (
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

// This file exercises the SQL fast path (matcher_sql.go) end-to-end against
// a real SQLite database, including values crafted to break a naive
// translation (LIKE wildcard characters, mixed case, negation, an
// untranslatable matcher mixed with translatable ones) - and cross-checks
// every result against XMLBackend evaluating the exact same data, so any
// divergence between the SQL-narrowed path and the true in-memory result
// would show up as a parity failure, not just a "the SQL didn't crash" test.

func edgeCaseFixtureLibrary() *library.ComicLibrary {
	return &library.ComicLibrary{
		ID: "edge-case-library",
		Books: []library.ComicBook{
			{ID: "book-1", Series: "Batman", Title: "100% Off_Beat #1", Publisher: "DC Comics", Year: 2020, Rating: 4.5, Checked: true},
			{ID: "book-2", Series: "batman", Title: "Normal Title", Publisher: "DC Comics", Year: 2005, Rating: 2.0, Checked: false},
			{ID: "book-3", Series: "BATMAN Beyond", Title: "Beyond #1", Publisher: "DC Comics", Year: 2015, Rating: 3.5, Checked: false, Tags: "hero"},
			{ID: "book-4", Series: "Spider-Man", Title: "Amazing #1", Publisher: "Marvel Comics", Year: 2020, Rating: 5.0, Checked: true, BlackAndWhite: "Yes"},
			{ID: "book-5", Series: "Watchmen", Title: "Watchmen #1", Publisher: "DC Comics", Year: 1986, Rating: 5.0, Checked: false, BlackAndWhite: ""},
		},
	}
}

func newEdgeCaseSQLiteBackend(t *testing.T) *SQLiteBackend {
	t.Helper()
	dir := t.TempDir()
	xmlPath := filepath.Join(dir, "ComicDb.xml")
	dbPath := filepath.Join(dir, "test.db")
	if err := library.SaveLibrary(xmlPath, edgeCaseFixtureLibrary()); err != nil {
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

func newEdgeCaseXMLBackend(t *testing.T) *library.XMLBackend {
	t.Helper()
	xmlPath := filepath.Join(t.TempDir(), "ComicDb.xml")
	if err := library.SaveLibrary(xmlPath, edgeCaseFixtureLibrary()); err != nil {
		t.Fatalf("SaveLibrary: %v", err)
	}
	backend, err := library.NewXMLBackend(xmlPath, 0)
	if err != nil {
		t.Fatalf("NewXMLBackend: %v", err)
	}
	t.Cleanup(func() { backend.Close() })
	return backend
}

func matchIDs(t *testing.T, backend library.Backend, list *library.ComicListItem) map[string]bool {
	t.Helper()
	books, err := backend.MatchBooks(list)
	if err != nil {
		t.Fatalf("MatchBooks: %v", err)
	}
	ids := make(map[string]bool, len(books))
	for _, b := range books {
		ids[b.ID] = true
	}
	return ids
}

// assertSameMatches runs list against both a fresh SQLite and XML backend
// (built from identical data) and fails if their result sets differ.
func assertSameMatches(t *testing.T, list *library.ComicListItem, wantIDs map[string]bool) {
	t.Helper()

	sqliteBackend := newEdgeCaseSQLiteBackend(t)
	xmlBackend := newEdgeCaseXMLBackend(t)

	got := matchIDs(t, sqliteBackend, list)
	if len(got) != len(wantIDs) {
		t.Errorf("SQLite: expected %d matches %v, got %d %v", len(wantIDs), wantIDs, len(got), got)
	}
	for id := range wantIDs {
		if !got[id] {
			t.Errorf("SQLite: expected %s to match, got %v", id, got)
		}
	}

	xmlGot := matchIDs(t, xmlBackend, list)
	if len(xmlGot) != len(got) {
		t.Fatalf("SQLite and XML backends diverged: SQLite=%v XML=%v", got, xmlGot)
	}
	for id := range got {
		if !xmlGot[id] {
			t.Errorf("SQLite/XML parity failure: SQLite matched %s but XML did not", id)
		}
	}
}

func TestSQLFastPath_ContainsWithLikeWildcardCharacters(t *testing.T) {
	// "100% Off_Beat #1" contains literal % and _ - a naive (unescaped) LIKE
	// translation of Contains("%") or Contains("_") would match everything.
	list := &library.ComicListItem{
		Type: "ComicSmartListItem", MatcherMode: "And",
		Matchers: []library.ComicBookMatcher{
			{Type: "Title", MatchOperator: "1", MatchValue: "% Off_Beat"},
		},
	}
	assertSameMatches(t, list, map[string]bool{"book-1": true})
}

func TestSQLFastPath_EqualsIsCaseInsensitiveSuperset(t *testing.T) {
	// book-1/2/3 all have "Batman" in Series with different casing.
	// SQLite's Equals(0) is documented (and here verified) to be
	// case-insensitive - matching ComicRack's actual case-insensitive
	// string comparison, not just a "safe but overly broad" fallback.
	list := &library.ComicListItem{
		Type: "ComicSmartListItem", MatcherMode: "And",
		Matchers: []library.ComicBookMatcher{
			{Type: "Series", MatchOperator: "0", MatchValue: "batman"},
		},
	}
	assertSameMatches(t, list, map[string]bool{"book-1": true, "book-2": true})
}

func TestSQLFastPath_NumericRangeAcrossBackends(t *testing.T) {
	list := &library.ComicListItem{
		Type: "ComicSmartListItem", MatcherMode: "And",
		Matchers: []library.ComicBookMatcher{
			{Type: "Year", MatchOperator: "3", MatchValue: "2000", MatchValue2: "2020"},
		},
	}
	assertSameMatches(t, list, map[string]bool{"book-1": true, "book-2": true, "book-3": true, "book-4": true})
}

func TestSQLFastPath_NegationFallsBackButStillCorrect(t *testing.T) {
	// Not:true makes the whole list untranslatable (see
	// TestTranslateMatchers_FallsBackOnNegation) - this proves the fallback
	// path still produces the right answer, not just that it doesn't crash.
	list := &library.ComicListItem{
		Type: "ComicSmartListItem", MatcherMode: "And",
		Matchers: []library.ComicBookMatcher{
			{Type: "Publisher", MatchOperator: "0", MatchValue: "DC Comics", Not: true},
		},
	}
	assertSameMatches(t, list, map[string]bool{"book-4": true})
}

func TestSQLFastPath_MixedWithUntranslatableTagsMatcherFallsBackCorrectly(t *testing.T) {
	// Series is translatable but Tags is not - the whole list must fall
	// back to in-memory evaluation and still produce the right AND result.
	list := &library.ComicListItem{
		Type: "ComicSmartListItem", MatcherMode: "And",
		Matchers: []library.ComicBookMatcher{
			{Type: "Series", MatchOperator: "1", MatchValue: "Batman"},
			{Type: "Tags", MatchOperator: "1", MatchValue: "hero"},
		},
	}
	assertSameMatches(t, list, map[string]bool{"book-3": true})
}

func TestSQLFastPath_EnumYesNoUnknownAcrossBackends(t *testing.T) {
	list := &library.ComicListItem{
		Type: "ComicSmartListItem", MatcherMode: "And",
		Matchers: []library.ComicBookMatcher{
			{Type: "BlackAndWhite", MatchOperator: "2"}, // Unknown: NULL, "", or "Unknown"
		},
	}
	assertSameMatches(t, list, map[string]bool{"book-1": true, "book-2": true, "book-3": true, "book-5": true})
}

func TestSQLFastPath_NestedGroupAcrossBackends(t *testing.T) {
	list := &library.ComicListItem{
		Type: "ComicSmartListItem", MatcherMode: "And",
		Matchers: []library.ComicBookMatcher{
			{Type: "Publisher", MatchOperator: "0", MatchValue: "DC Comics"},
			{
				Type:        "ComicBookGroupMatcher",
				MatcherMode: "Or",
				Matchers: []library.ComicBookMatcher{
					{Type: "Year", MatchOperator: "0", MatchValue: "1986"},
					{Type: "Rating", MatchOperator: "0", MatchValue: "4.5"},
				},
			},
		},
	}
	assertSameMatches(t, list, map[string]bool{"book-1": true, "book-5": true})
}
