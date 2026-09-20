// Package cbl parses ComicRack's CBL reading-list XML format
// (comic-server-r1j4, spec: docs/plans/2026-09-20-cbl-reading-list-import.md
// §1). The schema is ground-truthed against ../ComicRackCE source
// (ComicRack.Engine/Database/ComicReadingListContainer.cs and
// ComicReadingListItem.cs), not guessed.
package cbl

import (
	"encoding/xml"
	"fmt"
	"io"
)

// maxCBLSize caps how much a single CBL file/upload is allowed to be
// (spec §7: untrusted XML from the network needs a size limit). A CBL
// listing every issue of a long-running series still fits comfortably
// under this; anything bigger is far more likely a malformed/malicious
// file than a real reading list.
const maxCBLSize = 32 << 20 // 32MB

// ReadingList is the root <ReadingList> element. MatcherMode/Matchers
// being non-empty means this CBL is a smart list rather than a plain
// book list (ComicReadingListContainer.cs) - v1 of the importer only
// needs to detect that case, not evaluate it (spec §1).
type ReadingList struct {
	XMLName     xml.Name `xml:"ReadingList"`
	Name        string   `xml:"Name"`
	NumIssues   int      `xml:"NumIssues"` // DieselTech extension, not in ComicRack's own schema; informational only, never trust over len(Books)
	MatcherMode string   `xml:"MatcherMode,attr"`
	Books       []Book   `xml:"Books>Book"`
	// Matchers is ComicRack's ComicBookMatcherCollection, serialized as
	// its own <Matchers> element - ALWAYS present in a real
	// ComicRack-generated CBL, even as an empty self-closing
	// <Matchers/> when there are no rules (confirmed against real
	// DieselTech samples - do not treat mere presence of <Matchers> as
	// "this is a smart list", only presence of children matters). Left
	// unparsed for v1 (raw child-element capture only) - see
	// IsSmartList.
	Matchers matchersElement `xml:"Matchers"`
}

// matchersElement captures whether <Matchers> has any child elements
// (actual ComicBookMatcher rules) without parsing them - see
// ReadingList.Matchers and IsSmartList.
type matchersElement struct {
	Children []struct {
		XMLName xml.Name
	} `xml:",any"`
}

// Book is one <Book> entry. Series/Number/Volume/Year/Format match
// ComicReadingListItem's ComicRack-compatible attributes exactly. CV is
// the DieselTech/Kavita/Komga <Database Name="cv"> extension - absent on
// a plain ComicRack-authored CBL, present on every sampled DieselTech
// file (see spec §1).
type Book struct {
	Series string `xml:"Series,attr"`
	// Number is free text, not always numeric - one sampled real CBL
	// used "¼" for a one-shot issue (spec §1). Never parse as an int.
	Number   string     `xml:"Number,attr"`
	Volume   int        `xml:"Volume,attr"` // -1 in ComicRack's own default when unset; a bare CBL may omit the attribute entirely, which XML decodes as Go's zero value 0, not -1 - callers must not treat 0 as "unset" the way ComicRack's own code does
	Year     int        `xml:"Year,attr"`
	Format   string     `xml:"Format,attr"`
	ID       string     `xml:"Id"`       // ComicRack's own internal book GUID - meaningless across libraries, only useful reimporting your own exported list
	FileName string     `xml:"FileName"` // optional, often empty in the wild
	Database []Database `xml:"Database"`
}

// Database is the DieselTech/Kavita/Komga <Database> extension. A Book
// can in principle carry more than one (different catalogs), though
// every sample seen has exactly one with Name="cv".
type Database struct {
	Name   string `xml:"Name,attr"`
	Series string `xml:"Series,attr"` // ComicVine volume ID, as a string (CBL stores it that way)
	Issue  string `xml:"Issue,attr"`  // ComicVine issue ID, as a string
}

// CVIssueID returns this Database entry's Issue as an int, and whether
// it parsed as a usable ComicVine issue ID (Name must be "cv" and Issue
// must be a positive integer).
func (d Database) CVIssueID() (int, bool) {
	if d.Name != "cv" || d.Issue == "" {
		return 0, false
	}
	var id int
	if _, err := fmt.Sscanf(d.Issue, "%d", &id); err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// CVIssueID returns the first usable ComicVine issue ID among this
// Book's <Database> entries, and whether one was found.
func (b Book) CVIssueID() (int, bool) {
	for _, db := range b.Database {
		if id, ok := db.CVIssueID(); ok {
			return id, true
		}
	}
	return 0, false
}

// IsSmartList reports whether this CBL embeds ComicBookMatcher rules
// instead of (or in addition to) a fixed book list - see ReadingList's
// doc comment. v1 of the importer should refuse/flag these rather than
// silently importing an empty or partial list.
func (r ReadingList) IsSmartList() bool {
	return len(r.Matchers.Children) > 0
}

// Parse decodes a CBL file from r. It enforces maxCBLSize and rejects
// external entity references (Go's encoding/xml does not resolve DTDs
// or external entities by default - no XXE surface - but a size limit
// is still applied here as defense against a huge/malformed upload,
// per spec §7).
func Parse(r io.Reader) (*ReadingList, error) {
	limited := &io.LimitedReader{R: r, N: maxCBLSize + 1}
	decoder := xml.NewDecoder(limited)
	// Explicitly disable entity expansion beyond XML's five predefined
	// entities - encoding/xml has no external entity/DTD support to
	// begin with, but being explicit here documents the intent rather
	// than relying on an implicit default.
	decoder.Entity = map[string]string{}

	var rl ReadingList
	if err := decoder.Decode(&rl); err != nil {
		return nil, fmt.Errorf("cbl: parse: %w", err)
	}
	if limited.N <= 0 {
		return nil, fmt.Errorf("cbl: parse: file exceeds %d byte limit", maxCBLSize)
	}
	return &rl, nil
}
