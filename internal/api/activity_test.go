package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBeginActivity_ShowsProgressAndClearsOnEnd(t *testing.T) {
	s := &Server{}
	progress, end := s.beginActivity("convert_cbz", 10)

	got := s.currentActivities()
	if len(got) != 1 || got[0].Stage != "convert_cbz" || got[0].Total != 10 || got[0].Done != 0 {
		t.Fatalf("after begin: %+v", got)
	}
	progress(4)
	if got := s.currentActivities(); got[0].Done != 4 {
		t.Errorf("done = %d, want 4", got[0].Done)
	}
	end()
	if got := s.currentActivities(); len(got) != 0 {
		t.Errorf("after end: %+v, want none", got)
	}
}

func TestBeginActivity_EarlierRunEndingDoesNotClearALaterOne(t *testing.T) {
	s := &Server{}
	_, endFirst := s.beginActivity("write_metadata", 5)
	_, endSecond := s.beginActivity("write_metadata", 8)
	endFirst()
	got := s.currentActivities()
	if len(got) != 1 || got[0].Total != 8 {
		t.Fatalf("after the first run ended: %+v, want the second run still listed", got)
	}
	endSecond()
	if len(s.currentActivities()) != 0 {
		t.Error("second run did not clear")
	}
}

func TestHandleWorkflowActivity_ListsRunningStages(t *testing.T) {
	s := &Server{}
	_, end := s.beginActivity("scan_info", 3)
	defer end()

	w := httptest.NewRecorder()
	s.handleWorkflowActivity(w, httptest.NewRequest(http.MethodGet, "/api/library/workflow/activity", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var body struct {
		Activities []stageActivity `json:"activities"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || len(body.Activities) != 1 || body.Activities[0].Stage != "scan_info" {
		t.Errorf("body = %s (%v)", w.Body.String(), err)
	}

	w = httptest.NewRecorder()
	s.handleWorkflowActivity(w, httptest.NewRequest(http.MethodPost, "/api/library/workflow/activity", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status %d, want 405", w.Code)
	}
}
