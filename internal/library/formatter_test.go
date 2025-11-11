package library

import (
	"testing"
)

func TestFormatMatcher_SeriesContains(t *testing.T) {
	matcher := ComicBookMatcher{
		Type:          "Series",
		MatchOperator: "1", // Contains
		MatchValue:    "Batman",
	}

	result := FormatMatcher(matcher)
	expected := "Series contains 'Batman'"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestFormatMatcher_YearAfter(t *testing.T) {
	matcher := ComicBookMatcher{
		Type:          "Year",
		MatchOperator: "1", // Greater
		MatchValue:    "2020",
	}

	result := FormatMatcher(matcher)
	expected := "Year is after 2020"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestFormatMatcher_PublisherEquals(t *testing.T) {
	matcher := ComicBookMatcher{
		Type:          "Publisher",
		MatchOperator: "0", // Equals
		MatchValue:    "DC",
	}

	result := FormatMatcher(matcher)
	expected := "Publisher equals 'DC'"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestFormatMatcherMode_And(t *testing.T) {
	result := FormatMatcherMode("And")
	expected := "Match ALL (AND)"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestFormatMatcherMode_Or(t *testing.T) {
	result := FormatMatcherMode("Or")
	expected := "Match ANY (OR)"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestFormatMatcher_StartsWith(t *testing.T) {
	matcher := ComicBookMatcher{
		Type:          "Series",
		MatchOperator: "4", // StartsWith
		MatchValue:    "Amazing",
	}

	result := FormatMatcher(matcher)
	expected := "Series starts with 'Amazing'"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestFormatMatcher_ContainsAll(t *testing.T) {
	matcher := ComicBookMatcher{
		Type:          "Tags",
		MatchOperator: "3", // ContainsAll
		MatchValue:    "superhero,action",
	}

	result := FormatMatcher(matcher)
	expected := "Tags contains all of 'superhero,action'"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestFormatMatcher_NumericGreater(t *testing.T) {
	matcher := ComicBookMatcher{
		Type:          "PageCount",
		MatchOperator: "1", // Greater
		MatchValue:    "100",
	}

	result := FormatMatcher(matcher)
	expected := "PageCount is greater than 100"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestFormatMatcher_Regex(t *testing.T) {
	matcher := ComicBookMatcher{
		Type:          "Title",
		MatchOperator: "7", // Regex
		MatchValue:    "^Amazing.*",
	}

	result := FormatMatcher(matcher)
	expected := "Title matches regex '^Amazing.*'"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestFormatMatcherMode_Unknown(t *testing.T) {
	result := FormatMatcherMode("Unknown")
	expected := "Unknown"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}
