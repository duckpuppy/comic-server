package api

import (
	"net/http"
	"time"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

const (
	throughputDays   = 14
	throughputRecent = 8
)

// ThroughputDay is one calendar day of stage advances.
type ThroughputDay struct {
	Date  string `json:"date"` // YYYY-MM-DD, server local time
	Count int    `json:"count"`
}

// ThroughputRecent is a book that advanced recently, for the cover strip.
type ThroughputRecent struct {
	BookID string `json:"book_id"`
	Series string `json:"series"`
	Number string `json:"number"`
}

// Throughput is the response for GET /api/library/workflow/throughput.
// A stage advance is one audit row where the stage moved forward; a book
// moving two stages counts twice, a rollback counts nothing.
type Throughput struct {
	Days     []ThroughputDay    `json:"days"`      // oldest first, 14 entries
	ThisWeek int                `json:"this_week"` // last 7 days incl. today
	LastWeek int                `json:"last_week"` // the 7 days before that
	Recent   []ThroughputRecent `json:"recent"`
}

// buildThroughput folds stage moves into per-day counts ending at now.
// Pure so it is table-testable.
func buildThroughput(moves []configdb.StageMove, now time.Time) Throughput {
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	t := Throughput{Days: make([]ThroughputDay, throughputDays), Recent: []ThroughputRecent{}}
	idx := map[string]int{}
	for i := range t.Days {
		d := today.AddDate(0, 0, i-(throughputDays-1))
		t.Days[i].Date = d.Format("2006-01-02")
		idx[t.Days[i].Date] = i
	}
	seen := map[string]bool{}
	for _, m := range moves { // newest first
		before := workflow.StageIndex(workflow.ParseStage(m.StageBefore))
		after := workflow.StageIndex(workflow.ParseStage(m.StageAfter))
		if before == 0 || after <= before {
			continue
		}
		if i, ok := idx[m.At.In(loc).Format("2006-01-02")]; ok {
			t.Days[i].Count++
		}
		if len(t.Recent) < throughputRecent && !seen[m.BookID] {
			seen[m.BookID] = true
			t.Recent = append(t.Recent, ThroughputRecent{BookID: m.BookID, Series: m.Series, Number: m.Number})
		}
	}
	for i, d := range t.Days {
		if i >= throughputDays-7 {
			t.ThisWeek += d.Count
		} else {
			t.LastWeek += d.Count
		}
	}
	return t
}

// handleThroughput serves the workflow hero's sparkline and cover strip,
// derived from the audit log (comic-server-p7ti) - no extra table.
// GET /api/library/workflow/throughput
func (s *Server) handleThroughput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.configDB == nil {
		http.Error(w, "Config database not available", http.StatusServiceUnavailable)
		return
	}
	now := time.Now()
	since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(throughputDays - 1))
	moves, err := s.configDB.ListStageMoves(since)
	if err != nil {
		http.Error(w, "Failed to load throughput", http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, http.StatusOK, buildThroughput(moves, now))
}
