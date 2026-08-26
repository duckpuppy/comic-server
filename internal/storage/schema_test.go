package storage

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestMigrateV2ToV3_AddsSoftDeleteColumns simulates an existing database
// at schema v2 (pre comic-server-b53) and confirms opening it upgrades in
// place: deleted_at columns get added to both tables, existing rows are
// untouched (NULL deleted_at, i.e. not deleted), and the schema version
// advances so the migration doesn't re-run.
func TestMigrateV2ToV3_AddsSoftDeleteColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// Build a v2-shaped database by hand: same createTables DDL, minus the
	// deleted_at columns/indexes that only exist from v3 onward, with
	// PRAGMA user_version pinned to 2 rather than left to Open (which
	// always creates the current schema fresh).
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	// Real v1+v2 column set (schema.go's createTables/migrateV1ToV2), minus
	// only the v3 deleted_at column this migration adds - an authentic v2
	// fixture, not a stand-in subset, so scanBook's full column list
	// (booksSelectColumns) works against it unmodified.
	v2Books := `
		CREATE TABLE books (
			id TEXT PRIMARY KEY,
			file_path TEXT NOT NULL,
			title TEXT, series TEXT, number TEXT, volume INTEGER, year INTEGER, month INTEGER, day INTEGER,
			publisher TEXT, imprint TEXT, genre TEXT, format TEXT, age_rating TEXT, language_iso TEXT,
			summary TEXT, notes TEXT, review TEXT, story_arc TEXT, series_group TEXT,
			alternate_series TEXT, alternate_number TEXT, alternate_count INTEGER, count INTEGER,
			writer TEXT, penciller TEXT, inker TEXT, colorist TEXT, letterer TEXT, cover_artist TEXT, editor TEXT, translator TEXT,
			characters TEXT, teams TEXT, locations TEXT, main_character_or_team TEXT,
			current_page INTEGER DEFAULT 0, last_page INTEGER DEFAULT 0, last_page_read INTEGER DEFAULT 0,
			open_count INTEGER DEFAULT 0, opened_time TEXT,
			rating REAL DEFAULT 0, community_rating REAL DEFAULT 0,
			checked INTEGER DEFAULT 0, file_is_missing INTEGER DEFAULT 0, comic_info_is_dirty INTEGER DEFAULT 0,
			page_count INTEGER DEFAULT 0, web TEXT, scan_information TEXT,
			series_complete TEXT, black_and_white TEXT, manga TEXT,
			preferred_front_cover INTEGER DEFAULT 0, added_time TEXT, released_time TEXT,
			file_size INTEGER DEFAULT 0, file_modified_time TEXT, file_creation_time TEXT,
			isbn TEXT, book_age TEXT, book_condition TEXT, book_store TEXT, book_owner TEXT,
			book_collection_status TEXT, book_notes TEXT, book_location TEXT,
			book_price REAL DEFAULT -1, new_pages INTEGER DEFAULT 0,
			enable_proposed INTEGER DEFAULT 0, enable_dynamic_update INTEGER DEFAULT 0, last_opened_from_list_id TEXT,
			pages TEXT,
			import_hash TEXT, updated_at TEXT
		)
	`
	v2Lists := `
		CREATE TABLE lists (
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
	`
	v2CustomValues := `
		CREATE TABLE book_custom_values (
			book_id TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
			key TEXT NOT NULL,
			value TEXT,
			PRIMARY KEY (book_id, key)
		)
	`
	v2Tags := `
		CREATE TABLE book_tags (
			book_id TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
			tag TEXT NOT NULL,
			PRIMARY KEY (book_id, tag)
		)
	`
	v2ReadingListItems := `
		CREATE TABLE reading_list_items (
			list_id TEXT NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
			book_id TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
			position INTEGER,
			PRIMARY KEY (list_id, book_id)
		)
	`
	for _, stmt := range []string{v2Books, v2Lists, v2CustomValues, v2Tags, v2ReadingListItems, "PRAGMA user_version = 2"} {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	// Seed via a raw INSERT matching the v2 fixture's own column list -
	// can't reuse production insertBook here since it now also targets
	// xml_snapshot (comic-server-aio, schema v4), which doesn't exist on
	// this deliberately-v2-shaped table yet. Every TEXT column needs an
	// explicit value (not omitted) since scanBook can't handle NULL in a
	// non-nullable Go string field - real inserts always supply every
	// column, this fixture needs to match that, not just the DDL.
	if _, err := raw.Exec(`
		INSERT INTO books (
			id, file_path, title, series, number, volume, year, month, day,
			alternate_series, alternate_number, alternate_count, count,
			publisher, imprint, genre, format, age_rating, language_iso,
			summary, notes, review, story_arc, series_group,
			writer, penciller, inker, colorist, letterer, cover_artist, editor, translator,
			characters, teams, locations, main_character_or_team,
			opened_time, web, scan_information, series_complete, black_and_white, manga,
			preferred_front_cover, added_time, released_time, file_modified_time, file_creation_time,
			isbn, book_age, book_condition, book_store, book_owner,
			book_collection_status, book_notes, book_location, last_opened_from_list_id,
			pages, import_hash, updated_at
		) VALUES (
			?, ?, ?, '', '', 0, 0, 0, 0,
			'', '', 0, 0,
			'', '', '', '', '', '',
			'', '', '', '', '',
			'', '', '', '', '', '', '', '',
			'', '', '', '',
			'', '', '', '', '', '',
			0, '', '', '', '',
			'', '', '', '', '',
			'', '', '', '',
			'[]', '', datetime('now')
		)
	`, "book-1", "/comics/a.cbz", "A"); err != nil {
		t.Fatalf("seed book: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	// Now open it through the real path - this must run migrateV2ToV3.
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open (should migrate v2->v3): %v", err)
	}
	defer db.Close()

	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("get schema version: %v", err)
	}
	if version != schemaVersion {
		t.Errorf("schema version = %d, want %d", version, schemaVersion)
	}

	// The pre-existing book must survive the migration, untouched
	// (visible, not accidentally marked deleted).
	book, err := db.GetBook("book-1")
	if err != nil || book == nil {
		t.Fatalf("GetBook(book-1) after migration: book=%+v err=%v", book, err)
	}
	if book.Title != "A" {
		t.Errorf("migrated book has wrong content: %+v", book)
	}

	var deletedAt sql.NullString
	if err := db.QueryRow("SELECT deleted_at FROM books WHERE id = ?", "book-1").Scan(&deletedAt); err != nil {
		t.Fatalf("query deleted_at: %v", err)
	}
	if deletedAt.Valid {
		t.Errorf("expected pre-existing row's deleted_at to be NULL after migration, got %q", deletedAt.String)
	}

	// Soft-delete now actually works post-migration (deleted_at column and
	// its index exist and are usable, not just present).
	if _, err := db.Exec("UPDATE books SET deleted_at = datetime('now') WHERE id = ?", "book-1"); err != nil {
		t.Fatalf("soft-delete after migration: %v", err)
	}
	if book, err := db.GetBook("book-1"); err != nil || book != nil {
		t.Errorf("expected soft-deleted book to be invisible, got book=%+v err=%v", book, err)
	}
}
