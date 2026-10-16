package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/backup"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
)

func backupServer(t *testing.T) *Server {
	t.Helper()
	return newDuplicatesTestServer(t, t.TempDir(), []library.ComicBook{{ID: "b1", Series: "S", FilePath: "a.cbz"}})
}

func putBackupSettings(t *testing.T, s *Server, body string) (int, BackupSettingsResponse, string) {
	t.Helper()
	w := httptest.NewRecorder()
	s.handleBackupSettings(w, httptest.NewRequest(http.MethodPut, "/api/settings/backups", strings.NewReader(body)))
	var resp BackupSettingsResponse
	if w.Code == http.StatusOK {
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
	}
	return w.Code, resp, w.Body.String()
}

func TestBackupSettings_DefaultsAndValidation(t *testing.T) {
	s := backupServer(t)
	w := httptest.NewRecorder()
	s.handleBackupSettings(w, httptest.NewRequest(http.MethodGet, "/api/settings/backups", nil))
	var def BackupSettingsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &def); err != nil || w.Code != 200 {
		t.Fatalf("GET: %d %v", w.Code, err)
	}
	if def.Settings.Enabled || def.Settings.Keep != 7 || len(def.Includes) == 0 || def.Includes[0] != "config.db" {
		t.Errorf("defaults = %+v", def)
	}

	dir := filepath.Join(t.TempDir(), "bk")
	bad := []string{
		`{"enabled":true,"dir":"","mode":"daily","at":"03:30","every_hours":24,"keep":7}`,
		`{"enabled":true,"dir":"relative/path","mode":"daily","at":"03:30","every_hours":24,"keep":7}`,
		`{"enabled":true,"dir":"` + dir + `","mode":"daily","at":"3pm","every_hours":24,"keep":7}`,
		`{"enabled":true,"dir":"` + dir + `","mode":"interval","at":"03:30","every_hours":0,"keep":7}`,
		`{"enabled":true,"dir":"` + dir + `","mode":"daily","at":"03:30","every_hours":24,"keep":0}`,
	}
	for _, b := range bad {
		if code, _, _ := putBackupSettings(t, s, b); code != http.StatusBadRequest {
			t.Errorf("accepted %s (status %d)", b, code)
		}
	}
	code, resp, body := putBackupSettings(t, s, `{"enabled":true,"dir":"`+dir+`","mode":"daily","at":"04:15","every_hours":24,"keep":5,"include_cv_cache":true,"include_config_yaml":true}`)
	if code != 200 || resp.Settings.At != "04:15" || resp.Status.NextRun == nil {
		t.Fatalf("PUT: %d %s", code, body)
	}
	fi, err := os.Stat(dir)
	if err != nil || fi.Mode().Perm()&0o077 != 0 {
		t.Errorf("backup folder = %v, %v; want private", fi, err)
	}
}

func TestBackup_RunNowListsAndRecords(t *testing.T) {
	s := backupServer(t)
	dir := filepath.Join(t.TempDir(), "bk")
	w := httptest.NewRecorder()
	s.handleBackupRun(w, httptest.NewRequest(http.MethodPost, "/api/backups/run", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("run with no folder: %d", w.Code)
	}
	if code, _, body := putBackupSettings(t, s, `{"enabled":false,"dir":"`+dir+`","mode":"daily","at":"03:30","every_hours":24,"keep":3,"include_cv_cache":true,"include_config_yaml":true}`); code != 200 {
		t.Fatalf("PUT: %d %s", code, body)
	}

	if err := s.runBackup(context.Background(), "manual"); err != nil {
		t.Fatal(err)
	}
	entries, _ := backup.List(dir)
	if len(entries) != 1 {
		t.Fatalf("backups = %d", len(entries))
	}
	if name := entries[0].Name; backup.Verify(dir, name) != nil {
		t.Errorf("backup does not verify")
	}
	runs, _ := s.configDB.ListBackupRuns(5)
	if len(runs) != 1 || !runs[0].OK || runs[0].Trigger != "manual" || runs[0].Name != entries[0].Name {
		t.Errorf("runs = %+v", runs)
	}

	w = httptest.NewRecorder()
	s.handleBackupList(w, httptest.NewRequest(http.MethodGet, "/api/backups", nil))
	var list struct {
		Backups []struct{ Name string } `json:"backups"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Backups) != 1 {
		t.Errorf("list = %s", w.Body.String())
	}
}

func TestBackup_OneAtATimeAndFailureRecorded(t *testing.T) {
	s := backupServer(t)
	s.backupState.running = true
	w := httptest.NewRecorder()
	if err := os.MkdirAll(t.TempDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	_ = s.configDB.PutBackupSettings(configdb.BackupSettings{Dir: t.TempDir(), Mode: "daily", At: "03:30", EveryHours: 24, Keep: 3})
	s.handleBackupRun(w, httptest.NewRequest(http.MethodPost, "/api/backups/run", nil))
	if w.Code != http.StatusConflict {
		t.Errorf("second run: %d", w.Code)
	}
	s.backupState.running = false

	// A folder that cannot be a directory: the run fails, is recorded, and
	// the schedule backs off.
	file := filepath.Join(t.TempDir(), "afile")
	_ = os.WriteFile(file, []byte("x"), 0o600)
	_ = s.configDB.PutBackupSettings(configdb.BackupSettings{Dir: filepath.Join(file, "sub"), Mode: "daily", At: "03:30", EveryHours: 24, Keep: 3})
	if err := s.runBackup(context.Background(), "schedule"); err == nil {
		t.Fatal("expected a failure")
	}
	runs, _ := s.configDB.ListBackupRuns(5)
	if len(runs) != 1 || runs[0].OK || runs[0].Error == "" {
		t.Errorf("failure not recorded: %+v", runs)
	}
	if s.backupState.lastFailure.IsZero() {
		t.Error("no back-off after failure")
	}
}

func TestBackup_TickRunsWhenDueAndBacksOffAfterFailure(t *testing.T) {
	s := backupServer(t)
	dir := filepath.Join(t.TempDir(), "bk")
	_ = s.configDB.PutBackupSettings(configdb.BackupSettings{Enabled: true, Dir: dir, Mode: "interval", At: "03:30", EveryHours: 1, Keep: 3, IncludeConfig: true})

	s.backupTick(context.Background()) // never ran: due at once
	if e, _ := backup.List(dir); len(e) != 1 {
		t.Fatalf("first tick made %d backups", len(e))
	}
	s.backupTick(context.Background()) // an hour has not passed
	if e, _ := backup.List(dir); len(e) != 1 {
		t.Errorf("second tick made another backup (%d)", len(e))
	}

	// Disabled settings never run, however stale.
	_ = s.configDB.PutBackupSettings(configdb.BackupSettings{Enabled: false, Dir: dir, Mode: "interval", EveryHours: 1, Keep: 3})
	_, _ = s.configDB.Exec(`UPDATE backup_runs SET started_at = ?`, time.Now().Add(-5*time.Hour).UTC().Format(time.RFC3339))
	s.backupTick(context.Background())
	if e, _ := backup.List(dir); len(e) != 1 {
		t.Errorf("disabled backups ran (%d)", len(e))
	}
}
