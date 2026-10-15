package api

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/log"
)

// Optional archive scan (comic-server-yn10). The corrupted_archive flag is
// normally recorded lazily from reads that already happen (4inw). This is the
// opt-in pass that opens every archive's index on purpose, so the flag also
// covers files nobody has read. It is off unless archive_scan_interval_hours
// is set, because on a big library on shared disks it is tens of thousands of
// header reads.
//
// It goes through comicvine.ListPageNames, which reads only the container
// index (zip central directory, RAR headers, 7z header) and no page data, and
// whose open failures reach RecordArchiveFailure through the same hook as any
// other read. A file that opens cleanly is remembered with its mtime-size
// (config.db archive_verified), so the pass resumes after a restart and a
// later pass reads only new or changed files.

const (
	// archiveScanBatch is how many archives are opened between breathers.
	archiveScanBatch = 20
	// ArchiveScanPause is the breather: 20 opens, then 250 ms. A full
	// 66K-book pass therefore sleeps about 14 minutes on top of the reads.
	ArchiveScanPause = 250 * time.Millisecond
)

var archiveScanExts = map[string]bool{
	".cbz": true, ".zip": true, ".cbr": true, ".rar": true, ".cb7": true, ".7z": true,
}

// ArchiveScanResult counts what one pass did.
type ArchiveScanResult struct {
	Opened   int // archives opened this pass
	Skipped  int // already verified or already recorded broken, at this version
	Corrupt  int // newly found corrupt (recorded by the failure hook)
	Failed   int // open errors that say nothing about the archive (I/O, permission)
	Missing  int // file gone: file_missing owns that
	Canceled bool
}

// RunArchiveScan runs a pass now and then every interval until ctx ends.
func (s *Server) RunArchiveScan(ctx context.Context, interval time.Duration, pause time.Duration) {
	if interval <= 0 {
		return
	}
	for {
		res := s.scanArchives(ctx, pause)
		log.Info().Int("opened", res.Opened).Int("skipped", res.Skipped).Int("corrupt", res.Corrupt).
			Int("failed", res.Failed).Int("missing", res.Missing).Bool("canceled", res.Canceled).
			Msg("Archive scan finished")
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

// scanArchives makes one pass over every book's archive. pause is the sleep
// taken every archiveScanBatch opens.
func (s *Server) scanArchives(ctx context.Context, pause time.Duration) ArchiveScanResult {
	var res ArchiveScanResult
	if s.backend == nil || s.configDB == nil {
		return res
	}
	books, err := s.backend.GetAllBooks()
	if err != nil {
		log.Warn().Err(err).Msg("Archive scan: failed to load books")
		return res
	}
	verified, err := s.configDB.VerifiedArchives()
	if err != nil {
		log.Warn().Err(err).Msg("Archive scan: failed to load verified archives")
		return res
	}

	s.health.mu.Lock()
	s.loadArchiveHealthLocked()
	s.health.mu.Unlock()

	seen := map[string]bool{}
	for i := range books {
		if ctx.Err() != nil {
			res.Canceled = true
			return res
		}
		raw := books[i].FilePath
		if raw == "" || !archiveScanExts[strings.ToLower(filepath.Ext(raw))] {
			continue
		}
		path := filepath.Clean(s.resolveBookFilePath(raw))
		if seen[path] {
			continue
		}
		seen[path] = true

		version := fileVersion(path)
		if version == "" {
			res.Missing++
			continue
		}
		s.health.mu.Lock()
		prev, broken := s.health.broken[path]
		s.health.mu.Unlock()
		if verified[path] == version || (broken && prev.Version == version) {
			res.Skipped++
			continue
		}

		_, err := comicvine.ListPageNames(path)
		res.Opened++
		switch {
		case err == nil:
			if e := s.configDB.MarkArchiveVerified(path, version); e != nil {
				log.Warn().Err(e).Str("path", path).Msg("Archive scan: failed to remember verified archive")
			}
		case comicvine.IsCorruptOpenError(err):
			res.Corrupt++ // the failure hook already recorded it
		default:
			res.Failed++
		}
		if res.Opened%archiveScanBatch == 0 && pause > 0 {
			select {
			case <-ctx.Done():
				res.Canceled = true
				return res
			case <-time.After(pause):
			}
		}
	}
	return res
}
