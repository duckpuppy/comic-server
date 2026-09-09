package libraryorganizer

import (
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func realProfile() Profile {
	return Profile{
		Months:                DefaultMonths,
		IllegalCharacters:     DefaultIllegalCharacters,
		ReplaceMultipleSpaces: true,
		EmptyFolder:           "",
	}
}

// TestMakeFolderPath_RealDefaultProfileTemplate ground-truths the engine
// against the user's REAL, actively-used "Default" profile template from
// losettingsx.dat (comic-server-3bz.1):
//
//	{<publisher>}\{<imprint>}\{<series>} ({<volume>}{ <format>})
func TestMakeFolderPath_RealDefaultProfileTemplate(t *testing.T) {
	template := `{<publisher>}\{<imprint>}\{<series>} ({<volume>}{ <format>})`

	tests := []struct {
		name string
		book *library.ComicBook
		want []string
	}{
		{
			name: "publisher, series, volume, format all present",
			book: &library.ComicBook{Publisher: "DC Comics", Imprint: "Vertigo", Series: "Sandman", Volume: 1989, Format: "TPB"},
			want: []string{"DC Comics", "Vertigo", "Sandman (1989 TPB)"},
		},
		{
			name: "no imprint - middle segment collapses to empty (EmptyFolder fallback)",
			book: &library.ComicBook{Publisher: "Ace Magazines", Series: "Atomic War!", Volume: 1952},
			want: []string{"Ace Magazines", "", "Atomic War! (1952)"},
		},
		{
			name: "no format - trailing optional group inside the parens drops cleanly",
			book: &library.ComicBook{Publisher: "Marvel", Series: "X-Men", Volume: 1963},
			want: []string{"Marvel", "", "X-Men (1963)"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := MakeFolderPath(tt.book, template, realProfile())
			if !ok {
				t.Fatalf("MakeFolderPath() ok = false, want true")
			}
			if len(got) != len(tt.want) {
				t.Fatalf("MakeFolderPath() = %#v, want %#v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("segment %d = %q, want %q (full: %#v)", i, got[i], tt.want[i], got)
				}
			}
		})
	}
}

// TestMakeFileName_RealDefaultProfileTemplate ground-truths against the
// real "Default" profile's FileTemplate:
//
//	{<series>}{ Vol.<volume>}{ #<number2>}{ (of <count2>)}{ ({<month>, }<year>)}
func TestMakeFileName_RealDefaultProfileTemplate(t *testing.T) {
	template := `{<series>}{ Vol.<volume>}{ #<number2>}{ (of <count2>)}{ ({<month>, }<year>)}`

	tests := []struct {
		name string
		book *library.ComicBook
		want string
	}{
		{
			name: "full metadata: volume, padded number, count, month+year",
			book: &library.ComicBook{Series: "Sandman", Volume: 1989, Number: "1", Count: 75, Month: 1, Year: 1989},
			want: "Sandman Vol.1989 #01 (of 75) (January, 1989)",
		},
		{
			name: "no count, no month - both optional groups drop cleanly, year alone still shows",
			book: &library.ComicBook{Series: "Atomic War!", Volume: 1952, Number: "1", Year: 1952},
			want: "Atomic War! Vol.1952 #01 (1952)",
		},
		{
			name: "non-numeric issue number passes through unpadded",
			book: &library.ComicBook{Series: "X-Men", Volume: 1963, Number: "1A", Year: 1963},
			want: "X-Men Vol.1963 #1A (1963)",
		},
		{
			name: "no year at all - trailing parenthetical group drops entirely",
			book: &library.ComicBook{Series: "Unknown Comic", Volume: 1, Number: "1"},
			want: "Unknown Comic Vol.1 #01",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := MakeFileName(tt.book, template, realProfile())
			if !ok {
				t.Fatalf("MakeFileName() ok = false, want true")
			}
			if got != tt.want {
				t.Errorf("MakeFileName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMakeFolderPath_IllegalCharactersReplacedPerSegment(t *testing.T) {
	book := &library.ComicBook{Publisher: `A/B: Comics`, Series: "Foo"}
	template := `{<publisher>}\{<series>}`
	got, ok := MakeFolderPath(book, template, realProfile())
	if !ok {
		t.Fatalf("ok = false")
	}
	want := []string{"AB - Comics", "Foo"} // '/' -> '', ':' -> ' - ' per DefaultIllegalCharacters
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestMakeFolderPath_UnknownFieldReportedInvalid(t *testing.T) {
	book := &library.ComicBook{Series: "Foo"}
	_, ok := MakeFolderPath(book, `{<series>}\{<notarealfield>}`, realProfile())
	if ok {
		t.Error("expected ok=false for a template referencing an unsupported field")
	}
}

func TestPadNumeric(t *testing.T) {
	tests := []struct {
		value string
		width int
		want  string
	}{
		{"1", 2, "01"},
		{"75", 2, "75"},
		{"1.5", 3, "001.5"},
		{"-1", 2, "-01"},
		{"1A", 2, "1A"}, // non-numeric passes through unpadded
		{"", 2, ""},
	}
	for _, tt := range tests {
		if got := padNumeric(tt.value, tt.width); got != tt.want {
			t.Errorf("padNumeric(%q, %d) = %q, want %q", tt.value, tt.width, got, tt.want)
		}
	}
}

func TestMakeFileName_ReplaceMultipleSpacesCollapsesRuns(t *testing.T) {
	// A missing Series produces a leading run of spaces before "Vol." -
	// ReplaceMultipleSpaces (on for the real Default profile) collapses
	// it to a single space, matching the Python's own \s\s+ -> " " pass.
	book := &library.ComicBook{Volume: 1, Number: "1"}
	template := `{<series>}{ Vol.<volume>}{ #<number>}`
	got, ok := MakeFileName(book, template, realProfile())
	if !ok {
		t.Fatalf("ok = false")
	}
	if strings.Contains(got, "  ") {
		t.Errorf("expected no double-space runs, got %q", got)
	}
}
