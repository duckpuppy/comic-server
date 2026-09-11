package api

import (
	"archive/zip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

func newWatchFolderTestServer(t *testing.T, books []library.ComicBook, watchFolders []string) *Server {
	t.Helper()
	s := newWorkflowTestServer(t, books)
	s.config = &config.Config{Server: config.ServerConfig{WatchFolders: watchFolders}}
	return s
}

func TestHandleGetWatchFolderNewFiles_FindsFileNotInLibrary(t *testing.T) {
	dir := t.TempDir()
	newPath := filepath.Join(dir, "Batman 001 (2020).cbz")
	if err := os.WriteFile(newPath, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	s := newWatchFolderTestServer(t, nil, []string{dir})

	req := httptest.NewRequest(http.MethodGet, "/api/library/workflow/new-files", nil)
	w := httptest.NewRecorder()
	s.handleGetWatchFolderNewFiles(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result struct {
		Files []struct {
			Path string `json:"path"`
		} `json:"files"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Total != 1 || len(result.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", result.Total)
	}
	if result.Files[0].Path != newPath {
		t.Errorf("Path = %q, want %q", result.Files[0].Path, newPath)
	}
}

func TestHandleGetWatchFolderNewFiles_ExcludesFileAlreadyInLibrary(t *testing.T) {
	dir := t.TempDir()
	knownPath := filepath.Join(dir, "Known 001.cbz")
	if err := os.WriteFile(knownPath, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	s := newWatchFolderTestServer(t, []library.ComicBook{{ID: "1", FilePath: knownPath}}, []string{dir})

	req := httptest.NewRequest(http.MethodGet, "/api/library/workflow/new-files", nil)
	w := httptest.NewRecorder()
	s.handleGetWatchFolderNewFiles(w, req)

	var result struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("expected 0 files (already known), got %d", result.Total)
	}
}

func TestHandleGetWatchFolderNewFiles_NoFoldersConfiguredReturnsEmpty(t *testing.T) {
	s := newWatchFolderTestServer(t, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/library/workflow/new-files", nil)
	w := httptest.NewRecorder()
	s.handleGetWatchFolderNewFiles(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("expected 0 files with no watch folders configured, got %d", result.Total)
	}
}

// TestHandleStartProcessingNewFiles_CreatesBookAndAssignsStartingStage is
// the core comic-server-chh regression test: a raw file sitting in a
// watch folder becomes a real book record with a sensible starting
// workflow stage, without the caller having to guess an ID or stage.
func TestHandleStartProcessingNewFiles_CreatesBookAndAssignsStartingStage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Sandman 001 (2020).cbr")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	s := newWatchFolderTestServer(t, nil, []string{dir})

	body := strings.NewReader(`{"paths": ["` + strings.ReplaceAll(path, `\`, `\\`) + `"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/library/workflow/new-files/start", body)
	w := httptest.NewRecorder()
	s.handleStartProcessingNewFiles(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result struct {
		Created int `json:"created"`
		Results []struct {
			Path   string `json:"path"`
			BookID string `json:"book_id"`
			Error  string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Created != 1 {
		t.Fatalf("expected 1 book created, got %d: %+v", result.Created, result.Results)
	}
	if result.Results[0].Error != "" {
		t.Fatalf("unexpected error: %s", result.Results[0].Error)
	}

	book, err := s.backend.GetBook(result.Results[0].BookID)
	if err != nil || book == nil {
		t.Fatalf("GetBook: %v", err)
	}
	if book.FilePath != path {
		t.Errorf("FilePath = %q, want %q", book.FilePath, path)
	}
	if book.Series != "Sandman" {
		t.Errorf("Series = %q, want %q (parsed from filename)", book.Series, "Sandman")
	}
	// .cbr needs conversion - a fresh watch-folder book must land at the
	// real starting stage, not StageUnknown (invisible to the dashboard).
	if got := workflow.GetStage(book); got != workflow.StageConvertToCBZ {
		t.Errorf("workflow stage = %v, want StageConvertToCBZ", got)
	}
}

// TestHandleStartProcessingNewFiles_RejectsPathNoLongerNew covers the
// re-validation guard: a path the client believes is still a
// watch-folder file (stale drill-in load) but is actually already a
// known book must not be silently re-created as a duplicate.
func TestHandleStartProcessingNewFiles_RejectsPathNoLongerNew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Already Known.cbz")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	s := newWatchFolderTestServer(t, []library.ComicBook{{ID: "existing", FilePath: path}}, []string{dir})

	body := strings.NewReader(`{"paths": ["` + strings.ReplaceAll(path, `\`, `\\`) + `"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/library/workflow/new-files/start", body)
	w := httptest.NewRecorder()
	s.handleStartProcessingNewFiles(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result struct {
		Created int `json:"created"`
		Results []struct {
			Error string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Created != 0 {
		t.Fatalf("expected 0 books created, got %d", result.Created)
	}
	if result.Results[0].Error == "" {
		t.Error("expected an error explaining the path is no longer new")
	}
}

// TestHandleStartProcessingNewFiles_ComicInfoXMLWinsOverFilenameGuess is
// the regression test for the ComicInfo.xml-first metadata precedence: a
// watch-folder archive that already has a tagger's ComicInfo.xml embedded
// must seed the new book from THAT, not the filename-only guess - the
// filename guess is only the fallback for whatever ComicInfo.xml doesn't
// provide.
func TestHandleStartProcessingNewFiles_ComicInfoXMLWinsOverFilenameGuess(t *testing.T) {
	dir := t.TempDir()
	// The filename alone would parse to Series="Guessed From Filename",
	// Number="1" - deliberately different from the embedded ComicInfo.xml
	// below, so a pass proves the XML value was actually used.
	path := filepath.Join(dir, "Guessed From Filename 001.cbz")
	writeTestCBZWithComicInfo(t, path, `<ComicInfo><Series>Real Series</Series><Number>7</Number><Publisher>Real Publisher</Publisher></ComicInfo>`)

	s := newWatchFolderTestServer(t, nil, []string{dir})

	body := strings.NewReader(`{"paths": ["` + strings.ReplaceAll(path, `\`, `\\`) + `"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/library/workflow/new-files/start", body)
	w := httptest.NewRecorder()
	s.handleStartProcessingNewFiles(w, req)

	var result struct {
		Created int `json:"created"`
		Results []struct {
			BookID string `json:"book_id"`
		} `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Created != 1 {
		t.Fatalf("expected 1 book created, got %d: %s", result.Created, w.Body.String())
	}

	book, err := s.backend.GetBook(result.Results[0].BookID)
	if err != nil || book == nil {
		t.Fatalf("GetBook: %v", err)
	}
	if book.Series != "Real Series" {
		t.Errorf("Series = %q, want %q (from embedded ComicInfo.xml, not filename)", book.Series, "Real Series")
	}
	if book.Number != "7" {
		t.Errorf("Number = %q, want %q (from embedded ComicInfo.xml)", book.Number, "7")
	}
	if book.Publisher != "Real Publisher" {
		t.Errorf("Publisher = %q, want %q (ComicInfo.xml-only field, no filename equivalent)", book.Publisher, "Real Publisher")
	}
}

// writeTestCBZWithComicInfo writes a minimal valid CBZ containing only a
// ComicInfo.xml entry at path.
func writeTestCBZWithComicInfo(t *testing.T, path, comicInfoXML string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	w, err := zw.Create("ComicInfo.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(comicInfoXML)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}
