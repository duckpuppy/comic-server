package api

import (
	"time"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

// workflowCacheEntry is one snapshot of "which book is at which workflow
// stage" - the exact same information the Workflow dashboard summary
// already computes by scanning every book once. Reused for the stage
// drill-in view so opening it doesn't repeat that same full-library scan
// a second time just to re-derive a subset of what the summary already
// knew a moment ago.
//
// This is a READ-ONLY convenience cache, not the source of truth an
// action handler should select its real work from - booksAtStage (used by
// handleRunCBZConvertWorkflow, handleRunScanInfoWorkflow, etc. to decide
// which books to actually process) deliberately does NOT go through this
// cache and always re-scans, since serving a stage-processing action a
// stale book list is a correctness problem, not just a stale display.
// InvalidateWorkflowCache is called after every such action instead, so
// the NEXT summary/drill-in view reflects what just happened rather than
// trusting a snapshot from before it.
type workflowCacheEntry struct {
	computedAt   time.Time
	booksByStage map[workflow.Stage][]*library.ComicBook
	cvdbSkip     int
}

// getOrBuildWorkflowCache returns the current cached snapshot, building a
// fresh one (and caching it) if none exists yet - e.g. right after
// InvalidateWorkflowCache, or on the very first request since startup.
func (s *Server) getOrBuildWorkflowCache() (*workflowCacheEntry, error) {
	s.workflowCacheMu.RLock()
	entry := s.workflowCache
	s.workflowCacheMu.RUnlock()
	if entry != nil {
		return entry, nil
	}

	allBooks, err := s.backend.GetAllBooks()
	if err != nil {
		return nil, err
	}
	built := &workflowCacheEntry{
		computedAt:   time.Now(),
		booksByStage: make(map[workflow.Stage][]*library.ComicBook, len(workflow.Stages)+1),
	}
	for i := range allBooks {
		book := &allBooks[i]
		st := workflow.GetStage(book)
		built.booksByStage[st] = append(built.booksByStage[st], book)
		if workflow.IsCVDBSkip(book) {
			built.cvdbSkip++
		}
	}

	s.workflowCacheMu.Lock()
	// Another request may have built and stored one while we were
	// scanning - keep whichever won the race rather than clobbering it,
	// same "don't bother being the second writer" shortcut listCache's
	// own callers already accept.
	if s.workflowCache == nil {
		s.workflowCache = built
	}
	entry = s.workflowCache
	s.workflowCacheMu.Unlock()

	return entry, nil
}

// InvalidateWorkflowCache discards the cached stage snapshot so the next
// summary/drill-in request rebuilds it from scratch. Called after any
// action that can move a book from one workflow stage to another
// (CBZ convert, scan info, Data Manager, scrape, Library Organizer apply)
// and from the library reload watcher (cmd/server.go), alongside the
// existing InvalidateListCache call for the same event.
func (s *Server) InvalidateWorkflowCache() {
	s.workflowCacheMu.Lock()
	s.workflowCache = nil
	s.workflowCacheMu.Unlock()
}
