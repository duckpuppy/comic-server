package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
)

func newScanInfoConfigTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{
		config:   &config.Config{},
		configDB: newTestConfigDB(t),
	}
}

func TestHandleGetScanInfoConfig_FallsBackToConfigYAML(t *testing.T) {
	server := newScanInfoConfigTestServer(t)
	server.config.Server.ScanInfo = config.ScanInfoConfig{
		Enabled:  true,
		Scanners: []string{"DCP"},
		Prefix:   "Scanner:",
		Unknown:  "Unknown",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/settings/scan-info", nil)
	w := httptest.NewRecorder()
	server.handleScanInfoConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got ScanInfoConfigResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !got.Enabled || len(got.Scanners) != 1 || got.Scanners[0] != "DCP" {
		t.Errorf("expected config.yaml fallback value, got %+v", got)
	}
}

func TestHandleGetScanInfoConfig_PrefersConfigDB(t *testing.T) {
	server := newScanInfoConfigTestServer(t)
	server.config.Server.ScanInfo = config.ScanInfoConfig{
		Enabled:  true,
		Scanners: []string{"FromYAML"},
	}
	if err := server.configDB.UpsertScanInfo(config.ScanInfoConfig{
		Enabled:  true,
		Scanners: []string{"FromConfigDB"},
	}); err != nil {
		t.Fatalf("failed to seed config.db: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/settings/scan-info", nil)
	w := httptest.NewRecorder()
	server.handleScanInfoConfig(w, req)

	var got ScanInfoConfigResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got.Scanners) != 1 || got.Scanners[0] != "FromConfigDB" {
		t.Errorf("expected config.db value to take priority over config.yaml, got %+v", got)
	}
}

func TestHandlePutScanInfoConfig_SavesAndReturnsConfig(t *testing.T) {
	server := newScanInfoConfigTestServer(t)

	body, _ := json.Marshal(ScanInfoConfigResponse{
		Enabled:   true,
		Scanners:  []string{"DCP", "Empire"},
		Blacklist: []string{"digital"},
		Prefix:    "Scanner:",
		Unknown:   "Unknown",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/scan-info", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleScanInfoConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	stored, err := server.configDB.GetScanInfo()
	if err != nil {
		t.Fatalf("GetScanInfo failed: %v", err)
	}
	if stored == nil || len(stored.Scanners) != 2 {
		t.Errorf("expected the PUT to persist to config.db, got %+v", stored)
	}
}

func TestHandlePutScanInfoConfig_RejectsEnabledWithNoScanners(t *testing.T) {
	server := newScanInfoConfigTestServer(t)

	body, _ := json.Marshal(ScanInfoConfigResponse{Enabled: true})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/scan-info", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleScanInfoConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for enabled=true with no scanners, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlePutScanInfoConfig_RejectsInvalidRegex(t *testing.T) {
	server := newScanInfoConfigTestServer(t)

	body, _ := json.Marshal(ScanInfoConfigResponse{
		Enabled:   true,
		Scanners:  []string{"DCP"},
		Blacklist: []string{"*invalid"}, // missing argument to repetition operator
	})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/scan-info", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleScanInfoConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for an invalid blacklist regex, got %d: %s", w.Code, w.Body.String())
	}

	stored, err := server.configDB.GetScanInfo()
	if err != nil {
		t.Fatalf("GetScanInfo failed: %v", err)
	}
	if stored != nil {
		t.Errorf("expected rejected config to not be persisted, got %+v", stored)
	}
}

func TestHandlePutScanInfoConfig_DisabledSkipsRegexValidation(t *testing.T) {
	server := newScanInfoConfigTestServer(t)

	// A disabled config with no scanners and even bad-regex-shaped
	// blacklist entries should still save - Validate() and the Detector
	// build only apply while enabled, matching handleRunScanInfo's own
	// enabled-gate (a disabled config never reaches NewDetector).
	body, _ := json.Marshal(ScanInfoConfigResponse{
		Enabled:   false,
		Blacklist: []string{"*invalid"},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/scan-info", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleScanInfoConfig(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for a disabled config, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleScanInfoConfig_WrongMethod(t *testing.T) {
	server := newScanInfoConfigTestServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/settings/scan-info", nil)
	w := httptest.NewRecorder()
	server.handleScanInfoConfig(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d: %s", w.Code, w.Body.String())
	}
}
