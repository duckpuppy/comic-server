package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/library"
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

func TestHandleRunScanInfo_MethodNotAllowed(t *testing.T) {
	s := newScanInfoTestServer(t, config.ScanInfoConfig{Enabled: true, Scanners: []string{"X"}}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/library/lists/list-1/scan-info", nil)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}
