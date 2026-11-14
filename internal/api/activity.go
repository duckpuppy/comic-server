package api

import (
	"net/http"
	"sort"
	"time"

	"github.com/duckpuppy/comic-server/internal/comicvine"
)

// stageActivity is one pipeline action that is running now: which stage, how
// many books it has finished and how many it has in all (tracker #73). The
// workflow page polls it so a stage shows it is working, with a counter, even
// after the page was reloaded or the run was started from another tab.
type stageActivity struct {
	Stage   string    `json:"stage"`
	Done    int       `json:"done"`
	Total   int       `json:"total"`
	Started time.Time `json:"started"`
}

// beginActivity records that stage started a run over total books. It returns
// a progress function (call it with the number of books finished so far) and an
// end function (always call it, with defer). Two runs of the same stage at once
// share one entry; the later one wins until it ends.
func (s *Server) beginActivity(stage string, total int) (progress func(done int), end func()) {
	act := &stageActivity{Stage: stage, Total: total, Started: time.Now()}
	s.activityMu.Lock()
	if s.activities == nil {
		s.activities = make(map[string]*stageActivity)
	}
	s.activities[stage] = act
	s.activityMu.Unlock()

	progress = func(done int) {
		s.activityMu.Lock()
		act.Done = done
		s.activityMu.Unlock()
	}
	end = func() {
		s.activityMu.Lock()
		if s.activities[stage] == act {
			delete(s.activities, stage)
		}
		s.activityMu.Unlock()
	}
	return progress, end
}

// currentActivities lists the running actions, plus the scrape job when one is
// running, sorted by stage so the answer is stable.
func (s *Server) currentActivities() []stageActivity {
	s.activityMu.Lock()
	out := make([]stageActivity, 0, len(s.activities)+1)
	for _, a := range s.activities {
		out = append(out, *a)
	}
	s.activityMu.Unlock()

	if s.scraper != nil {
		if job := s.scraper.CurrentJob(); job != nil && job.Status == comicvine.JobStatusRunning {
			done := job.Completed + job.Skipped + job.Failed + job.PendingReview
			out = append(out, stageActivity{Stage: "scrape", Done: done, Total: job.Total, Started: job.StartedAt})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Stage < out[j].Stage })
	return out
}

// handleWorkflowActivity serves GET /api/library/workflow/activity.
func (s *Server) handleWorkflowActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"activities": s.currentActivities()})
}
