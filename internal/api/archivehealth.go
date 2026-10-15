package api

import (
	"path/filepath"
	"sync"

	"github.com/duckpuppy/comic-server/internal/attention"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// archiveHealth is the corrupted_archive flag's state (comic-server-4inw).
// Nothing here opens an archive: failures arrive from reads that already
// happen (comicvine.SetArchiveFailureHook), so the flag costs no disk I/O.
type archiveHealth struct {
	mu     sync.Mutex
	loaded bool
	broken map[string]configdb.ArchiveFailure // cleaned resolved path -> failure
}

// RecordArchiveFailure is the comicvine hook: path is the path the reader
// opened (already resolved). It remembers the failure once per file version,
// so a page view that fails on every request writes one row, not hundreds.
func (s *Server) RecordArchiveFailure(path string, err error) {
	if s.configDB == nil || path == "" {
		return
	}
	key := filepath.Clean(path)
	version := fileVersion(key)
	s.health.mu.Lock()
	s.loadArchiveHealthLocked()
	prev, known := s.health.broken[key]
	if known && prev.Version == version {
		s.health.mu.Unlock()
		return
	}
	f := configdb.ArchiveFailure{Path: key, Version: version, Reason: err.Error()}
	s.health.broken[key] = f
	s.health.mu.Unlock()

	if e := s.configDB.RecordArchiveFailure(f); e != nil {
		log.Warn().Err(e).Str("path", key).Msg("Failed to record archive failure")
	}
	s.flagCorrupted(key)
}

// loadArchiveHealthLocked reads the table once. Caller holds health.mu.
func (s *Server) loadArchiveHealthLocked() {
	if s.health.loaded {
		return
	}
	s.health.loaded = true
	s.health.broken = map[string]configdb.ArchiveFailure{}
	if s.configDB == nil {
		return
	}
	rows, err := s.configDB.ArchiveFailures()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load archive failures")
		return
	}
	for _, f := range rows {
		s.health.broken[filepath.Clean(f.Path)] = f
	}
}

// corruptedArchives returns the still-valid failures keyed by resolved path.
// Entries whose file changed since (or is gone: file_missing owns that) are
// dropped and forgotten, which is how a replaced file stops being flagged.
func (s *Server) corruptedArchives() map[string]configdb.ArchiveFailure {
	s.health.mu.Lock()
	defer s.health.mu.Unlock()
	s.loadArchiveHealthLocked()
	if len(s.health.broken) == 0 {
		return nil
	}
	out := make(map[string]configdb.ArchiveFailure, len(s.health.broken))
	for path, f := range s.health.broken {
		if v := fileVersion(path); v == "" || v != f.Version {
			delete(s.health.broken, path)
			if s.configDB != nil {
				if err := s.configDB.DeleteArchiveFailure(path); err != nil {
					log.Warn().Err(err).Str("path", path).Msg("Failed to forget archive failure")
				}
			}
			continue
		}
		out[path] = f
	}
	return out
}

// flagCorrupted patches the cached flag lists in place of invalidating the
// whole 66K-book cache for one new failure.
func (s *Server) flagCorrupted(resolvedPath string) {
	s.workflowCacheMu.Lock()
	defer s.workflowCacheMu.Unlock()
	cur := s.workflowCache
	if cur == nil {
		return // nothing cached; the next build reads the table
	}
	var hit *library.ComicBook
	for _, group := range cur.booksByStage {
		for _, b := range group {
			if b.FilePath != "" && filepath.Clean(s.resolveBookFilePath(b.FilePath)) == resolvedPath {
				hit = b
				break
			}
		}
		if hit != nil {
			break
		}
	}
	if hit == nil || attention.Accepted(hit)[attention.FlagCorruptedArchive] {
		return
	}
	for _, b := range cur.booksByFlag[attention.FlagCorruptedArchive] {
		if b.ID == hit.ID {
			return
		}
	}
	next := *cur
	next.booksByFlag = make(map[attention.Flag][]*library.ComicBook, len(cur.booksByFlag)+1)
	for f, v := range cur.booksByFlag {
		next.booksByFlag[f] = v
	}
	next.booksByFlag[attention.FlagCorruptedArchive] = append(
		append([]*library.ComicBook(nil), cur.booksByFlag[attention.FlagCorruptedArchive]...), hit)
	s.workflowCache = &next
}
