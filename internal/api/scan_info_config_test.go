package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/scaninfo"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

func newScanInfoConfigTestServer(t *testing.T) *Server {
	t.Helper()
	resetScanInfoStage(t)
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

func TestEffectiveScanInfo_FillsEmptyListsWithDefaults(t *testing.T) {
	server := newScanInfoConfigTestServer(t)

	cfg, err := server.effectiveScanInfo()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Scanners) != len(scaninfo.DefaultScanners()) || len(cfg.Blacklist) != len(scaninfo.DefaultBlacklist()) {
		t.Fatalf("nothing configured: want the built-in lists, got %d scanners and %d blacklist", len(cfg.Scanners), len(cfg.Blacklist))
	}

	// A saved row with both lists empty has never been configured: same.
	if err := server.configDB.UpsertScanInfo(config.ScanInfoConfig{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	cfg, _ = server.effectiveScanInfo()
	if len(cfg.Scanners) == 0 {
		t.Error("an untouched saved row should still get the defaults")
	}
}

func TestEffectiveScanInfo_KeepsUserLists(t *testing.T) {
	server := newScanInfoConfigTestServer(t)
	if err := server.configDB.UpsertScanInfo(config.ScanInfoConfig{Scanners: []string{"Mine"}}); err != nil {
		t.Fatal(err)
	}
	cfg, _ := server.effectiveScanInfo()
	if len(cfg.Scanners) != 1 || cfg.Scanners[0] != "Mine" {
		t.Errorf("scanners = %v, want only the user's entry", cfg.Scanners)
	}
	if len(cfg.Blacklist) != 0 {
		t.Errorf("blacklist = %d entries, want the user's empty list kept when scanners were edited", len(cfg.Blacklist))
	}
}

func TestGetScanInfoConfig_ReturnsDefaultsForResetButton(t *testing.T) {
	server := newScanInfoConfigTestServer(t)
	w := httptest.NewRecorder()
	server.handleScanInfoConfig(w, httptest.NewRequest(http.MethodGet, "/api/settings/scan-info", nil))
	var got ScanInfoConfigResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.DefaultScanners) == 0 || len(got.DefaultBlacklist) == 0 {
		t.Errorf("default lists missing from the response: %d / %d", len(got.DefaultScanners), len(got.DefaultBlacklist))
	}
}

// Saving the setting turns the Scan Info stage off or on in the pipeline
// summary at once (tracker #70).
func TestPutScanInfoConfig_SkipsStageWhileOff(t *testing.T) {
	book := library.ComicBook{ID: "1", Series: "Batman", FilePath: "Batman 001.cbz"}
	workflow.SetStage(&book, workflow.StageScanInfo)
	s := newScanInfoTestServer(t, config.ScanInfoConfig{Enabled: true, Scanners: []string{"X"}}, []library.ComicBook{book})

	stageCounts := func() map[string]int {
		w := httptest.NewRecorder()
		s.handleGetWorkflowSummary(w, httptest.NewRequest(http.MethodGet, "/api/library/workflow", nil))
		var sum WorkflowSummary
		if err := json.Unmarshal(w.Body.Bytes(), &sum); err != nil {
			t.Fatal(err)
		}
		m := map[string]int{}
		for _, st := range sum.Stages {
			m[st.Stage] = st.Count
		}
		return m
	}
	put := func(enabled bool) {
		body, _ := json.Marshal(ScanInfoConfigResponse{Enabled: enabled, Scanners: []string{"X"}, Prefix: "Scanner:", Unknown: "Unknown"})
		w := httptest.NewRecorder()
		s.handleScanInfoConfig(w, httptest.NewRequest(http.MethodPut, "/api/settings/scan-info", bytes.NewReader(body)))
		if w.Code != http.StatusOK {
			t.Fatalf("PUT enabled=%v: %d %s", enabled, w.Code, w.Body.String())
		}
	}

	on := stageCounts()
	if on["scan_info"] != 1 {
		t.Fatalf("with the setting on, want the book at scan_info, got %v", on)
	}

	put(false)
	off := stageCounts()
	if _, present := off["scan_info"]; present {
		t.Errorf("with the setting off, the summary must not list scan_info: %v", off)
	}
	if off["data_manager"] != 1 {
		t.Errorf("with the setting off, the book should count under data_manager: %v", off)
	}

	put(true)
	back := stageCounts()
	if back["scan_info"] != 1 {
		t.Errorf("turning the setting back on should bring the book back to scan_info: %v", back)
	}
}
