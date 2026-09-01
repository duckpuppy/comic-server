package datamanager

import (
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestApplyAll_LaterRulesetOverwritesEarlier(t *testing.T) {
	// Mirrors the real plugin's documented evaluation order semantics
	// (see comic-server-764's design notes): rulesets run in document
	// order, and a later one's action can overwrite an earlier one's
	// write to the same field.
	book := &library.ComicBook{Series: "Batman", Publisher: "DC Comics"}
	rulesets := []Ruleset{
		{
			Name: "first",
			Mode: "AND",
			Rules: []Rule{
				{Field: "Publisher", Modifier: "Is", Value: "DC Comics"},
			},
			Actions: []Action{
				{Field: "SeriesGroup", Modifier: "SetValue", Value: "Batman Family"},
			},
		},
		{
			Name: "second",
			Mode: "AND",
			Rules: []Rule{
				{Field: "Series", Modifier: "Is", Value: "Batman"},
			},
			Actions: []Action{
				{Field: "SeriesGroup", Modifier: "SetValue", Value: "Batman"},
			},
		},
	}

	changes, err := ApplyAll(book, rulesets)
	if err != nil {
		t.Fatalf("ApplyAll: %v", err)
	}
	if book.SeriesGroup != "Batman" {
		t.Fatalf("SeriesGroup = %q, want %q (second ruleset should win)", book.SeriesGroup, "Batman")
	}

	// The diff should show the field's FINAL value, going from the
	// original empty string straight to "Batman" - not an intermediate
	// "Batman Family" bounce visible anywhere in the reported change.
	var sg *FieldChange
	for i := range changes {
		if changes[i].Field == "SeriesGroup" {
			sg = &changes[i]
		}
	}
	if sg == nil {
		t.Fatal("expected a SeriesGroup change in the diff")
	}
	if sg.Old != "" || sg.New != "Batman" {
		t.Errorf("SeriesGroup change = %+v, want Old=\"\" New=\"Batman\"", sg)
	}
}

func TestApplyAll_NoMatchProducesNoChanges(t *testing.T) {
	book := &library.ComicBook{Series: "Aquaman"}
	rulesets := []Ruleset{
		{
			Name: "only",
			Mode: "AND",
			Rules: []Rule{
				{Field: "Series", Modifier: "Is", Value: "Batman"},
			},
			Actions: []Action{
				{Field: "SeriesGroup", Modifier: "SetValue", Value: "Batman"},
			},
		},
	}

	changes, err := ApplyAll(book, rulesets)
	if err != nil {
		t.Fatalf("ApplyAll: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected no changes for a non-matching book, got %+v", changes)
	}
}

func TestApplyAll_CustomValueChangesTracked(t *testing.T) {
	book := &library.ComicBook{Tags: "DMProc, action"}
	rulesets := []Ruleset{
		{
			Name: "Cleanup DMProc",
			Mode: "AND",
			Rules: []Rule{
				{Field: "Tags", Modifier: "Contains", Value: "DMProc"},
			},
			Actions: []Action{
				{Field: "Tags", Modifier: "Remove", Value: "DMProc"},
				{Field: "Data Manager processed", Modifier: "SetValue", Value: "DMProc"},
			},
		},
	}

	changes, err := ApplyAll(book, rulesets)
	if err != nil {
		t.Fatalf("ApplyAll: %v", err)
	}

	byField := map[string]FieldChange{}
	for _, c := range changes {
		byField[c.Field] = c
	}

	tags, ok := byField["Tags"]
	if !ok || tags.Custom || tags.Old != "DMProc, action" || tags.New != "action" {
		t.Errorf("Tags change = %+v (ok=%v), want built-in Old=%q New=%q", tags, ok, "DMProc, action", "action")
	}

	dmproc, ok := byField["Data Manager processed"]
	if !ok || !dmproc.Custom || dmproc.Old != "" || dmproc.New != "DMProc" {
		t.Errorf("Data Manager processed change = %+v (ok=%v), want Custom=true Old=\"\" New=%q", dmproc, ok, "DMProc")
	}
}

func TestApplyAll_OneRulesetErrorDoesNotStopOthers(t *testing.T) {
	book := &library.ComicBook{Series: "Batman"}
	rulesets := []Ruleset{
		{
			Name: "bad",
			Mode: "AND",
			Rules: []Rule{
				{Field: "Series", Modifier: "Is", Value: "Batman"},
			},
			Actions: []Action{
				{Field: "Series", Modifier: "Calc", Value: "1+1"}, // unsupported, always errors
			},
		},
		{
			Name: "good",
			Mode: "AND",
			Rules: []Rule{
				{Field: "Series", Modifier: "Is", Value: "Batman"},
			},
			Actions: []Action{
				{Field: "SeriesGroup", Modifier: "SetValue", Value: "Bat Family"},
			},
		},
	}

	changes, err := ApplyAll(book, rulesets)
	if err == nil {
		t.Fatal("expected an error from the bad ruleset's Calc action")
	}
	if book.SeriesGroup != "Bat Family" {
		t.Errorf("SeriesGroup = %q, want %q (the good ruleset should still have run)", book.SeriesGroup, "Bat Family")
	}
	found := false
	for _, c := range changes {
		if c.Field == "SeriesGroup" && c.New == "Bat Family" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected SeriesGroup change in diff despite the earlier ruleset's error, got %+v", changes)
	}
}
