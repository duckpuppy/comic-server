package api

import (
	"net/http"
	"time"

	"github.com/duckpuppy/comic-server/internal/comicvine"
)

// Pending tasks (tracker #29, first step): a read-only view of what is waiting
// for ComicVine, plus Cancel for a scrape job. There is no queue behind it:
// requests still run (and wait) exactly as before, the pacer just lists who is
// paused right now.

// CVTaskWaiter is one caller paused by the pacer.
type CVTaskWaiter struct {
	Label        string `json:"label"`
	Background   bool   `json:"background"`
	Reason       string `json:"reason"`        // "burst", "window" or "hourly"
	WaitingFor   int    `json:"waiting_for"`   // seconds it has waited so far
	WaitSeconds  int    `json:"wait_seconds"`  // estimate: seconds until it may go
	EstimateOnly bool   `json:"estimate_only"` // always true: later callers can still change it
}

// CVTasksResponse is the answer of GET /api/comicvine/tasks.
type CVTasksResponse struct {
	Pacing  CVPacingResponse `json:"pacing"`
	Waiters []CVTaskWaiter   `json:"waiters"`
	// Scrape is the running or last scrape job, if any.
	Scrape        *comicvine.ScrapeJob `json:"scrape,omitempty"`
	ScrapeRunning bool                 `json:"scrape_running"`
}

// handleCVTasks GET /api/comicvine/tasks
func (s *Server) handleCVTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	now := time.Now()
	resp := CVTasksResponse{Pacing: cvPacingNow(), Waiters: []CVTaskWaiter{}}
	for _, wt := range comicvine.SharedWaiters() {
		resp.Waiters = append(resp.Waiters, CVTaskWaiter{
			Label: wt.Label, Background: wt.Background, Reason: wt.Reason,
			WaitingFor:   max(int(now.Sub(wt.Since).Seconds()), 0),
			WaitSeconds:  max(int(wt.RunsAt.Sub(now).Seconds())+1, 0),
			EstimateOnly: true,
		})
	}
	if s.scraper != nil {
		if job := s.scraper.CurrentJob(); job != nil {
			copied := *job
			resp.Scrape = &copied
			resp.ScrapeRunning = copied.Status == comicvine.JobStatusRunning
		}
	}
	s.writeJSON(w, http.StatusOK, resp)
}

// handleScrapeCancel stops the running scrape job. Books it already finished
// keep their results. POST /api/scrape/cancel
func (s *Server) handleScrapeCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.scrapeCancelMu.Lock()
	cancel := s.scrapeCancel
	s.scrapeCancelMu.Unlock()
	if cancel == nil {
		http.Error(w, "No scrape job is running", http.StatusConflict)
		return
	}
	cancel(comicvine.ErrJobCancelled)
	s.writeJSON(w, http.StatusAccepted, map[string]string{"status": "cancelling"})
}
