package attention

import (
	"slices"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func cvBook(id, num string, count int, store string) *library.ComicBook {
	return &library.ComicBook{ID: id, Series: "Batman", Number: num, Count: count, CustomValuesStore: store}
}

func TestCVFlagsFor(t *testing.T) {
	cv := &CVData{
		Counts: map[int]int{100: 12},
		Issues: map[int]CVIssue{1: {Volume: 100, Number: "1"}, 2: {Volume: 200, Number: "2"}},
	}
	cases := []struct {
		name string
		book *library.ComicBook
		want []Flag
	}{
		{"all agree", cvBook("a", "001", 12, "comicvine_volume=100,comicvine_issue=1"), nil},
		{"count differs", cvBook("b", "1", 10, "comicvine_volume=100,comicvine_issue=1"), []Flag{FlagCountMismatch}},
		{"count zero is missing_count's job", cvBook("c", "1", 0, "comicvine_volume=100,comicvine_issue=1"), nil},
		{"number differs", cvBook("d", "3", 12, "comicvine_volume=100,comicvine_issue=1"), []Flag{FlagMetadataMismatch}},
		{"issue from other volume", cvBook("e", "2", 12, "comicvine_volume=100,comicvine_issue=2"), []Flag{FlagMetadataMismatch}},
		{"unsynced volume and unknown issue", cvBook("f", "9", 5, "comicvine_volume=300,comicvine_issue=9"), nil},
		{"untagged", cvBook("g", "1", 5, ""), nil},
	}
	for _, tc := range cases {
		if got := CVFlagsFor(tc.book, cv); !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	if got := CVFlagsFor(cases[1].book, nil); got != nil {
		t.Errorf("nil cache: got %v", got)
	}
}

func TestVolumeConflicts(t *testing.T) {
	vb := func(id, series string, vol int, format string, cv string) *library.ComicBook {
		return &library.ComicBook{ID: id, Series: series, Volume: vol, Format: format, CustomValuesStore: "comicvine_volume=" + cv}
	}
	books := []*library.ComicBook{
		// Majority 100, one outlier.
		vb("a1", "Batman", 2011, "", "100"),
		vb("a2", "The Batman", 2011, "", "100"),
		vb("a3", "Batman", 2011, "", "999"),
		// Annuals of the same series are their own sibling set.
		vb("an", "Batman", 2011, "Annual", "555"),
		// No Volume: never compared.
		vb("n1", "Robin", 0, "", "1"),
		vb("n2", "Robin", 0, "", "2"),
		// Tie: both flagged.
		vb("t1", "Flash", 1987, "", "7"),
		vb("t2", "Flash", 1987, "", "8"),
	}
	got := VolumeConflicts(books)
	want := map[string]bool{"a3": true, "t1": true, "t2": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for id := range want {
		if !got[id] {
			t.Errorf("missing %s in %v", id, got)
		}
	}
}

func TestAccepted(t *testing.T) {
	b := &library.ComicBook{CustomValuesStore: "comicvine_volume=1"}
	if Accepted(b) != nil {
		t.Fatal("new book has accepted flags")
	}
	if !SetAccepted(b, FlagCountMismatch, true) || !SetAccepted(b, FlagCVVolumeConflict, true) {
		t.Fatal("accept reported no change")
	}
	if SetAccepted(b, FlagCountMismatch, true) {
		t.Error("re-accept reported a change")
	}
	if v, _ := library.GetCustomValue(b.CustomValuesStore, KeyAccepted); v != "count_mismatch+cv_volume_conflict" {
		t.Errorf("stored %q", v)
	}
	SetAccepted(b, FlagCountMismatch, false)
	SetAccepted(b, FlagCVVolumeConflict, false)
	if len(Accepted(b)) != 0 {
		t.Errorf("accepted = %v", Accepted(b))
	}
	if v, _ := library.GetCustomValue(b.CustomValuesStore, "comicvine_volume"); v != "1" {
		t.Error("other custom values changed")
	}
}

func TestFormatMismatches(t *testing.T) {
	fb := func(id, cv, format string) *library.ComicBook {
		return &library.ComicBook{ID: id, Format: format, CustomValuesStore: "comicvine_volume=" + cv}
	}
	books := []*library.ComicBook{
		fb("a1", "100", "Series"),
		fb("a2", "100", "series "), // case and space ignored
		fb("a3", "100", "Limited Series"),
		fb("a4", "100", ""), // empty: missing_format's job
		fb("b1", "200", "One-Shot"),
		fb("t1", "300", "Series"),
		fb("t2", "300", "TPB"),
		{ID: "u1", Format: "TPB"}, // untagged
	}
	got := FormatMismatches(books)
	want := map[string]bool{"a3": true, "t1": true, "t2": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for id := range want {
		if !got[id] {
			t.Errorf("missing %s in %v", id, got)
		}
	}
}
