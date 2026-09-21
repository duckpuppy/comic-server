package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/storage"
)

func newCBLUploadRequest(t *testing.T, cblBytes []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", "Test.cbl")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(cblBytes); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/library/import-cbl", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func newCBLTestServer(t *testing.T) (*Server, *storage.SQLiteBackend) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	sb, err := storage.NewSQLiteBackend(dbPath, "")
	if err != nil {
		t.Fatalf("create backend: %v", err)
	}
	t.Cleanup(func() { sb.Close() })

	book := library.ComicBook{ID: "book-1", FilePath: "/x/atomic1.cbz", Series: "Atomic War!", Number: "1", Volume: 1952, Year: 1952}
	if _, err := sb.DB().Import(&library.ComicLibrary{ID: "lib", Books: []library.ComicBook{book}}, storage.ImportOptions{}); err != nil {
		t.Fatalf("seed library: %v", err)
	}

	return &Server{backend: sb, listCache: library.NewListCache(0)}, sb
}

func TestHandleImportCBL_MatchesAndCreatesList(t *testing.T) {
	s, _ := newCBLTestServer(t)

	const cblXML = `<?xml version="1.0" encoding="utf-8"?>
<ReadingList><Name>Smoke Test</Name><Books>
<Book Series="Atomic War!" Number="1" Volume="1952" Year="1952"/>
<Book Series="Nonexistent" Number="1" Volume="2020" Year="2020"/>
</Books></ReadingList>`

	req := newCBLUploadRequest(t, []byte(cblXML))
	w := httptest.NewRecorder()
	s.handleImportCBL(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp CBLImportResultWire
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Name != "Smoke Test" {
		t.Errorf("Name = %q, want Smoke Test", resp.Name)
	}
	if resp.MatchedOther != 1 {
		t.Errorf("MatchedOther = %d, want 1", resp.MatchedOther)
	}
	if resp.Unmatched != 1 {
		t.Errorf("Unmatched = %d, want 1", resp.Unmatched)
	}
	if len(resp.Entries) != 2 {
		t.Fatalf("len(Entries) = %d, want 2", len(resp.Entries))
	}
	if resp.ListID == "" {
		t.Error("expected a non-empty ListID")
	}
}

func TestHandleImportCBL_RejectsSmartList(t *testing.T) {
	s, _ := newCBLTestServer(t)

	const cblXML = `<ReadingList><Name>Smart</Name><Matchers><ComicBookMatcher/></Matchers></ReadingList>`
	req := newCBLUploadRequest(t, []byte(cblXML))
	w := httptest.NewRecorder()
	s.handleImportCBL(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleImportCBL_RejectsEmptyBookList(t *testing.T) {
	s, _ := newCBLTestServer(t)

	const cblXML = `<ReadingList><Name>Empty</Name></ReadingList>`
	req := newCBLUploadRequest(t, []byte(cblXML))
	w := httptest.NewRecorder()
	s.handleImportCBL(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleImportCBL_MethodNotAllowed(t *testing.T) {
	s, _ := newCBLTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/library/import-cbl", nil)
	w := httptest.NewRecorder()
	s.handleImportCBL(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleImportCBL_RequiresSQLiteBackend(t *testing.T) {
	lib := &library.ComicLibrary{}
	xmlBackend := library.NewXMLBackendFromLibrary(lib, "", nil)
	s := &Server{backend: xmlBackend}

	req := newCBLUploadRequest(t, []byte(`<ReadingList><Name>x</Name></ReadingList>`))
	w := httptest.NewRecorder()
	s.handleImportCBL(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}
