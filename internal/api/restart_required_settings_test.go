package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
)

func newRestartRequiredSettingsTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := &config.Config{
		Server: config.ServerConfig{
			LibraryPath:            "/library/ComicDb.xml",
			ServerPort:             7620,
			DiscoveryPort:          7615,
			LogLevel:               "info",
			LogFormat:              "text",
			RateLimitWindowSeconds: 60,
		},
	}
	s := &Server{config: cfg, configPath: filepath.Join(t.TempDir(), "config.yaml")}
	s.SetActiveRestartRequiredSettings(cfg)
	return s
}

func TestHandleGetRestartRequiredSettings_NotRequiredBeforeAnyChange(t *testing.T) {
	s := newRestartRequiredSettingsTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/settings/restart-required", nil)
	w := httptest.NewRecorder()
	s.handleRestartRequiredSettings(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got RestartRequiredSettingsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.RestartRequired {
		t.Error("expected restart_required=false when saved matches active")
	}
	if got.Saved.LibraryPath != "/library/ComicDb.xml" || got.Saved.ServerPort != 7620 {
		t.Errorf("Saved = %+v, want it to reflect the starting config", got.Saved)
	}
}

func TestHandlePutRestartRequiredSettings_PersistsAndFlagsRestartRequired(t *testing.T) {
	s := newRestartRequiredSettingsTestServer(t)

	body, _ := json.Marshal(restartRequiredSettingsPutRequest{
		LibraryPath:            "/new/ComicDb.xml",
		ServerPort:             8080,
		DiscoveryPort:          7615,
		RateLimitWindowSeconds: 60,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/restart-required", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleRestartRequiredSettings(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got RestartRequiredSettingsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.RestartRequired {
		t.Error("expected restart_required=true once Saved diverges from Active")
	}
	if got.Saved.LibraryPath != "/new/ComicDb.xml" || got.Saved.ServerPort != 8080 {
		t.Errorf("Saved = %+v, want the just-PUT values", got.Saved)
	}
	if got.Active.LibraryPath != "/library/ComicDb.xml" || got.Active.ServerPort != 7620 {
		t.Errorf("Active = %+v, want the untouched startup snapshot", got.Active)
	}

	// In-memory config must reflect the change immediately (GET .../lists
	// etc. would see the new LibraryPath), even though the running
	// backend/listeners don't rebuild until restart.
	if s.config.Server.LibraryPath != "/new/ComicDb.xml" {
		t.Errorf("s.config.Server.LibraryPath = %q, want the new value", s.config.Server.LibraryPath)
	}

	// And config.yaml on disk must have the durable copy.
	saved, err := config.Load(s.configPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if saved.Server.LibraryPath != "/new/ComicDb.xml" || saved.Server.ServerPort != 8080 {
		t.Errorf("saved config.yaml = %+v, want the new values persisted", saved.Server)
	}
}

// TestHandlePutRestartRequiredSettings_EmptyAPIKeyLeavesExistingKeyAlone
// covers the write-only API key contract: GET never returns the actual
// key (only *Set booleans), so a PUT that re-submits the form with an
// empty key field must NOT wipe out a key that was already configured.
func TestHandlePutRestartRequiredSettings_EmptyAPIKeyLeavesExistingKeyAlone(t *testing.T) {
	s := newRestartRequiredSettingsTestServer(t)
	s.config.Server.ComicVineAPIKey = "existing-cv-key"
	s.config.Server.Komga.APIKey = "existing-komga-key"

	body, _ := json.Marshal(restartRequiredSettingsPutRequest{
		ServerPort:             7620,
		DiscoveryPort:          7615,
		RateLimitWindowSeconds: 60,
		// ComicVineAPIKey and KomgaAPIKey both left empty.
	})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/restart-required", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleRestartRequiredSettings(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if s.config.Server.ComicVineAPIKey != "existing-cv-key" {
		t.Errorf("ComicVineAPIKey = %q, want unchanged", s.config.Server.ComicVineAPIKey)
	}
	if s.config.Server.Komga.APIKey != "existing-komga-key" {
		t.Errorf("Komga.APIKey = %q, want unchanged", s.config.Server.Komga.APIKey)
	}

	var got RestartRequiredSettingsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Saved.ComicVineAPIKeySet || !got.Saved.KomgaAPIKeySet {
		t.Errorf("Saved = %+v, want both *Set flags true (key preserved)", got.Saved)
	}
}

func TestHandlePutRestartRequiredSettings_NonEmptyAPIKeyReplacesExisting(t *testing.T) {
	s := newRestartRequiredSettingsTestServer(t)
	s.config.Server.ComicVineAPIKey = "old-key"

	body, _ := json.Marshal(restartRequiredSettingsPutRequest{
		ServerPort:             7620,
		DiscoveryPort:          7615,
		RateLimitWindowSeconds: 60,
		ComicVineAPIKey:        "new-key",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/restart-required", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleRestartRequiredSettings(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if s.config.Server.ComicVineAPIKey != "new-key" {
		t.Errorf("ComicVineAPIKey = %q, want %q", s.config.Server.ComicVineAPIKey, "new-key")
	}
}

func TestHandlePutRestartRequiredSettings_InvalidPortRejected(t *testing.T) {
	s := newRestartRequiredSettingsTestServer(t)

	body, _ := json.Marshal(restartRequiredSettingsPutRequest{
		ServerPort:    99999,
		DiscoveryPort: 7615,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/restart-required", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleRestartRequiredSettings(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	// Rejected candidate must never have mutated the live config.
	if s.config.Server.ServerPort != 7620 {
		t.Errorf("ServerPort = %d, want unchanged at 7620 after a rejected PUT", s.config.Server.ServerPort)
	}
}

func TestHandleRestartRequiredSettings_MethodNotAllowed(t *testing.T) {
	s := newRestartRequiredSettingsTestServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/settings/restart-required", nil)
	w := httptest.NewRecorder()
	s.handleRestartRequiredSettings(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}
