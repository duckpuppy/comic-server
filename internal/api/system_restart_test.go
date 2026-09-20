package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleSystemRestart_PostReturns202AndCallsFuncOnce(t *testing.T) {
	s := &Server{}
	calls := 0
	s.SetRestartFunc(func() error { calls++; return nil })

	req := httptest.NewRequest(http.MethodPost, "/api/system/restart", nil)
	w := httptest.NewRecorder()
	s.handleSystemRestart(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	if calls != 1 {
		t.Errorf("restart func called %d times, want exactly 1", calls)
	}
}

func TestHandleSystemRestart_RespondsBeforeRestartFuncRuns(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	var codeAtCall int
	s.SetRestartFunc(func() error {
		// The response must already be written when the restart is requested.
		codeAtCall = w.Code
		return nil
	})

	s.handleSystemRestart(w, httptest.NewRequest(http.MethodPost, "/api/system/restart", nil))

	if codeAtCall != http.StatusAccepted {
		t.Errorf("status at restart-func call time = %d, want 202 already written", codeAtCall)
	}
}

func TestHandleSystemRestart_NonPostReturns405(t *testing.T) {
	s := &Server{}
	calls := 0
	s.SetRestartFunc(func() error { calls++; return nil })

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		w := httptest.NewRecorder()
		s.handleSystemRestart(w, httptest.NewRequest(method, "/api/system/restart", nil))
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: expected 405, got %d", method, w.Code)
		}
	}
	if calls != 0 {
		t.Errorf("restart func called %d times on non-POST requests, want 0", calls)
	}
}

func TestHandleSystemRestart_NoRestartFuncReturns501(t *testing.T) {
	s := &Server{}

	w := httptest.NewRecorder()
	s.handleSystemRestart(w, httptest.NewRequest(http.MethodPost, "/api/system/restart", nil))

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetRestartRequiredSettings_ReportsRestartSupported(t *testing.T) {
	s := newRestartRequiredSettingsTestServer(t)

	get := func() bool {
		w := httptest.NewRecorder()
		s.handleRestartRequiredSettings(w, httptest.NewRequest(http.MethodGet, "/api/settings/restart-required", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var got RestartRequiredSettingsResponse
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return got.RestartSupported
	}

	if get() {
		t.Error("restart_supported = true with no restart func wired, want false")
	}
	s.SetRestartFunc(func() error { return nil })
	if !get() {
		t.Error("restart_supported = false with a restart func wired, want true")
	}
}
