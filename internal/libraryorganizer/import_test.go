package libraryorganizer

import (
	"strings"
	"testing"
)

// sampleLOSettings mirrors the real losettingsx.dat's actual shape (see
// comic-server-3bz.2's design notes, ground-truthed against the user's
// real file): multiple <Profile> elements, seven Item-list collections,
// an ExcludeRules block with its own Operator/ExcludeMode attrs plus
// child ExcludeRule elements, and a profile with an empty FolderTemplate
// (the real "Move To 0Day" profile moves files OUT of the library, using
// only a FileTemplate).
const sampleLOSettings = `<?xml version="1.0" encoding="utf-8"?>
<Profiles LastUsed="Default">
  <Profile Name="Default" Version="2.1">
    <FolderTemplate>{&lt;publisher&gt;}\{&lt;series&gt;} ({&lt;volume&gt;})</FolderTemplate>
    <BaseFolder>G:\Comics</BaseFolder>
    <FileTemplate>{&lt;series&gt;}{ #&lt;number2&gt;}</FileTemplate>
    <EmptyFolder />
    <Postfix>
      <Item Name="Format" Value=")" />
    </Postfix>
    <IllegalCharacters>
      <Item Name="&quot;" Value="'" />
      <Item Name="/" Value="" />
    </IllegalCharacters>
    <Months>
      <Item Name="1" Value="January" />
      <Item Name="13" Value="Spring" />
    </Months>
    <UseFolder>true</UseFolder>
    <UseFileName>true</UseFileName>
    <RemoveEmptyFolder>true</RemoveEmptyFolder>
    <MoveFileless>false</MoveFileless>
    <FilelessFormat>.jpg</FilelessFormat>
    <ExcludeMode>Only</ExcludeMode>
    <FailEmptyValues>false</FailEmptyValues>
    <MoveFailed>false</MoveFailed>
    <Mode>Move</Mode>
    <CopyMode>false</CopyMode>
    <AutoSpaceFields>true</AutoSpaceFields>
    <ReplaceMultipleSpaces>true</ReplaceMultipleSpaces>
    <ExcludeRules Operator="Any" ExcludeMode="Only" />
  </Profile>
  <Profile Name="Archive" Version="2.1">
    <FolderTemplate>{&lt;publisher&gt;}\{&lt;series&gt;} ({&lt;volume&gt;})</FolderTemplate>
    <BaseFolder>\\192.168.0.40\Backup\ComicsArchive</BaseFolder>
    <FileTemplate>{&lt;series&gt;}{ #&lt;number2&gt;}</FileTemplate>
    <Mode>Move</Mode>
    <CopyMode>true</CopyMode>
    <ExcludeRules Operator="All" ExcludeMode="Only">
      <ExcludeRule Field="Tags" Operator="contains" Value="Archive" />
      <ExcludeRule Field="File Path" Operator="is not" Value="" />
    </ExcludeRules>
  </Profile>
  <Profile Name="Move To 0Day" Version="2.1">
    <FolderTemplate />
    <BaseFolder>G:\Downloads\DC++</BaseFolder>
    <FileTemplate>{&lt;series&gt;}{ #&lt;number2&gt;}</FileTemplate>
    <Mode>Move</Mode>
    <CopyMode>false</CopyMode>
    <ExcludeRules Operator="Any" ExcludeMode="Only" />
  </Profile>
</Profiles>
`

func TestParseLOSettings_SampleShape(t *testing.T) {
	n := 0
	genID := func() string {
		n++
		return "id" + string(rune('0'+n))
	}

	res, err := ParseLOSettings(strings.NewReader(sampleLOSettings), genID)
	if err != nil {
		t.Fatalf("ParseLOSettings: %v", err)
	}

	if res.LastUsed != "Default" {
		t.Errorf("LastUsed = %q, want %q", res.LastUsed, "Default")
	}
	if len(res.Profiles) != 3 {
		t.Fatalf("len(Profiles) = %d, want 3", len(res.Profiles))
	}

	byName := map[string]ImportedProfile{}
	for _, p := range res.Profiles {
		byName[p.Name] = p
	}

	def, ok := byName["Default"]
	if !ok {
		t.Fatal("missing Default profile")
	}
	if def.Mode != "Move" || def.BaseFolder != `G:\Comics` || def.CopyMode {
		t.Errorf("Default profile = %+v, want Mode=Move BaseFolder=G:\\Comics CopyMode=false", def)
	}
	if def.ExcludeMode != "Only" || def.ExcludeOperator != "Any" {
		t.Errorf("Default ExcludeMode/Operator = %s/%s, want Only/Any", def.ExcludeMode, def.ExcludeOperator)
	}

	// Item collections must be tagged with the right category and
	// preserve their real Name/Value pairs.
	var illegalCount, monthsCount, postfixCount int
	for _, item := range def.Items {
		switch item.Category {
		case "illegal_characters":
			illegalCount++
		case "months":
			monthsCount++
			if item.Name == "13" && item.Value != "Spring" {
				t.Errorf("months item 13 = %q, want Spring", item.Value)
			}
		case "postfix":
			postfixCount++
		}
	}
	if illegalCount != 2 || monthsCount != 2 || postfixCount != 1 {
		t.Errorf("item category counts: illegal=%d months=%d postfix=%d, want 2/2/1", illegalCount, monthsCount, postfixCount)
	}

	archive, ok := byName["Archive"]
	if !ok {
		t.Fatal("missing Archive profile")
	}
	if !archive.CopyMode {
		t.Error("Archive profile CopyMode should be true")
	}
	if archive.ExcludeOperator != "All" {
		t.Errorf("Archive ExcludeOperator = %q, want All", archive.ExcludeOperator)
	}
	if len(archive.ExcludeRules) != 2 {
		t.Fatalf("Archive ExcludeRules = %+v, want 2 entries", archive.ExcludeRules)
	}
	if archive.ExcludeRules[0].Field != "Tags" || archive.ExcludeRules[0].Operator != "contains" || archive.ExcludeRules[0].Value != "Archive" {
		t.Errorf("Archive ExcludeRules[0] = %+v, want {Tags contains Archive}", archive.ExcludeRules[0])
	}

	zeroDay, ok := byName["Move To 0Day"]
	if !ok {
		t.Fatal("missing Move To 0Day profile")
	}
	if zeroDay.FolderTemplate != "" {
		t.Errorf("Move To 0Day FolderTemplate = %q, want empty (this profile moves files OUT using only a FileTemplate)", zeroDay.FolderTemplate)
	}
	if zeroDay.FileTemplate == "" {
		t.Error("Move To 0Day FileTemplate should not be empty")
	}

	// Document order preserved via SortOrder.
	if !(def.SortOrder < archive.SortOrder && archive.SortOrder < zeroDay.SortOrder) {
		t.Errorf("sort order not in document order: Default=%d Archive=%d Move To 0Day=%d", def.SortOrder, archive.SortOrder, zeroDay.SortOrder)
	}
}

func TestParseLOSettings_ProfileWithNoExcludeRulesChildren(t *testing.T) {
	res, err := ParseLOSettings(strings.NewReader(sampleLOSettings), func() string { return "x" })
	if err != nil {
		t.Fatalf("ParseLOSettings: %v", err)
	}
	for _, p := range res.Profiles {
		if p.Name == "Default" && len(p.ExcludeRules) != 0 {
			t.Errorf("Default profile has an empty <ExcludeRules /> element, expected 0 rules, got %d", len(p.ExcludeRules))
		}
	}
}
