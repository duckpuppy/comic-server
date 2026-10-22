package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duckpuppy/comic-server/internal/comicvine"
)

func TestCVTasks_EmptyWithoutAClient(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	s.handleCVTasks(w, httptest.NewRequest(http.MethodGet, "/api/comicvine/tasks", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var resp CVTasksResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Waiters == nil || len(resp.Waiters) != 0 || resp.ScrapeRunning || resp.Scrape != nil {
		t.Errorf("resp = %+v", resp)
	}
	w = httptest.NewRecorder()
	s.handleCVTasks(w, httptest.NewRequest(http.MethodPost, "/api/comicvine/tasks", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", w.Code)
	}
}

func TestScrapeCancel(t *testing.T) {
	s := &Server{}
	post := func() int {
		w := httptest.NewRecorder()
		s.handleScrapeCancel(w, httptest.NewRequest(http.MethodPost, "/api/scrape/cancel", nil))
		return w.Code
	}
	if code := post(); code != http.StatusConflict {
		t.Errorf("nothing running: %d, want 409", code)
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	s.scrapeCancelMu.Lock()
	s.scrapeCancel = cancel
	s.scrapeCancelMu.Unlock()
	if code := post(); code != http.StatusAccepted {
		t.Fatalf("running: %d, want 202", code)
	}
	if ctx.Err() == nil || context.Cause(ctx) != comicvine.ErrJobCancelled {
		t.Errorf("job context cause = %v, want ErrJobCancelled", context.Cause(ctx))
	}

	w := httptest.NewRecorder()
	s.handleScrapeCancel(w, httptest.NewRequest(http.MethodGet, "/api/scrape/cancel", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", w.Code)
	}
}
