package storage

import "fmt"

// Schema version for migrations
const schemaVersion = 1

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

	// Create tables
	if err := db.createTables(); err != nil {
		return err
	}

	// Update schema version
	_, err = db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion))
	if err != nil {
		return fmt.Errorf("set schema version: %w", err)
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

			-- Dynamic features
			enable_proposed INTEGER DEFAULT 0,
			enable_dynamic_update INTEGER DEFAULT 0,
			last_opened_from_list_id TEXT,

			-- Pages stored as JSON array
			pages TEXT,

			-- Import tracking
			import_hash TEXT,
			updated_at TEXT
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
			book_count INTEGER DEFAULT 0,
			import_hash TEXT,
			updated_at TEXT
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
