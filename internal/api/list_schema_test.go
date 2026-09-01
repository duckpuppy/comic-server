package api

import "testing"

// TestGetListSchema_CustomValuesMatcher guards against comic-server-arn
// regressing: ComicBookCustomValuesMatcher was missing from the editor's
// type dropdown entirely, so the browser fell back to showing the FIRST
// unrelated option ("Series") for any real list using it (e.g. ComicRack's
// own Data Manager plugin marks processed books via a custom value) - a
// real production list ("07 DataManager") was found showing this exact
// mislabeling. The fix needs its own "customvalue" fieldType (not "string")
// because MatchValue holds the custom field's KEY, not a comparison value.
func TestGetListSchema_CustomValuesMatcher(t *testing.T) {
	schema := getListSchema()

	var found *MatcherTypeInfo
	for i := range schema.MatcherTypes {
		if schema.MatcherTypes[i].ID == "ComicBookCustomValuesMatcher" {
			found = &schema.MatcherTypes[i]
		}
	}
	if found == nil {
		t.Fatal("ComicBookCustomValuesMatcher missing from schema.MatcherTypes")
	}
	if found.FieldType != "customvalue" {
		t.Errorf("FieldType = %q, want %q (must not be \"string\" - MatchValue is a key name here, not a comparison value)", found.FieldType, "customvalue")
	}

	ops, ok := schema.Operators["customvalue"]
	if !ok || len(ops) == 0 {
		t.Fatal("schema.Operators[\"customvalue\"] missing or empty")
	}
	var hasEquals bool
	for _, op := range ops {
		if op.Label == "equals" {
			hasEquals = true
			if !op.HasValue || op.HasValue2 {
				t.Errorf("equals operator = %+v, want HasValue=true HasValue2=false", op)
			}
		}
	}
	if !hasEquals {
		t.Error("expected an \"equals\" operator in schema.Operators[\"customvalue\"]")
	}
}
