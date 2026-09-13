package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
)

func newServerMiscSettingsTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{config: &config.Config{}, configDB: newTestConfigDB(t)}
}

func TestHandleGetServerMiscSettings_FallsBackToInMemoryConfig(t *testing.T) {
	s := newServerMiscSettingsTestServer(t)
	s.config.Server.CBZConvert.Enabled = true
	s.config.Server.IgnoreDevices = []string{"192.168.0.24"}

	req := httptest.NewRequest(http.MethodGet, "/api/settings/server-misc", nil)
	w := httptest.NewRecorder()
	s.handleServerMiscSettings(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got ServerMiscSettingsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.CBZConvertEnabled || len(got.IgnoreDevices) != 1 || got.IgnoreDevices[0] != "192.168.0.24" {
		t.Errorf("got %+v, want the in-memory fallback values", got)
	}
}

func TestHandlePutServerMiscSettings_PersistsAndAppliesLive(t *testing.T) {
	s := newServerMiscSettingsTestServer(t)

	body, _ := json.Marshal(ServerMiscSettingsResponse{CBZConvertEnabled: true, IgnoreDevices: []string{"SM-T970"}})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/server-misc", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleServerMiscSettings(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// In-memory config must reflect the change immediately (no restart).
	if !s.config.Server.CBZConvert.Enabled {
		t.Error("expected s.config.Server.CBZConvert.Enabled to be updated live")
	}
	if len(s.config.Server.IgnoreDevices) != 1 || s.config.Server.IgnoreDevices[0] != "SM-T970" {
		t.Errorf("s.config.Server.IgnoreDevices = %v, want [SM-T970]", s.config.Server.IgnoreDevices)
	}

	// And config.db must have the durable copy.
	stored, err := s.configDB.GetServerMiscSettings()
	if err != nil {
		t.Fatalf("GetServerMiscSettings: %v", err)
	}
	if stored == nil || !stored.CBZConvertEnabled || len(stored.IgnoreDevices) != 1 {
		t.Errorf("stored settings = %+v, want persisted", stored)
	}

	// A subsequent GET must reflect config.db, not the (now stale) in-memory fallback.
	getReq := httptest.NewRequest(http.MethodGet, "/api/settings/server-misc", nil)
	getW := httptest.NewRecorder()
	s.handleServerMiscSettings(getW, getReq)
	var got ServerMiscSettingsResponse
	if err := json.Unmarshal(getW.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.CBZConvertEnabled || len(got.IgnoreDevices) != 1 || got.IgnoreDevices[0] != "SM-T970" {
		t.Errorf("GET after PUT = %+v, want the saved values", got)
	}
}

func TestHandleServerMiscSettings_ConfigDBUnavailable(t *testing.T) {
	s := &Server{config: &config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/api/settings/server-misc", nil)
	w := httptest.NewRecorder()
	s.handleServerMiscSettings(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}
