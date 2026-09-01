package datamanager

import (
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

// TestApplyAction_RealCases mirrors real actions found in the user's
// actual dataman.dat where possible - see comic-server-764's design
// notes for why that matters more than synthetic-only coverage.
func TestApplyAction_RealCases(t *testing.T) {
	t.Run("SetValue with {FieldName} substitution", func(t *testing.T) {
		// Real ruleset "Cosmic Marvel": SeriesGroup SetValue "{AlternateSeries}".
		book := &library.ComicBook{AlternateSeries: "War of Kings"}
		changed, err := ApplyAction(Action{Field: "SeriesGroup", Modifier: "SetValue", Value: "{AlternateSeries}"}, book)
		if err != nil {
			t.Fatalf("ApplyAction error: %v", err)
		}
		if !changed || book.SeriesGroup != "War of Kings" {
			t.Errorf("SeriesGroup = %q, changed = %v, want %q, true", book.SeriesGroup, changed, "War of Kings")
		}
	})

	t.Run("Add on a multi-value field appends without duplicating", func(t *testing.T) {
		// Real: Tags Add "DC K.O."
		book := &library.ComicBook{Tags: "Dawn of DC"}
		changed, err := ApplyAction(Action{Field: "Tags", Modifier: "Add", Value: "DC K.O."}, book)
		if err != nil {
			t.Fatalf("ApplyAction error: %v", err)
		}
		want := "Dawn of DC, DC K.O."
		if !changed || book.Tags != want {
			t.Errorf("Tags = %q, changed = %v, want %q, true", book.Tags, changed, want)
		}

		// Adding the same tag again is a no-op.
		changed2, err := ApplyAction(Action{Field: "Tags", Modifier: "Add", Value: "DC K.O."}, book)
		if err != nil {
			t.Fatalf("ApplyAction error: %v", err)
		}
		if changed2 || book.Tags != want {
			t.Errorf("re-adding existing tag: Tags = %q, changed = %v, want unchanged", book.Tags, changed2)
		}
	})

	t.Run("Add on a string field concatenates with no separator", func(t *testing.T) {
		// Real: Series Add " Annual" - the real rules compensate for the
		// no-separator behavior by including their own leading space.
		book := &library.ComicBook{Series: "Batman"}
		changed, err := ApplyAction(Action{Field: "Series", Modifier: "Add", Value: " Annual"}, book)
		if err != nil {
			t.Fatalf("ApplyAction error: %v", err)
		}
		if !changed || book.Series != "Batman Annual" {
			t.Errorf("Series = %q, changed = %v, want %q, true", book.Series, changed, "Batman Annual")
		}
	})

	t.Run("Remove on a string field removes the substring", func(t *testing.T) {
		// Real: Series Remove " Annual"
		book := &library.ComicBook{Series: "Batman Annual"}
		changed, err := ApplyAction(Action{Field: "Series", Modifier: "Remove", Value: " Annual"}, book)
		if err != nil {
			t.Fatalf("ApplyAction error: %v", err)
		}
		if !changed || book.Series != "Batman" {
			t.Errorf("Series = %q, changed = %v, want %q, true", book.Series, changed, "Batman")
		}
	})

	t.Run("Remove on a multi-value field removes the list item", func(t *testing.T) {
		book := &library.ComicBook{Tags: "DMProc, action"}
		changed, err := ApplyAction(Action{Field: "Tags", Modifier: "Remove", Value: "DMProc"}, book)
		if err != nil {
			t.Fatalf("ApplyAction error: %v", err)
		}
		if !changed || book.Tags != "action" {
			t.Errorf("Tags = %q, changed = %v, want %q, true", book.Tags, changed, "action")
		}
	})

	t.Run("RemoveLeading strips a matching prefix", func(t *testing.T) {
		// Real: MainCharacterOrTeam RemoveLeading "All-New"
		book := &library.ComicBook{MainCharacterOrTeam: "All-New Wolverine"}
		changed, err := ApplyAction(Action{Field: "MainCharacterOrTeam", Modifier: "RemoveLeading", Value: "All-New"}, book)
		if err != nil {
			t.Fatalf("ApplyAction error: %v", err)
		}
		if !changed || book.MainCharacterOrTeam != " Wolverine" {
			t.Errorf("MainCharacterOrTeam = %q, changed = %v, want %q, true", book.MainCharacterOrTeam, changed, " Wolverine")
		}
	})

	t.Run("RemoveLeading is a no-op when the prefix isn't present", func(t *testing.T) {
		book := &library.ComicBook{MainCharacterOrTeam: "Wolverine"}
		changed, err := ApplyAction(Action{Field: "MainCharacterOrTeam", Modifier: "RemoveLeading", Value: "All-New"}, book)
		if err != nil {
			t.Fatalf("ApplyAction error: %v", err)
		}
		if changed {
			t.Errorf("expected no change, got changed=%v value=%q", changed, book.MainCharacterOrTeam)
		}
	})

	t.Run("SetValue on a custom field", func(t *testing.T) {
		book := &library.ComicBook{}
		changed, err := ApplyAction(Action{Field: "Data Manager processed", Modifier: "SetValue", Value: "DMProc"}, book)
		if err != nil {
			t.Fatalf("ApplyAction error: %v", err)
		}
		got, ok := getCustomValue(book, "Data Manager processed")
		if !changed || !ok || got != "DMProc" {
			t.Errorf("custom value = %q (ok=%v), changed = %v, want %q, true, true", got, ok, changed, "DMProc")
		}
	})

	t.Run("RegexReplace real usage", func(t *testing.T) {
		// Real: MainCharacterOrTeam RegexReplace "!$||" (strip a trailing "!").
		book := &library.ComicBook{MainCharacterOrTeam: "Deadpool!"}
		changed, err := ApplyAction(Action{Field: "MainCharacterOrTeam", Modifier: "RegexReplace", Value: "!$||"}, book)
		if err != nil {
			t.Fatalf("ApplyAction error: %v", err)
		}
		if !changed || book.MainCharacterOrTeam != "Deadpool" {
			t.Errorf("MainCharacterOrTeam = %q, changed = %v, want %q, true", book.MainCharacterOrTeam, changed, "Deadpool")
		}
	})
}

// TestRuleset_Apply_CleanupDMProc replays the real "Cleanup DMProc"
// ruleset end to end: condition + both actions together.
func TestRuleset_Apply_CleanupDMProc(t *testing.T) {
	rs := Ruleset{
		Name: "Cleanup DMProc",
		Mode: "AND",
		Rules: []Rule{
			{Field: "Tags", Modifier: "Contains", Value: "DMProc"},
		},
		Actions: []Action{
			{Field: "Tags", Modifier: "Remove", Value: "DMProc"},
			{Field: "Data Manager processed", Modifier: "SetValue", Value: "DMProc"},
		},
	}

	t.Run("matching book gets both actions applied", func(t *testing.T) {
		book := &library.ComicBook{Tags: "DMProc, action"}
		changed, err := rs.Apply(book)
		if err != nil {
			t.Fatalf("Apply error: %v", err)
		}
		if !changed {
			t.Fatal("expected changed=true")
		}
		if book.Tags != "action" {
			t.Errorf("Tags = %q, want %q", book.Tags, "action")
		}
		got, ok := getCustomValue(book, "Data Manager processed")
		if !ok || got != "DMProc" {
			t.Errorf("Data Manager processed = %q (ok=%v), want %q, true", got, ok, "DMProc")
		}
	})

	t.Run("non-matching book is untouched", func(t *testing.T) {
		book := &library.ComicBook{Tags: "action"}
		changed, err := rs.Apply(book)
		if err != nil {
			t.Fatalf("Apply error: %v", err)
		}
		if changed {
			t.Errorf("expected no change, got Tags=%q", book.Tags)
		}
	})
}

// TestApplyAction_RegExVarReplace is synthetic - the user's real
// dataman.dat never exercises this modifier, so this is built from the
// plugin's documented behavior only, not ground-truthed against real
// usage (see comic-server-764's design notes and actions.go's own
// comment on applyRegExVarReplace).
func TestApplyAction_RegExVarReplace(t *testing.T) {
	book := &library.ComicBook{Series: "Batman Annual"}
	changed, err := ApplyAction(Action{
		Field:    "Series",
		Modifier: "RegExVarReplace",
		Value:    `^(?P<Series>.+?)(?: (?P<TempAnnual>Annual))?$`,
	}, book)
	if err != nil {
		t.Fatalf("ApplyAction error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if book.Series != "Batman" {
		t.Errorf("Series = %q, want %q", book.Series, "Batman")
	}
	got, ok := getCustomValue(book, "TempAnnual")
	if !ok || got != "Annual" {
		t.Errorf("TempAnnual = %q (ok=%v), want %q, true", got, ok, "Annual")
	}
}
