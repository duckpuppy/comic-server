package api

import (
	"archive/zip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

func writeTestCBZFixture(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	w, err := zw.Create("page001.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("fake page bytes")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func newCBZConvertTestServer(t *testing.T, cbzCfg config.CBZConvertConfig, trashPath string, books []library.ComicBook) *Server {
	t.Helper()
	lib := &library.ComicLibrary{
		Books: books,
		ComicLists: []library.ComicListItem{
			{
				ID:          "list-1",
				Name:        "All Batman",
				Type:        "ComicSmartListItem",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "Series", MatchOperator: "0", MatchValue: "Batman"},
				},
			},
		},
	}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	return &Server{
		backend: backend,
		config: &config.Config{
			Server: config.ServerConfig{
				CBZConvert:         cbzCfg,
				TrashPath:          trashPath,
				TrashRetentionDays: 30,
			},
		},
		configPath: filepath.Join(t.TempDir(), "config.yaml"),
	}
}

func TestHandleRunCBZConvert_DisabledReturns503(t *testing.T) {
	s := newCBZConvertTestServer(t, config.CBZConvertConfig{Enabled: false}, "", nil)

	req := httptest.NewRequest(http.MethodPost, "/api/library/lists/list-1/convert-cbz", nil)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRunCBZConvert_ConvertsAndUpdatesBooks(t *testing.T) {
	libDir := t.TempDir()
	trashDir := t.TempDir()
	src := filepath.Join(libDir, "book.cbz")
	writeTestCBZFixture(t, src)

	books := []library.ComicBook{
		{ID: "1", Series: "Batman", FilePath: src},
	}
	s := newCBZConvertTestServer(t, config.CBZConvertConfig{Enabled: true}, trashDir, books)

	req := httptest.NewRequest(http.MethodPost, "/api/library/lists/list-1/convert-cbz", nil)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result CBZConvertResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Processed != 1 || result.Converted != 1 || len(result.Errors) != 0 {
		t.Errorf("unexpected result: %+v", result)
	}

	book1, err := s.backend.GetBook("1")
	if err != nil || book1 == nil {
		t.Fatalf("GetBook(1) error = %v", err)
	}
	if book1.FilePath != src {
		t.Errorf("FilePath = %q, want unchanged %q (source was already .cbz)", book1.FilePath, src)
	}
	if book1.PageCount != 1 {
		t.Errorf("PageCount = %d, want 1", book1.PageCount)
	}
}

// TestHandleRunCBZConvert_AdvancesWorkflowStage covers comic-server-1iv.2:
// a book explicitly tracked at StageConvertToCBZ (the real production
// state a backfilled book would have) advances to StageScrape once
// conversion succeeds.
func TestHandleRunCBZConvert_AdvancesWorkflowStage(t *testing.T) {
	libDir := t.TempDir()
	trashDir := t.TempDir()
	src := filepath.Join(libDir, "book.cbz")
	writeTestCBZFixture(t, src)

	book := library.ComicBook{ID: "1", Series: "Batman", FilePath: src}
	workflow.SetStage(&book, workflow.StageConvertToCBZ)
	s := newCBZConvertTestServer(t, config.CBZConvertConfig{Enabled: true}, trashDir, []library.ComicBook{book})

	req := httptest.NewRequest(http.MethodPost, "/api/library/lists/list-1/convert-cbz", nil)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	book1, err := s.backend.GetBook("1")
	if err != nil || book1 == nil {
		t.Fatalf("GetBook(1) error = %v", err)
	}
	if got := workflow.GetStage(book1); got != workflow.StageScrape {
		t.Errorf("workflow stage = %v, want StageScrape", got)
	}
}

// TestHandleRunCBZConvertWorkflow_OnlyProcessesBooksAtThatStage covers
// comic-server-1iv.3: the whole-library workflow entry point must only
// convert books currently at StageConvertToCBZ.
func TestHandleRunCBZConvertWorkflow_OnlyProcessesBooksAtThatStage(t *testing.T) {
	libDir := t.TempDir()
	trashDir := t.TempDir()
	src1 := filepath.Join(libDir, "book1.cbz")
	src2 := filepath.Join(libDir, "book2.cbz")
	writeTestCBZFixture(t, src1)
	writeTestCBZFixture(t, src2)

	atStage := library.ComicBook{ID: "1", Series: "Batman", FilePath: src1}
	workflow.SetStage(&atStage, workflow.StageConvertToCBZ)
	notAtStage := library.ComicBook{ID: "2", Series: "Batman", FilePath: src2}
	workflow.SetStage(&notAtStage, workflow.StageScrape)

	s := newCBZConvertTestServer(t, config.CBZConvertConfig{Enabled: true}, trashDir, []library.ComicBook{atStage, notAtStage})

	req := httptest.NewRequest(http.MethodPost, "/api/library/workflow/convert-cbz", nil)
	w := httptest.NewRecorder()
	s.handleRunCBZConvertWorkflow(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result CBZConvertResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Processed != 1 || result.Converted != 1 {
		t.Errorf("unexpected result: %+v", result)
	}

	book2, err := s.backend.GetBook("2")
	if err != nil || book2 == nil {
		t.Fatalf("GetBook(2): %v", err)
	}
	if book2.PageCount != 0 {
		t.Errorf("book at a different stage must be untouched, got PageCount=%d", book2.PageCount)
	}
}

func TestHandleRunCBZConvert_MissingSourceFileIsReportedNotFatal(t *testing.T) {
	trashDir := t.TempDir()
	books := []library.ComicBook{
		{ID: "1", Series: "Batman", FilePath: filepath.Join(t.TempDir(), "missing.cbz")},
	}
	s := newCBZConvertTestServer(t, config.CBZConvertConfig{Enabled: true}, trashDir, books)

	req := httptest.NewRequest(http.MethodPost, "/api/library/lists/list-1/convert-cbz", nil)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 even with a per-book failure, got %d: %s", w.Code, w.Body.String())
	}

	var result CBZConvertResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Processed != 1 || result.Converted != 0 || len(result.Errors) != 1 {
		t.Errorf("unexpected result: %+v", result)
	}
}
