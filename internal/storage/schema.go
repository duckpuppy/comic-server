package storage

import "fmt"

// Schema version for migrations
const schemaVersion = 5

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
			xml_snapshot TEXT
		)
	`)
	if err != nil {
		return fmt.Errorf("create books table: %w", err)
	}

	// Indexes for common queries (smart list filtering)
	indexes := []string{
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
			deleted_at TEXT
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
