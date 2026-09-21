package storage

import (
	"fmt"
	"time"

	"github.com/duckpuppy/comic-server/internal/cbl"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/google/uuid"
)

// CBLImportSource records where an imported reading list came from
// (spec §4, comic-server-tnv4). Kept separate from library.ComicListItem
// deliberately - that struct round-trips through ComicDb.xml for real
// ComicRackCE compatibility, and CBL import provenance has no ComicRack
// equivalent to preserve there. Zero value means "not imported from a
// CBL" (the `lists.cbl_source` column is NULL for every ordinary list).
type CBLImportSource struct {
	// Source identifies where the CBL came from: "local_file" for a
	// direct upload, or "git:<repo-url>:<path>" for an entry fetched
	// from a cloned repo (e.g. DieselTech/CBL-ReadingLists) - see spec
	// §3/§5.
	Source string
	// SourceRef is the commit SHA (git sources) or content hash (local
	// file sources) this import was taken from - used by watch/reimport
	// (comic-server-zw0o, not yet implemented) to detect upstream
	// changes.
	SourceRef string
	// ImportedAt is when this list was last (re)imported, RFC3339.
	ImportedAt string
}

// CBLImportEntry is one row of cbl_import_entries: a single CBL <Book>
// entry as it was seen at import time, whether or not it matched a real
// book. Kept even for unmatched entries so the match-correction UI
// (comic-server-a2hz) and wanted-list integration (comic-server-sx2d)
// have something to act on without re-parsing the original CBL file.
type CBLImportEntry struct {
	ID       string
	ListID   string
	BookID   string // empty when unmatched (Path == cbl.MatchNone)
	Position int
	Path     cbl.MatchPath

	// Raw CBL entry fields, always populated regardless of match
	// outcome - see internal/cbl.Book.
	Series    string
	Number    string
	Volume    int
	Year      int
	Format    string
	CVIssueID int // 0 if the entry carried no <Database Name="cv"> id
}

// ErrCBLIsSmartList is returned by ImportCBL when the parsed CBL embeds
// matcher rules (cbl.ReadingList.IsSmartList()) - v1 of the importer
// only supports plain book lists (spec §1); smart-list CBLs are a
// planned fast-follow, not yet implemented.
var ErrCBLIsSmartList = fmt.Errorf("cbl: smart-list CBLs are not yet supported (v1 only imports plain book lists)")

// CBLImportResult summarizes one ImportCBL call: the new list's ID, a
// per-entry match-path breakdown, and every entry seen (matched or not)
// for callers that want to build a full import summary (spec §4/§6).
type CBLImportResult struct {
	ListID       string
	MatchedCVID  int
	MatchedOther int
	Unmatched    int
	Entries      []CBLImportEntry
}

// ImportCBL matches every entry in rl against the library (internal/cbl's
// two-path matcher, spec §2a/§2b) and creates a new ComicReadingList
// containing every matched book, in the CBL's original order. Unmatched
// entries are NOT added to the list (spec §2c: default drop + report -
// wanted-list integration is a separate, later, explicit step -
// comic-server-sx2d) but ARE recorded in cbl_import_entries so later
// features (match-correction UI comic-server-a2hz, wanted-list
// comic-server-sx2d) have something to act on.
//
// source describes where this CBL came from (spec §4); its zero value is
// valid (e.g. an ad-hoc import with no provenance to record).
func (db *DB) ImportCBL(rl *cbl.ReadingList, source CBLImportSource) (*CBLImportResult, error) {
	if rl.IsSmartList() {
		return nil, ErrCBLIsSmartList
	}

	candidates, err := db.GetAllBooks()
	if err != nil {
		return nil, fmt.Errorf("load library for matching: %w", err)
	}
	candidatePtrs := make([]*library.ComicBook, len(candidates))
	for i := range candidates {
		candidatePtrs[i] = &candidates[i]
	}

	result := &CBLImportResult{
		ListID:  uuid.NewString(),
		Entries: make([]CBLImportEntry, 0, len(rl.Books)),
	}
	list := &library.ComicListItem{
		Type: "ComicReadingList",
		ID:   result.ListID,
		Name: rl.Name,
	}

	for pos, entry := range rl.Books {
		m := cbl.MatchEntry(entry, candidatePtrs)
		cvIssueID, _ := entry.CVIssueID()
		ie := CBLImportEntry{
			ID:        uuid.NewString(),
			ListID:    result.ListID,
			Position:  pos,
			Path:      m.Path,
			Series:    entry.Series,
			Number:    entry.Number,
			Volume:    entry.Volume,
			Year:      entry.Year,
			Format:    entry.Format,
			CVIssueID: cvIssueID,
		}
		switch m.Path {
		case cbl.MatchCVID:
			result.MatchedCVID++
			ie.BookID = m.Book.ID
			list.Items = append(list.Items, library.ComicReadingListItem{
				ID: m.Book.ID, Series: m.Book.Series, Number: m.Book.Number,
				Volume: m.Book.Volume, Year: m.Book.Year, Format: m.Book.Format,
			})
		case cbl.MatchSeriesNumber:
			result.MatchedOther++
			ie.BookID = m.Book.ID
			list.Items = append(list.Items, library.ComicReadingListItem{
				ID: m.Book.ID, Series: m.Book.Series, Number: m.Book.Number,
				Volume: m.Book.Volume, Year: m.Book.Year, Format: m.Book.Format,
			})
		default:
			result.Unmatched++
		}
		result.Entries = append(result.Entries, ie)
	}
	list.BookCount = len(list.Items)

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := db.insertList(tx, list, "", ""); err != nil {
		return nil, fmt.Errorf("insert list: %w", err)
	}

	importedAt := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(
		`UPDATE lists SET cbl_source = ?, cbl_source_ref = ?, cbl_imported_at = ? WHERE id = ?`,
		nullIfEmpty(source.Source), nullIfEmpty(source.SourceRef), importedAt, result.ListID,
	); err != nil {
		return nil, fmt.Errorf("set cbl source provenance: %w", err)
	}

	for _, ie := range result.Entries {
		if _, err := tx.Exec(`
			INSERT INTO cbl_import_entries (
				id, list_id, book_id, position, match_path,
				series, number, volume, year, format, cv_issue_id
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			ie.ID, ie.ListID, nullIfEmpty(ie.BookID), ie.Position, ie.Path.String(),
			ie.Series, ie.Number, ie.Volume, ie.Year, ie.Format, nullIfZero(ie.CVIssueID),
		); err != nil {
			return nil, fmt.Errorf("insert cbl_import_entries row: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return result, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullIfZero(n int) any {
	if n == 0 {
		return nil
	}
	return n
}
