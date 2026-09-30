package library

import (
	"strings"
	"testing"
)

func testCaptionBook() *ComicBook {
	return &ComicBook{
		ID:        "b1",
		FilePath:  `C:\Comics\Marvel\X-Men 012.cbz`,
		Series:    "X-Men",
		Number:    "12",
		Count:     50,
		Volume:    2,
		Year:      1992,
		Month:     3,
		Publisher: "Marvel",
		Title:     "Fatal Attractions, Part 1",
		Rating:    4.5,
		PageCount: 32,
	}
}

func TestExpandCaptionFormat(t *testing.T) {
	book := testCaptionBook()
	tests := []struct {
		name   string
		format string
		want   string
	}{
		{"plain field", "{Series}", "X-Men"},
		{"field case insensitive", "{sErIeS}", "X-Men"},
		{"literal text", "Hello {Publisher}!", "Hello Marvel!"},
		// Number resolves to NumberAsText, which appends "(of Count)".
		{"number uses AsText", "#{Number}", "#12 (of 50)"},
		{"number only", "#{NumberOnly}", "#12"},
		{"volume as text", "{Volume}", "V2"},
		{"year", "{Year}", "1992"},
		{"rating as text", "{Rating}", "4.5"},
		{"numeric format", "{Year:00000}", "01992"},
		{"numeric format on non-numeric ignored", "{Series:0000}", "X-Men"},
		{"escape", `a\[b\]c`, "a[b]c"},
		{"unknown field is empty", "x{Nope}y", "xy"},

		{"optional group shown", "{Series}[ v{Publisher}]", "X-Men vMarvel"},
		{"optional group dropped when field empty", "{Series}[ {Imprint}]", "X-Men"},
		{"optional group dropped when any field empty", "{Series}[ {Publisher}/{Imprint}]", "X-Men"},
		{"optional group keeps literal when fields set", "[({Year})]", "(1992)"},
		{"optional group, unset volume", "{Series}[ {Volume}]", "X-Men V2"},

		{"filename", "{FileName}", "X-Men 012"},
		{"file directory raw", "{FileNameWithExtension}", "X-Men 012.cbz"},

		{"function year", "$year<1992-03-15>", "1992"},
		{"function month is D4", "$month<1992-03-15>", "0003"},
		{"function day is D4", "$day<1992-03-15>", "0015"},
		{"function date", "$date<1992-03-15,yyyy/MM/dd>", "1992/03/15"},
		{"function date month name", "$date<1992-03-15,MMM d yyyy>", "Mar 15 1992"},
		{"function int", "$int<42>", "42"},
		{"function int bad", "$int<abc>", "-1"},
		{"function double", "$double<2.5>", "2.5"},
		{"function double bad", "$double<x>", "-1"},
		{"function expr int math", "$expr<2+3*4>", "14"},
		{"function expr int division truncates", "$expr<7/2>", "3"},
		{"function expr double", "$expr<7/2.0>", "3.5"},
		{"function expr from field", "$expr<{Year}-1900>", "92"},
		{"function expr string concat", `$expr<"a"+"b">`, "ab"},
		{"function expr comparison needs escaped >", `$expr<3\>2>`, "True"},
		{"unescaped > ends the function (ComicRack behavior)", "$expr<3>2>", "32>"},
		{"function if true", "$if<1==1,yes,no>", "yes"},
		{"function if false", "$if<1==2,yes,no>", "no"},
		{"function if literal", "$if<true,yes,no>", "yes"},
		{"function if expr on field", `$if<{PageCount}\>=32,long,short>`, "long"},
		{"function if && ||", "$if<1==1 && 2==3 || 4==4,yes,no>", "yes"},
		{"function if non-boolean is empty", "$if<5,yes,no>", ""},
		{"function not", "$not<true>", "False"},
		{"function substring", "$substring<abcdef,1,3>", "bcd"},
		{"function substring to end", "$substring<abcdef,2,-1>", "cdef"},
		{"function regexmatch true", "$regexmatch<X-Men,^x>", "True"},
		{"function regexmatch false", "$regexmatch<X-Men,^y>", "False"},
		{"function regexreplace", `$regexreplace<a1b22,\[0-9\]+,#>`, "a#b#"},
		{"unescaped [ opens an optional group inside args (ComicRack behavior)", "$regexreplace<a1b22,[0-9]+,#>", "a1b22"},
		{"function regexreplace group", `$regexreplace<ab,(a)(b),\$2\$1>`, "ba"},
		{"function escape", "$escape<a'b>", `a\'b`},
		{"nested functions", "$if<$regexmatch<{Series},men>,match,nomatch>", "match"},
		{"function inside optional group, empty result drops group", "{Series}[-$regexreplace<{Series},.*,>]", "X-Men"},
		{"field with comma inside function arg is not split", "$escape<{Title}>", `Fatal Attractions\, Part 1`},
		{"function using field with comma in regexmatch", "$regexmatch<{Title},Attractions>", "True"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandCaptionFormat(tt.format, book)
			if got != tt.want {
				t.Errorf("ExpandCaptionFormat(%q) = %q, want %q", tt.format, got, tt.want)
			}
		})
	}
}

func TestExpandCaptionFormat_Errors(t *testing.T) {
	book := testCaptionBook()
	for _, format := range []string{
		"[{Series}",                // unbalanced bracket
		"{Series",                  // unbalanced brace
		`trailing\`,                // trailing backslash
		"$nosuchfunc<1>",           // unknown function
		"$year<notadate>",          // unparseable date
		"$int<1,2>",                // wrong arg count
		"$expr<1/0>",               // divide by zero
		"$if<garbage!!,a,b>",       // bad condition expression
		"$not<maybe>",              // not of a non-boolean
		"$substring<abc,5,1>",      // start past end
		"$regexmatch<a,(unclosed>", // bad regex
	} {
		got := ExpandCaptionFormat(format, book)
		if !strings.HasPrefix(got, "#ERROR") {
			t.Errorf("ExpandCaptionFormat(%q) = %q, want an #ERROR result", format, got)
		}
	}
}

// ComicRack's bracket success bookkeeping is `success |= nestedSuccess`, so a
// nested group that renders can re-validate an outer group that already saw
// an empty field. Ported as-is; this pins that behavior.
func TestExpandCaptionFormat_NestedBracketQuirk(t *testing.T) {
	book := testCaptionBook()
	got := ExpandCaptionFormat("[{Imprint}[{Series}]]", book)
	if got != "X-Men" {
		t.Errorf("got %q, want %q (nested group re-validates the outer one, as in ComicRack)", got, "X-Men")
	}
}

func TestExpandCaptionFormat_EmptyFormat(t *testing.T) {
	if got := ExpandCaptionFormat("", testCaptionBook()); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestFormatDotNetNumber(t *testing.T) {
	tests := []struct {
		f    float64
		spec string
		want string
	}{
		{5, "0000", "0005"},
		{5, "D3", "005"},
		{1234.5, "N1", "1,234.5"},
		{1234.5, "#,##0.00", "1,234.50"},
		{3.14159, "0.00", "3.14"},
		{3.1, "0.##", "3.1"},
		{0.25, "0%", "25%"},
		{7, "F2", "7.00"},
		{-5, "000", "-005"},
	}
	for _, tt := range tests {
		got, ok := formatDotNetNumber(tt.f, tt.spec)
		if !ok || got != tt.want {
			t.Errorf("formatDotNetNumber(%v, %q) = %q (ok=%v), want %q", tt.f, tt.spec, got, ok, tt.want)
		}
	}
}

func TestVirtualTagMatcher(t *testing.T) {
	SetVirtualTags([]VirtualTag{
		{ID: 1, Name: "Label", CaptionFormat: "{Series}[ v{Volume}]", Enabled: true},
		{ID: 2, Name: "Disabled", CaptionFormat: "{Series}", Enabled: false},
		{ID: 3, Name: "", CaptionFormat: "{Series}", Enabled: true},
		{ID: 4, Name: "Decade", CaptionFormat: "$substring<{Year},0,3>0s", Enabled: true},
	})
	t.Cleanup(func() { SetVirtualTags(nil) })

	book := testCaptionBook()

	tests := []struct {
		name     string
		xmlType  string
		operator string
		value    string
		not      bool
		want     bool
	}{
		{"equals", "ComicBookVirtualTag1Matcher", "0", "X-Men vV2", false, true},
		{"equals is case-insensitive", "ComicBookVirtualTag1Matcher", "0", "x-men vv2", false, true},
		{"contains", "ComicBookVirtualTag1Matcher", "1", "men", false, true},
		{"starts with", "ComicBookVirtualTag1Matcher", "4", "X-", false, true},
		{"no match", "ComicBookVirtualTag1Matcher", "0", "Batman", false, false},
		{"negated", "ComicBookVirtualTag1Matcher", "0", "Batman", true, true},
		{"disabled slot is empty", "ComicBookVirtualTag2Matcher", "0", "X-Men", false, false},
		{"disabled slot matches empty", "ComicBookVirtualTag2Matcher", "0", "", false, true},
		{"nameless slot inactive", "ComicBookVirtualTag3Matcher", "0", "X-Men", false, false},
		{"undefined slot is empty", "ComicBookVirtualTag9Matcher", "0", "X-Men", false, false},
		{"function-based tag", "ComicBookVirtualTag4Matcher", "0", "1990s", false, true},
		{"regex operator", "ComicBookVirtualTag1Matcher", "7", `^X-.*V\d$`, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := NewMatcherFromXML(&ComicBookMatcher{
				Type:          tt.xmlType,
				MatchOperator: tt.operator,
				MatchValue:    tt.value,
				Not:           tt.not,
			})
			if err != nil {
				t.Fatalf("NewMatcherFromXML: %v", err)
			}
			if got := m.Match(book); got != tt.want {
				t.Errorf("Match = %v, want %v (tag value %q)", got, tt.want, VirtualTagValue(book, 1))
			}
		})
	}
}

func TestVirtualTagSlot(t *testing.T) {
	for in, want := range map[MatcherType]int{"VirtualTag1": 1, "VirtualTag20": 20, "VirtualTag07": 7} {
		if got, ok := virtualTagSlot(in); !ok || got != want {
			t.Errorf("virtualTagSlot(%q) = %d,%v, want %d,true", in, got, ok, want)
		}
	}
	for _, in := range []MatcherType{"VirtualTag0", "VirtualTag21", "VirtualTag", "VirtualTagX", "Series", "Virtual"} {
		if _, ok := virtualTagSlot(in); ok {
			t.Errorf("virtualTagSlot(%q) unexpectedly ok", in)
		}
	}
}

func TestSetVirtualTags_ReplacesDefinitions(t *testing.T) {
	t.Cleanup(func() { SetVirtualTags(nil) })
	book := testCaptionBook()

	SetVirtualTags([]VirtualTag{{ID: 1, Name: "A", CaptionFormat: "one", Enabled: true}})
	if got := VirtualTagValue(book, 1); got != "one" {
		t.Fatalf("got %q, want one", got)
	}
	SetVirtualTags([]VirtualTag{{ID: 1, Name: "A", CaptionFormat: "two", Enabled: true}})
	if got := VirtualTagValue(book, 1); got != "two" {
		t.Fatalf("got %q, want two", got)
	}
	SetVirtualTags(nil)
	if got := VirtualTagValue(book, 1); got != "" {
		t.Fatalf("got %q, want empty after clearing", got)
	}
}
