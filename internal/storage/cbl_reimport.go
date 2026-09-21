package storage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/duckpuppy/comic-server/internal/cbl"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/google/uuid"
)

// CBLReimportPolicy (comic-server-zw0o, decided 2026-09-21):
//
// A CBL-imported list's book membership is owned entirely by its source
// CBL - comic-server has no API or UI that edits a reading list's
// membership at all (add/remove/reorder book), for ANY list, imported or
// not. So there is nothing to merge: reimport always does a clean full
// replace of the list's Items and cbl_import_entries from the freshly
// re-matched CBL. List metadata the user CAN edit today - Name,
// Description, Favorite - is untouched by reimport.
//
// If a manual add/remove-book feature is ever built for reading lists,
// it must refuse to operate on a list with a non-NULL cbl_source -
// enforcing that a CBL-imported list stays reimport-only, not
// hand-edited out from under its own source of truth.
//
// The match-correction UI (comic-server-a2hz, cbl_import_entries.book_id
// re-pointed via CorrectCBLImportEntry) is a narrower, deliberate
// exception to that "no hand-editing" rule - it corrects a specific
// entry's match within one import's results (spec §2d), never adds or
// removes a CBL entry itself. A subsequent reimport still fully replaces
// cbl_import_entries from a fresh match, so a manual correction does NOT
// survive across reimports - accepted per the same reasoning as the rest
// of this policy: reimport is authoritative, and the fresh match is what
// a user asked for by triggering it.

// GetCBLSource returns listID's CBL import provenance, or nil if the
// list wasn't imported from a CBL (cbl_source is NULL).
func (db *DB) GetCBLSource(listID string) (*CBLImportSource, error) {
	var source, sourceRef, importedAt sql.NullString
	err := db.QueryRow(
		`SELECT cbl_source, cbl_source_ref, cbl_imported_at FROM lists WHERE id = ? AND deleted_at IS NULL`,
		listID,
	).Scan(&source, &sourceRef, &importedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get cbl source for list %s: %w", listID, err)
	}
	if !source.Valid {
		return nil, nil
	}
	return &CBLImportSource{Source: source.String, SourceRef: sourceRef.String, ImportedAt: importedAt.String}, nil
}

// CBLImportedList is one row of ListCBLImportedLists - enough to drive a
// watch/reimport-check sweep without loading each list's full body.
type CBLImportedList struct {
	ListID string
	Name   string
	CBLImportSource
}

// ListCBLImportedLists returns every non-deleted list that was created by
// a CBL import, for a watch/reimport-check sweep across all of them.
func (db *DB) ListCBLImportedLists() ([]CBLImportedList, error) {
	rows, err := db.Query(`
		SELECT id, name, cbl_source, cbl_source_ref, cbl_imported_at
		FROM lists
		WHERE cbl_source IS NOT NULL AND deleted_at IS NULL
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("query cbl-imported lists: %w", err)
	}
	defer rows.Close()

	var out []CBLImportedList
	for rows.Next() {
		var l CBLImportedList
		var sourceRef, importedAt sql.NullString
		if err := rows.Scan(&l.ListID, &l.Name, &l.Source, &sourceRef, &importedAt); err != nil {
			return nil, fmt.Errorf("scan cbl-imported list row: %w", err)
		}
		l.SourceRef = sourceRef.String
		l.ImportedAt = importedAt.String
		out = append(out, l)
	}
	return out, rows.Err()
}

// ErrListNotCBLImported is returned by ReimportCBL when listID isn't a
// CBL-imported list (cbl_source is NULL) - reimport only ever applies to
// a list that came from one.
var ErrListNotCBLImported = fmt.Errorf("cbl: list was not created by a CBL import")

// ReimportCBL re-matches rl (freshly parsed from the CBL's new content)
// against the current library and replaces listID's entire book
// membership and cbl_import_entries with the result - see
// CBLReimportPolicy above for why this is always a full replace, never a
// merge. list Name/Description/Favorite are left untouched. newSource
// becomes the list's new provenance (updated SourceRef/ImportedAt, and
// Source itself when the caller detected a rename - see
// cblrepo.CheckPathChange).
func (db *DB) ReimportCBL(listID string, rl *cbl.ReadingList, newSource CBLImportSource) (*CBLImportResult, error) {
	if rl.IsSmartList() {
		return nil, ErrCBLIsSmartList
	}

	existingSource, err := db.GetCBLSource(listID)
	if err != nil {
		return nil, err
	}
	if existingSource == nil {
		return nil, ErrListNotCBLImported
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
		ListID:  listID,
		Entries: make([]CBLImportEntry, 0, len(rl.Books)),
	}
	var items []library.ComicReadingListItem

	for pos, entry := range rl.Books {
		m := cbl.MatchEntry(entry, candidatePtrs)
		cvIssueID, _ := entry.CVIssueID()
		ie := CBLImportEntry{
			ID:               uuid.NewString(),
			ListID:           listID,
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
		case cbl.MatchCVID, cbl.MatchSeriesNumber:
			if m.Path == cbl.MatchCVID {
				result.MatchedCVID++
			} else {
				result.MatchedOther++
			}
			ie.BookID = m.Book.ID
			items = append(items, library.ComicReadingListItem{
				ID: m.Book.ID, Series: m.Book.Series, Number: m.Book.Number,
				Volume: m.Book.Volume, Year: m.Book.Year, Format: m.Book.Format,
			})
		default:
			result.Unmatched++
		}
		result.Entries = append(result.Entries, ie)
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(
		`UPDATE lists SET book_count = ?, updated_at = ?, cbl_source = ?, cbl_source_ref = ?, cbl_imported_at = ? WHERE id = ?`,
		len(items), now, nullIfEmpty(newSource.Source), nullIfEmpty(newSource.SourceRef), now, listID,
	); err != nil {
		return nil, fmt.Errorf("update list for reimport: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM reading_list_items WHERE list_id = ?`, listID); err != nil {
		return nil, fmt.Errorf("clear reading list items: %w", err)
	}
	for i, item := range items {
		if _, err := tx.Exec(
			`INSERT INTO reading_list_items (list_id, book_id, position) VALUES (?, ?, ?)`,
			listID, item.ID, i,
		); err != nil {
			return nil, fmt.Errorf("insert reading list item: %w", err)
		}
	}

	if _, err := tx.Exec(`DELETE FROM cbl_import_entries WHERE list_id = ?`, listID); err != nil {
		return nil, fmt.Errorf("clear cbl_import_entries: %w", err)
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
