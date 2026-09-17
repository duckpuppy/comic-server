package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
)

func newWatchFoldersTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{config: &config.Config{}, configDB: newTestConfigDB(t)}
}

func TestHandleGetWatchFoldersSettings_FallsBackToInMemoryConfig(t *testing.T) {
	s := newWatchFoldersTestServer(t)
	s.config.Server.WatchFolders = []string{"/dump/incoming"}

	req := httptest.NewRequest(http.MethodGet, "/api/settings/watch-folders", nil)
	w := httptest.NewRecorder()
	s.handleWatchFoldersSettings(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got WatchFoldersResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Folders) != 1 || got.Folders[0] != "/dump/incoming" {
		t.Errorf("got %+v, want the in-memory fallback value", got)
	}
}

func TestHandlePutWatchFoldersSettings_PersistsAndAppliesLive(t *testing.T) {
	s := newWatchFoldersTestServer(t)

	body, _ := json.Marshal(WatchFoldersResponse{Folders: []string{"/comics/incoming", "/comics/staging"}})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/watch-folders", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleWatchFoldersSettings(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// In-memory config must reflect the change immediately (no restart).
	if len(s.config.Server.WatchFolders) != 2 {
		t.Errorf("s.config.Server.WatchFolders = %v, want 2 entries updated live", s.config.Server.WatchFolders)
	}

	// And config.db must have the durable copy.
	stored, err := s.configDB.GetWatchFolders()
	if err != nil {
		t.Fatalf("GetWatchFolders: %v", err)
	}
	if stored == nil || len(stored.Folders) != 2 {
		t.Errorf("stored watch folders = %+v, want persisted", stored)
	}

	// A subsequent GET must reflect config.db, not the (now stale) in-memory fallback.
	getReq := httptest.NewRequest(http.MethodGet, "/api/settings/watch-folders", nil)
	getW := httptest.NewRecorder()
	s.handleWatchFoldersSettings(getW, getReq)
	var got WatchFoldersResponse
	if err := json.Unmarshal(getW.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Folders) != 2 || got.Folders[0] != "/comics/incoming" {
		t.Errorf("GET after PUT = %+v, want the saved values", got)
	}
}

func TestHandleWatchFoldersSettings_ConfigDBUnavailable(t *testing.T) {
	s := &Server{config: &config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/api/settings/watch-folders", nil)
	w := httptest.NewRecorder()
	s.handleWatchFoldersSettings(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleWatchFoldersSettings_MethodNotAllowed(t *testing.T) {
	s := newWatchFoldersTestServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/settings/watch-folders", nil)
	w := httptest.NewRecorder()
	s.handleWatchFoldersSettings(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}
