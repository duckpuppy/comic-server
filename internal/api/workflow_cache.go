package api

import (
	"github.com/duckpuppy/comic-server/internal/attention"
	"path/filepath"
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
	// cvNoMatch holds every book marked not in ComicVine (comic-server-5wer),
	// so the workflow card can list them, not just count them.
	cvNoMatch []*library.ComicBook
	unstaged  int // books with a file but no stored stage
	// booksByFlag holds every book with each attention flag (comic-server-z77p),
	// computed in the same pass as booksByStage.
	booksByFlag map[attention.Flag][]*library.ComicBook
}

// workflowBuild is one snapshot build in progress. Requests that arrive while
// it runs share its result instead of each loading every book again.
type workflowBuild struct {
	done  chan struct{}
	entry *workflowCacheEntry
	err   error
}

// getOrBuildWorkflowCache returns the current cached snapshot, building a
// fresh one (and caching it) if none exists yet - e.g. right after
// InvalidateWorkflowCache, or on the very first request since startup.
//
// Only one build runs at a time. A request that arrives during a build gets
// the previous snapshot at once when there is one (it may be a few seconds
// old), otherwise it waits for the build. Before this, every request that
// found the cache empty loaded all books by itself, and a busy pipeline run
// piled up dozens of full loads at the same time.
func (s *Server) getOrBuildWorkflowCache() (*workflowCacheEntry, error) {
	s.workflowCacheMu.Lock()
	if entry := s.workflowCache; entry != nil {
		s.workflowCacheMu.Unlock()
		return entry, nil
	}
	if call := s.workflowBuilding; call != nil {
		stale := s.workflowStale
		s.workflowCacheMu.Unlock()
		if stale != nil {
			return stale, nil
		}
		<-call.done
		return call.entry, call.err
	}
	call := &workflowBuild{done: make(chan struct{})}
	s.workflowBuilding = call
	gen := s.workflowGen
	s.workflowCacheMu.Unlock()

	defer func() {
		// Runs even if the build panics, so waiters are never stuck.
		s.workflowCacheMu.Lock()
		s.workflowBuilding = nil
		s.workflowCacheMu.Unlock()
		close(call.done)
	}()

	call.entry, call.err = s.buildWorkflowCache()
	if call.err != nil {
		return nil, call.err
	}

	s.workflowCacheMu.Lock()
	// Keep the result only if nothing invalidated the cache during the build;
	// otherwise it may already be out of date and the next request rebuilds.
	if gen == s.workflowGen && s.workflowCache == nil {
		s.workflowCache = call.entry
		s.workflowStale = nil
	}
	s.workflowCacheMu.Unlock()

	return call.entry, nil
}

// buildWorkflowCache scans every book once and builds a snapshot.
func (s *Server) buildWorkflowCache() (*workflowCacheEntry, error) {
	allBooks, err := s.backend.GetAllBooks()
	if err != nil {
		return nil, err
	}
	built := &workflowCacheEntry{
		computedAt:   time.Now(),
		booksByStage: make(map[workflow.Stage][]*library.ComicBook, len(workflow.Stages)+1),
		booksByFlag:  make(map[attention.Flag][]*library.ComicBook),
	}
	missing := s.missingFiles()
	corrupt := s.corruptedArchives()
	cv := s.cvAttentionData(allBooks)
	ptrs := make([]*library.ComicBook, len(allBooks))
	for i := range allBooks {
		ptrs[i] = &allBooks[i]
	}
	conflicts := attention.VolumeConflicts(ptrs)
	formatOdd := attention.FormatMismatches(ptrs)
	for i := range allBooks {
		book := &allBooks[i]
		st := workflow.GetStage(book)
		built.booksByStage[st] = append(built.booksByStage[st], book)
		// Flags the user accepted on this book are left out (comic-server-x7rd).
		accepted := attention.Accepted(book)
		add := func(f attention.Flag) {
			if !accepted[f] {
				built.booksByFlag[f] = append(built.booksByFlag[f], book)
			}
		}
		for _, f := range attention.FlagsFor(book) {
			add(f)
		}
		for _, f := range attention.CVFlagsFor(book, cv) {
			add(f)
		}
		if conflicts[book.ID] {
			add(attention.FlagCVVolumeConflict)
		}
		if formatOdd[book.ID] {
			add(attention.FlagFormatMismatch)
		}
		if missing[book.ID] {
			add(attention.FlagFileMissing)
		}
		if len(corrupt) > 0 && book.FilePath != "" {
			if _, bad := corrupt[filepath.Clean(s.resolveBookFilePath(book.FilePath))]; bad {
				add(attention.FlagCorruptedArchive)
			}
		}
		if st == workflow.StageUnknown && book.FilePath != "" {
			built.unstaged++
		}
		if workflow.IsCVDBSkip(book) {
			built.cvdbSkip++
		}
		if workflow.IsNoCVMatch(book) {
			built.cvNoMatch = append(built.cvNoMatch, book)
		}
	}

	return built, nil
}

// InvalidateWorkflowCache discards the cached stage snapshot so the next
// summary/drill-in request rebuilds it from scratch. Called after any
// action that can move a book from one workflow stage to another
// (CBZ convert, scan info, Data Manager, scrape, Library Organizer apply)
// and from the library reload watcher (cmd/server.go), alongside the
// existing InvalidateListCache call for the same event.
func (s *Server) InvalidateWorkflowCache() {
	s.workflowCacheMu.Lock()
	if s.workflowCache != nil {
		// Kept so requests that arrive while the rebuild runs are not blocked.
		s.workflowStale = s.workflowCache
	}
	s.workflowCache = nil
	s.workflowGen++
	s.workflowCacheMu.Unlock()
}

// InvalidateWorkflowCacheNow is InvalidateWorkflowCache for actions where the
// user must see the effect at once (removing a record, relinking a file): it
// also drops the previous snapshot, so the next request waits for a fresh
// build instead of being handed one that still lists the old state.
func (s *Server) InvalidateWorkflowCacheNow() {
	s.workflowCacheMu.Lock()
	s.workflowCache = nil
	s.workflowStale = nil
	s.workflowGen++
	s.workflowCacheMu.Unlock()
}
