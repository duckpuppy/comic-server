package cbl

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// TestParse_RealSamples parses real CBL files downloaded from
// DieselTech/CBL-ReadingLists (comic-server-r1j4 spec §1 samples) to
// confirm the struct tags match real-world files, not just a hand-built
// fixture. These files are NOT checked into the repo (fetched live
// during research, gitignored/untracked scratch downloads) - this test
// is skipped if they aren't present locally.
func TestParse_RealSamples(t *testing.T) {
	samples := []struct {
		path          string
		wantBooks     int
		wantAllHaveCV bool
	}{
		{"/tmp/claude-1000/-home-duckpuppy-src-comic-server/cbl-samples/batman.cbl", 1156, true},
		{"/tmp/claude-1000/-home-duckpuppy-src-comic-server/cbl-samples/elmstreet.cbl", 40, true},
		{"/tmp/claude-1000/-home-duckpuppy-src-comic-server/cbl-samples/wonderland.cbl", 25, true},
	}

	for _, s := range samples {
		t.Run(s.path, func(t *testing.T) {
			data, err := os.ReadFile(s.path)
			if err != nil {
				t.Skipf("sample not present locally: %v", err)
			}
			rl, err := Parse(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if rl.Name == "" {
				t.Error("expected non-empty Name")
			}
			if len(rl.Books) != s.wantBooks {
				t.Errorf("len(Books) = %d, want %d", len(rl.Books), s.wantBooks)
			}
			if rl.IsSmartList() {
				t.Error("expected a plain book-list CBL, IsSmartList() = true")
			}
			cvCount := 0
			for _, b := range rl.Books {
				if b.Series == "" {
					t.Error("book with empty Series")
				}
				if _, ok := b.CVIssueID(); ok {
					cvCount++
				}
			}
			if s.wantAllHaveCV && cvCount != len(rl.Books) {
				t.Errorf("CV ID coverage = %d/%d, want all", cvCount, len(rl.Books))
			}
		})
	}
}

func TestParse_PlainComicRackCBL_NoDatabase(t *testing.T) {
	// A hand-built fixture matching the base ComicRack schema exactly
	// (ComicReadingListContainer.cs / ComicReadingListItem.cs) - no
	// <Database> extension, as a real ComicRack-only export would look.
	const xmlData = `<?xml version="1.0" encoding="utf-8"?>
<ReadingList xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
<Name>Test List</Name>
<Books>
<Book Series="Detective Comics" Number="27" Volume="1937" Year="1939" Format="">
<Id>00000000-0000-0000-0000-000000000000</Id>
<FileName></FileName>
</Book>
</Books>
</ReadingList>`

	rl, err := Parse(strings.NewReader(xmlData))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if rl.Name != "Test List" {
		t.Errorf("Name = %q, want Test List", rl.Name)
	}
	if len(rl.Books) != 1 {
		t.Fatalf("len(Books) = %d, want 1", len(rl.Books))
	}
	b := rl.Books[0]
	if b.Series != "Detective Comics" || b.Number != "27" || b.Volume != 1937 || b.Year != 1939 {
		t.Errorf("unexpected book fields: %+v", b)
	}
	if _, ok := b.CVIssueID(); ok {
		t.Error("expected no CV issue ID on a plain ComicRack CBL")
	}
	if rl.IsSmartList() {
		t.Error("plain book list should not be detected as a smart list")
	}
}

func TestParse_NonNumericIssueNumber(t *testing.T) {
	// Sampled real file used "¼" for a one-shot (spec §1) - Number must
	// stay a free-text string, never parsed as an int.
	const xmlData = `<ReadingList><Name>x</Name><Books>
<Book Series="Sonic the Hedgehog" Number="&#188;" Volume="1993" Year="1992"/>
</Books></ReadingList>`
	rl, err := Parse(strings.NewReader(xmlData))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if rl.Books[0].Number != "¼" {
		t.Errorf("Number = %q, want ¼", rl.Books[0].Number)
	}
}

func TestParse_SizeLimit(t *testing.T) {
	huge := "<ReadingList><Name>" + strings.Repeat("x", maxCBLSize+1) + "</Name></ReadingList>"
	_, err := Parse(strings.NewReader(huge))
	if err == nil {
		t.Fatal("expected an error for an oversized CBL")
	}
}

func TestParse_MalformedXML(t *testing.T) {
	_, err := Parse(strings.NewReader("<not-xml"))
	if err == nil {
		t.Fatal("expected an error for malformed XML")
	}
}
