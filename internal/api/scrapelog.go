package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	ws "github.com/duckpuppy/comic-server/internal/websocket"
)

// Live scrape log (comic-server-5wer): each job's steps are kept in memory
// for the last few jobs and pushed over the WebSocket, so the Re-scrape
// dialog can show a backlog when it opens and follow along after.
const (
	scrapeLogMaxLines = 2000 // per job; the oldest go first
	scrapeLogMaxJobs  = 10
)

type scrapeLogLine struct {
	Seq   int       `json:"seq"`
	Time  time.Time `json:"time"`
	Level string    `json:"level"`
	Msg   string    `json:"msg"`
}

type scrapeLogStore struct {
	mu    sync.Mutex
	jobs  map[string][]scrapeLogLine
	seqs  map[string]int
	order []string // job ids, oldest first
}

func newScrapeLogStore() *scrapeLogStore {
	return &scrapeLogStore{jobs: map[string][]scrapeLogLine{}, seqs: map[string]int{}}
}

func (st *scrapeLogStore) add(jobID, level, msg string) scrapeLogLine {
	st.mu.Lock()
	defer st.mu.Unlock()
	if _, ok := st.seqs[jobID]; !ok {
		st.order = append(st.order, jobID)
		for len(st.order) > scrapeLogMaxJobs {
			delete(st.jobs, st.order[0])
			delete(st.seqs, st.order[0])
			st.order = st.order[1:]
		}
	}
	st.seqs[jobID]++
	line := scrapeLogLine{Seq: st.seqs[jobID], Time: time.Now().UTC(), Level: level, Msg: msg}
	lines := append(st.jobs[jobID], line)
	if len(lines) > scrapeLogMaxLines {
		lines = append([]scrapeLogLine(nil), lines[len(lines)-scrapeLogMaxLines:]...)
	}
	st.jobs[jobID] = lines
	return line
}

func (st *scrapeLogStore) lines(jobID string) []scrapeLogLine {
	if st == nil {
		return []scrapeLogLine{}
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return append([]scrapeLogLine{}, st.jobs[jobID]...)
}

// recordScrapeLog is the scraper's job log sink.
func (s *Server) recordScrapeLog(jobID, level, msg string) {
	line := s.scrapeLogs.add(jobID, level, msg)
	if s.wsHub != nil {
		s.wsHub.Broadcast(ws.EventScrapeLog, map[string]any{
			"job_id": jobID, "seq": line.Seq, "time": line.Time, "level": line.Level, "msg": line.Msg,
		})
	}
}

// handleScrapeLog returns a job's log so far. GET /api/scrape/log?job_id=
func (s *Server) handleScrapeLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.scraper == nil {
		http.Error(w, "ComicVine scraper is not configured", http.StatusServiceUnavailable)
		return
	}
	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		http.Error(w, "job_id is required", http.StatusBadRequest)
		return
	}
	running := false
	if job := s.scraper.CurrentJob(); job != nil && job.ID == jobID {
		running = job.Status == comicvine.JobStatusRunning
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"job_id": jobID, "running": running, "lines": s.scrapeLogs.lines(jobID),
	})
}
