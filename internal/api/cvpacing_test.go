package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleCVPacing(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	s.handleCVPacing(w, httptest.NewRequest(http.MethodGet, "/api/comicvine/pacing", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"configured", "requests_last_hour", "next_request_seconds", "circuit", "burst_size"} {
		if _, ok := out[k]; !ok {
			t.Errorf("missing %q in %v", k, out)
		}
	}
	w = httptest.NewRecorder()
	s.handleCVPacing(w, httptest.NewRequest(http.MethodPost, "/api/comicvine/pacing", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want 405", w.Code)
	}
}
