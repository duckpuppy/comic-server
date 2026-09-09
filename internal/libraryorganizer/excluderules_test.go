package libraryorganizer

import (
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestShouldMove_ZeroRulesAlwaysMoves(t *testing.T) {
	// Ground-truthed against locommon.py's check_metadata_rules: "no
	// rules so the book should be moved regardless" - true no matter
	// what ExcludeMode/ExcludeOperator say. Matches the real Default and
	// Move To 0Day profiles (ExcludeMode=Only, zero actual rules).
	cfg := ExcludeConfig{ExcludeMode: "Only", ExcludeOperator: "Any"}
	move, ok := ShouldMove(&library.ComicBook{}, cfg)
	if !ok {
		t.Fatal("ok = false")
	}
	if !move {
		t.Error("expected move=true with zero configured rules")
	}
}

// TestShouldMove_RealArchiveProfile ground-truths against the user's
// real "Archive" profile: ExcludeMode=Only, ExcludeOperator=All, rules
// [Tags contains "Archive", File Path is not ""] - only books tagged
// Archive AND actually having a file get archived.
func TestShouldMove_RealArchiveProfile(t *testing.T) {
	cfg := ExcludeConfig{
		ExcludeMode:     "Only",
		ExcludeOperator: "All",
		Rules: []ExcludeRule{
			{Field: "Tags", Operator: "contains", Value: "Archive"},
			{Field: "File Path", Operator: "is not", Value: ""},
		},
	}

	tests := []struct {
		name string
		book *library.ComicBook
		want bool
	}{
		{"tagged and has file - moves", &library.ComicBook{Tags: "Archive, DC", FilePath: "x.cbz"}, true},
		{"tagged but no file - does not move (fails the All requirement)", &library.ComicBook{Tags: "Archive, DC", FilePath: ""}, false},
		{"has file but not tagged - does not move", &library.ComicBook{Tags: "DC", FilePath: "x.cbz"}, false},
		{"neither - does not move", &library.ComicBook{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ShouldMove(tt.book, cfg)
			if !ok {
				t.Fatal("ok = false")
			}
			if got != tt.want {
				t.Errorf("ShouldMove() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldMove_DoNotExcludeMode(t *testing.T) {
	// "Do not" means: move UNLESS the book qualifies under the rules -
	// the inverse of "Only".
	cfg := ExcludeConfig{
		ExcludeMode:     "Do not",
		ExcludeOperator: "Any",
		Rules: []ExcludeRule{
			{Field: "Tags", Operator: "contains", Value: "Skip"},
		},
	}
	moveTagged, ok := ShouldMove(&library.ComicBook{Tags: "Skip"}, cfg)
	if !ok {
		t.Fatal("ok = false")
	}
	if moveTagged {
		t.Error("expected a Skip-tagged book to NOT move under ExcludeMode=Do not")
	}
	moveUntagged, ok := ShouldMove(&library.ComicBook{Tags: "Other"}, cfg)
	if !ok {
		t.Fatal("ok = false")
	}
	if !moveUntagged {
		t.Error("expected an untagged book to move under ExcludeMode=Do not")
	}
}

func TestShouldMove_AnyOperatorNeedsOnlyOneMatch(t *testing.T) {
	cfg := ExcludeConfig{
		ExcludeMode:     "Only",
		ExcludeOperator: "Any",
		Rules: []ExcludeRule{
			{Field: "Publisher", Operator: "is", Value: "DC Comics"},
			{Field: "Publisher", Operator: "is", Value: "Marvel"},
		},
	}
	move, ok := ShouldMove(&library.ComicBook{Publisher: "Marvel"}, cfg)
	if !ok {
		t.Fatal("ok = false")
	}
	if !move {
		t.Error("expected a match on ONE of two \"Any\" rules to qualify")
	}
}

func TestCompareExcludeRule_CaseSensitive(t *testing.T) {
	// Ground-truthed: the Python uses plain `==`/`in`, no case-folding -
	// a real, deliberate difference from internal/library's smart-list
	// matchers (which default to IgnoreCase).
	if compareExcludeRule("archive", "is", "Archive") {
		t.Error("expected case-sensitive comparison to NOT match \"archive\" vs \"Archive\"")
	}
	if !compareExcludeRule("Archive", "is", "Archive") {
		t.Error("expected exact case match to succeed")
	}
}

func TestCompareExcludeRule_GreaterLessThanNumericFirst(t *testing.T) {
	if !compareExcludeRule("10", "greater than", "9") {
		t.Error("expected numeric comparison: 10 > 9")
	}
	// String comparison would say "10" < "9" (lexicographic) - numeric
	// parsing must win when both sides parse as integers.
	if compareExcludeRule("10", "less than", "9") {
		t.Error("expected numeric comparison to override lexicographic ordering")
	}
}

func TestShouldMove_UnsupportedFieldReportsInvalid(t *testing.T) {
	cfg := ExcludeConfig{
		ExcludeMode:     "Only",
		ExcludeOperator: "Any",
		Rules:           []ExcludeRule{{Field: "Start Year", Operator: "is", Value: "1990"}},
	}
	_, ok := ShouldMove(&library.ComicBook{}, cfg)
	if ok {
		t.Error("expected ok=false for a field this build doesn't support (needs earliest-book-in-series lookup)")
	}
}
