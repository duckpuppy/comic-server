package configdb

import (
	"path/filepath"
	"testing"
	"time"
)

func TestBackupSettingsAndRuns(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	s, err := db.GetBackupSettings()
	if err != nil || s.Enabled || s.Mode != "daily" || s.At != "03:30" || s.Keep != 7 || !s.IncludeCV {
		t.Fatalf("defaults = %+v, %v", s, err)
	}
	s.Enabled, s.Dir, s.Keep = true, "/backups", 3
	if err := db.PutBackupSettings(s); err != nil {
		t.Fatal(err)
	}
	got, _ := db.GetBackupSettings()
	if got != s {
		t.Errorf("saved %+v, got %+v", s, got)
	}

	base := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)
	for i := 0; i < backupRunsKept+5; i++ {
		if err := db.AddBackupRun(BackupRun{StartedAt: base, FinishedAt: base, Trigger: "schedule", OK: i%2 == 0, Name: "backup-x", Size: int64(i)}); err != nil {
			t.Fatal(err)
		}
	}
	runs, err := db.ListBackupRuns(1000)
	if err != nil || len(runs) != backupRunsKept {
		t.Fatalf("runs = %d, %v; want %d", len(runs), err, backupRunsKept)
	}
	if runs[0].Size != int64(backupRunsKept+4) || !runs[0].StartedAt.Equal(base) {
		t.Errorf("newest first expected, got %+v", runs[0])
	}
}
