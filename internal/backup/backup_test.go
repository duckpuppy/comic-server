package backup

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// liveDB makes a WAL database with n rows and an open writer connection, the
// way the server holds its databases.
func liveDB(t *testing.T, dir, name string, n int) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(dir, name)
	db, err := sql.Open("sqlite", "file:"+path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE books (id INTEGER PRIMARY KEY, deleted_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		if _, err := db.Exec(`INSERT INTO books (id) VALUES (?)`, i); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`PRAGMA user_version = 7`); err != nil {
		t.Fatal(err)
	}
	return db, path
}

func testOptions(t *testing.T, bdir string) (Options, *sql.DB) {
	t.Helper()
	src := t.TempDir()
	db, path := liveDB(t, src, "library.db", 5)
	yaml := filepath.Join(src, "config.yaml")
	if err := os.WriteFile(yaml, []byte("server: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return Options{
		Dir: bdir, Keep: 3, AppVersion: "9.9.9",
		Sources: []Source{{File: "library.db", Path: path,
			Snapshot: func(dest string) error { return VacuumInto(db, dest) },
			Count:    CountRows(`SELECT COUNT(*) FROM books WHERE deleted_at IS NULL`)}},
		Extras: []ExtraFile{{File: "config.yaml", Path: yaml}},
	}, db
}

func TestRun_ConsistentOpenableAndPrivate(t *testing.T) {
	bdir := filepath.Join(t.TempDir(), "backups")
	opt, db := testOptions(t, bdir)
	m, name, err := Run(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, NamePrefix) || m.AppVersion != "9.9.9" || m.BookCount != 5 || m.SchemaVersions["library.db"] != 7 || len(m.Files) != 2 {
		t.Fatalf("name=%q manifest=%+v", name, m)
	}
	// The backup is a snapshot: later writes to the live DB do not change it.
	if _, err := db.Exec(`INSERT INTO books (id) VALUES (100)`); err != nil {
		t.Fatal(err)
	}
	if n, err := CountRows(`SELECT COUNT(*) FROM books`)(filepath.Join(bdir, name, "library.db")); err != nil || n != 5 {
		t.Errorf("snapshot rows = %d, %v", n, err)
	}
	if err := Verify(bdir, name); err != nil {
		t.Errorf("verify: %v", err)
	}
	// Private: nothing is group or world readable.
	for _, p := range []string{bdir, filepath.Join(bdir, name), filepath.Join(bdir, name, "library.db"), filepath.Join(bdir, name, "config.yaml"), filepath.Join(bdir, name, ManifestName)} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm()&0o077 != 0 {
			t.Errorf("%s mode %v is not private", p, fi.Mode().Perm())
		}
	}
	// No ".partial-" leftovers.
	des, _ := os.ReadDir(bdir)
	for _, de := range des {
		if strings.HasPrefix(de.Name(), partialPrefix) {
			t.Errorf("leftover %s", de.Name())
		}
	}
}

func TestVerify_DetectsChange(t *testing.T) {
	bdir := t.TempDir()
	opt, _ := testOptions(t, bdir)
	_, name, err := Run(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bdir, name, "config.yaml"), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Verify(bdir, name); err == nil {
		t.Error("tampered file passed verification")
	}
}

func TestRun_FailureLeavesNothingBehind(t *testing.T) {
	bdir := t.TempDir()
	opt, _ := testOptions(t, bdir)
	opt.Sources = append(opt.Sources, Source{File: "bad.db", Snapshot: func(string) error { return os.ErrPermission }})
	if _, _, err := Run(context.Background(), opt); err == nil {
		t.Fatal("expected an error")
	}
	des, _ := os.ReadDir(bdir)
	if len(des) != 0 {
		t.Errorf("failed run left %d entries", len(des))
	}
}

func TestRun_NeedsAbsoluteDirAndSources(t *testing.T) {
	if _, _, err := Run(context.Background(), Options{Dir: "relative", Sources: []Source{{}}}); err == nil {
		t.Error("relative dir accepted")
	}
	if _, _, err := Run(context.Background(), Options{Dir: t.TempDir()}); err == nil {
		t.Error("empty backup accepted")
	}
}

func TestRetention_KeepsNewestAndIgnoresOthers(t *testing.T) {
	bdir := t.TempDir()
	opt, _ := testOptions(t, bdir)
	opt.Keep = 2
	if err := os.Mkdir(filepath.Join(bdir, "photos"), 0o700); err != nil { // not ours
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(bdir, NamePrefix+"nomanifest"), 0o700); err != nil { // not complete
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)
	var names []string
	for i := 0; i < 4; i++ {
		i := i
		opt.Now = func() time.Time { return base.AddDate(0, 0, i) }
		_, name, err := Run(context.Background(), opt)
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	got, _ := List(bdir)
	if len(got) != 2 || got[0].Name != names[3] || got[1].Name != names[2] {
		t.Fatalf("kept %+v, want the two newest of %v", got, names)
	}
	for _, keep := range []string{"photos", NamePrefix + "nomanifest"} {
		if _, err := os.Stat(filepath.Join(bdir, keep)); err != nil {
			t.Errorf("prune removed %s", keep)
		}
	}
}

func TestRun_SameSecondGetsDistinctName(t *testing.T) {
	bdir := t.TempDir()
	opt, _ := testOptions(t, bdir)
	opt.Now = func() time.Time { return time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC) }
	_, a, _ := Run(context.Background(), opt)
	_, b, err := Run(context.Background(), opt)
	if err != nil || a == b {
		t.Errorf("names %q %q err %v", a, b, err)
	}
}

func TestSchedule(t *testing.T) {
	loc := time.UTC
	at := func(d, h, m int) time.Time { return time.Date(2026, 10, d, h, m, 0, 0, loc) }
	daily := Schedule{Mode: "daily", At: "03:30"}
	cases := []struct {
		name     string
		s        Schedule
		last     time.Time
		now      time.Time
		wantDue  bool
		wantNext time.Time
	}{
		{"daily never ran, after slot", daily, time.Time{}, at(5, 10, 0), true, at(6, 3, 30)},
		{"daily ran today after slot", daily, at(5, 3, 31), at(5, 10, 0), false, at(6, 3, 30)},
		{"daily ran yesterday, slot passed", daily, at(4, 3, 31), at(5, 4, 0), true, at(6, 3, 30)},
		{"daily ran yesterday, slot not yet", daily, at(4, 3, 31), at(5, 3, 0), false, at(5, 3, 30)},
		{"interval never ran", Schedule{Mode: "interval", EveryHours: 6}, time.Time{}, at(5, 10, 0), true, at(5, 16, 0)},
		{"interval not yet", Schedule{Mode: "interval", EveryHours: 6}, at(5, 8, 0), at(5, 10, 0), false, at(5, 14, 0)},
		{"interval due", Schedule{Mode: "interval", EveryHours: 6}, at(5, 3, 0), at(5, 10, 0), true, at(5, 16, 0)},
	}
	for _, c := range cases {
		due, next := c.s.NextRun(c.last, c.now)
		if due != c.wantDue || !next.Equal(c.wantNext) {
			t.Errorf("%s: due=%v next=%v, want %v %v", c.name, due, next, c.wantDue, c.wantNext)
		}
	}
	for _, bad := range []Schedule{{Mode: "x"}, {Mode: "daily", At: "25:00"}, {Mode: "interval", EveryHours: 0}} {
		if bad.Validate() == nil {
			t.Errorf("%+v validated", bad)
		}
	}
}

func TestDelete_OnlyCompleteBackups(t *testing.T) {
	dir := t.TempDir()
	mk := func(name string, manifest bool) {
		_ = os.MkdirAll(filepath.Join(dir, name), 0o700)
		if manifest {
			_ = os.WriteFile(filepath.Join(dir, name, ManifestName), []byte(`{"created_at":"2026-01-01T00:00:00Z"}`), 0o600)
		}
	}
	mk(NamePrefix+"20260101-000000", true)
	mk(NamePrefix+"20260102-000000", false) // no manifest: not a complete backup
	mk(".partial-x", true)
	mk("other", true)
	for _, bad := range []string{NamePrefix + "20260102-000000", ".partial-x", "other", "../" + NamePrefix + "x", ""} {
		if err := Delete(dir, bad); !errors.Is(err, ErrNotABackup) {
			t.Errorf("Delete(%q) = %v, want ErrNotABackup", bad, err)
		}
	}
	for _, keep := range []string{NamePrefix + "20260102-000000", ".partial-x", "other"} {
		if _, err := os.Stat(filepath.Join(dir, keep)); err != nil {
			t.Errorf("%s was removed", keep)
		}
	}
	if err := Delete(dir, NamePrefix+"20260101-000000"); err != nil {
		t.Fatal(err)
	}
	if got, _ := List(dir); len(got) != 0 {
		t.Errorf("backup still listed: %d", len(got))
	}
}
