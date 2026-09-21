package storage

import "fmt"

// Schema version for migrations
const schemaVersion = 8

// initSchema creates the database tables if they don't exist.
func (db *DB) initSchema() error {
	// Check current schema version
	var version int
	err := db.QueryRow("PRAGMA user_version").Scan(&version)
	if err != nil {
		return fmt.Errorf("get schema version: %w", err)
	}

	if version >= schemaVersion {
		return nil // Already up to date
	}

	if version == 0 {
		// Fresh database: create all tables at once
		if err := db.createTables(); err != nil {
			return err
		}
	} else {
		// Incremental migrations for existing databases
		if version < 2 {
			if err := db.migrateV1ToV2(); err != nil {
				return fmt.Errorf("migrate v1→v2: %w", err)
			}
		}
		if version < 3 {
			if err := db.migrateV2ToV3(); err != nil {
				return fmt.Errorf("migrate v2→v3: %w", err)
			}
		}
		if version < 4 {
			if err := db.migrateV3ToV4(); err != nil {
				return fmt.Errorf("migrate v3→v4: %w", err)
			}
		}
		if version < 5 {
			if err := db.migrateV4ToV5(); err != nil {
				return fmt.Errorf("migrate v4→v5: %w", err)
			}
		}
		if version < 6 {
			if err := db.migrateV5ToV6(); err != nil {
				return fmt.Errorf("migrate v5→v6: %w", err)
			}
		}
		if version < 7 {
			if err := db.migrateV6ToV7(); err != nil {
				return fmt.Errorf("migrate v6→v7: %w", err)
			}
		}
		if version < 8 {
			if err := db.migrateV7ToV8(); err != nil {
				return fmt.Errorf("migrate v7→v8: %w", err)
			}
		}
	}

	// Update schema version
	_, err = db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion))
	if err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}

	return nil
}

// migrateV1ToV2 adds file system metadata, catalog/ownership, pricing, and dynamic fields.
func (db *DB) migrateV1ToV2() error {
	alters := []string{
		// File system metadata
		"ALTER TABLE books ADD COLUMN file_size INTEGER DEFAULT 0",
		"ALTER TABLE books ADD COLUMN file_modified_time TEXT",
		"ALTER TABLE books ADD COLUMN file_creation_time TEXT",
		// Catalog / ownership
		"ALTER TABLE books ADD COLUMN isbn TEXT",
		"ALTER TABLE books ADD COLUMN book_age TEXT",
		"ALTER TABLE books ADD COLUMN book_condition TEXT",
		"ALTER TABLE books ADD COLUMN book_store TEXT",
		"ALTER TABLE books ADD COLUMN book_owner TEXT",
		"ALTER TABLE books ADD COLUMN book_collection_status TEXT",
		"ALTER TABLE books ADD COLUMN book_notes TEXT",
		"ALTER TABLE books ADD COLUMN book_location TEXT",
		// Pricing
		"ALTER TABLE books ADD COLUMN book_price REAL DEFAULT -1",
		// Dynamic / sync metadata
		"ALTER TABLE books ADD COLUMN new_pages INTEGER DEFAULT 0",
	}
	for _, stmt := range alters {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	return nil
}

// migrateV2ToV3 adds soft-delete support (comic-server-b53): a book/list
// removed from an XML import is marked deleted_at instead of being
// DELETEd outright, so it's recoverable if it reappears in a later
// import and doesn't vanish with no trace if it was removed by mistake -
// same "quarantine, don't destroy" reasoning as internal/trash
// (comic-server-1up). NULL means not deleted; every read query needs a
// "WHERE deleted_at IS NULL" (or equivalent) filter to keep soft-deleted
// rows invisible to normal use - see queryBooks/GetBook/GetList/
// GetAllLists/GetBookCount.
func (db *DB) migrateV2ToV3() error {
	stmts := []string{
		"ALTER TABLE books ADD COLUMN deleted_at TEXT",
		"ALTER TABLE lists ADD COLUMN deleted_at TEXT",
		"CREATE INDEX IF NOT EXISTS idx_books_not_deleted ON books(id) WHERE deleted_at IS NULL",
		"CREATE INDEX IF NOT EXISTS idx_lists_not_deleted ON lists(id) WHERE deleted_at IS NULL",
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	return nil
}

// migrateV3ToV4 adds xml_snapshot (comic-server-aio): a JSON snapshot of
// the book exactly as it was parsed from the XML at the time it was last
// imported, kept alongside import_hash. A reimport whose hash differs no
// longer overwrites every column from the new XML wholesale - it diffs
// the new XML struct against this snapshot field-by-field and only
// touches columns that genuinely changed in the XML, so a live write
// comic-server itself made to some other field (ScanInformation via
// scan-info, FilePath via CBZ-convert, reading progress via reverse sync)
// survives a reimport triggered by an unrelated XML edit. See
// diffBookColumns/mergeUpdateBook in import.go.
//
// Existing rows get xml_snapshot NULL after this migration - there's no
// prior XML parse available to backfill from. The
// first reimport after upgrading degrades to a full overwrite for each
// changed row (same as today's pre-fix behavior, not worse), and from
// then on a real snapshot exists and future reimports merge properly.
func (db *DB) migrateV3ToV4() error {
	if _, err := db.Exec("ALTER TABLE books ADD COLUMN xml_snapshot TEXT"); err != nil {
		return fmt.Errorf("add xml_snapshot column: %w", err)
	}
	return nil
}

// migrateV4ToV5 adds base_list_id (comic-server-38j): a smart list's
// BaseListId (scopes matcher evaluation to another list's result set
// instead of the whole library - see library.ComicListItem.BaseListId)
// was never persisted anywhere in the SQL schema at all, despite
// SQLiteBackend.evaluationLibrary/MatchBooks already having logic that
// reads it (comic-server-hha fixed the LOOKUP mechanism, assuming this
// field would be populated - it never was). Every scoped smart list
// silently evaluated against the entire library instead of its actual
// base list's members, a correctness gap wider than just miscounting -
// found live while validating comic-server-254 against the real library
// ("Lady Death", scoped to a small horror-imprint base list, was
// matching against all 67K books instead of that base list's members).
func (db *DB) migrateV4ToV5() error {
	if _, err := db.Exec("ALTER TABLE lists ADD COLUMN base_list_id TEXT"); err != nil {
		return fmt.Errorf("add base_list_id column: %w", err)
	}
	return nil
}

// migrateV5ToV6 adds CBL reading-list import support (comic-server-tnv4,
// spec docs/plans/2026-09-20-cbl-reading-list-import.md §4): source
// provenance on lists that came from a CBL import, and a per-entry
// record of every CBL entry seen at import time (matched or not),
// needed by the later match-correction UI (comic-server-a2hz) and
// wanted-list integration (comic-server-sx2d) beads.
//
// cbl_source/cbl_source_ref/cbl_imported_at are NULL for every list not
// imported from a CBL (the overwhelming majority) - this is additive,
// not a behavior change for existing lists. Deliberately separate from
// the existing import_hash/updated_at columns on `lists`, which mean
// something different (ComicDb.xml-wide import/reimport change
// detection - see import.go, reimport_merge.go); reusing those would
// conflate two unrelated import concepts.
func (db *DB) migrateV5ToV6() error {
	stmts := []string{
		"ALTER TABLE lists ADD COLUMN cbl_source TEXT",
		"ALTER TABLE lists ADD COLUMN cbl_source_ref TEXT",
		"ALTER TABLE lists ADD COLUMN cbl_imported_at TEXT",
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("migrate v5->v6: %s: %w", s, err)
		}
	}
	return db.createCBLImportEntriesTable()
}

// migrateV6ToV7 adds cbl_import_entries.wanted_book_id
// (comic-server-sx2d, spec §2e): when a user explicitly adds an
// unmatched CBL entry to the wanted-books list (comic-server-38f7), this
// records which wanted book that entry became, both for idempotency
// (running "add unmatched to wanted" twice must not create duplicates)
// and so a later feature (comic-server-a2hz's match-correction UI) can
// distinguish "unmatched, nothing done yet" from "unmatched, already
// wanted-listed".
func (db *DB) migrateV6ToV7() error {
	// createCBLImportEntriesTable's DDL already includes wanted_book_id
	// (it must, to give a truly fresh v0 database - which only ever runs
	// createTables(), never the numbered migrations - the full current
	// schema). That means an upgrade that goes v5->v6->v7 in the same
	// Open() call already gets the column from v5->v6's table creation,
	// making this ALTER redundant - check first rather than erroring on
	// "duplicate column name" for that path. A database that was
	// already sitting at v6 (table existed without the column) still
	// needs the real ALTER.
	hasColumn, err := db.hasColumn("cbl_import_entries", "wanted_book_id")
	if err != nil {
		return fmt.Errorf("check wanted_book_id column: %w", err)
	}
	if hasColumn {
		return nil
	}
	if _, err := db.Exec("ALTER TABLE cbl_import_entries ADD COLUMN wanted_book_id TEXT"); err != nil {
		return fmt.Errorf("add wanted_book_id column: %w", err)
	}
	return nil
}

// hasColumn reports whether table has a column named column, via
// SQLite's pragma_table_info.
func (db *DB) hasColumn(table, column string) (bool, error) {
	rows, err := db.Query(`SELECT 1 FROM pragma_table_info(?) WHERE name = ?`, table, column)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), rows.Err()
}

// migrateV7ToV8 promotes ComicVine volume/issue ID to first-class,
// indexed columns on `books` (comic-server-r8td), instead of living only
// as comicvine_volume=N/comicvine_issue=N rows in the generic
// book_custom_values table. This is a REAL migration, not a permanent
// shadow field: book_custom_values stops storing these two keys at rest.
//
// Backward compatibility is preserved by construction, not by keeping a
// duplicate copy: every place that reconstructs a book's
// CustomValuesStore string for reads (loadBookCustomValues,
// loadTagsAndCustomValuesBatch, liveBookSnapshot) now also synthesizes
// comicvine_volume=/comicvine_issue= into that string from these new
// columns. So book.CustomValuesStore looks IDENTICAL to before this
// migration for every existing caller (internal/cbl/match.go,
// internal/workflow/stage.go, internal/comicvine/*,
// internal/api/cbl_wanted.go) and for ComicDb.xml export
// (internal/storage/export.go, which reuses GetAllBooks/GetBook) -
// none of them needed to change. See comic-server-r8td's bead notes for
// the exact touch points this was designed against.
func (db *DB) migrateV7ToV8() error {
	hasColumn, err := db.hasColumn("books", "cv_volume_id")
	if err != nil {
		return fmt.Errorf("check cv_volume_id column: %w", err)
	}
	if hasColumn {
		return nil // already current (fresh-DB createTables path got there directly)
	}

	stmts := []string{
		"ALTER TABLE books ADD COLUMN cv_volume_id INTEGER",
		"ALTER TABLE books ADD COLUMN cv_issue_id INTEGER",
		"CREATE INDEX IF NOT EXISTS idx_books_cv_volume_id ON books(cv_volume_id) WHERE cv_volume_id IS NOT NULL",
		"CREATE INDEX IF NOT EXISTS idx_books_cv_issue_id ON books(cv_issue_id) WHERE cv_issue_id IS NOT NULL",
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("migrate v7->v8: %s: %w", s, err)
		}
	}

	// Backfill from whatever's already sitting in book_custom_values,
	// then remove those rows - canonical storage moves, it doesn't
	// duplicate. CAST(... AS INTEGER) is forgiving (returns 0 on a
	// non-numeric value) rather than erroring, matching this codebase's
	// existing tolerance for malformed custom-value data elsewhere
	// (e.g. cvIssueID's strconv.Atoi error just means "not tagged").
	if _, err := db.Exec(`
		UPDATE books SET cv_volume_id = (
			SELECT CAST(value AS INTEGER) FROM book_custom_values
			WHERE book_id = books.id AND key = 'comicvine_volume'
		)
		WHERE id IN (SELECT book_id FROM book_custom_values WHERE key = 'comicvine_volume')
	`); err != nil {
		return fmt.Errorf("backfill cv_volume_id: %w", err)
	}
	if _, err := db.Exec(`
		UPDATE books SET cv_issue_id = (
			SELECT CAST(value AS INTEGER) FROM book_custom_values
			WHERE book_id = books.id AND key = 'comicvine_issue'
		)
		WHERE id IN (SELECT book_id FROM book_custom_values WHERE key = 'comicvine_issue')
	`); err != nil {
		return fmt.Errorf("backfill cv_issue_id: %w", err)
	}
	if _, err := db.Exec(`DELETE FROM book_custom_values WHERE key IN ('comicvine_volume', 'comicvine_issue')`); err != nil {
		return fmt.Errorf("remove migrated comicvine_volume/comicvine_issue rows: %w", err)
	}
	return nil
}

// createCBLImportEntriesTable creates cbl_import_entries if it doesn't
// exist - shared between the fresh-database path (createTables) and the
// v5->v6 migration path for existing databases.
func (db *DB) createCBLImportEntriesTable() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS cbl_import_entries (
			id TEXT PRIMARY KEY,
			list_id TEXT NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
			-- book_id is set when this entry matched a real book; NULL
			-- when unmatched (dropped from the list itself, per spec
			-- §2c, but kept here so the match-correction UI and
			-- wanted-list flow can see what was missed).
			book_id TEXT REFERENCES books(id) ON DELETE SET NULL,
			position INTEGER NOT NULL,
			-- match_path: 'cv_id' / 'series_number' / 'none' - see
			-- internal/cbl.MatchPath.
			match_path TEXT NOT NULL,
			-- Raw CBL entry fields, kept even when matched, so an
			-- unmatched entry still has something to display/act on
			-- (add to wanted list, manual re-match) without re-parsing
			-- the original file.
			series TEXT NOT NULL,
			number TEXT NOT NULL,
			volume INTEGER,
			year INTEGER,
			format TEXT,
			cv_issue_id INTEGER,
			-- Set when this entry was explicitly added to the
			-- wanted-books list (comic-server-sx2d) - see
			-- migrateV6ToV7's doc comment.
			wanted_book_id TEXT
		)
	`)
	if err != nil {
		return fmt.Errorf("create cbl_import_entries table: %w", err)
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_cbl_import_entries_list ON cbl_import_entries(list_id, position)`)
	if err != nil {
		return fmt.Errorf("create cbl_import_entries index: %w", err)
	}
	return nil
}

func (db *DB) createTables() error {
	// Books table - mirrors ComicBook struct
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS books (
			-- Identification
			id TEXT PRIMARY KEY,
			file_path TEXT NOT NULL,

			-- Basic metadata
			title TEXT,
			series TEXT,
			number TEXT,
			volume INTEGER,
			year INTEGER,
			month INTEGER,
			day INTEGER,

			-- Publishing info
			publisher TEXT,
			imprint TEXT,
			genre TEXT,
			format TEXT,
			age_rating TEXT,
			language_iso TEXT,

			-- Story info
			summary TEXT,
			notes TEXT,
			review TEXT,
			story_arc TEXT,
			series_group TEXT,
			alternate_series TEXT,
			alternate_number TEXT,
			alternate_count INTEGER,
			count INTEGER,

			-- Credits (stored as comma-separated strings, matching ComicRack format)
			writer TEXT,
			penciller TEXT,
			inker TEXT,
			colorist TEXT,
			letterer TEXT,
			cover_artist TEXT,
			editor TEXT,
			translator TEXT,

			-- Characters and teams
			characters TEXT,
			teams TEXT,
			locations TEXT,
			main_character_or_team TEXT,

			-- Reading progress
			current_page INTEGER DEFAULT 0,
			last_page INTEGER DEFAULT 0,
			last_page_read INTEGER DEFAULT 0,
			open_count INTEGER DEFAULT 0,
			opened_time TEXT,

			-- Ratings
			rating REAL DEFAULT 0,
			community_rating REAL DEFAULT 0,

			-- Flags
			checked INTEGER DEFAULT 0,
			file_is_missing INTEGER DEFAULT 0,
			comic_info_is_dirty INTEGER DEFAULT 0,

			-- Book info
			page_count INTEGER DEFAULT 0,
			web TEXT,
			scan_information TEXT,

			-- Series info
			series_complete TEXT,

			-- Physical properties
			black_and_white TEXT,
			manga TEXT,

			-- Cover and display
			preferred_front_cover INTEGER DEFAULT 0,

			-- Timestamps
			added_time TEXT,
			released_time TEXT,

			-- File system metadata
			file_size INTEGER DEFAULT 0,
			file_modified_time TEXT,
			file_creation_time TEXT,

			-- Catalog / ownership
			isbn TEXT,
			book_age TEXT,
			book_condition TEXT,
			book_store TEXT,
			book_owner TEXT,
			book_collection_status TEXT,
			book_notes TEXT,
			book_location TEXT,

			-- Pricing
			book_price REAL DEFAULT -1,

			-- Dynamic / sync metadata
			new_pages INTEGER DEFAULT 0,

			-- Dynamic features
			enable_proposed INTEGER DEFAULT 0,
			enable_dynamic_update INTEGER DEFAULT 0,
			last_opened_from_list_id TEXT,

			-- Pages stored as JSON array
			pages TEXT,

			-- Import tracking
			import_hash TEXT,
			updated_at TEXT,

			-- Soft delete (comic-server-b53): NULL = not deleted
			deleted_at TEXT,

			-- Reimport merge (comic-server-aio): JSON snapshot of the book
			-- as last parsed from XML, for field-level diffing on reimport
			xml_snapshot TEXT,

			-- ComicVine identity, first-class (comic-server-r8td) - see
			-- migrateV7ToV8's doc comment for the full design. NULL when
			-- untagged; synthesized back into CustomValuesStore
			-- (comicvine_volume/comicvine_issue) on every read for
			-- backward compatibility.
			cv_volume_id INTEGER,
			cv_issue_id INTEGER
		)
	`)
	if err != nil {
		return fmt.Errorf("create books table: %w", err)
	}

	// Indexes for common queries (smart list filtering)
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_books_cv_volume_id ON books(cv_volume_id) WHERE cv_volume_id IS NOT NULL",
		"CREATE INDEX IF NOT EXISTS idx_books_cv_issue_id ON books(cv_issue_id) WHERE cv_issue_id IS NOT NULL",
		"CREATE INDEX IF NOT EXISTS idx_books_series ON books(series)",
		"CREATE INDEX IF NOT EXISTS idx_books_publisher ON books(publisher)",
		"CREATE INDEX IF NOT EXISTS idx_books_year ON books(year)",
		"CREATE INDEX IF NOT EXISTS idx_books_rating ON books(rating)",
		"CREATE INDEX IF NOT EXISTS idx_books_added_time ON books(added_time)",
		"CREATE INDEX IF NOT EXISTS idx_books_genre ON books(genre)",
		"CREATE INDEX IF NOT EXISTS idx_books_format ON books(format)",
		"CREATE INDEX IF NOT EXISTS idx_books_writer ON books(writer)",
		"CREATE INDEX IF NOT EXISTS idx_books_file_path ON books(file_path)",
		// Partial index: every read query filters "WHERE deleted_at IS
		// NULL" (or the reverse, to find soft-deleted rows), and the huge
		// majority of rows are never deleted - a partial index keeps this
		// small instead of indexing (mostly-NULL) deleted_at across the
		// whole table.
		"CREATE INDEX IF NOT EXISTS idx_books_not_deleted ON books(id) WHERE deleted_at IS NULL",
	}
	for _, idx := range indexes {
		if _, err := db.Exec(idx); err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}

	// Custom values table (normalized for query performance)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS book_custom_values (
			book_id TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
			key TEXT NOT NULL,
			value TEXT,
			PRIMARY KEY (book_id, key)
		)
	`)
	if err != nil {
		return fmt.Errorf("create book_custom_values table: %w", err)
	}

	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_custom_values_lookup
		ON book_custom_values(key, value)
	`)
	if err != nil {
		return fmt.Errorf("create custom_values index: %w", err)
	}

	// Tags table (normalized for query performance)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS book_tags (
			book_id TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
			tag TEXT NOT NULL,
			PRIMARY KEY (book_id, tag)
		)
	`)
	if err != nil {
		return fmt.Errorf("create book_tags table: %w", err)
	}

	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_tags_lookup ON book_tags(tag)
	`)
	if err != nil {
		return fmt.Errorf("create tags index: %w", err)
	}

	// Lists table (smart lists, reading lists, folders)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS lists (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			parent_id TEXT REFERENCES lists(id) ON DELETE CASCADE,
			description TEXT,
			favorite INTEGER DEFAULT 0,
			collapsed INTEGER DEFAULT 0,
			matcher_mode TEXT,
			matchers TEXT,
			base_list_id TEXT,
			book_count INTEGER DEFAULT 0,
			import_hash TEXT,
			updated_at TEXT,

			-- Soft delete (comic-server-b53): NULL = not deleted
			deleted_at TEXT,

			-- CBL reading-list import provenance (comic-server-tnv4).
			-- NULL for every list not imported from a CBL. Deliberately
			-- separate from import_hash/updated_at above, which track
			-- ComicDb.xml-wide import/reimport, an unrelated concept.
			cbl_source TEXT,
			cbl_source_ref TEXT,
			cbl_imported_at TEXT
		)
	`)
	if err != nil {
		return fmt.Errorf("create lists table: %w", err)
	}

	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_lists_parent ON lists(parent_id)
	`)
	if err != nil {
		return fmt.Errorf("create lists parent index: %w", err)
	}

	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_lists_not_deleted ON lists(id) WHERE deleted_at IS NULL
	`)
	if err != nil {
		return fmt.Errorf("create lists not-deleted index: %w", err)
	}

	// Reading list items (junction table)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS reading_list_items (
			list_id TEXT NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
			book_id TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
			position INTEGER,
			PRIMARY KEY (list_id, book_id)
		)
	`)
	if err != nil {
		return fmt.Errorf("create reading_list_items table: %w", err)
	}

	if err := db.createCBLImportEntriesTable(); err != nil {
		return err
	}

	// Library metadata table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS library_metadata (
			key TEXT PRIMARY KEY,
			value TEXT
		)
	`)
	if err != nil {
		return fmt.Errorf("create library_metadata table: %w", err)
	}

	return nil
}
