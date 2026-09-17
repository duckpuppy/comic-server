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

// TestHandleListTrash_ComputesDeletesAt covers comic-server-ci31: each
// entry's deletes_at must be quarantined_at + the server's
// TrashRetentionDays (30, per newTrashTestServer), computed server-side
// so the UI never has to fetch retention_days separately or duplicate
// the day-math Sweep itself uses.
func TestHandleListTrash_ComputesDeletesAt(t *testing.T) {
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

	var resp struct {
		Entries []TrashEntryResponse `json:"entries"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(resp.Entries))
	}
	wantDeletesAt := resp.Entries[0].QuarantinedAt.AddDate(0, 0, 30)
	if !resp.Entries[0].DeletesAt.Equal(wantDeletesAt) {
		t.Errorf("DeletesAt = %v, want QuarantinedAt+30d = %v", resp.Entries[0].DeletesAt, wantDeletesAt)
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

func TestHandlePostTrashDelete_MethodNotAllowed(t *testing.T) {
	s := newTrashTestServer(t, t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "/api/trash/delete", nil)
	w := httptest.NewRecorder()
	s.handlePostTrashDelete(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandlePostTrashDelete_EmptyIDsReturns400(t *testing.T) {
	s := newTrashTestServer(t, t.TempDir())

	req := httptest.NewRequest(http.MethodPost, "/api/trash/delete", bytes.NewBufferString(`{"ids":[]}`))
	w := httptest.NewRecorder()
	s.handlePostTrashDelete(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestHandlePostTrashDelete_DeletesAndReportsErrors covers
// comic-server-2y3p: a valid id is permanently removed (unlike Restore,
// nothing is left behind to undo), and an unknown id is reported as a
// per-item error rather than failing the whole request.
func TestHandlePostTrashDelete_DeletesAndReportsErrors(t *testing.T) {
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

	body, _ := json.Marshal(TrashDeleteRequest{IDs: []string{entries[0].ID, "does/not/exist.cbz~123"}})
	req := httptest.NewRequest(http.MethodPost, "/api/trash/delete", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handlePostTrashDelete(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result TrashDeleteResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", result.Deleted)
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error for the unknown id, got %d: %v", len(result.Errors), result.Errors)
	}

	remaining, err := tr.List()
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected trash empty after permanent delete, got %d entries", len(remaining))
	}
	// Unlike Restore, the original file must NOT reappear - it's gone for good.
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("expected original path to remain gone after permanent delete, stat err=%v", err)
	}
}
