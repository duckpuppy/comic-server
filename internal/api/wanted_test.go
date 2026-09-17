package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

func TestHandleGetWantedBooks_OnlyReturnsBooksWithNoFilePath(t *testing.T) {
	withFile := library.ComicBook{ID: "1", Series: "Batman", FilePath: "/comics/batman.cbz"}
	wanted := library.ComicBook{ID: "2", Series: "Wanted Series"}
	s := newWorkflowTestServer(t, []library.ComicBook{withFile, wanted})

	req := httptest.NewRequest(http.MethodGet, "/api/library/workflow/wanted", nil)
	w := httptest.NewRecorder()
	s.handleWantedBooks(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Comics []ComicPreview `json:"comics"`
		Total  int            `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 1 || len(resp.Comics) != 1 || resp.Comics[0].ID != "2" {
		t.Fatalf("expected only the fileless book, got %+v", resp)
	}
}

func TestHandleCreateWantedBook_CreatesBookWithNoFilePath(t *testing.T) {
	s := newWorkflowTestServer(t, nil)

	body, _ := json.Marshal(wantedCreateRequest{Series: "Batman", Number: "1", Year: 2020, Publisher: "DC"})
	req := httptest.NewRequest(http.MethodPost, "/api/library/workflow/wanted", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleWantedBooks(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var created ComicPreview
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ID == "" || created.Series != "Batman" {
		t.Fatalf("unexpected created book: %+v", created)
	}

	book, err := s.backend.GetBook(created.ID)
	if err != nil || book == nil {
		t.Fatalf("GetBook(%s): %v", created.ID, err)
	}
	if book.FilePath != "" {
		t.Errorf("expected FilePath empty for a wanted book, got %q", book.FilePath)
	}
	if got := workflow.GetStage(book); got != workflow.StageUnknown {
		t.Errorf("expected a wanted book to have no workflow stage yet, got %v", got)
	}
}

func TestHandleCreateWantedBook_RequiresSeries(t *testing.T) {
	s := newWorkflowTestServer(t, nil)

	body, _ := json.Marshal(wantedCreateRequest{Number: "1"})
	req := httptest.NewRequest(http.MethodPost, "/api/library/workflow/wanted", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleWantedBooks(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleWantedBooks_MethodNotAllowed(t *testing.T) {
	s := newWorkflowTestServer(t, nil)
	req := httptest.NewRequest(http.MethodDelete, "/api/library/workflow/wanted", nil)
	w := httptest.NewRecorder()
	s.handleWantedBooks(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// TestHandleLinkWantedBook_LinksFileAndInfersStage covers
// comic-server-38f7's "Link" piece and the confirmed design decision: a
// linked book re-enters the pipeline via the normal InferStage
// classification (ConvertToCBZ/Scrape/etc, whatever the file actually
// implies), NOT a special "New Files" path - New Files only applies to
// files with no book record at all.
func TestHandleLinkWantedBook_LinksFileAndInfersStage(t *testing.T) {
	libDir := t.TempDir()
	target := filepath.Join(libDir, "batman-1.cbz")
	if err := os.WriteFile(target, []byte("fake cbz content"), 0o644); err != nil {
		t.Fatal(err)
	}

	wanted := library.ComicBook{ID: "{wanted-1}", Series: "Batman", Number: "1"}
	s := newWorkflowTestServer(t, []library.ComicBook{wanted})

	body, _ := json.Marshal(wantedLinkRequest{BookID: "{wanted-1}", FilePath: target})
	req := httptest.NewRequest(http.MethodPost, "/api/library/workflow/wanted/link", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleLinkWantedBook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	book, err := s.backend.GetBook("{wanted-1}")
	if err != nil || book == nil {
		t.Fatalf("GetBook: %v", err)
	}
	if book.FilePath != target {
		t.Errorf("FilePath = %q, want %q", book.FilePath, target)
	}
	if book.FileSize == 0 {
		t.Error("expected FileSize to be set from the linked file")
	}
	// A .cbz with no embedded ComicInfo.xml goes straight to StageScrape
	// per workflow.InferStage - it's already the right container format.
	if got := workflow.GetStage(book); got != workflow.StageScrape {
		t.Errorf("workflow stage = %v, want StageScrape (file already .cbz, no conversion needed)", got)
	}
}

func TestHandleLinkWantedBook_RejectsAlreadyLinkedBook(t *testing.T) {
	book := library.ComicBook{ID: "1", Series: "Batman", FilePath: "/comics/already-here.cbz"}
	s := newWorkflowTestServer(t, []library.ComicBook{book})

	tmpFile := filepath.Join(t.TempDir(), "whatever.cbz")
	if err := os.WriteFile(tmpFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(wantedLinkRequest{BookID: "1", FilePath: tmpFile})
	req := httptest.NewRequest(http.MethodPost, "/api/library/workflow/wanted/link", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleLinkWantedBook(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleLinkWantedBook_MissingFileIs400(t *testing.T) {
	wanted := library.ComicBook{ID: "1", Series: "Batman"}
	s := newWorkflowTestServer(t, []library.ComicBook{wanted})

	body, _ := json.Marshal(wantedLinkRequest{BookID: "1", FilePath: "/does/not/exist.cbz"})
	req := httptest.NewRequest(http.MethodPost, "/api/library/workflow/wanted/link", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleLinkWantedBook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleLinkWantedBook_UnknownBookIs404(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "book.cbz")
	if err := os.WriteFile(tmpFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newWorkflowTestServer(t, nil)

	body, _ := json.Marshal(wantedLinkRequest{BookID: "does-not-exist", FilePath: tmpFile})
	req := httptest.NewRequest(http.MethodPost, "/api/library/workflow/wanted/link", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleLinkWantedBook(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
