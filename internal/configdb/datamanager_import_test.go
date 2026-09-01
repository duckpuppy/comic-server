package configdb

import "testing"

func TestImportDataManagerRules_WipeExistingReplacesNotDuplicates(t *testing.T) {
	db := newTestDMDB(t)

	first := []DMImportGroup{{ID: "g1", Name: "Old Group"}}
	firstRulesets := []DMImportRuleset{
		{ID: "rs1", GroupID: "g1", Name: "Old Ruleset", Mode: "AND",
			Rules:   []DMImportRule{{Field: "Series", Modifier: "Is", Value: "Old"}},
			Actions: []DMImportAction{{Field: "SeriesGroup", Modifier: "SetValue", Value: "Old"}}},
	}
	if err := db.ImportDataManagerRules(first, firstRulesets, false); err != nil {
		t.Fatalf("first import: %v", err)
	}

	second := []DMImportGroup{{ID: "g2", Name: "New Group"}}
	secondRulesets := []DMImportRuleset{
		{ID: "rs2", GroupID: "g2", Name: "New Ruleset", Mode: "AND",
			Rules:   []DMImportRule{{Field: "Series", Modifier: "Is", Value: "New"}},
			Actions: []DMImportAction{{Field: "SeriesGroup", Modifier: "SetValue", Value: "New"}}},
	}
	if err := db.ImportDataManagerRules(second, secondRulesets, true); err != nil {
		t.Fatalf("second import with wipeExisting: %v", err)
	}

	groups, err := db.ListDMGroups("")
	if err != nil {
		t.Fatalf("ListDMGroups: %v", err)
	}
	if len(groups) != 1 || groups[0].ID != "g2" {
		t.Fatalf("groups = %+v, want exactly [g2] (old group must be gone, not duplicated)", groups)
	}

	rulesets, err := db.ListDMRulesets("g2")
	if err != nil {
		t.Fatalf("ListDMRulesets: %v", err)
	}
	if len(rulesets) != 1 || rulesets[0].ID != "rs2" {
		t.Fatalf("rulesets under g2 = %+v, want exactly [rs2]", rulesets)
	}

	if g, err := db.GetDMGroup("g1"); err != nil || g != nil {
		t.Errorf("expected g1 to be gone after wipe, got %+v err=%v", g, err)
	}
	if rs, err := db.GetDMRuleset("rs1"); err != nil || rs != nil {
		t.Errorf("expected rs1 to be gone after wipe, got %+v err=%v", rs, err)
	}

	rules, err := db.ListDMRules("rs1")
	if err != nil {
		t.Fatalf("ListDMRules(rs1) after wipe: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("expected rs1's rules gone too, got %d", len(rules))
	}
}

func TestImportDataManagerRules_WipeExistingClearsTopLevelRulesets(t *testing.T) {
	// A top-level ruleset (GroupID="") has no group row to cascade from -
	// this is the specific case comic-server-cge's fix needed to handle
	// explicitly, not just rely on ON DELETE CASCADE from dm_groups.
	db := newTestDMDB(t)

	first := []DMImportGroup{}
	firstRulesets := []DMImportRuleset{
		{ID: "rs-top", Name: "Top Level", Mode: "AND"},
	}
	if err := db.ImportDataManagerRules(first, firstRulesets, false); err != nil {
		t.Fatalf("first import: %v", err)
	}

	if err := db.ImportDataManagerRules(nil, nil, true); err != nil {
		t.Fatalf("wipe-only reimport: %v", err)
	}

	top, err := db.ListDMRulesets("")
	if err != nil {
		t.Fatalf("ListDMRulesets(\"\"): %v", err)
	}
	if len(top) != 0 {
		t.Errorf("expected top-level rulesets wiped, got %+v", top)
	}
}

func TestImportDataManagerRules_WithoutWipeStillAppends(t *testing.T) {
	// wipeExisting defaults to false in normal (non-force) use - the
	// CLI's own "already has rules" guard is what actually prevents
	// accidental duplication in that path, not this function refusing to
	// append. This test documents that ImportDataManagerRules itself
	// still just appends when wipeExisting is false, matching its
	// pre-comic-server-cge behavior.
	db := newTestDMDB(t)

	if err := db.ImportDataManagerRules([]DMImportGroup{{ID: "g1", Name: "A"}}, nil, false); err != nil {
		t.Fatalf("first import: %v", err)
	}
	if err := db.ImportDataManagerRules([]DMImportGroup{{ID: "g2", Name: "B"}}, nil, false); err != nil {
		t.Fatalf("second import: %v", err)
	}

	groups, err := db.ListDMGroups("")
	if err != nil {
		t.Fatalf("ListDMGroups: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %+v, want both g1 and g2 present (no wipe requested)", groups)
	}
}
