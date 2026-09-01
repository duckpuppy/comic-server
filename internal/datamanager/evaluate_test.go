package datamanager

import (
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

// These test cases are drawn directly from the user's real dataman.dat
// (594 rulesets), not synthetic fixtures - see comic-server-764's design
// notes for why that matters (a hand-built fixture can accidentally avoid
// the exact rules that would break a naive implementation).

func TestRuleset_Matches_RealRules(t *testing.T) {
	tests := []struct {
		name    string
		ruleset Ruleset
		book    *library.ComicBook
		want    bool
	}{
		{
			// Real ruleset "Atom" (DC Comics group).
			name: "ContainsAnyOf + Is - match",
			ruleset: Ruleset{
				Mode: "AND",
				Rules: []Rule{
					{Field: "Series", Modifier: "ContainsAnyOf", Value: "The Atom||All-new Atom||Atom"},
					{Field: "Publisher", Modifier: "Is", Value: "DC Comics"},
				},
			},
			book: &library.ComicBook{Series: "All-new Atom", Publisher: "DC Comics"},
			want: true,
		},
		{
			name: "ContainsAnyOf + Is - no match (wrong publisher)",
			ruleset: Ruleset{
				Mode: "AND",
				Rules: []Rule{
					{Field: "Series", Modifier: "ContainsAnyOf", Value: "The Atom||All-new Atom||Atom"},
					{Field: "Publisher", Modifier: "Is", Value: "DC Comics"},
				},
			},
			book: &library.ComicBook{Series: "All-new Atom", Publisher: "Marvel"},
			want: false,
		},
		{
			// Real ruleset "Buffy the Vampire Slayer" (Dark Horse group).
			name: "ContainsAllOf + Is - match",
			ruleset: Ruleset{
				Mode: "AND",
				Rules: []Rule{
					{Field: "Series", Modifier: "ContainsAllOf", Value: "Buffy the Vampire Slayer"},
					{Field: "Publisher", Modifier: "Is", Value: "Dark Horse Comics"},
				},
			},
			book: &library.ComicBook{Series: "Buffy the Vampire Slayer Season 12", Publisher: "Dark Horse Comics"},
			want: true,
		},
		{
			// Real ruleset "Cosmic Marvel" - IsAnyOf expansion + a custom
			// field (Chronology) combined with built-in fields, all
			// under one AND.
			name: "IsAnyOf group + custom field - match",
			ruleset: Ruleset{
				Mode: "AND",
				Rules: []Rule{
					{Field: "AlternateSeries", Modifier: "IsAnyOf", Value: "Annihilation||Annihilation Conquest||War of Kings||Realm of Kings||Road to War of Kings"},
					{Field: "Publisher", Modifier: "Is", Value: "Marvel"},
					{Field: "SeriesGroup", Modifier: "Is", Value: ""},
					{Field: "Chronology", Modifier: "Is", Value: "Marvel Cosmic Chronology"},
				},
			},
			book: &library.ComicBook{
				AlternateSeries:   "War of Kings",
				Publisher:         "Marvel",
				SeriesGroup:       "",
				CustomValuesStore: ",Chronology=Marvel Cosmic Chronology",
			},
			want: true,
		},
		{
			name: "IsAnyOf group + custom field - no match (custom field wrong)",
			ruleset: Ruleset{
				Mode: "AND",
				Rules: []Rule{
					{Field: "AlternateSeries", Modifier: "IsAnyOf", Value: "Annihilation||War of Kings"},
					{Field: "Publisher", Modifier: "Is", Value: "Marvel"},
					{Field: "Chronology", Modifier: "Is", Value: "Marvel Cosmic Chronology"},
				},
			},
			book: &library.ComicBook{
				AlternateSeries:   "War of Kings",
				Publisher:         "Marvel",
				CustomValuesStore: ",Chronology=Something Else",
			},
			want: false,
		},
		{
			// Real usage: Number Range "135||152" (Batman example).
			name: "numeric Range - inside",
			ruleset: Ruleset{
				Mode:  "AND",
				Rules: []Rule{{Field: "Number", Modifier: "Range", Value: "135||152"}},
			},
			book: &library.ComicBook{Number: "140"},
			want: true,
		},
		{
			name: "numeric Range - outside",
			ruleset: Ruleset{
				Mode:  "AND",
				Rules: []Rule{{Field: "Number", Modifier: "Range", Value: "135||152"}},
			},
			book: &library.ComicBook{Number: "200"},
			want: false,
		},
		{
			// Real usage: GreaterEq/LessEq pair bracketing a number range.
			name: "GreaterEq + LessEq - inside inclusive bounds",
			ruleset: Ruleset{
				Mode: "AND",
				Rules: []Rule{
					{Field: "Number", Modifier: "GreaterEq", Value: "32"},
					{Field: "Number", Modifier: "LessEq", Value: "33"},
				},
			},
			book: &library.ComicBook{Number: "32"},
			want: true,
		},
		{
			name: "GreaterEq + LessEq - just outside",
			ruleset: Ruleset{
				Mode: "AND",
				Rules: []Rule{
					{Field: "Number", Modifier: "GreaterEq", Value: "32"},
					{Field: "Number", Modifier: "LessEq", Value: "33"},
				},
			},
			book: &library.ComicBook{Number: "34"},
			want: false,
		},
		{
			// Real ruleset "Manga".
			name: "Manga Is Yes - match",
			ruleset: Ruleset{
				Mode:  "AND",
				Rules: []Rule{{Field: "Manga", Modifier: "Is", Value: "Yes"}},
			},
			book: &library.ComicBook{Manga: "Yes"},
			want: true,
		},
		{
			// Real ruleset "Cleanup DMProc" condition side.
			name: "Tags Contains DMProc - match",
			ruleset: Ruleset{
				Mode:  "AND",
				Rules: []Rule{{Field: "Tags", Modifier: "Contains", Value: "DMProc"}},
			},
			book: &library.ComicBook{Tags: "DMProc, action"},
			want: true,
		},
		{
			// Real ruleset "Deadpool Minis": custom field Not modifier.
			name: "custom field Not - excludes matching value",
			ruleset: Ruleset{
				Mode: "AND",
				Rules: []Rule{
					{Field: "Concept", Modifier: "Not", Value: "ANAD"},
					{Field: "Volume", Modifier: "GreaterEq", Value: "2016"},
				},
			},
			book: &library.ComicBook{Volume: 2018, CustomValuesStore: ",Concept=Marvel Now"},
			want: true,
		},
		{
			name: "custom field Not - rejects the excluded value",
			ruleset: Ruleset{
				Mode: "AND",
				Rules: []Rule{
					{Field: "Concept", Modifier: "Not", Value: "ANAD"},
					{Field: "Volume", Modifier: "GreaterEq", Value: "2016"},
				},
			},
			book: &library.ComicBook{Volume: 2018, CustomValuesStore: ",Concept=ANAD"},
			want: false,
		},
		{
			name:    "zero rules matches everything",
			ruleset: Ruleset{Mode: "AND"},
			book:    &library.ComicBook{},
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.ruleset.Matches(tt.book)
			if err != nil {
				t.Fatalf("Matches() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}
