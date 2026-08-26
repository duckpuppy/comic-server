package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/trash"
)

func newTrashTestServer(t *testing.T, trashPath string) *Server {
	t.Helper()
	return &Server{
		config: &config.Config{
			Server: config.ServerConfig{
				TrashPath:          trashPath,
				TrashRetentionDays: 30,
			},
		},
		configPath: filepath.Join(t.TempDir(), "config.yaml"),
	}
}

func TestHandleListTrash_NotConfiguredReturns503(t *testing.T) {
	s := newTrashTestServer(t, "")

	req := httptest.NewRequest(http.MethodGet, "/api/trash", nil)
	w := httptest.NewRecorder()
	s.handleListTrash(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListTrash_MethodNotAllowed(t *testing.T) {
	s := newTrashTestServer(t, t.TempDir())

	req := httptest.NewRequest(http.MethodPost, "/api/trash", nil)
	w := httptest.NewRecorder()
	s.handleListTrash(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleListTrash_ReturnsEntries(t *testing.T) {
	trashDir := t.TempDir()
	libDir := t.TempDir()
	s := newTrashTestServer(t, trashDir)

	target := filepath.Join(libDir, "book.cbz")
	if err := os.WriteFile(target, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	tr, err := trash.New(trashDir, 30)
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.Quarantine(target); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/trash", nil)
	w := httptest.NewRecorder()
	s.handleListTrash(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Entries []TrashEntryResponse `json:"entries"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(resp.Entries))
	}
	if resp.Entries[0].OriginalPath != target {
		t.Errorf("expected OriginalPath %s, got %s", target, resp.Entries[0].OriginalPath)
	}
	if resp.Entries[0].Size != int64(len("content")) {
		t.Errorf("expected size %d, got %d", len("content"), resp.Entries[0].Size)
	}
}

func TestHandlePostTrashRestore_MethodNotAllowed(t *testing.T) {
	s := newTrashTestServer(t, t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "/api/trash/restore", nil)
	w := httptest.NewRecorder()
	s.handlePostTrashRestore(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandlePostTrashRestore_EmptyIDsReturns400(t *testing.T) {
	s := newTrashTestServer(t, t.TempDir())

	req := httptest.NewRequest(http.MethodPost, "/api/trash/restore", bytes.NewBufferString(`{"ids":[]}`))
	w := httptest.NewRecorder()
	s.handlePostTrashRestore(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandlePostTrashRestore_RestoresAndReportsErrors(t *testing.T) {
	trashDir := t.TempDir()
	libDir := t.TempDir()
	s := newTrashTestServer(t, trashDir)

	target := filepath.Join(libDir, "book.cbz")
	if err := os.WriteFile(target, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	tr, err := trash.New(trashDir, 30)
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.Quarantine(target); err != nil {
		t.Fatal(err)
	}
	entries, err := tr.List()
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d, err=%v", len(entries), err)
	}

	body, _ := json.Marshal(TrashRestoreRequest{IDs: []string{entries[0].ID, "does/not/exist.cbz~123"}})
	req := httptest.NewRequest(http.MethodPost, "/api/trash/restore", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handlePostTrashRestore(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result TrashRestoreResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Restored != 1 {
		t.Errorf("expected 1 restored, got %d", result.Restored)
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error for the unknown id, got %d: %v", len(result.Errors), result.Errors)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("expected restored file to exist at %s: %v", target, err)
	}
}
