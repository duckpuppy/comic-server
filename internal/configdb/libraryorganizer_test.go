package configdb

import "testing"

// TestLOProfile_RoundTrip mirrors the real "Default" profile's actual
// shape from the user's losettingsx.dat (comic-server-3bz.2): Move mode,
// BaseFolder=G:\Comics, real templates.
func TestLOProfile_RoundTrip(t *testing.T) {
	db := newTestDMDB(t)

	profile := LOProfile{
		ID:                    "p-default",
		Name:                  "Default",
		BaseFolder:            `G:\Comics`,
		FolderTemplate:        `{<publisher>}\{<imprint>}\{<series>} ({<volume>}{ <format>})`,
		FileTemplate:          `{<series>}{ Vol.<volume>}{ #<number2>}{ (of <count2>)}{ ({<month>, }<year>)}`,
		Mode:                  "Move",
		UseFolder:             true,
		UseFileName:           true,
		ReplaceMultipleSpaces: true,
		AutoSpaceFields:       true,
		RemoveEmptyFolder:     true,
		FilelessFormat:        ".jpg",
		ExcludeMode:           "Only",
		ExcludeOperator:       "Any",
		SortOrder:             0,
	}
	if err := db.CreateLOProfile(profile); err != nil {
		t.Fatalf("CreateLOProfile: %v", err)
	}

	got, err := db.GetLOProfile("p-default")
	if err != nil {
		t.Fatalf("GetLOProfile: %v", err)
	}
	if got == nil {
		t.Fatal("GetLOProfile returned nil")
	}
	if got.BaseFolder != profile.BaseFolder || got.FolderTemplate != profile.FolderTemplate ||
		got.FileTemplate != profile.FileTemplate || got.Mode != "Move" || !got.UseFolder || !got.UseFileName {
		t.Errorf("round-tripped profile = %+v, want match of %+v", got, profile)
	}

	list, err := db.ListLOProfiles()
	if err != nil {
		t.Fatalf("ListLOProfiles: %v", err)
	}
	if len(list) != 1 || list[0].ID != "p-default" {
		t.Fatalf("ListLOProfiles = %+v, want [p-default]", list)
	}
}

func TestLOProfileItems_RoundTrip(t *testing.T) {
	db := newTestDMDB(t)
	if err := db.CreateLOProfile(LOProfile{ID: "p1", Name: "Default"}); err != nil {
		t.Fatalf("CreateLOProfile: %v", err)
	}

	// Real IllegalCharacters entries from the user's losettingsx.dat.
	items := []LOProfileItem{
		{ProfileID: "p1", Category: "illegal_characters", Name: `"`, Value: "'"},
		{ProfileID: "p1", Category: "illegal_characters", Name: "/", Value: ""},
		{ProfileID: "p1", Category: "illegal_characters", Name: ":", Value: " - "},
		{ProfileID: "p1", Category: "months", Name: "1", Value: "January"},
		{ProfileID: "p1", Category: "months", Name: "13", Value: "Spring"},
	}
	for _, item := range items {
		if _, err := db.CreateLOProfileItem(item); err != nil {
			t.Fatalf("CreateLOProfileItem(%+v): %v", item, err)
		}
	}

	illegal, err := db.ListLOProfileItems("p1", "illegal_characters")
	if err != nil {
		t.Fatalf("ListLOProfileItems(illegal_characters): %v", err)
	}
	if len(illegal) != 3 {
		t.Fatalf("expected 3 illegal_characters items, got %d: %+v", len(illegal), illegal)
	}

	months, err := db.ListLOProfileItems("p1", "months")
	if err != nil {
		t.Fatalf("ListLOProfileItems(months): %v", err)
	}
	if len(months) != 2 {
		t.Fatalf("expected 2 months items, got %d", len(months))
	}

	// Category isolation: a different category must not leak in.
	empty, err := db.ListLOProfileItems("p1", "prefix")
	if err != nil {
		t.Fatalf("ListLOProfileItems(prefix): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected 0 prefix items, got %d", len(empty))
	}
}

func TestLOExcludeRules_RoundTrip(t *testing.T) {
	db := newTestDMDB(t)
	if err := db.CreateLOProfile(LOProfile{ID: "p1", Name: "Archive"}); err != nil {
		t.Fatalf("CreateLOProfile: %v", err)
	}

	// Real ExcludeRules from the user's "Archive" profile.
	if _, err := db.CreateLOExcludeRule(LOExcludeRule{ProfileID: "p1", Field: "Tags", Operator: "contains", Value: "Archive", SortOrder: 0}); err != nil {
		t.Fatalf("CreateLOExcludeRule(1): %v", err)
	}
	if _, err := db.CreateLOExcludeRule(LOExcludeRule{ProfileID: "p1", Field: "File Path", Operator: "is not", Value: "", SortOrder: 1}); err != nil {
		t.Fatalf("CreateLOExcludeRule(2): %v", err)
	}

	rules, err := db.ListLOExcludeRules("p1")
	if err != nil {
		t.Fatalf("ListLOExcludeRules: %v", err)
	}
	if len(rules) != 2 || rules[0].Field != "Tags" || rules[1].Field != "File Path" {
		t.Fatalf("ListLOExcludeRules = %+v, want [Tags, File Path] in order", rules)
	}
}

func TestDeleteLOProfile_CascadesToItemsAndRules(t *testing.T) {
	db := newTestDMDB(t)
	if err := db.CreateLOProfile(LOProfile{ID: "p1", Name: "Default"}); err != nil {
		t.Fatalf("CreateLOProfile: %v", err)
	}
	if _, err := db.CreateLOProfileItem(LOProfileItem{ProfileID: "p1", Category: "months", Name: "1", Value: "January"}); err != nil {
		t.Fatalf("CreateLOProfileItem: %v", err)
	}
	if _, err := db.CreateLOExcludeRule(LOExcludeRule{ProfileID: "p1", Field: "Tags", Operator: "contains", Value: "Archive"}); err != nil {
		t.Fatalf("CreateLOExcludeRule: %v", err)
	}

	if err := db.DeleteLOProfile("p1"); err != nil {
		t.Fatalf("DeleteLOProfile: %v", err)
	}

	items, err := db.ListLOProfileItems("p1", "months")
	if err != nil {
		t.Fatalf("ListLOProfileItems after cascade: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items after cascade delete, got %d", len(items))
	}
	rules, err := db.ListLOExcludeRules("p1")
	if err != nil {
		t.Fatalf("ListLOExcludeRules after cascade: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 exclude rules after cascade delete, got %d", len(rules))
	}
}

// TestLOProfile_UpdateAndExcludeRuleCRUD exercises the CRUD surface added
// for comic-server-7ecr's native profile editor: UpdateLOProfile,
// GetLOExcludeRule/UpdateLOExcludeRule/DeleteLOExcludeRule.
func TestLOProfile_UpdateAndExcludeRuleCRUD(t *testing.T) {
	db := newTestDMDB(t)

	p := LOProfile{ID: "p1", Name: "Original", BaseFolder: "/old", Mode: "Move"}
	if err := db.CreateLOProfile(p); err != nil {
		t.Fatalf("CreateLOProfile: %v", err)
	}
	p.Name = "Renamed"
	p.BaseFolder = "/new"
	p.CopyMode = true
	if err := db.UpdateLOProfile(p); err != nil {
		t.Fatalf("UpdateLOProfile: %v", err)
	}
	got, err := db.GetLOProfile("p1")
	if err != nil {
		t.Fatalf("GetLOProfile: %v", err)
	}
	if got == nil || got.Name != "Renamed" || got.BaseFolder != "/new" || !got.CopyMode {
		t.Fatalf("GetLOProfile after update = %+v, want Name=Renamed BaseFolder=/new CopyMode=true", got)
	}

	ruleID, err := db.CreateLOExcludeRule(LOExcludeRule{ProfileID: "p1", Field: "Tags", Operator: "contains", Value: "Archive"})
	if err != nil {
		t.Fatalf("CreateLOExcludeRule: %v", err)
	}
	rule, err := db.GetLOExcludeRule(ruleID)
	if err != nil || rule == nil || rule.Field != "Tags" {
		t.Fatalf("GetLOExcludeRule = %+v err=%v, want Field=Tags", rule, err)
	}
	rule.Value = "Vault"
	if err := db.UpdateLOExcludeRule(*rule); err != nil {
		t.Fatalf("UpdateLOExcludeRule: %v", err)
	}
	updated, err := db.GetLOExcludeRule(ruleID)
	if err != nil || updated == nil || updated.Value != "Vault" {
		t.Fatalf("GetLOExcludeRule after update = %+v err=%v, want Value=Vault", updated, err)
	}
	if err := db.DeleteLOExcludeRule(ruleID); err != nil {
		t.Fatalf("DeleteLOExcludeRule: %v", err)
	}
	if gone, err := db.GetLOExcludeRule(ruleID); err != nil || gone != nil {
		t.Errorf("expected exclude rule gone after delete, got %+v err=%v", gone, err)
	}
}
