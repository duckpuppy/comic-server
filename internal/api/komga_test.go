package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/device"
	"github.com/duckpuppy/comic-server/internal/komga"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/syncstate"
	"github.com/duckpuppy/comic-server/internal/websocket"
	"path/filepath"
)

func newKomgaTestServer(t *testing.T) *Server {
	t.Helper()
	lib := &library.ComicLibrary{}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	configDB, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("failed to open test config db: %v", err)
	}
	t.Cleanup(func() { configDB.Close() })
	return NewServer(syncstate.NewManager(10), device.NewRegistry(), backend, &config.Config{}, "", VersionInfo{}, websocket.NewHub(), configDB)
}

func TestHandleKomgaStatus_NotConfigured(t *testing.T) {
	srv := newKomgaTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/komga/status", nil)
	w := httptest.NewRecorder()
	srv.handleKomgaStatus(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestHandleKomgaStatus_MethodNotAllowed(t *testing.T) {
	srv := newKomgaTestServer(t)
	srv.SetKomgaStatus(komga.NewStatusStore())

	req := httptest.NewRequest(http.MethodPost, "/api/komga/status", nil)
	w := httptest.NewRecorder()
	srv.handleKomgaStatus(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleKomgaStatus_ReturnsSnapshot(t *testing.T) {
	srv := newKomgaTestServer(t)

	status := komga.NewStatusStore()
	status.Record(komga.TargetResult{
		Target:       komga.Target{ListID: "{list-1}", KomgaName: "Batman Collection", Type: komga.TargetCollection},
		MatchedCount: 3,
		Unmatched: []komga.UnmatchedBook{
			{Book: &library.ComicBook{ID: "1", Series: "Batman", FilePath: "/data/x.cbz"}, Reason: "not found"},
		},
	})
	srv.SetKomgaStatus(status)

	req := httptest.NewRequest(http.MethodGet, "/api/komga/status", nil)
	w := httptest.NewRecorder()
	srv.handleKomgaStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var snap komga.Snapshot
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(snap.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(snap.Targets))
	}
	if snap.Targets[0].KomgaName != "Batman Collection" || snap.Targets[0].MatchedCount != 3 {
		t.Errorf("unexpected target: %+v", snap.Targets[0])
	}
	if len(snap.Targets[0].Unmatched) != 1 || snap.Targets[0].Unmatched[0].BookID != "1" {
		t.Errorf("unexpected unmatched books: %+v", snap.Targets[0].Unmatched)
	}
}

func TestInvalidateListCache(t *testing.T) {
	cache := library.NewListCache(5 * time.Minute)
	cache.SetCount("list-1", 42)

	srv := newKomgaTestServer(t)
	srv.listCache = cache

	if _, found := cache.GetCount("list-1"); !found {
		t.Fatal("expected cache to have a count before invalidation")
	}

	srv.InvalidateListCache()

	if _, found := cache.GetCount("list-1"); found {
		t.Error("expected InvalidateListCache to clear cached counts, but one was still present")
	}
}
