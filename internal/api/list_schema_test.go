package api

import (
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func schemaType(s *ListSchema, id string) *MatcherTypeInfo {
	for i := range s.MatcherTypes {
		if s.MatcherTypes[i].ID == id {
			return &s.MatcherTypes[i]
		}
	}
	return nil
}

// comic-server-0elp: backend-supported matcher types must be offered by the
// editor, with operator sets whose value inputs match how they evaluate.
func TestGetListSchema_AdvancedAndCVMatchers(t *testing.T) {
	s := listSchemaWithVirtualTags()

	cases := map[string]string{
		"ComicBookDuplicateMatcher":          "duplicate",
		"ComicBookExpressionMatcher":         "expression",
		"ComicServerCVSeriesCompleteMatcher": "yesno",
		"ComicServerCVMissingCountMatcher":   "numeric",
		"ComicServerCVPercentOwnedMatcher":   "numeric",
	}
	for id, ft := range cases {
		m := schemaType(s, id)
		if m == nil {
			t.Errorf("%s missing from schema", id)
			continue
		}
		if m.FieldType != ft {
			t.Errorf("%s FieldType = %q, want %q", id, m.FieldType, ft)
		}
		if len(s.Operators[ft]) == 0 {
			t.Errorf("no operators for fieldType %q", ft)
		}
	}

	for _, op := range s.Operators["duplicate"] {
		if op.HasValue {
			t.Errorf("duplicate operator %q must not take a value", op.Label)
		}
	}
	if len(s.Operators["duplicate"]) != 2 {
		t.Errorf("duplicate operators = %d, want 2 (On/Off)", len(s.Operators["duplicate"]))
	}
	for _, op := range s.Operators["expression"] {
		if !op.HasValue {
			t.Errorf("expression operator %q must take a value (the expression)", op.Label)
		}
	}
}

func TestGetListSchema_VirtualTagsOnlyWhenConfigured(t *testing.T) {
	t.Cleanup(func() { library.SetVirtualTags(nil) })

	library.SetVirtualTags(nil)
	for _, m := range listSchemaWithVirtualTags().MatcherTypes {
		if strings.Contains(m.ID, "VirtualTag") {
			t.Fatalf("unexpected virtual tag entry %q with none configured", m.ID)
		}
	}

	library.SetVirtualTags([]library.VirtualTag{
		{ID: 7, Name: "Decade", CaptionFormat: "{Year}", Enabled: true},
		{ID: 3, Name: "Disabled", CaptionFormat: "{Year}", Enabled: false},
	})
	s := listSchemaWithVirtualTags()
	m := schemaType(s, "ComicBookVirtualTag7Matcher")
	if m == nil {
		t.Fatal("configured virtual tag 7 missing")
	}
	if m.Label != "Decade" || m.FieldType != "string" {
		t.Errorf("tag entry = %+v, want label Decade / string", *m)
	}
	if schemaType(s, "ComicBookVirtualTag3Matcher") != nil {
		t.Error("disabled virtual tag must not be offered")
	}
	// The cached static schema must not have been mutated.
	if schemaType(getListSchema(), "ComicBookVirtualTag7Matcher") != nil {
		t.Error("static schema was mutated by virtual tag injection")
	}
}

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
