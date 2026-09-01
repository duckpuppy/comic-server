package configdb

import (
	"path/filepath"
	"testing"
)

func newTestDMDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestDMGroups_RoundTrip mirrors the real dataman.dat's actual nesting
// depth (Quality > Series Groups > Dark Horse Comics), not a synthetic
// 1-level fixture - see comic-server-764's design notes.
func TestDMGroups_RoundTrip(t *testing.T) {
	db := newTestDMDB(t)

	root := DMGroup{ID: "g-quality", Name: "Quality", SortOrder: 0}
	if err := db.CreateDMGroup(root); err != nil {
		t.Fatalf("CreateDMGroup(root): %v", err)
	}
	mid := DMGroup{ID: "g-series-groups", ParentID: "g-quality", Name: "Series Groups", SortOrder: 0}
	if err := db.CreateDMGroup(mid); err != nil {
		t.Fatalf("CreateDMGroup(mid): %v", err)
	}
	leaf := DMGroup{ID: "g-dark-horse", ParentID: "g-series-groups", Name: "Dark Horse Comics", SortOrder: 0}
	if err := db.CreateDMGroup(leaf); err != nil {
		t.Fatalf("CreateDMGroup(leaf): %v", err)
	}
	// A sibling top-level group, to prove ListDMGroups("") doesn't leak
	// nested groups and vice versa.
	sibling := DMGroup{ID: "g-imprints", Name: "Imprints", SortOrder: 1}
	if err := db.CreateDMGroup(sibling); err != nil {
		t.Fatalf("CreateDMGroup(sibling): %v", err)
	}

	topLevel, err := db.ListDMGroups("")
	if err != nil {
		t.Fatalf("ListDMGroups(\"\"): %v", err)
	}
	if len(topLevel) != 2 || topLevel[0].ID != "g-quality" || topLevel[1].ID != "g-imprints" {
		t.Fatalf("top-level groups = %+v, want [g-quality, g-imprints] in that order", topLevel)
	}

	underQuality, err := db.ListDMGroups("g-quality")
	if err != nil {
		t.Fatalf("ListDMGroups(g-quality): %v", err)
	}
	if len(underQuality) != 1 || underQuality[0].ID != "g-series-groups" {
		t.Fatalf("groups under g-quality = %+v, want [g-series-groups]", underQuality)
	}

	got, err := db.GetDMGroup("g-dark-horse")
	if err != nil {
		t.Fatalf("GetDMGroup: %v", err)
	}
	if got == nil || got.ParentID != "g-series-groups" || got.Name != "Dark Horse Comics" {
		t.Fatalf("GetDMGroup(g-dark-horse) = %+v, want ParentID=g-series-groups Name=\"Dark Horse Comics\"", got)
	}
}

// TestDMRuleset_RoundTrip round-trips a real ruleset shape from the
// user's file (the "Atom" ruleset: Series ContainsAnyOf ... AND Publisher
// Is "DC Comics" -> SeriesGroup SetValue "Atom"), including sort_order
// preservation for both rules and actions.
func TestDMRuleset_RoundTrip(t *testing.T) {
	db := newTestDMDB(t)

	group := DMGroup{ID: "g-dc", Name: "DC Comics"}
	if err := db.CreateDMGroup(group); err != nil {
		t.Fatalf("CreateDMGroup: %v", err)
	}

	rs := DMRuleset{ID: "rs-atom", GroupID: "g-dc", Name: "Atom", Mode: "And", SortOrder: 3}
	if err := db.CreateDMRuleset(rs); err != nil {
		t.Fatalf("CreateDMRuleset: %v", err)
	}

	if _, err := db.CreateDMRule(DMRule{RulesetID: "rs-atom", Field: "Series", Modifier: "ContainsAnyOf", Value: "The Atom||All-new Atom||Atom", SortOrder: 0}); err != nil {
		t.Fatalf("CreateDMRule(1): %v", err)
	}
	if _, err := db.CreateDMRule(DMRule{RulesetID: "rs-atom", Field: "Publisher", Modifier: "Is", Value: "DC Comics", SortOrder: 1}); err != nil {
		t.Fatalf("CreateDMRule(2): %v", err)
	}
	if _, err := db.CreateDMAction(DMAction{RulesetID: "rs-atom", Field: "SeriesGroup", Modifier: "SetValue", Value: "Atom", SortOrder: 0}); err != nil {
		t.Fatalf("CreateDMAction: %v", err)
	}

	gotRS, err := db.GetDMRuleset("rs-atom")
	if err != nil {
		t.Fatalf("GetDMRuleset: %v", err)
	}
	if gotRS == nil || gotRS.GroupID != "g-dc" || gotRS.Name != "Atom" || gotRS.Mode != "And" || gotRS.SortOrder != 3 {
		t.Fatalf("GetDMRuleset = %+v, want GroupID=g-dc Name=Atom Mode=And SortOrder=3", gotRS)
	}

	rules, err := db.ListDMRules("rs-atom")
	if err != nil {
		t.Fatalf("ListDMRules: %v", err)
	}
	if len(rules) != 2 || rules[0].Field != "Series" || rules[1].Field != "Publisher" {
		t.Fatalf("rules = %+v, want [Series, Publisher] in that order", rules)
	}
	if rules[0].Value != "The Atom||All-new Atom||Atom" {
		t.Errorf("rule[0].Value = %q, want the raw \"||\"-joined value preserved as-is", rules[0].Value)
	}

	actions, err := db.ListDMActions("rs-atom")
	if err != nil {
		t.Fatalf("ListDMActions: %v", err)
	}
	if len(actions) != 1 || actions[0].Field != "SeriesGroup" || actions[0].Value != "Atom" {
		t.Fatalf("actions = %+v, want [{SeriesGroup SetValue Atom}]", actions)
	}
}

// TestDMGroup_Disabled confirms the disabled flag round-trips - mirrors
// the real dataman.dat's top-level <disabled> container.
func TestDMGroup_Disabled(t *testing.T) {
	db := newTestDMDB(t)

	if err := db.CreateDMGroup(DMGroup{ID: "g-disabled", Name: "Disabled", Comment: "Disabled Groups and Rulesets", Disabled: true}); err != nil {
		t.Fatalf("CreateDMGroup: %v", err)
	}
	got, err := db.GetDMGroup("g-disabled")
	if err != nil {
		t.Fatalf("GetDMGroup: %v", err)
	}
	if got == nil || !got.Disabled || got.Comment != "Disabled Groups and Rulesets" {
		t.Fatalf("GetDMGroup(g-disabled) = %+v, want Disabled=true Comment set", got)
	}
}

// TestDMGroup_DeleteCascades confirms deleting a group removes every
// descendant group/ruleset/rule/action nested inside it - the whole point
// of storing the hierarchy relationally instead of flattened.
func TestDMGroup_DeleteCascades(t *testing.T) {
	db := newTestDMDB(t)

	if err := db.CreateDMGroup(DMGroup{ID: "g-parent", Name: "Parent"}); err != nil {
		t.Fatalf("CreateDMGroup(parent): %v", err)
	}
	if err := db.CreateDMGroup(DMGroup{ID: "g-child", ParentID: "g-parent", Name: "Child"}); err != nil {
		t.Fatalf("CreateDMGroup(child): %v", err)
	}
	if err := db.CreateDMRuleset(DMRuleset{ID: "rs-child", GroupID: "g-child", Name: "Some Ruleset"}); err != nil {
		t.Fatalf("CreateDMRuleset: %v", err)
	}
	if _, err := db.CreateDMRule(DMRule{RulesetID: "rs-child", Field: "Series", Modifier: "Is", Value: "X"}); err != nil {
		t.Fatalf("CreateDMRule: %v", err)
	}
	if _, err := db.CreateDMAction(DMAction{RulesetID: "rs-child", Field: "SeriesGroup", Modifier: "SetValue", Value: "X"}); err != nil {
		t.Fatalf("CreateDMAction: %v", err)
	}

	if err := db.DeleteDMGroup("g-parent"); err != nil {
		t.Fatalf("DeleteDMGroup: %v", err)
	}

	if g, err := db.GetDMGroup("g-child"); err != nil || g != nil {
		t.Errorf("expected g-child to be gone after deleting g-parent, got %+v err=%v", g, err)
	}
	if rs, err := db.GetDMRuleset("rs-child"); err != nil || rs != nil {
		t.Errorf("expected rs-child to be gone after deleting g-parent, got %+v err=%v", rs, err)
	}
	rules, err := db.ListDMRules("rs-child")
	if err != nil {
		t.Fatalf("ListDMRules after cascade: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules after cascade delete, got %d", len(rules))
	}
}

// TestDMRuleset_TopLevel confirms a ruleset with no group (GroupID="")
// round-trips correctly - dataman.dat allows a <ruleset> directly under
// <collection>, not just nested inside a <group>.
func TestDMRuleset_TopLevel(t *testing.T) {
	db := newTestDMDB(t)

	if err := db.CreateDMRuleset(DMRuleset{ID: "rs-top", Name: "Top Level Ruleset"}); err != nil {
		t.Fatalf("CreateDMRuleset: %v", err)
	}

	top, err := db.ListDMRulesets("")
	if err != nil {
		t.Fatalf("ListDMRulesets(\"\"): %v", err)
	}
	if len(top) != 1 || top[0].ID != "rs-top" || top[0].GroupID != "" {
		t.Fatalf("top-level rulesets = %+v, want [{rs-top GroupID=\"\"}]", top)
	}
}
