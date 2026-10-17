package backup

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func countBooks(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM books`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// restoreFixture takes a backup of a 5-book library, then changes the live
// library to 2 books. It returns the backup folder, backup name and live path.
func restoreFixture(t *testing.T) (bdir, name, livePath string) {
	t.Helper()
	bdir = t.TempDir()
	opt, db := testOptions(t, bdir)
	m, name, err := Run(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	if m.SchemaVersions["library.db"] != 7 {
		t.Fatalf("schema = %v", m.SchemaVersions)
	}
	livePath = opt.Sources[0].Path
	if _, err := db.Exec(`DELETE FROM books WHERE id >= 2`); err != nil {
		t.Fatal(err)
	}
	db.Close() // checkpoint, like a stopped server
	return bdir, name, livePath
}

func TestRestore_SwapsFileAndTakesSafetyBackup(t *testing.T) {
	bdir, name, live := restoreFixture(t)
	// A stale WAL left by a crashed server must not be replayed.
	if err := os.WriteFile(live+"-wal", []byte("junk"), 0o600); err != nil {
		t.Fatal(err)
	}
	targets := []RestoreTarget{{File: "library.db", Path: live, MaxSchema: 7}}
	plan, err := ApplyRestore(context.Background(), bdir, name, targets, "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if got := countBooks(t, live); got != 5 {
		t.Fatalf("books after restore = %d, want 5", got)
	}
	if _, err := os.Stat(live + "-wal"); err == nil {
		t.Fatal("stale -wal survived")
	}
	if _, err := os.Stat(live + stagedSuffix); err == nil {
		t.Fatal("staged file left behind")
	}
	if plan.PreRestore == "" {
		t.Fatal("no safety backup")
	}
	if err := Verify(bdir, plan.PreRestore); err != nil {
		t.Fatal(err)
	}
	// The safety backup holds the 2-book state that was replaced.
	if got := countBooks(t, filepath.Join(bdir, plan.PreRestore, "library.db")); got != 2 {
		t.Fatalf("safety backup books = %d, want 2", got)
	}
}

func TestRestore_RefusesNewerSchemaAndTouchesNothing(t *testing.T) {
	bdir, name, live := restoreFixture(t)
	before, _ := os.ReadFile(live)
	_, err := ApplyRestore(context.Background(), bdir, name,
		[]RestoreTarget{{File: "library.db", Path: live, MaxSchema: 6}}, "9.9.9")
	if err == nil || !strings.Contains(err.Error(), "schema 7") {
		t.Fatalf("err = %v", err)
	}
	after, _ := os.ReadFile(live)
	if string(before) != string(after) {
		t.Fatal("live file changed")
	}
	entries, _ := List(bdir)
	if len(entries) != 1 {
		t.Fatalf("a safety backup was made for a refused restore: %d backups", len(entries))
	}
}

func TestRestore_RefusesDamagedBackup(t *testing.T) {
	bdir, name, live := restoreFixture(t)
	if err := os.WriteFile(filepath.Join(bdir, name, "library.db"), []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := PlanRestore(bdir, name, []RestoreTarget{{File: "library.db", Path: live, MaxSchema: 7}})
	if err == nil || !strings.Contains(err.Error(), "damaged") {
		t.Fatalf("err = %v", err)
	}
}

func TestRestore_RejectsBadNames(t *testing.T) {
	for _, n := range []string{"", "../x", "backup-1/../../x", "other"} {
		if _, err := PlanRestore(t.TempDir(), n, nil); err == nil {
			t.Errorf("name %q accepted", n)
		}
	}
}

func TestRestore_ConfigYAMLOnlyWhenAsked(t *testing.T) {
	bdir := t.TempDir()
	opt, db := testOptions(t, bdir)
	_, name, err := Run(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	yaml := opt.Extras[0].Path
	if err := os.WriteFile(yaml, []byte("changed: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Database only: the yaml must stay as it is.
	if _, err := ApplyRestore(context.Background(), bdir, name,
		[]RestoreTarget{{File: "library.db", Path: opt.Sources[0].Path, MaxSchema: 7}}, "9.9.9"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(yaml); string(b) != "changed: true\n" {
		t.Fatalf("config.yaml was restored without being asked: %q", b)
	}
	// Asked for: it comes back.
	if _, err := ApplyRestore(context.Background(), bdir, name,
		[]RestoreTarget{{File: "config.yaml", Path: yaml}}, "9.9.9"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(yaml); string(b) != "server: {}\n" {
		t.Fatalf("config.yaml = %q", b)
	}
}

func TestRestore_KeepsNamedTablesFromLiveFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.db")
	open := func() *sql.DB {
		db, err := sql.Open("sqlite", "file:"+path+"?_journal_mode=WAL&_busy_timeout=5000")
		if err != nil {
			t.Fatal(err)
		}
		return db
	}
	db := open()
	for _, q := range []string{
		`CREATE TABLE devices (id TEXT)`,
		`CREATE TABLE backup_settings (id INTEGER PRIMARY KEY, dir TEXT)`,
		`INSERT INTO devices VALUES ('from-backup')`,
		`INSERT INTO backup_settings VALUES (1, '/old/folder')`,
		`PRAGMA user_version = 3`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	bdir := t.TempDir()
	_, name, err := Run(context.Background(), Options{Dir: bdir, Sources: []Source{{
		File: "config.db", Path: path, Snapshot: func(dest string) error { return VacuumInto(db, dest) }}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`UPDATE devices SET id = 'live-now'`,
		`UPDATE backup_settings SET dir = '/new/folder'`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	if _, err := ApplyRestore(context.Background(), bdir, name, []RestoreTarget{{
		File: "config.db", Path: path, MaxSchema: 3, KeepTables: []string{"backup_settings", "backup_runs"}}}, "9.9.9"); err != nil {
		t.Fatal(err)
	}
	db = open()
	defer db.Close()
	var dev, folder string
	if err := db.QueryRow(`SELECT id FROM devices`).Scan(&dev); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT dir FROM backup_settings`).Scan(&folder); err != nil {
		t.Fatal(err)
	}
	if dev != "from-backup" {
		t.Errorf("devices = %q, want the backup's value", dev)
	}
	if folder != "/new/folder" {
		t.Errorf("backup_settings.dir = %q, want the live value", folder)
	}
}
