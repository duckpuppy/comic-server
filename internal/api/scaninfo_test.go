package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

func newScanInfoTestServer(t *testing.T, scanInfoCfg config.ScanInfoConfig, books []library.ComicBook) *Server {
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
			Server: config.ServerConfig{ScanInfo: scanInfoCfg},
		},
		configPath: filepath.Join(t.TempDir(), "config.yaml"),
		// effectiveScanInfo (comic-server-4ms) checks config.db before
		// falling back to config.yaml's Server.ScanInfo - an empty
		// config.db here means every test in this file exercises that
		// fallback path, matching what they set up scanInfoCfg for.
		configDB: newTestConfigDB(t),
	}
}

func TestHandleRunScanInfo_DisabledReturns503(t *testing.T) {
	s := newScanInfoTestServer(t, config.ScanInfoConfig{Enabled: false}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/library/lists/list-1/scan-info", nil)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRunScanInfo_DetectsAndUpdatesBooks(t *testing.T) {
	scanInfoCfg := config.ScanInfoConfig{
		Enabled:  true,
		Scanners: []string{"FakeScanCo"},
		Prefix:   "Scanner:",
		Unknown:  "Unknown",
	}
	books := []library.ComicBook{
		{ID: "1", Series: "Batman", FilePath: `Batman 001 (2016) (Zeta-Fictscans).cbz`},
		{ID: "2", Series: "Batman", FilePath: `Batman 002 (2016).cbz`, ScanInformation: "Scanner:Unknown"},
	}
	s := newScanInfoTestServer(t, scanInfoCfg, books)

	req := httptest.NewRequest(http.MethodPost, "/api/library/lists/list-1/scan-info", nil)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result ScanInfoResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Processed != 2 {
		t.Errorf("expected 2 processed, got %d", result.Processed)
	}
	if result.Updated != 1 {
		t.Errorf("expected 1 updated (book 1, newly tagged), got %d", result.Updated)
	}
	if result.Skipped != 1 {
		t.Errorf("expected 1 skipped (book 2 already Scanner:Unknown), got %d", result.Skipped)
	}

	book1, err := s.backend.GetBook("1")
	if err != nil || book1 == nil {
		t.Fatalf("GetBook(1) error = %v", err)
	}
	if book1.ScanInformation != "Scanner:Zeta-Fictscans" {
		t.Errorf("book 1 ScanInformation = %q, want %q", book1.ScanInformation, "Scanner:Zeta-Fictscans")
	}

	book2, err := s.backend.GetBook("2")
	if err != nil || book2 == nil {
		t.Fatalf("GetBook(2) error = %v", err)
	}
	if book2.ScanInformation != "Scanner:Unknown" {
		t.Errorf("book 2 ScanInformation changed unexpectedly: %q", book2.ScanInformation)
	}
}

// TestHandleRunScanInfo_AdvancesWorkflowStage covers comic-server-1iv.2:
// a book that gets a NEW scan-info tag, and one that already had a
// correct tag (detection still succeeds, nothing changes), must BOTH
// advance to StageDataManager - success is defined by detection
// succeeding, not by whether this specific run wrote a new value.
func TestHandleRunScanInfo_AdvancesWorkflowStage(t *testing.T) {
	scanInfoCfg := config.ScanInfoConfig{
		Enabled:  true,
		Scanners: []string{"FakeScanCo"},
		Prefix:   "Scanner:",
		Unknown:  "Unknown",
	}
	books := []library.ComicBook{
		{ID: "1", Series: "Batman", FilePath: `Batman 001 (2016) (Zeta-Fictscans).cbz`},
		{ID: "2", Series: "Batman", FilePath: `Batman 002 (2016).cbz`, ScanInformation: "Scanner:Unknown"},
	}
	s := newScanInfoTestServer(t, scanInfoCfg, books)

	req := httptest.NewRequest(http.MethodPost, "/api/library/lists/list-1/scan-info", nil)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	for _, id := range []string{"1", "2"} {
		book, err := s.backend.GetBook(id)
		if err != nil || book == nil {
			t.Fatalf("GetBook(%s) error = %v", id, err)
		}
		if got := workflow.GetStage(book); got != workflow.StageDataManager {
			t.Errorf("book %s workflow stage = %v, want StageDataManager", id, got)
		}
	}
}

// TestHandleRunScanInfoWorkflow_OnlyProcessesBooksAtThatStage covers
// comic-server-1iv.3: the whole-library workflow entry point must only
// touch books currently at StageScanInfo, ignoring one that's at an
// earlier or later stage even if it would otherwise match the detector.
func TestHandleRunScanInfoWorkflow_OnlyProcessesBooksAtThatStage(t *testing.T) {
	scanInfoCfg := config.ScanInfoConfig{
		Enabled:  true,
		Scanners: []string{"FakeScanCo"},
		Prefix:   "Scanner:",
		Unknown:  "Unknown",
	}
	atStage := library.ComicBook{ID: "1", FilePath: `Batman 001 (2016) (Zeta-Fictscans).cbz`}
	workflow.SetStage(&atStage, workflow.StageScanInfo)
	notAtStage := library.ComicBook{ID: "2", FilePath: `Batman 002 (2016) (Zeta-Fictscans).cbz`}
	workflow.SetStage(&notAtStage, workflow.StageDataManager)

	s := newScanInfoTestServer(t, scanInfoCfg, []library.ComicBook{atStage, notAtStage})

	req := httptest.NewRequest(http.MethodPost, "/api/library/workflow/scan-info", nil)
	w := httptest.NewRecorder()
	s.handleRunScanInfoWorkflow(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result ScanInfoResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Processed != 1 {
		t.Errorf("Processed = %d, want 1 (only the book at StageScanInfo)", result.Processed)
	}

	book2, err := s.backend.GetBook("2")
	if err != nil || book2 == nil {
		t.Fatalf("GetBook(2): %v", err)
	}
	if book2.ScanInformation != "" {
		t.Errorf("book at a different stage must be untouched, got ScanInformation=%q", book2.ScanInformation)
	}
}

func TestHandleRunScanInfo_MethodNotAllowed(t *testing.T) {
	s := newScanInfoTestServer(t, config.ScanInfoConfig{Enabled: true, Scanners: []string{"X"}}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/library/lists/list-1/scan-info", nil)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}
