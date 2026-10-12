package api

import (
	"os"
	"sync"
	"time"

	"github.com/duckpuppy/comic-server/internal/attention"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// diskCheckPause is the breather taken every 500 books during a pass.
const diskCheckPause = 10 * time.Millisecond

// diskCheck holds the result of the background file-existence pass
// (comic-server-4q1z). The pass stats every book's archive, which is far too
// slow for the request path on a 66K-book library, so the workflow cache
// only ever reads the last finished result.
type diskCheck struct {
	mu        sync.Mutex
	missing   map[string]bool // book id -> file not found
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
func (s *Server) missingFiles() map[string]bool {
	s.disk.mu.Lock()
	defer s.disk.mu.Unlock()
	if s.disk.interval <= 0 {
		return nil
	}
	if !s.disk.running && time.Since(s.disk.checkedAt) >= s.disk.interval {
		s.disk.running = true
		go s.runDiskCheck()
	}
	return s.disk.missing
}

func (s *Server) runDiskCheck() {
	// Reuse the workflow cache's books when it exists (it holds every book)
	// so the pass costs no extra library scan.
	var books []*library.ComicBook
	s.workflowCacheMu.RLock()
	entry := s.workflowCache
	s.workflowCacheMu.RUnlock()
	if entry != nil {
		for _, group := range entry.booksByStage {
			books = append(books, group...)
		}
	} else if s.backend != nil {
		all, err := s.backend.GetAllBooks()
		if err != nil {
			log.Warn().Err(err).Msg("Disk check: failed to load books")
		}
		for i := range all {
			books = append(books, &all[i])
		}
	}

	missing := map[string]bool{}
	var missingBooks []*library.ComicBook
	for i, b := range books {
		if b.FilePath == "" { // wanted placeholder: nothing to find
			continue
		}
		if _, err := os.Stat(s.resolveBookFilePath(b.FilePath)); os.IsNotExist(err) {
			missing[b.ID] = true
			missingBooks = append(missingBooks, b)
		}
		// Metadata only, but be gentle: the disks are shared with media
		// that may be streaming.
		if i%500 == 499 {
			time.Sleep(diskCheckPause)
		}
	}

	s.disk.mu.Lock()
	s.disk.missing = missing
	s.disk.checkedAt = time.Now()
	s.disk.running = false
	s.disk.mu.Unlock()
	log.Info().Int("missing", len(missing)).Msg("Disk check finished")

	// Patch the cache copy-on-write instead of invalidating it: dropping it
	// would force a full 66K-book rebuild on the next page load.
	s.workflowCacheMu.Lock()
	if cur := s.workflowCache; cur != nil {
		next := *cur
		next.booksByFlag = make(map[attention.Flag][]*library.ComicBook, len(cur.booksByFlag)+1)
		for f, v := range cur.booksByFlag {
			next.booksByFlag[f] = v
		}
		if len(missingBooks) > 0 {
			next.booksByFlag[attention.FlagFileMissing] = missingBooks
		} else {
			delete(next.booksByFlag, attention.FlagFileMissing)
		}
		s.workflowCache = &next
	}
	s.workflowCacheMu.Unlock()
}
