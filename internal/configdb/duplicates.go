package configdb

import (
	"fmt"
	"time"
)

// Duplicate-review decisions (comic-server-as8l). Deliberately created
// lazily with CREATE TABLE IF NOT EXISTS instead of bumping schemaVersion,
// so this feature adds no migration step (and no merge conflict with other
// work that bumps the version).
//
// Two kinds of remembered decision:
//   - duplicate_keep_all: a group (identified by a hash of its sorted book
//     IDs) the user marked as "not really duplicates". If a new book later
//     joins the group its hash changes and it is re-flagged.
//   - duplicate_resolved_books: a loser whose file was quarantined by
//     "keep and trash the rest". The library record stays (there is no
//     book-delete in library.Backend), so detection must skip it.

func (db *DB) ensureDuplicateTables() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS duplicate_keep_all (
			group_key  TEXT PRIMARY KEY,
			decided_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS duplicate_resolved_books (
			book_id    TEXT PRIMARY KEY,
			kept_id    TEXT NOT NULL,
			decided_at TEXT NOT NULL
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("create duplicate tables: %w", err)
		}
	}
	return nil
}

// MarkDuplicateGroupKeepAll remembers a group as an intentional duplicate.
func (db *DB) MarkDuplicateGroupKeepAll(groupKey string) error {
	if err := db.ensureDuplicateTables(); err != nil {
		return err
	}
	_, err := db.Exec(`INSERT OR REPLACE INTO duplicate_keep_all (group_key, decided_at) VALUES (?, ?)`,
		groupKey, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("mark duplicate group keep-all: %w", err)
	}
	return nil
}

// KeepAllDuplicateGroups returns the set of remembered keep-all group keys.
func (db *DB) KeepAllDuplicateGroups() (map[string]bool, error) {
	return db.stringSet(`SELECT group_key FROM duplicate_keep_all`)
}

// MarkDuplicateBookResolved records that bookID was trashed as a duplicate
// of keptID.
func (db *DB) MarkDuplicateBookResolved(bookID, keptID string) error {
	if err := db.ensureDuplicateTables(); err != nil {
		return err
	}
	_, err := db.Exec(`INSERT OR REPLACE INTO duplicate_resolved_books (book_id, kept_id, decided_at) VALUES (?, ?, ?)`,
		bookID, keptID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("mark duplicate book resolved: %w", err)
	}
	return nil
}

// ResolvedDuplicateBooks returns the set of book IDs trashed as duplicates.
func (db *DB) ResolvedDuplicateBooks() (map[string]bool, error) {
	return db.stringSet(`SELECT book_id FROM duplicate_resolved_books`)
}

func (db *DB) stringSet(query string) (map[string]bool, error) {
	if err := db.ensureDuplicateTables(); err != nil {
		return nil, err
	}
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query duplicate decisions: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out[k] = true
	}
	return out, rows.Err()
}
