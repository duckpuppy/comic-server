package relink

import (
	"reflect"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestFind(t *testing.T) {
	book := &library.ComicBook{Series: "Batman", Number: "5"}
	cases := []struct {
		name    string
		oldName string
		names   []string
		taken   map[string]bool
		want    Result
	}{
		{"converted in place", "Batman 005.cbr", []string{"Batman 005.cbz", "notes.txt"}, nil,
			Result{Outcome: Found, Match: "Batman 005.cbz", Reason: ReasonSameName}},
		{"case rename", "batman 005.cbz", []string{"Batman 005.cbz"}, nil,
			Result{Outcome: Found, Match: "Batman 005.cbz", Reason: ReasonSameName}},
		{"renamed, matched by series and number", "old name.cbz", []string{"Batman 005 (2016).cbz", "Batman 006 (2016).cbz"}, nil,
			Result{Outcome: Found, Match: "Batman 005 (2016).cbz", Reason: ReasonSeriesAndN}},
		{"leading zeros equal", "gone.cbz", []string{"Batman #05.cbz"}, nil,
			Result{Outcome: Found, Match: "Batman #05.cbz", Reason: ReasonSeriesAndN}},
		{"another series is not a match", "gone.cbz", []string{"Batman Beyond 005.cbz"}, nil,
			Result{Outcome: NotFound}},
		{"two fits are ambiguous", "gone.cbz", []string{"Batman 005.cbz", "Batman 005 (Variant).cbz"}, nil,
			Result{Outcome: Ambiguous, Candidates: []string{"Batman 005.cbz", "Batman 005 (Variant).cbz"}}},
		{"file used by another book is skipped", "gone.cbz", []string{"Batman 005.cbz"}, map[string]bool{"Batman 005.cbz": true},
			Result{Outcome: NotFound}},
		{"non-comic files ignored", "gone.cbz", []string{"Batman 005.jpg"}, nil,
			Result{Outcome: NotFound}},
		{"empty folder", "gone.cbz", nil, nil, Result{Outcome: NotFound}},
	}
	for _, c := range cases {
		taken := func(n string) bool { return c.taken[n] }
		got := Find(c.oldName, book, c.names, taken)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestFind_NoSeriesNeverGuesses(t *testing.T) {
	got := Find("gone.cbz", &library.ComicBook{}, []string{"Anything 001.cbz"}, nil)
	if got.Outcome != NotFound {
		t.Errorf("got %+v, want not_found", got)
	}
}

func TestSplitRawPath(t *testing.T) {
	cases := []struct{ in, dir, name string }{
		{`D:\Comics\DC\Batman 005.cbr`, `D:\Comics\DC\`, "Batman 005.cbr"},
		{"/data/comics/Batman 005.cbr", "/data/comics/", "Batman 005.cbr"},
		{"Batman.cbz", "", "Batman.cbz"},
	}
	for _, c := range cases {
		d, n := SplitRawPath(c.in)
		if d != c.dir || n != c.name {
			t.Errorf("SplitRawPath(%q) = %q, %q", c.in, d, n)
		}
	}
}
