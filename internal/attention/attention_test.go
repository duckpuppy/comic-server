package attention

import (
	"reflect"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestFlagsFor(t *testing.T) {
	good := library.ComicBook{FilePath: "a/b.cbz", Series: "S", Count: 12, Publisher: "P", Format: "Series", PageCount: 24}
	tests := []struct {
		name string
		edit func(b *library.ComicBook)
		want []Flag
	}{
		{"clean", func(*library.ComicBook) {}, nil},
		{"pdf", func(b *library.ComicBook) { b.FilePath = "x.PDF" }, []Flag{FlagIsPDF}},
		{"zip", func(b *library.ComicBook) { b.FilePath = "x.zip" }, []Flag{FlagIsZip}},
		{"cbr", func(b *library.ComicBook) { b.FilePath = "x.cbr" }, []Flag{FlagNeedsConversion}},
		{"cover only", func(b *library.ComicBook) { b.FilePath = `C:\x\My Cover.cbz`; b.PageCount = 2 }, []Flag{FlagCoverOnly}},
		{"cover name but many pages", func(b *library.ComicBook) { b.FilePath = "Covers.cbz"; b.PageCount = 40 }, nil},
		{"low pages", func(b *library.ComicBook) { b.PageCount = 4 }, []Flag{FlagLowPageCount}},
		{"uncounted pages not low", func(b *library.ComicBook) { b.PageCount = 0 }, nil},
		{"no series", func(b *library.ComicBook) { b.Series = " " }, []Flag{FlagNoSeries}},
		{"missing count", func(b *library.ComicBook) { b.Count = 0 }, []Flag{FlagMissingCount}},
		{"no series suppresses missing count", func(b *library.ComicBook) { b.Series = ""; b.Count = 0 }, []Flag{FlagNoSeries}},
		{"publisher", func(b *library.ComicBook) { b.Publisher = "" }, []Flag{FlagUnknownPublisher}},
		{"format", func(b *library.ComicBook) { b.Format = "" }, []Flag{FlagMissingFormat}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := good
			tt.edit(&b)
			if got := FlagsFor(&b); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSummarize_RanksSeverityThenCount(t *testing.T) {
	got := Summarize(map[Flag]int{
		FlagUnknownPublisher: 500, // info, big
		FlagIsPDF:            3,   // warn, small
		FlagNoSeries:         10,  // warn, bigger
		FlagMissingFormat:    0,   // omitted
	})
	var order []Flag
	for _, s := range got {
		order = append(order, s.Flag)
	}
	want := []Flag{FlagNoSeries, FlagIsPDF, FlagUnknownPublisher}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
}

func TestCatalogComplete(t *testing.T) {
	seen := map[Flag]bool{}
	for _, i := range Catalog {
		if seen[i.Flag] || i.Label == "" {
			t.Errorf("bad catalog entry %+v", i)
		}
		seen[i.Flag] = true
	}
}
