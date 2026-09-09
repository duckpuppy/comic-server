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
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

func newLibraryOrganizerTestServer(t *testing.T, trashPath string, books []library.ComicBook) *Server {
	t.Helper()
	lib := &library.ComicLibrary{Books: books}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)

	configDB, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("configdb.Open: %v", err)
	}
	t.Cleanup(func() { configDB.Close() })

	return &Server{
		backend: backend,
		config: &config.Config{
			Server: config.ServerConfig{
				TrashPath:          trashPath,
				TrashRetentionDays: 30,
			},
		},
		configDB:   configDB,
		configPath: filepath.Join(t.TempDir(), "config.yaml"),
	}
}

func createTestLOProfile(t *testing.T, s *Server, baseFolder string) string {
	t.Helper()
	id := "profile-1"
	err := s.configDB.CreateLOProfile(configdb.LOProfile{
		ID:              id,
		Name:            "Test Profile",
		BaseFolder:      baseFolder,
		FolderTemplate:  `{<publisher>}`,
		FileTemplate:    `{<series>}`,
		ExcludeMode:     "Only",
		ExcludeOperator: "Any",
	})
	if err != nil {
		t.Fatalf("CreateLOProfile: %v", err)
	}
	return id
}

func TestHandleOrganizePreview_MissingProfileParamIs400(t *testing.T) {
	s := newLibraryOrganizerTestServer(t, "", nil)
	req := httptest.NewRequest(http.MethodGet, "/api/library/workflow/organize-preview", nil)
	w := httptest.NewRecorder()
	s.handleOrganizePreview(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleOrganizePreview_UnknownProfileIs404(t *testing.T) {
	s := newLibraryOrganizerTestServer(t, "", nil)
	req := httptest.NewRequest(http.MethodGet, "/api/library/workflow/organize-preview?profile=nope", nil)
	w := httptest.NewRecorder()
	s.handleOrganizePreview(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleOrganizePreview_ComputesMovesForBooksAtStage(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "book.cbz")
	if err := os.WriteFile(src, []byte("bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	book := library.ComicBook{ID: "1", Series: "Sandman", Publisher: "DC", FilePath: src}
	workflow.SetStage(&book, workflow.StageToMove)
	notAtStage := library.ComicBook{ID: "2", Series: "Other", Publisher: "DC", FilePath: src}
	workflow.SetStage(&notAtStage, workflow.StageScrape)

	s := newLibraryOrganizerTestServer(t, "", []library.ComicBook{book, notAtStage})
	profileID := createTestLOProfile(t, s, dir)

	req := httptest.NewRequest(http.MethodGet, "/api/library/workflow/organize-preview?profile="+profileID, nil)
	w := httptest.NewRecorder()
	s.handleOrganizePreview(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result LOOrganizePreviewResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Moves) != 1 {
		t.Fatalf("expected 1 move (only the book at StageToMove), got %d: %+v", len(result.Moves), result.Moves)
	}
	if result.Moves[0].BookID != "1" {
		t.Errorf("BookID = %q, want 1", result.Moves[0].BookID)
	}
}

func TestHandleOrganizeApply_MovesFileAndAdvancesStage(t *testing.T) {
	dir := t.TempDir()
	trashDir := t.TempDir()
	src := filepath.Join(dir, "book.cbz")
	if err := os.WriteFile(src, []byte("bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	book := library.ComicBook{ID: "1", Series: "Sandman", Publisher: "DC", FilePath: src}
	workflow.SetStage(&book, workflow.StageToMove)

	s := newLibraryOrganizerTestServer(t, trashDir, []library.ComicBook{book})
	profileID := createTestLOProfile(t, s, dir)

	req := httptest.NewRequest(http.MethodPost, "/api/library/workflow/organize-apply?profile="+profileID, nil)
	w := httptest.NewRecorder()
	s.handleOrganizeApply(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result LOOrganizeApplyResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Processed != 1 || result.Applied != 1 || len(result.Errors) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	updated, err := s.backend.GetBook("1")
	if err != nil || updated == nil {
		t.Fatalf("GetBook(1): %v", err)
	}
	if updated.FilePath == src {
		t.Errorf("FilePath unchanged, want it updated to the new organized path")
	}
	if _, statErr := os.Stat(updated.FilePath); statErr != nil {
		t.Errorf("expected new file to exist at %s: %v", updated.FilePath, statErr)
	}
	if _, statErr := os.Stat(src); !os.IsNotExist(statErr) {
		t.Errorf("expected original file to be gone (quarantined): stat err = %v", statErr)
	}
	if got := workflow.GetStage(updated); got != workflow.StageOrganized {
		t.Errorf("workflow stage = %v, want StageOrganized", got)
	}
}

func TestHandleOrganizeApply_BookIDsFiltersToSelectedSubset(t *testing.T) {
	dir := t.TempDir()
	trashDir := t.TempDir()
	src1 := filepath.Join(dir, "book1.cbz")
	src2 := filepath.Join(dir, "book2.cbz")
	if err := os.WriteFile(src1, []byte("bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src2, []byte("bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	book1 := library.ComicBook{ID: "1", Series: "Sandman", Publisher: "DC", FilePath: src1}
	workflow.SetStage(&book1, workflow.StageToMove)
	book2 := library.ComicBook{ID: "2", Series: "Preacher", Publisher: "DC", FilePath: src2}
	workflow.SetStage(&book2, workflow.StageToMove)

	s := newLibraryOrganizerTestServer(t, trashDir, []library.ComicBook{book1, book2})
	profileID := createTestLOProfile(t, s, dir)

	body, _ := json.Marshal(map[string]any{"book_ids": []string{"1"}})
	req := httptest.NewRequest(http.MethodPost, "/api/library/workflow/organize-apply?profile="+profileID, bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleOrganizeApply(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result LOOrganizeApplyResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Applied != 1 || result.Skipped != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}

	updated1, _ := s.backend.GetBook("1")
	if updated1.FilePath == src1 {
		t.Error("book 1 should have been moved (was selected)")
	}
	updated2, _ := s.backend.GetBook("2")
	if updated2.FilePath != src2 {
		t.Error("book 2 should NOT have been moved (was not selected)")
	}
	if _, err := os.Stat(src2); err != nil {
		t.Errorf("book 2's source file should still exist: %v", err)
	}
}

func TestHandleOrganizeApply_NoTrashPathConfiguredIs503(t *testing.T) {
	s := newLibraryOrganizerTestServer(t, "", nil)
	profileID := createTestLOProfile(t, s, t.TempDir())
	req := httptest.NewRequest(http.MethodPost, "/api/library/workflow/organize-apply?profile="+profileID, nil)
	w := httptest.NewRecorder()
	s.handleOrganizeApply(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}
