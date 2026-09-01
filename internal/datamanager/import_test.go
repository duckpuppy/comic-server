package datamanager

import (
	"strings"
	"testing"
)

// sampleDataman mirrors the real dataman.dat's actual shape (see
// comic-server-764's design notes): nested groups holding both child
// groups and rulesets in document order, a ruleset with no name attribute
// (the real file has these), a "||"-joined multi-param value, and a
// <disabled> container whose children are NOT counted as a real group but
// whose contents get Disabled propagated onto them.
const sampleDataman = `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<collection name="Test User" version="2.7.4">
  <group name="Quality">
    <filtersanddefaults rulesetmode="AND" />
    <group name="Series Groups">
      <filtersanddefaults rulesetmode="AND" />
      <group name="Dark Horse Comics">
        <filtersanddefaults rulesetmode="AND" />
        <ruleset name="Buffy the Vampire Slayer" rulesetmode="AND">
          <rule field="Series" modifier="ContainsAllOf" value="Buffy the Vampire Slayer" />
          <rule field="Publisher" modifier="Is" value="Dark Horse Comics" />
          <action field="SeriesGroup" modifier="SetValue" value="Buffy the Vampire Slayer" />
        </ruleset>
      </group>
    </group>
    <ruleset rulesetmode="AND">
      <rule field="Series" modifier="IsAnyOf" value="Animal Man||Aquaman||Batman" />
      <action field="Format" modifier="SetValue" value="Main Series" />
    </ruleset>
  </group>
  <disabled name="Disabled" comment="Disabled Groups and Rulesets">
    <filtersanddefaults rulesetmode="AND" />
    <group name="Old Concept">
      <filtersanddefaults rulesetmode="AND" />
      <ruleset name="Retired Rule" rulesetmode="OR" comment="no longer used">
        <rule field="Series" modifier="Is" value="Foo" />
        <action field="Tags" modifier="Add" value="Retired" />
      </ruleset>
    </group>
  </disabled>
</collection>
`

func TestParseDataman_SampleShape(t *testing.T) {
	n := 0
	genID := func() string {
		n++
		return "id" + string(rune('0'+n))
	}

	res, err := ParseDataman(strings.NewReader(sampleDataman), genID)
	if err != nil {
		t.Fatalf("ParseDataman: %v", err)
	}

	if res.CollectionName != "Test User" {
		t.Errorf("CollectionName = %q, want %q", res.CollectionName, "Test User")
	}

	// 3 real <group> elements: Quality, Series Groups, Dark Horse Comics,
	// Old Concept - the <disabled> container itself must NOT produce a
	// group row (matches the real file: 130 <group> elements = 130
	// ImportedGroups, no +1 for <disabled>).
	if len(res.Groups) != 4 {
		t.Fatalf("len(Groups) = %d, want 4 (disabled container itself must not count)", len(res.Groups))
	}

	byName := map[string]ImportedGroup{}
	for _, g := range res.Groups {
		byName[g.Name] = g
	}

	quality, ok := byName["Quality"]
	if !ok || quality.ParentID != "" || quality.Disabled {
		t.Fatalf("Quality group = %+v, want top-level and not disabled", quality)
	}
	seriesGroups, ok := byName["Series Groups"]
	if !ok || seriesGroups.ParentID != quality.ID {
		t.Fatalf("Series Groups group = %+v, want ParentID %s", seriesGroups, quality.ID)
	}
	darkHorse, ok := byName["Dark Horse Comics"]
	if !ok || darkHorse.ParentID != seriesGroups.ID {
		t.Fatalf("Dark Horse Comics group = %+v, want ParentID %s", darkHorse, seriesGroups.ID)
	}

	// Old Concept was inside <disabled> - it must be top-level (ParentID
	// ""), not parented under a synthetic "Disabled" group, and must carry
	// Disabled=true.
	oldConcept, ok := byName["Old Concept"]
	if !ok || oldConcept.ParentID != "" || !oldConcept.Disabled {
		t.Fatalf("Old Concept group = %+v, want top-level and Disabled=true", oldConcept)
	}

	if len(res.Rulesets) != 3 {
		t.Fatalf("len(Rulesets) = %d, want 3", len(res.Rulesets))
	}

	byRSName := map[string]ImportedRuleset{}
	for _, rs := range res.Rulesets {
		byRSName[rs.Name] = rs
	}

	buffy, ok := byRSName["Buffy the Vampire Slayer"]
	if !ok || buffy.GroupID != darkHorse.ID || buffy.Disabled {
		t.Fatalf("Buffy ruleset = %+v, want GroupID %s, not disabled", buffy, darkHorse.ID)
	}
	if len(buffy.Rules) != 2 || len(buffy.Actions) != 1 {
		t.Fatalf("Buffy ruleset rules/actions = %d/%d, want 2/1", len(buffy.Rules), len(buffy.Actions))
	}

	// Anonymous ruleset (no name attribute) directly under "Quality" -
	// must still round-trip, empty Name and all, with its "||"-joined
	// multi-param value preserved verbatim.
	anon, ok := byRSName[""]
	if !ok || anon.GroupID != quality.ID {
		t.Fatalf("anonymous ruleset = %+v, want GroupID %s", anon, quality.ID)
	}
	if anon.Rules[0].Value != "Animal Man||Aquaman||Batman" {
		t.Errorf("anonymous ruleset rule value = %q, want the raw \"||\"-joined value untouched", anon.Rules[0].Value)
	}

	retired, ok := byRSName["Retired Rule"]
	if !ok || retired.GroupID != oldConcept.ID || !retired.Disabled || retired.Mode != "OR" {
		t.Fatalf("Retired Rule ruleset = %+v, want GroupID %s, Disabled=true, Mode=OR", retired, oldConcept.ID)
	}

	// Document order must be preserved via SortOrder for depth-first
	// evaluation replay: Quality(0) < Series Groups(1) < Dark Horse(2) <
	// Buffy ruleset(3) < anonymous ruleset(4) < Old Concept(5) < Retired
	// Rule(6).
	if !(quality.SortOrder < seriesGroups.SortOrder &&
		seriesGroups.SortOrder < darkHorse.SortOrder &&
		darkHorse.SortOrder < buffy.SortOrder &&
		buffy.SortOrder < anon.SortOrder &&
		anon.SortOrder < oldConcept.SortOrder &&
		oldConcept.SortOrder < retired.SortOrder) {
		t.Errorf("sort order not depth-first: Quality=%d SeriesGroups=%d DarkHorse=%d Buffy=%d Anon=%d OldConcept=%d Retired=%d",
			quality.SortOrder, seriesGroups.SortOrder, darkHorse.SortOrder, buffy.SortOrder, anon.SortOrder, oldConcept.SortOrder, retired.SortOrder)
	}
}

func TestParseDataman_NoDisabledSection(t *testing.T) {
	const xmlNoDisabled = `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<collection name="Empty Disabled Test" version="2.7.4">
  <group name="Solo">
    <ruleset name="Only" rulesetmode="AND">
      <rule field="Series" modifier="Is" value="X" />
      <action field="Tags" modifier="Add" value="Y" />
    </ruleset>
  </group>
</collection>
`
	n := 0
	res, err := ParseDataman(strings.NewReader(xmlNoDisabled), func() string {
		n++
		return "g" + string(rune('0'+n))
	})
	if err != nil {
		t.Fatalf("ParseDataman: %v", err)
	}
	if len(res.Groups) != 1 || len(res.Rulesets) != 1 {
		t.Fatalf("Groups=%d Rulesets=%d, want 1/1", len(res.Groups), len(res.Rulesets))
	}
	if res.Rulesets[0].Disabled {
		t.Error("ruleset with no <disabled> ancestor must not be marked Disabled")
	}
}
