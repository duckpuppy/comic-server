package configdb

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/duckpuppy/comic-server/internal/backup"
)

// BackupSettings is the stored backup configuration (comic-server-yzdt),
// edited from Settings > Backups. A single row; defaults when none is saved.
type BackupSettings struct {
	Enabled       bool   `json:"enabled"`
	Dir           string `json:"dir"`
	Mode          string `json:"mode"` // "daily" or "interval"
	At            string `json:"at"`   // "HH:MM" for daily
	EveryHours    int    `json:"every_hours"`
	Keep          int    `json:"keep"`
	IncludeCV     bool   `json:"include_cv_cache"`
	IncludeConfig bool   `json:"include_config_yaml"`
}

// DefaultBackupSettings is what Settings shows before anything is saved:
// off, nightly at 03:30, keep a week.
func DefaultBackupSettings() BackupSettings {
	return BackupSettings{Mode: "daily", At: "03:30", EveryHours: 24, Keep: 7, IncludeCV: true, IncludeConfig: true}
}

// BackupRun is one attempt, kept for the status card.
type BackupRun struct {
	ID         int64     `json:"id"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Trigger    string    `json:"trigger"` // "schedule" or "manual"
	OK         bool      `json:"ok"`
	Name       string    `json:"name,omitempty"`
	Size       int64     `json:"size,omitempty"`
	Error      string    `json:"error,omitempty"`
}

const backupRunsKept = 50

// The tables are created on first use, so no schema version bump.
func (db *DB) ensureBackupTables() error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS backup_settings (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		enabled INTEGER NOT NULL DEFAULT 0,
		dir TEXT NOT NULL DEFAULT '',
		mode TEXT NOT NULL DEFAULT 'daily',
		at TEXT NOT NULL DEFAULT '03:30',
		every_hours INTEGER NOT NULL DEFAULT 24,
		keep INTEGER NOT NULL DEFAULT 7,
		include_cv INTEGER NOT NULL DEFAULT 1,
		include_config INTEGER NOT NULL DEFAULT 1
	)`); err != nil {
		return fmt.Errorf("create backup_settings: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS backup_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		started_at TEXT NOT NULL,
		finished_at TEXT NOT NULL,
		trigger TEXT NOT NULL,
		ok INTEGER NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		size INTEGER NOT NULL DEFAULT 0,
		error TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		return fmt.Errorf("create backup_runs: %w", err)
	}
	return nil
}

// GetBackupSettings returns the saved settings, or the defaults.
func (db *DB) GetBackupSettings() (BackupSettings, error) {
	if err := db.ensureBackupTables(); err != nil {
		return BackupSettings{}, err
	}
	var s BackupSettings
	err := db.QueryRow(`SELECT enabled, dir, mode, at, every_hours, keep, include_cv, include_config
		FROM backup_settings WHERE id = 1`).
		Scan(&s.Enabled, &s.Dir, &s.Mode, &s.At, &s.EveryHours, &s.Keep, &s.IncludeCV, &s.IncludeConfig)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultBackupSettings(), nil
	}
	if err != nil {
		return BackupSettings{}, fmt.Errorf("get backup settings: %w", err)
	}
	return s, nil
}

// PutBackupSettings replaces the saved settings.
func (db *DB) PutBackupSettings(s BackupSettings) error {
	if err := db.ensureBackupTables(); err != nil {
		return err
	}
	_, err := db.Exec(`INSERT INTO backup_settings (id, enabled, dir, mode, at, every_hours, keep, include_cv, include_config)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET enabled=excluded.enabled, dir=excluded.dir, mode=excluded.mode, at=excluded.at,
			every_hours=excluded.every_hours, keep=excluded.keep, include_cv=excluded.include_cv, include_config=excluded.include_config`,
		s.Enabled, s.Dir, s.Mode, s.At, s.EveryHours, s.Keep, s.IncludeCV, s.IncludeConfig)
	if err != nil {
		return fmt.Errorf("save backup settings: %w", err)
	}
	return nil
}

// AddBackupRun records an attempt and trims the history.
func (db *DB) AddBackupRun(r BackupRun) error {
	if err := db.ensureBackupTables(); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT INTO backup_runs (started_at, finished_at, trigger, ok, name, size, error) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.StartedAt.UTC().Format(time.RFC3339), r.FinishedAt.UTC().Format(time.RFC3339), r.Trigger, r.OK, r.Name, r.Size, r.Error); err != nil {
		return fmt.Errorf("record backup run: %w", err)
	}
	_, _ = db.Exec(`DELETE FROM backup_runs WHERE id NOT IN (SELECT id FROM backup_runs ORDER BY id DESC LIMIT ?)`, backupRunsKept)
	return nil
}

// ListBackupRuns returns the newest attempts first.
func (db *DB) ListBackupRuns(limit int) ([]BackupRun, error) {
	if err := db.ensureBackupTables(); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT id, started_at, finished_at, trigger, ok, name, size, error FROM backup_runs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list backup runs: %w", err)
	}
	defer rows.Close()
	var out []BackupRun
	for rows.Next() {
		var r BackupRun
		var started, finished string
		if err := rows.Scan(&r.ID, &started, &finished, &r.Trigger, &r.OK, &r.Name, &r.Size, &r.Error); err != nil {
			return nil, err
		}
		r.StartedAt, _ = time.Parse(time.RFC3339, started)
		r.FinishedAt, _ = time.Parse(time.RFC3339, finished)
		out = append(out, r)
	}
	return out, rows.Err()
}

// SnapshotTo writes a consistent copy of config.db to dest (a backup).
func (db *DB) SnapshotTo(dest string) error { return backup.VacuumInto(db.DB, dest) }
