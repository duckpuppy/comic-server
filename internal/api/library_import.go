// Package api: on-demand library import (comic-server-szvk) - replaces
// the old always-on file-watcher/continuous-reimport model
// (library.Watcher, retired) with an explicit, user-triggered upload from
// the Settings screen. The mental model is now "one-time ComicRack ->
// comic-server migration, repeated occasionally by hand," not an ongoing
// sync.
//
// ComicRack backup .zip support was explicitly dropped from scope
// (2026-09-12, with the user) - no sample file exists to ground-truth its
// internal layout against, and the automated mediacenter sync pulls a raw
// ComicDb.xml directly, never a backup zip. XML only.
package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/storage"
	"github.com/duckpuppy/comic-server/internal/websocket"
)

// maxLibraryImportUploadBytes caps the uploaded ComicDb.xml at ~500MB -
// the user's real library is ~243MB, this gives real headroom without
// accepting an arbitrarily large upload by mistake.
const maxLibraryImportUploadBytes = 500 << 20 // 500MB

// LibraryImportStatsWire is the wire shape of one storage.ImportStats.
type LibraryImportStatsWire struct {
	BooksAdded     int     `json:"books_added"`
	BooksUpdated   int     `json:"books_updated"`
	BooksDeleted   int     `json:"books_deleted"`
	BooksUnchanged int     `json:"books_unchanged"`
	ListsAdded     int     `json:"lists_added"`
	ListsUpdated   int     `json:"lists_updated"`
	ListsDeleted   int     `json:"lists_deleted"`
	ListsUnchanged int     `json:"lists_unchanged"`
	DurationSec    float64 `json:"duration_sec"`
}

func toLibraryImportStatsWire(s *storage.ImportStats) *LibraryImportStatsWire {
	if s == nil {
		return nil
	}
	return &LibraryImportStatsWire{
		BooksAdded: s.BooksAdded, BooksUpdated: s.BooksUpdated, BooksDeleted: s.BooksDeleted, BooksUnchanged: s.BooksUnchanged,
		ListsAdded: s.ListsAdded, ListsUpdated: s.ListsUpdated, ListsDeleted: s.ListsDeleted, ListsUnchanged: s.ListsUnchanged,
		DurationSec: s.Duration.Seconds(),
	}
}

// LibraryImportJobStatus is the state of the single (at most one at a
// time) on-demand library import job - one combined "uploading then
// importing" progress experience from the client's point of view: the
// upload itself is a synchronous part of the POST request (the fetch call
// simply stays pending, same as a native browser upload indicator), and
// this job only exists to report the (fast, indeterminate-progress)
// import phase that follows once the file is fully received.
type LibraryImportJobStatus struct {
	JobID    string `json:"job_id"`
	Status   string `json:"status"` // "importing", "completed", "failed"
	Filename string `json:"filename"`
	// RestartRequired is true when this import provisioned a brand-new
	// database_path (comic-server-szvk's "Case B": nothing was configured
	// yet) - the server's live backend was chosen once at startup and
	// isn't hot-swappable, so the import took effect on disk but won't be
	// served until the process restarts. Making this case live too is
	// comic-server-9klu. False (the common case, "Case A") when an
	// already-configured, already-running SQLite backend was reimported
	// into directly and took effect immediately.
	RestartRequired bool                    `json:"restart_required,omitempty"`
	Stats           *LibraryImportStatsWire `json:"stats,omitempty"`
	Error           string                  `json:"error,omitempty"`
	StartedAt       time.Time               `json:"started_at"`
	CompletedAt     *time.Time              `json:"completed_at,omitempty"`
}

// handleLibraryImport serves POST /api/settings/library-import (start a
// new import from an uploaded ComicDb.xml) and GET (poll the current/last
// job's status).
func (s *Server) handleLibraryImport(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetLibraryImportStatus(w, r)
	case http.MethodPost:
		s.handlePostLibraryImport(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGetLibraryImportStatus(w http.ResponseWriter, r *http.Request) {
	s.libImportJobMu.RLock()
	job := s.libImportJob
	s.libImportJobMu.RUnlock()
	if job == nil {
		http.Error(w, "No library import has been run yet", http.StatusNotFound)
		return
	}
	s.writeJSON(w, http.StatusOK, job)
}

// handlePostLibraryImport receives the uploaded ComicDb.xml (multipart
// form field "file"), stages it to the XDG cache directory, and runs the
// import synchronously within this request - a real library (~243MB,
// ~65K books) imports in well under the time it takes to upload it in the
// first place at any real-world connection speed, so unlike Data
// Manager's whole-library evaluation (comic-server-wp9) there's no
// gateway-timeout risk here that would justify a separate background-job
// round trip; the "job" status object exists purely so a client that
// reloads the Settings page mid-import (or just wants a durable result to
// show) can still see the outcome, not because this handler returns
// before the work is done.
func (s *Server) handlePostLibraryImport(w http.ResponseWriter, r *http.Request) {
	if s.configDB == nil {
		http.Error(w, "Config database not available", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxLibraryImportUploadBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "Failed to read upload (may exceed the 500MB limit): "+err.Error(), http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Missing \"file\" upload field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	cacheDir, err := config.EnsureCacheDir()
	if err != nil {
		http.Error(w, "Failed to prepare staging directory: "+err.Error(), http.StatusInternalServerError)
		return
	}
	stagingPath := filepath.Join(cacheDir, "library-import-staging.xml")
	staged, err := os.Create(stagingPath)
	if err != nil {
		http.Error(w, "Failed to stage upload: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(staged, file); err != nil {
		staged.Close()
		os.Remove(stagingPath)
		http.Error(w, "Failed to stage upload: "+err.Error(), http.StatusInternalServerError)
		return
	}
	staged.Close()
	defer os.Remove(stagingPath)

	job := &LibraryImportJobStatus{
		JobID:     fmt.Sprintf("libimport-%d", time.Now().UnixNano()),
		Status:    "importing",
		Filename:  header.Filename,
		StartedAt: time.Now(),
	}
	s.libImportJobMu.Lock()
	s.libImportJob = job
	s.libImportJobMu.Unlock()

	s.runLibraryImport(job, stagingPath)
	s.writeJSON(w, http.StatusOK, job)
}

// runLibraryImport does the actual import (Case A: reimport into an
// already-running SQLite backend live, or Case B: provision a brand-new
// database_path that needs a restart to take effect - see
// LibraryImportJobStatus.RestartRequired's doc comment) and mutates job
// in place with the outcome.
func (s *Server) runLibraryImport(job *LibraryImportJobStatus, stagingPath string) {
	finish := func(stats *storage.ImportStats, restartRequired bool, err error) {
		now := time.Now()
		job.CompletedAt = &now
		if err != nil {
			job.Status = "failed"
			job.Error = err.Error()
			return
		}
		job.Status = "completed"
		job.Stats = toLibraryImportStatsWire(stats)
		job.RestartRequired = restartRequired
	}

	if sb, ok := s.backend.(*storage.SQLiteBackend); ok {
		// Case A: already running on SQLite - reimport live and reuse the
		// exact cache-invalidation/broadcast/Komga-trigger/WarmUp sequence
		// the old file-watcher's OnReload callback used to run.
		stats, err := sb.ImportFrom(stagingPath)
		if err != nil {
			finish(nil, false, err)
			return
		}
		s.InvalidateListCache()
		s.InvalidateWorkflowCache()
		if s.wsHub != nil {
			s.wsHub.Broadcast(websocket.EventLibraryReloaded, map[string]any{"book_count": sb.BookCount()})
		}
		if s.komgaSyncer != nil {
			s.komgaSyncer.TriggerNow()
		}
		go func() {
			if err := sb.WarmUp(); err != nil {
				log.Warn().Err(err).Msg("Background library warm-up failed after on-demand import")
			}
		}()
		finish(stats, false, nil)
		return
	}

	// Case B: nothing configured yet - provision a brand-new database_path
	// and import into it, but don't touch s.backend (comic-server-9klu
	// tracks making this live without a restart too).
	dataDir, err := config.EnsureDataDir()
	if err != nil {
		finish(nil, false, fmt.Errorf("prepare data directory: %w", err))
		return
	}
	dbPath := filepath.Join(dataDir, "library.db")

	sb, err := storage.NewSQLiteBackend(dbPath, "")
	if err != nil {
		finish(nil, false, fmt.Errorf("create database: %w", err))
		return
	}
	defer sb.Close()

	stats, err := sb.ImportFrom(stagingPath)
	if err != nil {
		finish(nil, false, err)
		return
	}

	s.configMu.Lock()
	s.config.Server.DatabasePath = dbPath
	cfgCopy := *s.config
	s.configMu.Unlock()
	if err := config.Save(&cfgCopy, s.configPath); err != nil {
		log.Error().Err(err).Msg("Failed to save config.yaml after provisioning library.db from an on-demand import")
	}

	finish(stats, true, nil)
}
