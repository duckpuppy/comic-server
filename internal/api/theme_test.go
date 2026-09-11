package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newThemeTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{configDB: newTestConfigDB(t)}
}

func TestHandleGetTheme_DefaultsToSystem(t *testing.T) {
	server := newThemeTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/settings/theme", nil)
	w := httptest.NewRecorder()
	server.handleThemeConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got ThemeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Theme != "system" {
		t.Errorf("Theme = %q, want %q", got.Theme, "system")
	}
}

func TestHandlePutTheme_SavesAndRoundTrips(t *testing.T) {
	server := newThemeTestServer(t)

	body, _ := json.Marshal(ThemeResponse{Theme: "dark"})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/theme", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleThemeConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/settings/theme", nil)
	w2 := httptest.NewRecorder()
	server.handleThemeConfig(w2, req2)

	var got ThemeResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Theme != "dark" {
		t.Errorf("Theme after PUT = %q, want %q", got.Theme, "dark")
	}
}

func TestHandlePutTheme_RejectsInvalidValue(t *testing.T) {
	server := newThemeTestServer(t)

	body, _ := json.Marshal(ThemeResponse{Theme: "blue"})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/theme", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleThemeConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for an invalid theme, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleThemeConfig_MethodNotAllowed(t *testing.T) {
	server := newThemeTestServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/settings/theme", nil)
	w := httptest.NewRecorder()
	server.handleThemeConfig(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}
