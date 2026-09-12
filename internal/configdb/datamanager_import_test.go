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

// TestImportDataManagerRules_WipeExistingPreservesManualTopLevelRuleset
// covers the simplest case of comic-server-vkpq's import-merge semantics:
// a ruleset created by hand (source="manual", the CreateDMRuleset
// default) at the top level must survive a wipe re-import untouched,
// while a top-level ruleset that came from the ORIGINAL import is wiped.
func TestImportDataManagerRules_WipeExistingPreservesManualTopLevelRuleset(t *testing.T) {
	db := newTestDMDB(t)

	if err := db.ImportDataManagerRules(nil, []DMImportRuleset{{ID: "rs-imported", Name: "From dataman.dat"}}, false); err != nil {
		t.Fatalf("first import: %v", err)
	}
	if err := db.CreateDMRuleset(DMRuleset{ID: "rs-manual", Name: "Hand-authored"}); err != nil {
		t.Fatalf("CreateDMRuleset(manual): %v", err)
	}

	if err := db.ImportDataManagerRules(nil, []DMImportRuleset{{ID: "rs-reimported", Name: "Re-imported"}}, true); err != nil {
		t.Fatalf("wipe re-import: %v", err)
	}

	if rs, err := db.GetDMRuleset("rs-imported"); err != nil || rs != nil {
		t.Errorf("expected original imported ruleset gone after wipe, got %+v err=%v", rs, err)
	}
	if rs, err := db.GetDMRuleset("rs-manual"); err != nil || rs == nil {
		t.Errorf("expected manual ruleset to SURVIVE the wipe, got %+v err=%v", rs, err)
	}
	if rs, err := db.GetDMRuleset("rs-reimported"); err != nil || rs == nil || rs.Source != "import" {
		t.Errorf("expected freshly re-imported ruleset present with Source=import, got %+v err=%v", rs, err)
	}
}

// TestImportDataManagerRules_WipeExistingReparentsManualContentOutOfDeletedGroup
// is the case that makes this feature non-trivial: a manual ruleset (and a
// manual subgroup) nested INSIDE a group that came from the original
// import must not be destroyed as collateral damage when that import-
// sourced parent group is wiped and cascades - both should survive,
// re-parented to the nearest surviving (non-import) ancestor, which here
// is the root since the entire original group tree is import-sourced.
func TestImportDataManagerRules_WipeExistingReparentsManualContentOutOfDeletedGroup(t *testing.T) {
	db := newTestDMDB(t)

	if err := db.ImportDataManagerRules([]DMImportGroup{{ID: "g-imported", Name: "Imported Folder"}}, nil, false); err != nil {
		t.Fatalf("first import: %v", err)
	}
	if err := db.CreateDMRuleset(DMRuleset{ID: "rs-manual-nested", GroupID: "g-imported", Name: "Hand-authored, nested"}); err != nil {
		t.Fatalf("CreateDMRuleset(nested manual): %v", err)
	}
	if err := db.CreateDMGroup(DMGroup{ID: "g-manual-nested", ParentID: "g-imported", Name: "Hand-authored subfolder"}); err != nil {
		t.Fatalf("CreateDMGroup(nested manual): %v", err)
	}

	if err := db.ImportDataManagerRules(nil, nil, true); err != nil {
		t.Fatalf("wipe re-import (no new content): %v", err)
	}

	if g, err := db.GetDMGroup("g-imported"); err != nil || g != nil {
		t.Errorf("expected the imported parent group gone after wipe, got %+v err=%v", g, err)
	}

	rs, err := db.GetDMRuleset("rs-manual-nested")
	if err != nil || rs == nil {
		t.Fatalf("expected manual nested ruleset to SURVIVE, got %+v err=%v", rs, err)
	}
	if rs.GroupID != "" {
		t.Errorf("rs-manual-nested.GroupID = %q, want \"\" (reparented to root, its deleted parent's own parent)", rs.GroupID)
	}

	g, err := db.GetDMGroup("g-manual-nested")
	if err != nil || g == nil {
		t.Fatalf("expected manual nested subfolder to SURVIVE, got %+v err=%v", g, err)
	}
	if g.ParentID != "" {
		t.Errorf("g-manual-nested.ParentID = %q, want \"\" (reparented to root)", g.ParentID)
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
