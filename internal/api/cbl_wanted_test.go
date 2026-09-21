package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/storage"
)

func TestHandleAddCBLUnmatchedToWanted_CreatesWantedBooksAndIsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	sb, err := storage.NewSQLiteBackend(dbPath, "")
	if err != nil {
		t.Fatalf("create backend: %v", err)
	}
	defer sb.Close()

	batman := library.ComicBook{ID: "book-1", FilePath: "/x/batman1.cbz", Series: "Batman", Number: "1", Volume: 1940, Year: 1940}
	if _, err := sb.DB().Import(&library.ComicLibrary{ID: "lib", Books: []library.ComicBook{batman}}, storage.ImportOptions{}); err != nil {
		t.Fatalf("seed library: %v", err)
	}

	s := &Server{backend: sb, listCache: library.NewListCache(0)}

	// Import a CBL with one matched entry and two unmatched entries.
	const cblXML = `<ReadingList><Name>Test</Name><Books>
<Book Series="Batman" Number="1" Volume="1940" Year="1940"/>
<Book Series="Missing One" Number="1" Volume="2020" Year="2020"/>
<Book Series="Missing Two" Number="1" Volume="2021" Year="2021"/>
</Books></ReadingList>`
	importReq := newCBLUploadRequest(t, []byte(cblXML))
	importW := httptest.NewRecorder()
	s.handleImportCBL(importW, importReq)
	if importW.Code != http.StatusOK {
		t.Fatalf("import setup failed: %d: %s", importW.Code, importW.Body.String())
	}
	var importResult CBLImportResultWire
	if err := json.NewDecoder(importW.Body).Decode(&importResult); err != nil {
		t.Fatalf("decode import result: %v", err)
	}
	if importResult.Unmatched != 2 {
		t.Fatalf("expected 2 unmatched, got %d", importResult.Unmatched)
	}

	// First call: creates 2 wanted books.
	body, _ := json.Marshal(cblAddUnmatchedToWantedRequest{ListID: importResult.ListID})
	req1 := httptest.NewRequest(http.MethodPost, "/api/library/workflow/wanted/from-cbl-import", bytes.NewReader(body))
	w1 := httptest.NewRecorder()
	s.handleAddCBLUnmatchedToWanted(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w1.Code, w1.Body.String())
	}
	var result1 CBLAddUnmatchedToWantedResultWire
	if err := json.NewDecoder(w1.Body).Decode(&result1); err != nil {
		t.Fatalf("decode result1: %v", err)
	}
	if result1.Created != 2 {
		t.Fatalf("expected 2 created, got %d: %+v", result1.Created, result1)
	}

	// Confirm real wanted books exist (FilePath == "").
	wantedReq := httptest.NewRequest(http.MethodGet, "/api/library/workflow/wanted", nil)
	wantedW := httptest.NewRecorder()
	s.handleGetWantedBooks(wantedW, wantedReq)
	var wantedResp struct {
		Comics []ComicPreview `json:"comics"`
		Total  int            `json:"total"`
	}
	if err := json.NewDecoder(wantedW.Body).Decode(&wantedResp); err != nil {
		t.Fatalf("decode wanted list: %v", err)
	}
	if wantedResp.Total != 2 {
		t.Fatalf("expected 2 wanted books to exist, got %d", wantedResp.Total)
	}

	// Second call: idempotent, creates 0 more (already resolved).
	req2 := httptest.NewRequest(http.MethodPost, "/api/library/workflow/wanted/from-cbl-import", bytes.NewReader(body))
	w2 := httptest.NewRecorder()
	s.handleAddCBLUnmatchedToWanted(w2, req2)
	var result2 CBLAddUnmatchedToWantedResultWire
	if err := json.NewDecoder(w2.Body).Decode(&result2); err != nil {
		t.Fatalf("decode result2: %v", err)
	}
	if result2.Created != 0 {
		t.Errorf("expected 0 created on second call (idempotent), got %d", result2.Created)
	}

	wantedW2 := httptest.NewRecorder()
	s.handleGetWantedBooks(wantedW2, httptest.NewRequest(http.MethodGet, "/api/library/workflow/wanted", nil))
	var wantedResp2 struct {
		Total int `json:"total"`
	}
	json.NewDecoder(wantedW2.Body).Decode(&wantedResp2)
	if wantedResp2.Total != 2 {
		t.Errorf("expected still exactly 2 wanted books after the idempotent re-run, got %d", wantedResp2.Total)
	}
}

func TestHandleAddCBLUnmatchedToWanted_RequiresListID(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	sb, err := storage.NewSQLiteBackend(dbPath, "")
	if err != nil {
		t.Fatalf("create backend: %v", err)
	}
	defer sb.Close()
	s := &Server{backend: sb}

	body, _ := json.Marshal(cblAddUnmatchedToWantedRequest{})
	req := httptest.NewRequest(http.MethodPost, "/api/library/workflow/wanted/from-cbl-import", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleAddCBLUnmatchedToWanted(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
