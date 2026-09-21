package storage

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestMigrateV5ToV6_AddsCBLImportSupport simulates an existing database
// at schema v5 (pre comic-server-tnv4) and confirms opening it upgrades
// in place: the three cbl_* columns are added to `lists`, the new
// `cbl_import_entries` table is created, and a pre-existing (non-CBL)
// list is untouched with its new cbl_* columns NULL.
func TestMigrateV5ToV6_AddsCBLImportSupport(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	// Only `lists` needs to exist in its real v5 shape (the only table
	// migrateV5ToV6 touches with ALTER) - Open() only runs initSchema,
	// no other table is a hard dependency for this migration to run.
	v5Lists := `
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
			base_list_id TEXT,
			book_count INTEGER DEFAULT 0,
			import_hash TEXT,
			updated_at TEXT,
			deleted_at TEXT
		)
	`
	// cbl_import_entries.book_id references books(id) - SQLite needs the
	// referenced table to exist even for a NULL FK value, so a minimal
	// stand-in is required here even though this migration doesn't
	// touch books itself.
	v5Books := `CREATE TABLE books (id TEXT PRIMARY KEY)`
	for _, stmt := range []string{v5Lists, v5Books, "PRAGMA user_version = 5"} {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	if _, err := raw.Exec(`
		INSERT INTO lists (id, name, type, book_count, updated_at)
		VALUES ('list-1', 'Existing List', 'ComicListItem', 3, datetime('now'))
	`); err != nil {
		t.Fatalf("seed list: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open (should migrate v5->v6): %v", err)
	}
	defer db.Close()

	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("get schema version: %v", err)
	}
	if version != schemaVersion {
		t.Errorf("schema version = %d, want %d", version, schemaVersion)
	}

	// Pre-existing list survives, and its new cbl_* columns are NULL -
	// this list was never CBL-imported.
	var name string
	var cblSource, cblSourceRef, cblImportedAt sql.NullString
	err = db.QueryRow(`SELECT name, cbl_source, cbl_source_ref, cbl_imported_at FROM lists WHERE id = ?`, "list-1").
		Scan(&name, &cblSource, &cblSourceRef, &cblImportedAt)
	if err != nil {
		t.Fatalf("query migrated list: %v", err)
	}
	if name != "Existing List" {
		t.Errorf("name = %q, want %q (list content must survive migration)", name, "Existing List")
	}
	if cblSource.Valid || cblSourceRef.Valid || cblImportedAt.Valid {
		t.Errorf("expected all cbl_* columns NULL for a non-imported list, got source=%v ref=%v imported_at=%v",
			cblSource, cblSourceRef, cblImportedAt)
	}

	// cbl_import_entries table exists and is usable post-migration.
	if _, err := db.Exec(`
		INSERT INTO cbl_import_entries (id, list_id, book_id, position, match_path, series, number, volume, year, format, cv_issue_id)
		VALUES ('entry-1', 'list-1', NULL, 0, 'none', 'Test Series', '1', 2020, 2020, '', 12345)
	`); err != nil {
		t.Fatalf("insert into cbl_import_entries post-migration: %v", err)
	}
}

// TestFreshDB_HasCBLColumnsAndTable confirms a brand-new database
// (createTables path, not the migration path) also has the cbl_*
// columns on `lists` and the cbl_import_entries table - the two
// createTables/migration code paths must stay in sync.
func TestFreshDB_HasCBLColumnsAndTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open fresh db: %v", err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT name FROM pragma_table_info('lists') WHERE name LIKE 'cbl_%'`)
	if err != nil {
		t.Fatalf("query lists columns: %v", err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan column name: %v", err)
		}
		got[name] = true
	}
	for _, want := range []string{"cbl_source", "cbl_source_ref", "cbl_imported_at"} {
		if !got[want] {
			t.Errorf("fresh lists table missing column %q", want)
		}
	}

	var tableName string
	err = db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='cbl_import_entries'`).Scan(&tableName)
	if err != nil {
		t.Fatalf("cbl_import_entries table missing from fresh db: %v", err)
	}

	hasWantedBookID, err := db.hasColumn("cbl_import_entries", "wanted_book_id")
	if err != nil {
		t.Fatalf("check wanted_book_id column: %v", err)
	}
	if !hasWantedBookID {
		t.Error("fresh cbl_import_entries table missing wanted_book_id column")
	}
}

// TestMigrateV6ToV7_AddsWantedBookIDColumn simulates a database already
// sitting at schema v6 (comic-server-tnv4 shipped, comic-server-sx2d not
// yet) - cbl_import_entries exists but without wanted_book_id - and
// confirms opening it adds the column via the real ALTER path (not the
// "already had it from v5->v6" skip path TestMigrateV5ToV6 exercises).
func TestMigrateV6ToV7_AddsWantedBookIDColumn(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	v6Lists := `
		CREATE TABLE lists (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, type TEXT NOT NULL,
			parent_id TEXT, description TEXT, favorite INTEGER DEFAULT 0,
			collapsed INTEGER DEFAULT 0, matcher_mode TEXT, matchers TEXT,
			base_list_id TEXT, book_count INTEGER DEFAULT 0, import_hash TEXT,
			updated_at TEXT, deleted_at TEXT,
			cbl_source TEXT, cbl_source_ref TEXT, cbl_imported_at TEXT
		)
	`
	v6Books := `CREATE TABLE books (id TEXT PRIMARY KEY)`
	// v6 shape: no wanted_book_id yet (comic-server-sx2d hasn't shipped).
	v6CBLImportEntries := `
		CREATE TABLE cbl_import_entries (
			id TEXT PRIMARY KEY, list_id TEXT NOT NULL, book_id TEXT,
			position INTEGER NOT NULL, match_path TEXT NOT NULL,
			series TEXT NOT NULL, number TEXT NOT NULL, volume INTEGER,
			year INTEGER, format TEXT, cv_issue_id INTEGER
		)
	`
	for _, stmt := range []string{v6Lists, v6Books, v6CBLImportEntries, "PRAGMA user_version = 6"} {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	if _, err := raw.Exec(`
		INSERT INTO cbl_import_entries (id, list_id, position, match_path, series, number)
		VALUES ('entry-1', 'list-1', 0, 'none', 'Old Series', '1')
	`); err != nil {
		t.Fatalf("seed entry: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open (should migrate v6->v7): %v", err)
	}
	defer db.Close()

	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("get schema version: %v", err)
	}
	if version != schemaVersion {
		t.Errorf("schema version = %d, want %d", version, schemaVersion)
	}

	// The pre-existing entry survives, and its new wanted_book_id is NULL.
	var series string
	var wantedBookID sql.NullString
	err = db.QueryRow(`SELECT series, wanted_book_id FROM cbl_import_entries WHERE id = ?`, "entry-1").Scan(&series, &wantedBookID)
	if err != nil {
		t.Fatalf("query migrated entry: %v", err)
	}
	if series != "Old Series" {
		t.Errorf("series = %q, want %q (entry content must survive migration)", series, "Old Series")
	}
	if wantedBookID.Valid {
		t.Errorf("expected wanted_book_id NULL on a pre-migration entry, got %q", wantedBookID.String)
	}

	if err := db.MarkCBLImportEntryWanted("entry-1", "wanted-1"); err != nil {
		t.Fatalf("MarkCBLImportEntryWanted post-migration: %v", err)
	}
}
