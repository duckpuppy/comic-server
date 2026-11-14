package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

// inPlaceFixture has one To Move book already at its planned path, one that
// needs a real move, and one that is not at To Move at all.
func inPlaceFixture(t *testing.T) (*Server, string, string, string) {
	t.Helper()
	dir := t.TempDir()
	inPlacePath := filepath.Join(dir, "DC", "Sandman.cbz")
	if err := os.MkdirAll(filepath.Dir(inPlacePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inPlacePath, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	misplaced := filepath.Join(dir, "misplaced.cbz")
	if err := os.WriteFile(misplaced, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := library.ComicBook{ID: "inplace", Series: "Sandman", Publisher: "DC", FilePath: inPlacePath}
	workflow.SetStage(&a, workflow.StageToMove)
	b := library.ComicBook{ID: "needsmove", Series: "Other", Publisher: "DC", FilePath: misplaced}
	workflow.SetStage(&b, workflow.StageToMove)
	c := library.ComicBook{ID: "scrape", Series: "Early", Publisher: "DC", FilePath: filepath.Join(dir, "DC", "Early.cbz")}
	workflow.SetStage(&c, workflow.StageScrape)

	s := newLibraryOrganizerTestServer(t, "", []library.ComicBook{a, b, c}) // no trash path: none must be needed
	profileID := createTestLOProfile(t, s, dir)
	return s, profileID, inPlacePath, misplaced
}

func TestHandleInPlacePreview_CountsOnlyBooksAlreadyAtTheirPlannedPath(t *testing.T) {
	s, profileID, _, _ := inPlaceFixture(t)
	w := httptest.NewRecorder()
	s.handleInPlacePreview(w, httptest.NewRequest(http.MethodGet, "/api/library/workflow/in-place-preview?profile="+profileID, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var got InPlacePreview
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ToMove != 2 || got.InPlace != 1 || got.NeedMove != 1 || got.Profile != "Test Profile" {
		t.Errorf("preview = %+v, want ToMove 2, InPlace 1, NeedMove 1", got)
	}
}

func TestHandleInPlacePreview_DefaultsToTheFirstProfile(t *testing.T) {
	s, _, _, _ := inPlaceFixture(t)
	w := httptest.NewRecorder()
	s.handleInPlacePreview(w, httptest.NewRequest(http.MethodGet, "/api/library/workflow/in-place-preview", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleInPlaceApply_AdvancesOnlyInPlaceBooksAndTouchesNoFile(t *testing.T) {
	s, profileID, inPlacePath, misplaced := inPlaceFixture(t)
	w := httptest.NewRecorder()
	s.handleInPlaceApply(w, httptest.NewRequest(http.MethodPost, "/api/library/workflow/in-place-apply?profile="+profileID, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var got InPlaceResult
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.InPlace != 1 || got.Advanced != 1 || len(got.Errors) != 0 {
		t.Fatalf("result = %+v", got)
	}

	stageOf := func(id string) workflow.Stage {
		b, err := s.backend.GetBook(id)
		if err != nil || b == nil {
			t.Fatalf("GetBook(%s): %v", id, err)
		}
		return workflow.GetStage(b)
	}
	if st := stageOf("inplace"); st != workflow.StageOrganized {
		t.Errorf("in-place book stage = %v, want Organized", st)
	}
	if st := stageOf("needsmove"); st != workflow.StageToMove {
		t.Errorf("book that needs a move stage = %v, want it left at To Move", st)
	}
	if st := stageOf("scrape"); st != workflow.StageScrape {
		t.Errorf("book at Scrape stage = %v, want it untouched", st)
	}
	for _, p := range []string{inPlacePath, misplaced} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("file %s is gone: %v", p, err)
		}
	}
	if b, _ := s.backend.GetBook("inplace"); b == nil || b.FilePath != inPlacePath {
		t.Errorf("in-place book path changed: %+v", b)
	}
}

func TestHandleInPlaceApply_RefusesASecondRunAtOnce(t *testing.T) {
	s, profileID, _, _ := inPlaceFixture(t)
	s.inPlace.running.Store(true)
	w := httptest.NewRecorder()
	s.handleInPlaceApply(w, httptest.NewRequest(http.MethodPost, "/api/library/workflow/in-place-apply?profile="+profileID, nil))
	if w.Code != http.StatusConflict {
		t.Errorf("status %d, want 409", w.Code)
	}
}

func TestHandleInPlacePreview_NoProfileIsAClearError(t *testing.T) {
	s := newLibraryOrganizerTestServer(t, "", nil)
	w := httptest.NewRecorder()
	s.handleInPlacePreview(w, httptest.NewRequest(http.MethodGet, "/api/library/workflow/in-place-preview", nil))
	if w.Code != http.StatusConflict {
		t.Errorf("status %d, want 409", w.Code)
	}
}
