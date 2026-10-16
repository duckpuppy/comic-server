package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/duckpuppy/comic-server/internal/backup"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/log"
)

// Backups of comic-server's own state (comic-server-yzdt). The engine is
// internal/backup; this file is the schedule loop, the settings and the
// endpoints. Restore is not built yet: a backup is a folder that can be
// copied back by hand (stop the server first) until it is.
//
//	GET|PUT /api/settings/backups   settings + status (last run, next run, warnings)
//	POST    /api/backups/run        back up now
//	GET     /api/backups            existing backups, newest first

const (
	backupCheckEvery = time.Minute
	// A failed scheduled run is not retried for this long, so a full disk or
	// an unmounted folder does not retry every minute.
	backupRetryAfterFailure = time.Hour
)

type backupState struct {
	mu          sync.Mutex
	running     bool
	startedAt   time.Time
	lastFailure time.Time
}

// StartBackups runs the schedule loop until ctx ends. The settings are read
// on every tick, so a change in the UI applies without a restart.
func (s *Server) StartBackups(ctx context.Context) {
	go func() {
		t := time.NewTicker(backupCheckEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.backupTick(ctx)
			}
		}
	}()
}

func (s *Server) backupTick(ctx context.Context) {
	if s.configDB == nil {
		return
	}
	set, err := s.configDB.GetBackupSettings()
	if err != nil || !set.Enabled || set.Dir == "" {
		return
	}
	sched := backup.Schedule{Mode: set.Mode, At: set.At, EveryHours: set.EveryHours}
	due, _ := sched.NextRun(s.lastBackupOK(), time.Now())
	if !due {
		return
	}
	s.backupState.mu.Lock()
	recentFail := !s.backupState.lastFailure.IsZero() && time.Since(s.backupState.lastFailure) < backupRetryAfterFailure
	s.backupState.mu.Unlock()
	if recentFail {
		return
	}
	// A copy of a large database competes for the same disks a device sync
	// reads, so a scheduled backup waits for syncs to finish.
	if s.syncManager != nil && len(s.syncManager.GetAllActiveSyncs()) > 0 {
		return
	}
	if err := s.runBackup(ctx, "schedule"); err != nil && !errors.Is(err, errBackupRunning) {
		log.Warn().Err(err).Msg("Scheduled backup failed")
	}
}

var errBackupRunning = errors.New("a backup is already running")

func (s *Server) lastBackupOK() time.Time {
	runs, err := s.configDB.ListBackupRuns(backupRunsToScan)
	if err != nil {
		return time.Time{}
	}
	for _, r := range runs {
		if r.OK {
			return r.StartedAt
		}
	}
	return time.Time{}
}

const backupRunsToScan = 50

// backupSources builds what a backup contains from the current settings.
// notes says what was left out and why.
func (s *Server) backupSources(set configdb.BackupSettings) (srcs []backup.Source, extras []backup.ExtraFile, notes []string) {
	srcs = append(srcs, backup.Source{File: "config.db", Path: s.configDB.Path(), Snapshot: s.configDB.SnapshotTo})
	if snap, ok := s.backend.(interface{ SnapshotTo(string) error }); ok {
		path := ""
		if s.config != nil {
			path = s.config.Server.DatabasePath
		}
		srcs = append(srcs, backup.Source{File: "library.db", Path: path, Snapshot: snap.SnapshotTo,
			Count: backup.CountRows(`SELECT COUNT(*) FROM books WHERE deleted_at IS NULL`)})
	} else {
		notes = append(notes, "The library is served from XML, so there is no library.db to back up.")
	}
	if set.IncludeCV && s.cvCache != nil {
		srcs = append(srcs, backup.Source{File: "comicvine_cache.db", Path: s.cvCache.Path(), Snapshot: s.cvCache.SnapshotTo})
	}
	if set.IncludeConfig && s.configPath != "" {
		if _, err := os.Stat(s.configPath); err == nil {
			extras = append(extras, backup.ExtraFile{File: "config.yaml", Path: s.configPath})
		}
	}
	return srcs, extras, notes
}

// runBackup takes one backup and records the attempt. Only one runs at a time.
func (s *Server) runBackup(ctx context.Context, trigger string) error {
	st := &s.backupState
	st.mu.Lock()
	if st.running {
		st.mu.Unlock()
		return errBackupRunning
	}
	st.running, st.startedAt = true, time.Now()
	st.mu.Unlock()
	defer func() {
		st.mu.Lock()
		st.running = false
		st.mu.Unlock()
	}()

	started := time.Now()
	run := configdb.BackupRun{StartedAt: started, Trigger: trigger}
	set, err := s.configDB.GetBackupSettings()
	var m *backup.Manifest
	var name string
	if err == nil {
		srcs, extras, notes := s.backupSources(set)
		m, name, err = backup.Run(ctx, backup.Options{Dir: set.Dir, Keep: set.Keep, AppVersion: s.version.Version,
			Sources: srcs, Extras: extras, Notes: notes})
	}
	run.FinishedAt = time.Now()
	if err != nil {
		run.Error = err.Error()
		st.mu.Lock()
		st.lastFailure = run.FinishedAt
		st.mu.Unlock()
		log.Error().Err(err).Str("trigger", trigger).Msg("Backup failed")
	} else {
		run.OK, run.Name, run.Size = true, name, m.TotalSize()
		st.mu.Lock()
		st.lastFailure = time.Time{}
		st.mu.Unlock()
		log.Info().Str("backup", name).Int64("bytes", run.Size).Str("trigger", trigger).Msg("Backup completed")
	}
	if rerr := s.configDB.AddBackupRun(run); rerr != nil {
		log.Warn().Err(rerr).Msg("Could not record the backup run")
	}
	return err
}

// BackupStatus is the status half of the settings response.
type BackupStatus struct {
	Running bool                `json:"running"`
	NextRun *time.Time          `json:"next_run,omitempty"`
	LastRun *configdb.BackupRun `json:"last_run,omitempty"`
	LastOK  *configdb.BackupRun `json:"last_ok,omitempty"`
}

// BackupSettingsResponse is the body of GET/PUT /api/settings/backups.
type BackupSettingsResponse struct {
	Settings configdb.BackupSettings `json:"settings"`
	Status   BackupStatus            `json:"status"`
	Warnings []string                `json:"warnings,omitempty"`
	Includes []string                `json:"includes"` // files the next backup will hold
}

func (s *Server) backupSettingsResponse(set configdb.BackupSettings) BackupSettingsResponse {
	resp := BackupSettingsResponse{Settings: set, Includes: []string{}}
	srcs, extras, notes := s.backupSources(set)
	for _, src := range srcs {
		resp.Includes = append(resp.Includes, src.File)
	}
	for _, e := range extras {
		resp.Includes = append(resp.Includes, e.File)
	}
	resp.Warnings = append(resp.Warnings, notes...)
	if set.Dir != "" {
		for _, src := range srcs {
			if src.Path != "" && backup.SameDevice(set.Dir, src.Path) {
				resp.Warnings = append(resp.Warnings, "The backup folder is on the same disk as "+src.File+". A disk failure would lose both. Use a different disk.")
				break
			}
		}
	}
	s.backupState.mu.Lock()
	resp.Status.Running = s.backupState.running
	s.backupState.mu.Unlock()
	runs, _ := s.configDB.ListBackupRuns(backupRunsToScan)
	if len(runs) > 0 {
		resp.Status.LastRun = &runs[0]
	}
	var lastOK time.Time
	for i := range runs {
		if runs[i].OK {
			resp.Status.LastOK = &runs[i]
			lastOK = runs[i].StartedAt
			break
		}
	}
	if set.Enabled && set.Dir != "" {
		if _, next := (backup.Schedule{Mode: set.Mode, At: set.At, EveryHours: set.EveryHours}).NextRun(lastOK, time.Now()); !next.IsZero() {
			resp.Status.NextRun = &next
		}
	}
	return resp
}

// handleBackupSettings GET|PUT /api/settings/backups
func (s *Server) handleBackupSettings(w http.ResponseWriter, r *http.Request) {
	if s.configDB == nil {
		http.Error(w, "Config database not available", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		set, err := s.configDB.GetBackupSettings()
		if err != nil {
			http.Error(w, "Failed to load backup settings", http.StatusInternalServerError)
			return
		}
		s.writeJSON(w, http.StatusOK, s.backupSettingsResponse(set))
	case http.MethodPut:
		var set configdb.BackupSettings
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&set); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if err := validateBackupSettings(set); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.configDB.PutBackupSettings(set); err != nil {
			log.Error().Err(err).Msg("Failed to save backup settings")
			http.Error(w, "Failed to save backup settings", http.StatusInternalServerError)
			return
		}
		s.writeJSON(w, http.StatusOK, s.backupSettingsResponse(set))
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func validateBackupSettings(set configdb.BackupSettings) error {
	if set.Keep < 1 || set.Keep > 365 {
		return errors.New("keep must be between 1 and 365")
	}
	if err := (backup.Schedule{Mode: set.Mode, At: set.At, EveryHours: set.EveryHours}).Validate(); err != nil {
		return err
	}
	if set.Dir == "" {
		if set.Enabled {
			return errors.New("choose a backup folder before turning backups on")
		}
		return nil
	}
	if !filepath.IsAbs(set.Dir) {
		return errors.New("the backup folder must be an absolute path")
	}
	// The folder must be usable now, so a typo shows here and not at 03:30.
	if err := os.MkdirAll(set.Dir, 0o700); err != nil {
		return fmt.Errorf("cannot create the backup folder: %w", err)
	}
	f, err := os.CreateTemp(set.Dir, ".write-test-")
	if err != nil {
		return fmt.Errorf("the backup folder is not writable: %w", err)
	}
	f.Close()
	os.Remove(f.Name())
	return nil
}

// handleBackupRun POST /api/backups/run starts a backup in the background.
func (s *Server) handleBackupRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.configDB == nil {
		http.Error(w, "Config database not available", http.StatusServiceUnavailable)
		return
	}
	set, err := s.configDB.GetBackupSettings()
	if err != nil {
		http.Error(w, "Failed to load backup settings", http.StatusInternalServerError)
		return
	}
	if set.Dir == "" {
		http.Error(w, "Choose a backup folder first", http.StatusBadRequest)
		return
	}
	s.backupState.mu.Lock()
	running := s.backupState.running
	s.backupState.mu.Unlock()
	if running {
		http.Error(w, errBackupRunning.Error(), http.StatusConflict)
		return
	}
	go func() {
		// A request context ends with the response; the backup must not.
		if err := s.runBackup(context.Background(), "manual"); err != nil && !errors.Is(err, errBackupRunning) {
			log.Warn().Err(err).Msg("Manual backup failed")
		}
	}()
	s.writeJSON(w, http.StatusAccepted, map[string]bool{"started": true})
}

// handleBackupList GET /api/backups
func (s *Server) handleBackupList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.configDB == nil {
		http.Error(w, "Config database not available", http.StatusServiceUnavailable)
		return
	}
	set, err := s.configDB.GetBackupSettings()
	if err != nil {
		http.Error(w, "Failed to load backup settings", http.StatusInternalServerError)
		return
	}
	entries, err := backup.List(set.Dir)
	if err != nil {
		http.Error(w, "Cannot read the backup folder: "+err.Error(), http.StatusInternalServerError)
		return
	}
	type row struct {
		Name       string    `json:"name"`
		CreatedAt  time.Time `json:"created_at"`
		Size       int64     `json:"size"`
		AppVersion string    `json:"app_version"`
		BookCount  int       `json:"book_count"`
		Files      []string  `json:"files"`
	}
	out := make([]row, 0, len(entries))
	for _, e := range entries {
		rw := row{Name: e.Name, CreatedAt: e.Manifest.CreatedAt, Size: e.Manifest.TotalSize(), AppVersion: e.Manifest.AppVersion, BookCount: e.Manifest.BookCount}
		for _, f := range e.Manifest.Files {
			rw.Files = append(rw.Files, f.Name)
		}
		out = append(out, rw)
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"dir": set.Dir, "backups": out})
}
