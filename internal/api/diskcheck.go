package api

import (
	"os"
	"sync"
	"time"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/log"
)

// diskCheck holds the result of the background file-existence pass
// (comic-server-4q1z). The pass stats every book's archive, which is far too
// slow for the request path on a 66K-book library, so the workflow cache
// only ever reads the last finished result.
type diskCheck struct {
	mu        sync.Mutex
	missing   map[string]bool // book id -> file not found
	misnamed  map[string]bool // book id -> real container differs from extension
	checkedAt time.Time
	running   bool
	interval  time.Duration // 0 disables the pass
}

// SetDiskCheckInterval turns on the background file_missing pass and sets
// how often it re-runs. Zero (the default) leaves it off.
func (s *Server) SetDiskCheckInterval(d time.Duration) {
	s.disk.mu.Lock()
	s.disk.interval = d
	s.disk.mu.Unlock()
}

// missingFiles returns the last pass's missing book ids (nil before the
// first finishes) and starts a new pass when none has run or the last one
// is older than the interval.
func (s *Server) missingFiles() (missing, misnamed map[string]bool) {
	s.disk.mu.Lock()
	defer s.disk.mu.Unlock()
	if s.disk.interval <= 0 {
		return nil, nil
	}
	if !s.disk.running && time.Since(s.disk.checkedAt) >= s.disk.interval {
		s.disk.running = true
		go s.runDiskCheck()
	}
	return s.disk.missing, s.disk.misnamed
}

func (s *Server) runDiskCheck() {
	missing := map[string]bool{}
	misnamed := map[string]bool{}
	if s.backend != nil {
		books, err := s.backend.GetAllBooks()
		if err != nil {
			log.Warn().Err(err).Msg("Disk check: failed to load books")
		}
		for i := range books {
			b := &books[i]
			if b.FilePath == "" { // wanted placeholder: nothing to find
				continue
			}
			path := s.resolveBookFilePath(b.FilePath)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				missing[b.ID] = true
				continue
			}
			// A RAR named .cbz etc. (comic-server-0ebv): content decides.
			if want := comicvine.ExtKind(path); want != comicvine.KindUnknown {
				if got, err := comicvine.SniffArchive(path); err == nil && got != comicvine.KindUnknown && got != want {
					misnamed[b.ID] = true
				}
			}
		}
	}
	s.disk.mu.Lock()
	s.disk.missing = missing
	s.disk.misnamed = misnamed
	s.disk.checkedAt = time.Now()
	s.disk.running = false
	s.disk.mu.Unlock()
	log.Info().Int("missing", len(missing)).Int("misnamed", len(misnamed)).Msg("Disk check finished")
	s.InvalidateWorkflowCache() // rebuild with the new flags
}
