package storage

import (
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestTranslateMatchers_SimpleEquals(t *testing.T) {
	pred, ok := translateMatchers("And", []library.ComicBookMatcher{
		{Type: "Series", MatchOperator: "0", MatchValue: "Batman"},
	})
	if !ok {
		t.Fatal("expected translation to succeed")
	}
	if pred.where != "(series = ? COLLATE NOCASE)" {
		t.Errorf("unexpected WHERE: %q", pred.where)
	}
	if len(pred.args) != 1 || pred.args[0] != "Batman" {
		t.Errorf("unexpected args: %+v", pred.args)
	}
}

func TestTranslateMatchers_ContainsEscapesWildcards(t *testing.T) {
	pred, ok := translateMatchers("And", []library.ComicBookMatcher{
		{Type: "Title", MatchOperator: "1", MatchValue: "100% off_beat"},
	})
	if !ok {
		t.Fatal("expected translation to succeed")
	}
	want := `%100\% off\_beat%`
	if len(pred.args) != 1 || pred.args[0] != want {
		t.Errorf("expected escaped LIKE pattern %q, got %+v", want, pred.args)
	}
}

func TestTranslateMatchers_NumericRange(t *testing.T) {
	pred, ok := translateMatchers("And", []library.ComicBookMatcher{
		{Type: "Year", MatchOperator: "3", MatchValue: "2000", MatchValue2: "2010"},
	})
	if !ok {
		t.Fatal("expected translation to succeed")
	}
	if pred.where != "((year >= ? AND year <= ?))" {
		t.Errorf("unexpected WHERE: %q", pred.where)
	}
	if len(pred.args) != 2 || pred.args[0] != 2000.0 || pred.args[1] != 2010.0 {
		t.Errorf("unexpected args: %+v", pred.args)
	}
}

func TestTranslateMatchers_AndOrCombination(t *testing.T) {
	andPred, ok := translateMatchers("And", []library.ComicBookMatcher{
		{Type: "Series", MatchOperator: "0", MatchValue: "Batman"},
		{Type: "Year", MatchOperator: "1", MatchValue: "2019"},
	})
	if !ok || andPred.where != "(series = ? COLLATE NOCASE AND year > ?)" {
		t.Fatalf("unexpected AND translation: ok=%v where=%q", ok, andPred.where)
	}

	orPred, ok := translateMatchers("Or", []library.ComicBookMatcher{
		{Type: "Series", MatchOperator: "0", MatchValue: "Batman"},
		{Type: "Publisher", MatchOperator: "0", MatchValue: "Marvel Comics"},
	})
	if !ok || orPred.where != "(series = ? COLLATE NOCASE OR publisher = ? COLLATE NOCASE)" {
		t.Fatalf("unexpected OR translation: ok=%v where=%q", ok, orPred.where)
	}
}

func TestTranslateMatchers_NestedGroup(t *testing.T) {
	pred, ok := translateMatchers("And", []library.ComicBookMatcher{
		{Type: "Publisher", MatchOperator: "0", MatchValue: "DC Comics"},
		{
			Type:        "ComicBookGroupMatcher",
			MatcherMode: "Or",
			Matchers: []library.ComicBookMatcher{
				{Type: "Series", MatchOperator: "0", MatchValue: "Batman"},
				{Type: "Series", MatchOperator: "0", MatchValue: "Superman"},
			},
		},
	})
	if !ok {
		t.Fatal("expected translation to succeed")
	}
	want := "(publisher = ? COLLATE NOCASE AND (series = ? COLLATE NOCASE OR series = ? COLLATE NOCASE))"
	if pred.where != want {
		t.Errorf("expected %q, got %q", want, pred.where)
	}
}

func TestTranslateMatchers_BoolYesNo(t *testing.T) {
	yes, ok := translateMatchers("And", []library.ComicBookMatcher{{Type: "Checked", MatchOperator: "0"}})
	if !ok || yes.where != "(checked = 1)" {
		t.Fatalf("expected Checked=Yes to translate to checked = 1, got ok=%v where=%q", ok, yes.where)
	}

	no, ok := translateMatchers("And", []library.ComicBookMatcher{{Type: "IsMissing", MatchOperator: "1"}})
	if !ok || no.where != "(file_is_missing = 0)" {
		t.Fatalf("expected IsMissing=No to translate to file_is_missing = 0, got ok=%v where=%q", ok, no.where)
	}

	// Unknown has no meaning for a native bool column - must fall back.
	if _, ok := translateMatchers("And", []library.ComicBookMatcher{{Type: "Checked", MatchOperator: "2"}}); ok {
		t.Error("expected Checked=Unknown to be untranslatable")
	}
}

func TestTranslateMatchers_EnumYesNoUnknown(t *testing.T) {
	pred, ok := translateMatchers("And", []library.ComicBookMatcher{{Type: "BlackAndWhite", MatchOperator: "2"}})
	if !ok {
		t.Fatal("expected translation to succeed")
	}
	if pred.where != "((black_and_white IS NULL OR black_and_white = '' OR black_and_white = ? COLLATE NOCASE))" {
		t.Errorf("unexpected WHERE: %q", pred.where)
	}
}

// --- Cases that must fall back to the in-memory path ---

func TestTranslateMatchers_FallsBackOnNegation(t *testing.T) {
	if _, ok := translateMatchers("And", []library.ComicBookMatcher{
		{Type: "Series", MatchOperator: "0", MatchValue: "Batman", Not: true},
	}); ok {
		t.Error("expected a negated matcher to be untranslatable")
	}
}

func TestTranslateMatchers_FallsBackOnUnsupportedType(t *testing.T) {
	for _, mt := range []string{"Tags", "CustomValues", "Expression", "ComicBookDuplicateMatcher", "SeriesAllComplete", "CVSeriesComplete", "AllProperties", "Directory"} {
		if _, ok := translateMatchers("And", []library.ComicBookMatcher{{Type: mt, MatchOperator: "0", MatchValue: "x"}}); ok {
			t.Errorf("expected matcher type %q to be untranslatable", mt)
		}
	}
}

func TestTranslateMatchers_FallsBackOnEmptyNumericValue(t *testing.T) {
	// matchNumeric treats "" specially (field unset); not worth replicating.
	if _, ok := translateMatchers("And", []library.ComicBookMatcher{{Type: "Year", MatchOperator: "0", MatchValue: ""}}); ok {
		t.Error("expected empty numeric MatchValue to be untranslatable")
	}
}

func TestTranslateMatchers_FallsBackOnUnparsableNumericValue(t *testing.T) {
	if _, ok := translateMatchers("And", []library.ComicBookMatcher{{Type: "Year", MatchOperator: "0", MatchValue: "not-a-number"}}); ok {
		t.Error("expected unparsable numeric MatchValue to be untranslatable")
	}
}

func TestTranslateMatchers_FallsBackOnUnsupportedOperator(t *testing.T) {
	// ContainsAny (2) has no safe single-LIKE translation for strings.
	if _, ok := translateMatchers("And", []library.ComicBookMatcher{{Type: "Series", MatchOperator: "2", MatchValue: "Bat,Man"}}); ok {
		t.Error("expected ContainsAny to be untranslatable")
	}
}

func TestTranslateMatchers_OneUntranslatableMatcherFailsTheWholeList(t *testing.T) {
	// Even though Series is translatable, Tags isn't - the whole list must
	// fall back so the two matchers are still evaluated together correctly.
	if _, ok := translateMatchers("And", []library.ComicBookMatcher{
		{Type: "Series", MatchOperator: "0", MatchValue: "Batman"},
		{Type: "Tags", MatchOperator: "0", MatchValue: "hero"},
	}); ok {
		t.Error("expected a mixed translatable/untranslatable list to be untranslatable as a whole")
	}
}

func TestTranslateMatchers_EmptyMatchersList(t *testing.T) {
	if _, ok := translateMatchers("And", nil); ok {
		t.Error("expected an empty matcher list to be untranslatable")
	}
}
