package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/storage"
)

// setXDGDirsToTemp points XDG_CACHE_HOME/XDG_DATA_HOME at fresh temp
// directories for the duration of the test, so a Case B provisioning
// test never touches the real user's home directory - restores the
// original env on cleanup.
func setXDGDirsToTemp(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	origCache, hadCache := os.LookupEnv("XDG_CACHE_HOME")
	origData, hadData := os.LookupEnv("XDG_DATA_HOME")
	os.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	os.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	t.Cleanup(func() {
		if hadCache {
			os.Setenv("XDG_CACHE_HOME", origCache)
		} else {
			os.Unsetenv("XDG_CACHE_HOME")
		}
		if hadData {
			os.Setenv("XDG_DATA_HOME", origData)
		} else {
			os.Unsetenv("XDG_DATA_HOME")
		}
	})
}

func sampleLibraryXMLBytes(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ComicDb.xml")
	lib := &library.ComicLibrary{
		ID: "uploaded-library",
		Books: []library.ComicBook{
			{ID: "book-1", FilePath: "/comics/book1.cbz", Series: "Batman", Title: "Uploaded Book"},
		},
	}
	if err := library.SaveLibrary(path, lib); err != nil {
		t.Fatalf("SaveLibrary: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return data
}

func newLibraryImportUploadRequest(t *testing.T, xmlBytes []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", "ComicDb.xml")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(xmlBytes); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/settings/library-import", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

// TestHandleLibraryImport_CaseA_LiveReimport covers the common case: an
// already-configured, already-running SQLite backend gets reimported into
// directly, taking effect without a restart.
func TestHandleLibraryImport_CaseA_LiveReimport(t *testing.T) {
	setXDGDirsToTemp(t)

	dbPath := filepath.Join(t.TempDir(), "test.db")
	backend, err := storage.NewSQLiteBackend(dbPath, "")
	if err != nil {
		t.Fatalf("NewSQLiteBackend: %v", err)
	}
	t.Cleanup(func() { backend.Close() })

	s := &Server{
		backend:   backend,
		configDB:  newTestConfigDB(t),
		config:    &config.Config{},
		listCache: library.NewListCache(time.Minute),
	}

	req := newLibraryImportUploadRequest(t, sampleLibraryXMLBytes(t))
	w := httptest.NewRecorder()
	s.handleLibraryImport(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var job LibraryImportJobStatus
	if err := json.NewDecoder(w.Body).Decode(&job); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if job.Status != "completed" {
		t.Fatalf("job.Status = %q, want completed (error: %s)", job.Status, job.Error)
	}
	if job.RestartRequired {
		t.Error("Case A should not require a restart")
	}
	if job.Stats == nil || job.Stats.BooksAdded != 1 {
		t.Errorf("job.Stats = %+v, want BooksAdded=1", job.Stats)
	}
	if got := backend.BookCount(); got != 1 {
		t.Errorf("backend.BookCount() = %d, want 1 (live reimport should have applied)", got)
	}

	// GET should now report the same completed job.
	statusReq := httptest.NewRequest(http.MethodGet, "/api/settings/library-import", nil)
	statusW := httptest.NewRecorder()
	s.handleLibraryImport(statusW, statusReq)
	if statusW.Code != http.StatusOK {
		t.Fatalf("GET status: expected 200, got %d: %s", statusW.Code, statusW.Body.String())
	}
}

// TestHandleLibraryImport_CaseB_ProvisionsNewDatabase covers first-time
// setup: no backend/database_path configured yet - the import provisions
// library.db and saves database_path, flagging restart_required, without
// touching the server's live (non-SQLite) backend.
func TestHandleLibraryImport_CaseB_ProvisionsNewDatabase(t *testing.T) {
	setXDGDirsToTemp(t)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &config.Config{}
	if err := config.Save(cfg, configPath); err != nil {
		t.Fatalf("seed config.Save: %v", err)
	}

	// Server's live backend is XML-backed (or could be nil) - not SQLite,
	// so this must take the Case B path.
	xmlBackend := library.NewXMLBackendFromLibrary(&library.ComicLibrary{}, "", nil)

	s := &Server{
		backend:    xmlBackend,
		configDB:   newTestConfigDB(t),
		config:     cfg,
		configPath: configPath,
	}

	req := newLibraryImportUploadRequest(t, sampleLibraryXMLBytes(t))
	w := httptest.NewRecorder()
	s.handleLibraryImport(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var job LibraryImportJobStatus
	if err := json.NewDecoder(w.Body).Decode(&job); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if job.Status != "completed" {
		t.Fatalf("job.Status = %q, want completed (error: %s)", job.Status, job.Error)
	}
	if !job.RestartRequired {
		t.Error("Case B should require a restart")
	}
	if job.Stats == nil || job.Stats.BooksAdded != 1 {
		t.Errorf("job.Stats = %+v, want BooksAdded=1", job.Stats)
	}

	// The live XML backend must be untouched (Case B never swaps it).
	if got := xmlBackend.BookCount(); got != 0 {
		t.Errorf("xmlBackend.BookCount() = %d, want 0 (Case B must not touch the live backend)", got)
	}

	// database_path must now be saved to config.yaml, pointing at a real,
	// populated database.
	reloaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if reloaded.Server.DatabasePath == "" {
		t.Fatal("expected database_path to be saved to config.yaml")
	}
	provisioned, err := storage.NewSQLiteBackend(reloaded.Server.DatabasePath, "")
	if err != nil {
		t.Fatalf("open provisioned database: %v", err)
	}
	defer provisioned.Close()
	if got := provisioned.BookCount(); got != 1 {
		t.Errorf("provisioned database book count = %d, want 1", got)
	}
}

func TestHandleLibraryImport_MissingFileField(t *testing.T) {
	setXDGDirsToTemp(t)

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/settings/library-import", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())

	s := &Server{configDB: newTestConfigDB(t), config: &config.Config{}}
	rec := httptest.NewRecorder()
	s.handleLibraryImport(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing file field, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleLibraryImport_GetBeforeAnyImport(t *testing.T) {
	s := &Server{configDB: newTestConfigDB(t)}
	req := httptest.NewRequest(http.MethodGet, "/api/settings/library-import", nil)
	w := httptest.NewRecorder()
	s.handleLibraryImport(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 before any import has run, got %d", w.Code)
	}
}
