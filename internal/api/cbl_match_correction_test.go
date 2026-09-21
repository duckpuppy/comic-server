package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/storage"
)

// newAmbiguousCBLTestServer seeds two identical-looking books (so the
// string-fallback matcher ties between them) and imports a CBL entry
// that matches both - the exact shape the match-correction UI exists for.
func newAmbiguousCBLTestServer(t *testing.T) (*Server, string, []string) {
	t.Helper()
	s, sb := newCBLTestServer(t)

	dup1 := library.ComicBook{ID: "dup-1", FilePath: "/x/dup1.cbz", Series: "Weird Duplicates", Number: "1", Volume: -1, Year: 2000}
	dup2 := library.ComicBook{ID: "dup-2", FilePath: "/x/dup2.cbz", Series: "Weird Duplicates", Number: "1", Volume: -1, Year: 2000}
	if _, err := sb.DB().Import(&library.ComicLibrary{ID: "lib", Books: []library.ComicBook{dup1, dup2}}, storage.ImportOptions{}); err != nil {
		t.Fatalf("seed duplicate books: %v", err)
	}

	const cblXML = `<?xml version="1.0" encoding="utf-8"?>
<ReadingList><Name>Ambiguous</Name><Books>
<Book Series="Weird Duplicates" Number="1" Volume="-1" Year="2000"/>
</Books></ReadingList>`
	req := newCBLUploadRequest(t, []byte(cblXML))
	w := httptest.NewRecorder()
	s.handleImportCBL(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("import: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp CBLImportResultWire
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode import response: %v", err)
	}
	return s, resp.ListID, []string{"dup-1", "dup-2"}
}

func getCBLImportEntries(t *testing.T, s *Server, listID string) []CBLImportEntryDetailWire {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/library/lists/"+listID+"/cbl-import-entries", nil)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET cbl-import-entries: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Entries []CBLImportEntryDetailWire `json:"entries"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode entries response: %v", err)
	}
	return resp.Entries
}

func TestHandleGetCBLImportEntries_ResolvesBookAndCandidates(t *testing.T) {
	s, listID, wantCandidateIDs := newAmbiguousCBLTestServer(t)

	entries := getCBLImportEntries(t, s, listID)
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.MatchPath != "series_number" {
		t.Errorf("MatchPath = %q, want series_number", e.MatchPath)
	}
	if e.Book == nil || e.Book.ID != wantCandidateIDs[0] {
		t.Errorf("Book = %+v, want ID %q (FirstOrDefault pick)", e.Book, wantCandidateIDs[0])
	}
	if len(e.Candidates) != 2 {
		t.Fatalf("len(Candidates) = %d, want 2", len(e.Candidates))
	}
	if e.Candidates[0].ID != wantCandidateIDs[0] || e.Candidates[1].ID != wantCandidateIDs[1] {
		t.Errorf("Candidates = %+v, want IDs %v", e.Candidates, wantCandidateIDs)
	}
}

func TestHandleGetCBLImportEntries_NotFoundForNonCBLList(t *testing.T) {
	s, sb := newCBLTestServer(t)
	list := &library.ComicListItem{ID: "plain-list", Type: "ComicSmartListItem", Name: "Not a CBL import"}
	if err := sb.CreateList(list); err != nil {
		t.Fatalf("create plain list: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/library/lists/plain-list/cbl-import-entries", nil)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCorrectCBLImportEntry_RepointsToOtherCandidate(t *testing.T) {
	s, listID, wantCandidateIDs := newAmbiguousCBLTestServer(t)
	entries := getCBLImportEntries(t, s, listID)
	entryID := entries[0].ID
	otherBook := wantCandidateIDs[1] // dup-2, the one FirstOrDefault didn't pick

	body, _ := json.Marshal(cblCorrectEntryRequest{BookID: otherBook})
	req := httptest.NewRequest(http.MethodPost,
		"/api/library/lists/"+listID+"/cbl-import-entries/"+entryID+"/correct", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("correct: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var corrected CBLImportEntryDetailWire
	if err := json.NewDecoder(w.Body).Decode(&corrected); err != nil {
		t.Fatalf("decode corrected response: %v", err)
	}
	if corrected.MatchPath != "manual" {
		t.Errorf("MatchPath = %q, want manual", corrected.MatchPath)
	}
	if corrected.Book == nil || corrected.Book.ID != otherBook {
		t.Errorf("Book = %+v, want ID %q", corrected.Book, otherBook)
	}

	// Re-fetching the entries reflects the correction, not the original pick.
	entries = getCBLImportEntries(t, s, listID)
	if entries[0].Book == nil || entries[0].Book.ID != otherBook {
		t.Errorf("after refetch, Book = %+v, want ID %q", entries[0].Book, otherBook)
	}

	// The list's own membership was updated too.
	list, err := s.backend.FindListByID(listID)
	if err != nil || list == nil {
		t.Fatalf("FindListByID: list=%v err=%v", list, err)
	}
	if list.BookCount != 1 || len(list.Items) != 1 || list.Items[0].ID != otherBook {
		t.Errorf("list membership = %+v, want exactly [%s]", list.Items, otherBook)
	}
}

func TestHandleCorrectCBLImportEntry_Unmatch(t *testing.T) {
	s, listID, _ := newAmbiguousCBLTestServer(t)
	entries := getCBLImportEntries(t, s, listID)
	entryID := entries[0].ID

	body, _ := json.Marshal(cblCorrectEntryRequest{BookID: ""})
	req := httptest.NewRequest(http.MethodPost,
		"/api/library/lists/"+listID+"/cbl-import-entries/"+entryID+"/correct", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("correct: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var corrected CBLImportEntryDetailWire
	if err := json.NewDecoder(w.Body).Decode(&corrected); err != nil {
		t.Fatalf("decode corrected response: %v", err)
	}
	if corrected.MatchPath != "none" {
		t.Errorf("MatchPath = %q, want none", corrected.MatchPath)
	}
	if corrected.Book != nil {
		t.Errorf("Book = %+v, want nil after unmatch", corrected.Book)
	}

	list, err := s.backend.FindListByID(listID)
	if err != nil || list == nil {
		t.Fatalf("FindListByID: list=%v err=%v", list, err)
	}
	if list.BookCount != 0 {
		t.Errorf("BookCount = %d, want 0 after unmatching the only entry", list.BookCount)
	}
}

// TestHandleCorrectCBLImportEntry_ListDetailReflectsCountImmediately is
// the regression test for a real bug this feature's own browser smoke
// test caught: a plain InvalidateListCache() call only marks the list
// count stale (ListCache.Invalidate), so the very next GET
// /api/library/lists/:id right after a correction served the OLD
// book_count from ListCache.GetStaleCounts, not the corrected one, until
// whatever else happened to trigger a refresh. The handler now
// synchronously recomputes and caches this one list's count instead.
func TestHandleCorrectCBLImportEntry_ListDetailReflectsCountImmediately(t *testing.T) {
	s, listID, _ := newAmbiguousCBLTestServer(t)
	entries := getCBLImportEntries(t, s, listID)
	entryID := entries[0].ID

	body, _ := json.Marshal(cblCorrectEntryRequest{BookID: ""}) // unmatch the list's only entry
	req := httptest.NewRequest(http.MethodPost,
		"/api/library/lists/"+listID+"/cbl-import-entries/"+entryID+"/correct", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("correct: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// The very next GET, with no delay - this is exactly the race the
	// browser smoke test hit.
	getReq := httptest.NewRequest(http.MethodGet, "/api/library/lists/"+listID, nil)
	getW := httptest.NewRecorder()
	s.handleListsRouter(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("GET list detail: expected 200, got %d: %s", getW.Code, getW.Body.String())
	}
	var detail ListDetail
	if err := json.NewDecoder(getW.Body).Decode(&detail); err != nil {
		t.Fatalf("decode list detail: %v", err)
	}
	if detail.BookCount != 0 {
		t.Errorf("BookCount = %d, want 0 immediately after unmatching the only entry (stale cache regression)", detail.BookCount)
	}
}

func TestHandleCorrectCBLImportEntry_UnknownEntry(t *testing.T) {
	s, listID, _ := newAmbiguousCBLTestServer(t)

	body, _ := json.Marshal(cblCorrectEntryRequest{BookID: "dup-2"})
	req := httptest.NewRequest(http.MethodPost,
		"/api/library/lists/"+listID+"/cbl-import-entries/does-not-exist/correct", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCorrectCBLImportEntry_UnknownBook(t *testing.T) {
	s, listID, _ := newAmbiguousCBLTestServer(t)
	entries := getCBLImportEntries(t, s, listID)
	entryID := entries[0].ID

	body, _ := json.Marshal(cblCorrectEntryRequest{BookID: "does-not-exist"})
	req := httptest.NewRequest(http.MethodPost,
		"/api/library/lists/"+listID+"/cbl-import-entries/"+entryID+"/correct", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCorrectCBLImportEntry_MethodNotAllowed(t *testing.T) {
	s, listID, _ := newAmbiguousCBLTestServer(t)
	entries := getCBLImportEntries(t, s, listID)
	entryID := entries[0].ID

	req := httptest.NewRequest(http.MethodGet,
		"/api/library/lists/"+listID+"/cbl-import-entries/"+entryID+"/correct", nil)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d: %s", w.Code, w.Body.String())
	}
}
