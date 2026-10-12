package library

import (
	"reflect"
	"testing"
)

func TestParseSearchQuery(t *testing.T) {
	tests := []struct {
		in   string
		want SearchQuery
	}{
		{"Batman", SearchQuery{Words: []string{"batman"}}},
		{"Batman V2017 #15 (2016)", SearchQuery{Words: []string{"batman"}, Number: "15", Year: 2016, Volume: 2017}},
		{"#015", SearchQuery{Number: "15"}},
		{"#0", SearchQuery{Number: "0"}},
		{"#12A", SearchQuery{Number: "12a"}},
		{"(1999)", SearchQuery{Year: 1999}},
		{"v2", SearchQuery{Volume: 2}},
		{"X-Men  v3  annual", SearchQuery{Words: []string{"x-men", "annual"}, Volume: 3}},
		{"Vampirella", SearchQuery{Words: []string{"vampirella"}}},
		{"(abc)", SearchQuery{Words: []string{"(abc)"}}},
		{"", SearchQuery{}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := ParseSearchQuery(tt.in)
			if len(got.Words) == 0 {
				got.Words = nil
			}
			if len(tt.want.Words) == 0 {
				tt.want.Words = nil
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSearchQueryMatch(t *testing.T) {
	book := &ComicBook{Series: "Batman", Title: "Year One", Publisher: "DC Comics", Number: "015", Year: 2016, Volume: 2017}
	tests := []struct {
		q    string
		want bool
	}{
		{"batman", true},
		{"BATMAN year", true},
		{"batman marvel", false},
		{"#15", true},
		{"#16", false},
		{"(2016)", true},
		{"(2015)", false},
		{"v2017", true},
		{"v2016", false},
		{"Batman V2017 #15 (2016)", true},
		{"Batman V2017 #15 (2015)", false},
		{"dc", true},
		{"", true},
	}
	for _, tt := range tests {
		if got := ParseSearchQuery(tt.q).Match(book); got != tt.want {
			t.Errorf("Match(%q) = %v, want %v", tt.q, got, tt.want)
		}
	}
	if !ParseSearchQuery("   ").IsEmpty() {
		t.Error("blank query should be empty")
	}
}
