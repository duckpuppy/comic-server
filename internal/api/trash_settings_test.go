package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/configdb"
)

func newTrashSettingsTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{
		config:   &config.Config{},
		configDB: newTestConfigDB(t),
	}
}

func TestHandleGetTrashSettings_FallsBackToConfigYAML(t *testing.T) {
	server := newTrashSettingsTestServer(t)
	server.config.Server.TrashPath = "/legacy/trash"
	server.config.Server.TrashRetentionDays = 30

	req := httptest.NewRequest(http.MethodGet, "/api/settings/trash", nil)
	w := httptest.NewRecorder()
	server.handleTrashSettings(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got TrashSettingsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Path != "/legacy/trash" || got.RetentionDays != 30 {
		t.Errorf("got %+v, want the config.yaml fallback values", got)
	}
}

func TestHandlePutTrashSettings_OverridesConfigYAMLFallback(t *testing.T) {
	server := newTrashSettingsTestServer(t)
	server.config.Server.TrashPath = "/legacy/trash"
	server.config.Server.TrashRetentionDays = 30

	body, _ := json.Marshal(TrashSettingsResponse{Path: "/new/trash", RetentionDays: 14})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/trash", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleTrashSettings(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// The stored config.db value must now win over config.yaml's - proves
	// effectiveTrashConfig actually prefers config.db once something has
	// been saved through it (comic-server-4hsz).
	req = httptest.NewRequest(http.MethodGet, "/api/settings/trash", nil)
	w = httptest.NewRecorder()
	server.handleTrashSettings(w, req)
	var got TrashSettingsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Path != "/new/trash" || got.RetentionDays != 14 {
		t.Errorf("got %+v, want the config.db value to win", got)
	}
}

func TestHandlePutTrashSettings_RejectsNegativeRetention(t *testing.T) {
	server := newTrashSettingsTestServer(t)

	body, _ := json.Marshal(TrashSettingsResponse{Path: "/trash", RetentionDays: -1})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/trash", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleTrashSettings(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for negative retention, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlePutTrashSettings_AllowsEmptyPathToDisable(t *testing.T) {
	server := newTrashSettingsTestServer(t)

	body, _ := json.Marshal(TrashSettingsResponse{Path: "", RetentionDays: 30})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/trash", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleTrashSettings(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for an empty (disabling) path, got %d: %s", w.Code, w.Body.String())
	}
}

func TestEffectiveTrashConfig_PrefersConfigDB(t *testing.T) {
	server := newTrashSettingsTestServer(t)
	server.config.Server.TrashPath = "/legacy/trash"
	server.config.Server.TrashRetentionDays = 30

	if err := server.configDB.UpsertTrashSettings(configdb.TrashSettings{Path: "/db/trash", RetentionDays: 7}); err != nil {
		t.Fatalf("UpsertTrashSettings: %v", err)
	}

	path, retention, err := server.effectiveTrashConfig()
	if err != nil {
		t.Fatalf("effectiveTrashConfig: %v", err)
	}
	if path != "/db/trash" || retention != 7 {
		t.Errorf("effectiveTrashConfig = (%q, %d), want (/db/trash, 7)", path, retention)
	}
}

func TestNewTrashFromConfig_ErrorsWhenUnconfigured(t *testing.T) {
	server := newTrashSettingsTestServer(t)

	_, err := server.newTrashFromConfig()
	if err == nil {
		t.Fatal("expected an error when no trash path is configured anywhere")
	}
}
