package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
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

	// CandidateBookIDs is the narrowed candidate set cbl.MatchEntry was
	// choosing among (cbl.Match.Candidates), when Path == MatchSeriesNumber
	// and it was ambiguous (len > 1) - see createCBLImportEntriesTable's
	// candidate_book_ids column comment. Empty otherwise.
	CandidateBookIDs []string
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
			ID:               uuid.NewString(),
			ListID:           result.ListID,
			Position:         pos,
			Path:             m.Path,
			Series:           entry.Series,
			Number:           entry.Number,
			Volume:           entry.Volume,
			Year:             entry.Year,
			Format:           entry.Format,
			CVIssueID:        cvIssueID,
			CandidateBookIDs: candidateBookIDs(m),
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
		candidateJSON, err := candidateBookIDsJSON(ie.CandidateBookIDs)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`
			INSERT INTO cbl_import_entries (
				id, list_id, book_id, position, match_path,
				series, number, volume, year, format, cv_issue_id, candidate_book_ids
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			ie.ID, ie.ListID, nullIfEmpty(ie.BookID), ie.Position, ie.Path.String(),
			ie.Series, ie.Number, ie.Volume, ie.Year, ie.Format, nullIfZero(ie.CVIssueID), candidateJSON,
		); err != nil {
			return nil, fmt.Errorf("insert cbl_import_entries row: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return result, nil
}

// candidateBookIDs extracts the book IDs from an ambiguous
// cbl.MatchSeriesNumber match's Candidates (spec §2d) - nil for anything
// else (a cv_id match, no match, or an unambiguous single-candidate
// series_number match has nothing worth persisting to second-guess).
func candidateBookIDs(m cbl.Match) []string {
	if m.Path != cbl.MatchSeriesNumber || len(m.Candidates) < 2 {
		return nil
	}
	ids := make([]string, len(m.Candidates))
	for i, b := range m.Candidates {
		ids[i] = b.ID
	}
	return ids
}

// candidateBookIDsJSON encodes ids as a JSON array for the
// candidate_book_ids column, or nil (SQL NULL) when there's nothing to
// store.
func candidateBookIDsJSON(ids []string) (any, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("marshal candidate_book_ids: %w", err)
	}
	return string(b), nil
}

// GetUnresolvedCBLImportEntries returns every entry from a CBL import
// that didn't match a real book AND hasn't already been added to the
// wanted-books list (comic-server-sx2d) - the set "add unmatched to
// wanted" should act on. Running that action twice is a no-op the
// second time: once an entry gets a wanted_book_id, it's excluded here.
func (db *DB) GetUnresolvedCBLImportEntries(listID string) ([]CBLImportEntry, error) {
	rows, err := db.Query(`
		SELECT id, list_id, position, match_path, series, number, volume, year, format, cv_issue_id
		FROM cbl_import_entries
		WHERE list_id = ? AND match_path = 'none' AND wanted_book_id IS NULL
		ORDER BY position
	`, listID)
	if err != nil {
		return nil, fmt.Errorf("query unresolved cbl_import_entries: %w", err)
	}
	defer rows.Close()

	var entries []CBLImportEntry
	for rows.Next() {
		var e CBLImportEntry
		var volume, year, cvIssueID sql.NullInt64
		var format sql.NullString
		var matchPath string
		if err := rows.Scan(&e.ID, &e.ListID, &e.Position, &matchPath, &e.Series, &e.Number, &volume, &year, &format, &cvIssueID); err != nil {
			return nil, fmt.Errorf("scan cbl_import_entries row: %w", err)
		}
		// This query's WHERE clause only ever returns match_path='none'
		// rows (see the SELECT above), so this is always cbl.MatchNone -
		// no need to parse the string column back into a MatchPath.
		e.Path = cbl.MatchNone
		e.Volume = int(volume.Int64)
		e.Year = int(year.Int64)
		e.Format = format.String
		e.CVIssueID = int(cvIssueID.Int64)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// MarkCBLImportEntryWanted records that entryID's unmatched CBL entry
// was added to the wanted-books list as bookID - see
// GetUnresolvedCBLImportEntries.
func (db *DB) MarkCBLImportEntryWanted(entryID, bookID string) error {
	_, err := db.Exec(`UPDATE cbl_import_entries SET wanted_book_id = ? WHERE id = ?`, bookID, entryID)
	if err != nil {
		return fmt.Errorf("mark cbl_import_entries wanted: %w", err)
	}
	return nil
}

// GetCBLImportEntries returns every entry recorded for listID's CBL
// import (matched, unmatched, or manually corrected), in the CBL's
// original order - the read side of the match-correction UI
// (comic-server-a2hz, spec §2d). Unlike GetUnresolvedCBLImportEntries,
// this includes matched entries (with BookID and, for an ambiguous
// series_number match, CandidateBookIDs) so the UI can show what every
// entry resolved to, not just the misses.
func (db *DB) GetCBLImportEntries(listID string) ([]CBLImportEntry, error) {
	rows, err := db.Query(`
		SELECT id, list_id, book_id, position, match_path, series, number, volume, year, format, cv_issue_id, candidate_book_ids
		FROM cbl_import_entries
		WHERE list_id = ?
		ORDER BY position
	`, listID)
	if err != nil {
		return nil, fmt.Errorf("query cbl_import_entries: %w", err)
	}
	defer rows.Close()

	var entries []CBLImportEntry
	for rows.Next() {
		e, err := scanCBLImportEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// cblImportEntryScanner is the subset of *sql.Rows/*sql.Row that
// scanCBLImportEntry needs - lets GetCBLImportEntry (single row) and
// GetCBLImportEntries (many rows) share one scan implementation.
type cblImportEntryScanner interface {
	Scan(dest ...any) error
}

func scanCBLImportEntry(row cblImportEntryScanner) (CBLImportEntry, error) {
	var e CBLImportEntry
	var bookID sql.NullString
	var volume, year, cvIssueID sql.NullInt64
	var format, candidateJSON sql.NullString
	var matchPath string
	if err := row.Scan(&e.ID, &e.ListID, &bookID, &e.Position, &matchPath, &e.Series, &e.Number, &volume, &year, &format, &cvIssueID, &candidateJSON); err != nil {
		return CBLImportEntry{}, fmt.Errorf("scan cbl_import_entries row: %w", err)
	}
	e.BookID = bookID.String
	e.Path = matchPathFromString(matchPath)
	e.Volume = int(volume.Int64)
	e.Year = int(year.Int64)
	e.Format = format.String
	e.CVIssueID = int(cvIssueID.Int64)
	if candidateJSON.Valid && candidateJSON.String != "" {
		if err := json.Unmarshal([]byte(candidateJSON.String), &e.CandidateBookIDs); err != nil {
			return CBLImportEntry{}, fmt.Errorf("unmarshal candidate_book_ids for entry %s: %w", e.ID, err)
		}
	}
	return e, nil
}

// matchPathFromString is the inverse of cbl.MatchPath.String() - parses
// the match_path column back into the typed enum. Falls back to
// cbl.MatchNone for anything unrecognized rather than erroring, matching
// this codebase's general tolerance for stored data it can't parse
// cleanly (e.g. cvIssueID's strconv.Atoi error handling).
func matchPathFromString(s string) cbl.MatchPath {
	switch s {
	case "cv_id":
		return cbl.MatchCVID
	case "series_number":
		return cbl.MatchSeriesNumber
	case "manual":
		return cbl.MatchManual
	default:
		return cbl.MatchNone
	}
}

// ErrCBLImportEntryNotFound is returned by CorrectCBLImportEntry when
// entryID doesn't exist.
var ErrCBLImportEntryNotFound = fmt.Errorf("cbl: import entry not found")

// ErrCBLCandidateBookNotFound is returned by CorrectCBLImportEntry when
// newBookID doesn't exist in the library.
var ErrCBLCandidateBookNotFound = fmt.Errorf("cbl: candidate book not found")

// CorrectCBLImportEntry re-points entryID's match to newBookID (any real
// book, not just one of its recorded Candidates - the UI is expected to
// offer the candidates as quick picks, but this doesn't enforce that),
// or unmatches it when newBookID is empty - the write side of the
// match-correction UI (comic-server-a2hz, spec §2d). Updates the list's
// reading_list_items membership and book_count to match, and marks the
// entry's match_path as cbl.MatchManual so the UI can distinguish a
// user-corrected entry from the matcher's own output. Scoped to one
// entry at a time, not a general relink tool (spec §2d's own scope note)
// - see CBLReimportPolicy for why this doesn't survive a later reimport.
func (db *DB) CorrectCBLImportEntry(entryID, newBookID string) (*CBLImportEntry, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	row := tx.QueryRow(`
		SELECT id, list_id, book_id, position, match_path, series, number, volume, year, format, cv_issue_id, candidate_book_ids
		FROM cbl_import_entries WHERE id = ?
	`, entryID)
	entry, err := scanCBLImportEntry(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCBLImportEntryNotFound
	}
	if err != nil {
		return nil, err
	}

	if newBookID != "" {
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM books WHERE id = ?`, newBookID).Scan(&exists); err == sql.ErrNoRows {
			return nil, ErrCBLCandidateBookNotFound
		} else if err != nil {
			return nil, fmt.Errorf("check candidate book: %w", err)
		}
	}

	oldBookID := entry.BookID
	if oldBookID != "" && oldBookID != newBookID {
		if _, err := tx.Exec(`DELETE FROM reading_list_items WHERE list_id = ? AND book_id = ?`, entry.ListID, oldBookID); err != nil {
			return nil, fmt.Errorf("remove old reading list item: %w", err)
		}
	}
	if newBookID != "" && newBookID != oldBookID {
		// INSERT OR REPLACE: newBookID may already be a member of this
		// list (re-pointing one entry to a book another entry already
		// matched) - overwrite its position rather than erroring on the
		// (list_id, book_id) primary key.
		if _, err := tx.Exec(
			`INSERT OR REPLACE INTO reading_list_items (list_id, book_id, position) VALUES (?, ?, ?)`,
			entry.ListID, newBookID, entry.Position,
		); err != nil {
			return nil, fmt.Errorf("insert corrected reading list item: %w", err)
		}
	}

	newPath := cbl.MatchManual
	if newBookID == "" {
		newPath = cbl.MatchNone
	}
	if _, err := tx.Exec(
		`UPDATE cbl_import_entries SET book_id = ?, match_path = ? WHERE id = ?`,
		nullIfEmpty(newBookID), newPath.String(), entryID,
	); err != nil {
		return nil, fmt.Errorf("update cbl_import_entries: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(
		`UPDATE lists SET book_count = (SELECT COUNT(*) FROM reading_list_items WHERE list_id = ?), updated_at = ? WHERE id = ?`,
		entry.ListID, now, entry.ListID,
	); err != nil {
		return nil, fmt.Errorf("update list book_count: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	entry.BookID = newBookID
	entry.Path = newPath
	return &entry, nil
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
