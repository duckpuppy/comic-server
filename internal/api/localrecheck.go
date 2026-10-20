package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/log"
)

// Background re-check of local-identity books (tracker #47). See
// comicvine.RecheckLocalIdentity for the pass itself; this is the schedule,
// the setting and the dismiss endpoint.

const (
	// localRecheckEvery is the gap between passes. A pass makes at most
	// comicvine.RecheckMaxSearches searches, so this is a few requests a day.
	localRecheckEvery = 24 * time.Hour
	// localRecheckFirstDelay lets the startup sync and a person's first
	// requests go first.
	localRecheckFirstDelay = 20 * time.Minute
)

// localIdentityRecheckOn reads the setting (off when config.db has none).
func (s *Server) localIdentityRecheckOn() bool {
	if s.configDB == nil {
		return false
	}
	st, err := s.configDB.GetServerMiscSettings()
	return err == nil && st != nil && st.LocalIdentityRecheck
}

// StartLocalIdentityRecheck runs the pass loop until ctx ends. The setting is
// read on every tick, so the toggle needs no restart. Does nothing without a
// ComicVine scraper.
func (s *Server) StartLocalIdentityRecheck(ctx context.Context) {
	if s.scraper == nil {
		return
	}
	go func() {
		wait := localRecheckFirstDelay
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			wait = localRecheckEvery
			s.localRecheckPass(ctx)
		}
	}()
}

// localRecheckPass runs one pass if the setting is on and no person-started
// scrape is running. Any ComicVine failure ends it quietly until the next tick.
func (s *Server) localRecheckPass(ctx context.Context) {
	if s.scraper == nil || s.backend == nil || !s.localIdentityRecheckOn() {
		return
	}
	if job := s.scraper.CurrentJob(); job != nil && job.Status == comicvine.JobStatusRunning {
		return // people first
	}
	books, err := s.backend.GetAllBooks()
	if err != nil {
		log.Warn().Err(err).Msg("Local identity re-check: could not read books")
		return
	}
	res, err := s.scraper.RecheckLocalIdentity(ctx, books)
	ev := log.Info()
	if err != nil {
		var open comicvine.ErrCircuitOpen
		if errors.As(err, &open) {
			ev = log.Info().Str("backoff_until", open.Until.Format(time.RFC3339))
		} else {
			ev = log.Warn().Err(err)
		}
	}
	ev.Int("eligible", res.Eligible).Int("searches", res.Searches).
		Int("suggested", res.Suggested).Int("not_found", res.NotFound).
		Msg("Local identity re-check pass finished")
	if res.Suggested > 0 {
		s.InvalidateWorkflowCache()
	}
}

// handleRecheckDismiss serves POST /api/scrape/recheck/dismiss/:bookId: the
// person does not want the suggested ComicVine listing for a book that keeps
// its local identity. The suggestion is removed and never queued again.
func (s *Server) handleRecheckDismiss(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cvCache == nil {
		http.Error(w, "ComicVine scraper is not configured", http.StatusServiceUnavailable)
		return
	}
	bookID := parsePathParam(r.URL.Path, "/api/scrape/recheck/dismiss/")
	if bookID == "" {
		http.Error(w, "Book ID is required", http.StatusBadRequest)
		return
	}
	if err := s.cvCache.DismissRecheck(bookID); err != nil {
		http.Error(w, "Failed to dismiss suggestion", http.StatusInternalServerError)
		return
	}
	s.InvalidateWorkflowCache()
	s.writeJSON(w, http.StatusOK, map[string]any{"book_id": bookID, "dismissed": true})
}
