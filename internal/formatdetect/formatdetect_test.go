package formatdetect

import (
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		book library.ComicBook
		want string
	}{
		{"omnibus keyword", library.ComicBook{Series: "Marvel Omnibus", Count: 1}, FormatOmnibus},
		{"omnibus pages", library.ComicBook{Series: "X", PageCount: 640, Count: 1}, FormatOmnibus},
		{"tpb keyword", library.ComicBook{Series: "X", Title: "Vol 1 TPB", Count: 1}, FormatTPB},
		{"tpb summary collects", library.ComicBook{Series: "X", PageCount: 120, Summary: "Collects the story.", Count: 1}, FormatTPB},
		{"tpb summary range", library.ComicBook{Series: "X", PageCount: 100, Summary: "Reprints #1-5 in one volume", Count: 6}, FormatTPB},
		{"collects but short", library.ComicBook{Series: "X", PageCount: 30, Summary: "Collects stuff", Count: 1}, FormatOneShot},
		{"directors cut", library.ComicBook{Series: "X Director's Cut", Count: 1}, FormatDirectorsCut},
		{"annual", library.ComicBook{Series: "X Annual", Count: 30}, FormatAnnual},
		{"annual not in word", library.ComicBook{Series: "Biannualish", Count: 3}, FormatLimited},
		{"preview", library.ComicBook{Series: "X", Title: "Preview", Count: 1}, FormatPreview},
		{"black and white", library.ComicBook{Series: "X Black & White", Count: 4}, FormatBlackAndWhite},
		{"one-shot", library.ComicBook{Series: "X", Count: 1}, FormatOneShot},
		{"limited low", library.ComicBook{Series: "X", Count: 2}, FormatLimited},
		{"limited high", library.ComicBook{Series: "X", Count: 15}, FormatLimited},
		{"series", library.ComicBook{Series: "X", Count: 16}, FormatSeries},
		{"continuation beats small count", library.ComicBook{Series: "X", Number: "40", Count: 12}, FormatSeries},
		{"continuation number with suffix", library.ComicBook{Series: "X", Number: "16a", Count: 4}, FormatSeries},
		{"issue 15 stays limited", library.ComicBook{Series: "X", Number: "15", Count: 15}, FormatLimited},
		{"one-shot with high number is series", library.ComicBook{Series: "X", Number: "40", Count: 1}, FormatSeries},
		{"one-shot low number", library.ComicBook{Series: "X", Number: "1", Count: 1}, FormatOneShot},
		{"omnibus in summary", library.ComicBook{Series: "X", Summary: "Now in an omnibus edition", Count: 4}, FormatOmnibus},
		{"tpb in summary", library.ComicBook{Series: "X", Summary: "Also available as a TPB", Count: 4}, FormatTPB},
		{"directors cut beats omnibus", library.ComicBook{Series: "X Omnibus Director's Cut", Count: 1}, FormatDirectorsCut},
		{"b&w short form", library.ComicBook{Series: "X B&W", Count: 4}, FormatBlackAndWhite},
		{"previews plural", library.ComicBook{Series: "Previews", Count: 1}, FormatPreview},
		{"500 pages after tpb rules", library.ComicBook{Series: "X", PageCount: 520, Summary: "Collects #1-5", Count: 5}, FormatTPB},
		{"no count", library.ComicBook{Series: "X"}, ""},
		{"no count even with high number", library.ComicBook{Series: "X", Number: "40"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(&tt.book).Format; got != tt.want {
				t.Errorf("Classify = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlan(t *testing.T) {
	base := func() library.ComicBook {
		return library.ComicBook{FilePath: "a.cbz", Series: "X", Count: 6}
	}
	t.Run("empty format is set", func(t *testing.T) {
		b := base()
		p := Plan(&b)
		if p.Decision != Set || p.Format != FormatLimited {
			t.Fatalf("got %+v", p)
		}
	})
	t.Run("wanted placeholder skipped", func(t *testing.T) {
		b := base()
		b.FilePath = ""
		if Plan(&b).Decision != Skip {
			t.Fatal("want skip")
		}
	})
	t.Run("manual format kept", func(t *testing.T) {
		b := base()
		b.Format = "Hardcover"
		if p := Plan(&b); p.Decision != Skip || p.Why != "format set by hand" {
			t.Fatalf("got %+v", p)
		}
	})
	t.Run("locked kept", func(t *testing.T) {
		b := base()
		b.CustomValuesStore = library.SetCustomValue("", KeyLocked, "1")
		if p := Plan(&b); p.Decision != Skip || p.Why != "locked" {
			t.Fatalf("got %+v", p)
		}
	})
	t.Run("our own format is re-detected after count change", func(t *testing.T) {
		b := base()
		Apply(&b, Plan(&b))
		if b.Format != FormatLimited {
			t.Fatalf("first apply: %q", b.Format)
		}
		b.Count = 40
		p := Plan(&b)
		if p.Decision != Set || p.Format != FormatSeries {
			t.Fatalf("got %+v", p)
		}
	})
	t.Run("hand edit after auto is kept", func(t *testing.T) {
		b := base()
		Apply(&b, Plan(&b))
		b.Format = "Graphic Novel"
		b.Count = 40
		if Plan(&b).Decision != Skip {
			t.Fatal("want skip")
		}
	})
	t.Run("unchanged is a no-op", func(t *testing.T) {
		b := base()
		Apply(&b, Plan(&b))
		if p := Plan(&b); p.Decision != Skip || p.Why != "already Limited Series" {
			t.Fatalf("got %+v", p)
		}
	})
}
