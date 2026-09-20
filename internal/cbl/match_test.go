package cbl

import (
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func book(id, series, number string, volume, year int, format, cvIssue string) *library.ComicBook {
	b := &library.ComicBook{ID: id, Series: series, Number: number, Volume: volume, Year: year, Format: format}
	if cvIssue != "" {
		b.CustomValuesStore = library.SetCustomValue(b.CustomValuesStore, "comicvine_issue", cvIssue)
	}
	return b
}

func TestMatchEntry_CVIDPath(t *testing.T) {
	books := []*library.ComicBook{
		book("1", "Batman", "1", 1940, 1940, "", "105811"),
		book("2", "Not Batman At All", "1", 1940, 1940, "", "999999"), // wrong CV id, must not match
	}
	entry := Book{Series: "Batman", Number: "1", Volume: 1940, Year: 1940, Database: []Database{{Name: "cv", Issue: "105811"}}}

	m := MatchEntry(entry, books)
	if m.Path != MatchCVID {
		t.Fatalf("Path = %v, want MatchCVID", m.Path)
	}
	if m.Book == nil || m.Book.ID != "1" {
		t.Fatalf("matched wrong book: %+v", m.Book)
	}
}

func TestMatchEntry_CVIDPresentButNoLibraryHit_FallsBackToStringMatch(t *testing.T) {
	// CV ID in the CBL doesn't exist in the library (e.g. book owned
	// under a different/older CV mapping) - must still fall back to the
	// string-matching path rather than reporting no match.
	books := []*library.ComicBook{
		book("1", "Batman", "1", 1940, 1940, "", ""), // no CV tag at all
	}
	entry := Book{Series: "Batman", Number: "1", Volume: 1940, Year: 1940, Database: []Database{{Name: "cv", Issue: "999999"}}}

	m := MatchEntry(entry, books)
	if m.Path != MatchSeriesNumber {
		t.Fatalf("Path = %v, want MatchSeriesNumber (fallback)", m.Path)
	}
	if m.Book == nil || m.Book.ID != "1" {
		t.Fatalf("matched wrong book: %+v", m.Book)
	}
}

func TestMatchEntry_ExactSeriesNumber(t *testing.T) {
	books := []*library.ComicBook{book("1", "Detective Comics", "27", 1937, 1939, "", "")}
	entry := Book{Series: "Detective Comics", Number: "27", Volume: 1937, Year: 1939}

	m := MatchEntry(entry, books)
	if m.Path != MatchSeriesNumber || m.Book.ID != "1" {
		t.Fatalf("expected exact match, got %+v", m)
	}
}

func TestMatchEntry_IgnoreVolumeInNameFallback(t *testing.T) {
	// Library has "Saga Vol. 2", CBL just says "Saga" - exact pass fails,
	// IgnoreVolumeInName pass should catch it.
	books := []*library.ComicBook{book("1", "Saga Vol. 2", "1", -1, 2014, "", "")}
	entry := Book{Series: "Saga", Number: "1", Volume: -1, Year: 2014}

	m := MatchEntry(entry, books)
	if m.Path != MatchSeriesNumber || m.Book.ID != "1" {
		t.Fatalf("expected IgnoreVolumeInName fallback match, got %+v", m)
	}
}

func TestMatchEntry_StripDownFallback(t *testing.T) {
	// "The Amazing Spider-Man" vs "Amazing Spider Man" - only the
	// loosest (StripDown) pass should catch this.
	books := []*library.ComicBook{book("1", "The Amazing Spider-Man", "1", -1, 1963, "", "")}
	entry := Book{Series: "Amazing Spider Man", Number: "1", Volume: -1, Year: 1963}

	m := MatchEntry(entry, books)
	if m.Path != MatchSeriesNumber || m.Book.ID != "1" {
		t.Fatalf("expected StripDown fallback match, got %+v", m)
	}
}

func TestMatchEntry_AmbiguousNarrowsByYearThenVolumeThenFormat(t *testing.T) {
	books := []*library.ComicBook{
		book("1", "X-Men", "1", 1, 1963, "TPB", ""),
		book("2", "X-Men", "1", 2, 1991, "", ""),
		book("3", "X-Men", "1", 3, 2004, "", ""),
	}
	entry := Book{Series: "X-Men", Number: "1", Volume: 2, Year: 1991, Format: ""}

	m := MatchEntry(entry, books)
	if m.Path != MatchSeriesNumber || m.Book.ID != "2" {
		t.Fatalf("expected narrowing to book 2, got %+v", m)
	}
}

func TestMatchEntry_NoMatch(t *testing.T) {
	books := []*library.ComicBook{book("1", "Batman", "1", 1940, 1940, "", "")}
	entry := Book{Series: "Superman", Number: "1", Volume: 1938, Year: 1938}

	m := MatchEntry(entry, books)
	if m.Path != MatchNone || m.Book != nil {
		t.Fatalf("expected no match, got %+v", m)
	}
}

func TestMatchEntry_StillAmbiguousTakesFirst(t *testing.T) {
	// Two genuinely identical candidates (same series/number/year/volume,
	// no format on either) - reference algorithm takes FirstOrDefault,
	// not "give up" (spec §2b step 7).
	books := []*library.ComicBook{
		book("1", "Weird Duplicates", "1", -1, 2000, "", ""),
		book("2", "Weird Duplicates", "1", -1, 2000, "", ""),
	}
	entry := Book{Series: "Weird Duplicates", Number: "1", Volume: -1, Year: 2000}

	m := MatchEntry(entry, books)
	if m.Path != MatchSeriesNumber || m.Book == nil {
		t.Fatalf("expected a match despite ambiguity, got %+v", m)
	}
	if m.Candidate != 2 {
		t.Errorf("Candidate count = %d, want 2 (diagnostic for match-correction UI)", m.Candidate)
	}
}
