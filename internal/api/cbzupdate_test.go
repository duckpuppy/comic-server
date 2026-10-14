package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/cbzconvert"
	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

func TestStaleWrites_DisabledReportsNothing(t *testing.T) {
	s := newCBZConvertTestServer(t, config.CBZConvertConfig{}, t.TempDir(), nil)
	w := httptest.NewRecorder()
	s.handleStaleWrites(w, httptest.NewRequest(http.MethodGet, "/api/library/workflow/stale-writes", nil))
	var st StaleWritesStatus
	if err := json.NewDecoder(w.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if st.Enabled || st.Stale != 0 {
		t.Errorf("got %+v", st)
	}
}

func TestWriteMetadata_RewritesOnlyStaleBooksAndClearsState(t *testing.T) {
	dir := t.TempDir()
	staleSrc := filepath.Join(dir, "stale.cbz")
	cleanSrc := filepath.Join(dir, "clean.cbz")
	writeTestCBZFixture(t, staleSrc)
	writeTestCBZFixture(t, cleanSrc)

	stale := library.ComicBook{ID: "1", Series: "Batman", Title: "Edited", FilePath: staleSrc}
	cbzconvert.MarkWritten(&stale, false)
	stale.Title = "Edited again" // edited after the last write
	clean := library.ComicBook{ID: "2", Series: "Batman", FilePath: cleanSrc}

	s := newCBZConvertTestServer(t, config.CBZConvertConfig{Enabled: true}, t.TempDir(), []library.ComicBook{stale, clean})

	w := httptest.NewRecorder()
	s.handleStaleWrites(w, httptest.NewRequest(http.MethodGet, "/", nil))
	var st StaleWritesStatus
	_ = json.NewDecoder(w.Body).Decode(&st)
	if !st.Enabled || st.Stale != 1 {
		t.Fatalf("status = %+v, want enabled with 1 stale", st)
	}

	w = httptest.NewRecorder()
	s.handleWriteMetadata(w, httptest.NewRequest(http.MethodPost, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var res WriteMetadataResult
	_ = json.NewDecoder(w.Body).Decode(&res)
	if res.Processed != 1 || res.Written != 1 || len(res.Errors) != 0 {
		t.Fatalf("result = %+v", res)
	}

	b, err := s.backend.GetBook("1")
	if err != nil || b == nil {
		t.Fatalf("GetBook: %v", err)
	}
	if cbzconvert.IsStale(b, false) {
		t.Error("book should be clean after write")
	}

	// Second run is a no-op.
	w = httptest.NewRecorder()
	s.handleWriteMetadata(w, httptest.NewRequest(http.MethodPost, "/", nil))
	_ = json.NewDecoder(w.Body).Decode(&res)
	if res.Processed != 0 {
		t.Errorf("second run processed %d, want 0", res.Processed)
	}
}

func TestWriteMetadata_RequiresEnabled(t *testing.T) {
	s := newCBZConvertTestServer(t, config.CBZConvertConfig{}, t.TempDir(), nil)
	w := httptest.NewRecorder()
	s.handleWriteMetadata(w, httptest.NewRequest(http.MethodPost, "/", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestCBZConvert_WebComicPointerIsAdvancedNotFailed(t *testing.T) {
	b := library.ComicBook{ID: "w1", Series: "Web", FilePath: "/comics/Web Comic/Web #1.cbw"}
	workflow.SetStage(&b, workflow.StageConvertToCBZ)
	s := newCBZConvertTestServer(t, config.CBZConvertConfig{Enabled: true}, t.TempDir(), []library.ComicBook{b})

	w := httptest.NewRecorder()
	s.handleRunCBZConvertWorkflow(w, httptest.NewRequest(http.MethodPost, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var res CBZConvertResult
	_ = json.NewDecoder(w.Body).Decode(&res)
	if res.Converted != 0 || res.Skipped != 1 || len(res.Errors) != 0 {
		t.Fatalf("result = %+v", res)
	}
	got, err := s.backend.GetBook("w1")
	if err != nil || got == nil {
		t.Fatalf("GetBook: %v", err)
	}
	if st := workflow.GetStage(got); st == workflow.StageConvertToCBZ {
		t.Errorf("stage still %v, want advanced", st)
	}
	if got.FilePath != b.FilePath {
		t.Errorf("file path changed to %q", got.FilePath)
	}
}

func TestWriteMetadata_BookIDsWritesThoseBooksEvenIfNotStale(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "clean.cbz")
	writeTestCBZFixture(t, src)
	books := []library.ComicBook{
		{ID: "1", Series: "Batman", FilePath: src},
		{ID: "2", Series: "Batman", FilePath: filepath.Join(dir, "b.cbr")},
		{ID: "3", Series: "Batman"},
	}
	s := newCBZConvertTestServer(t, config.CBZConvertConfig{Enabled: true}, t.TempDir(), books)

	w := httptest.NewRecorder()
	s.handleWriteMetadata(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"book_ids":["1","2","3","nope"]}`)))
	var res WriteMetadataResult
	_ = json.NewDecoder(w.Body).Decode(&res)
	if w.Code != http.StatusOK || res.Processed != 1 || res.Written != 1 || len(res.Errors) != 3 {
		t.Fatalf("status %d result = %+v", w.Code, res)
	}
	b, _ := s.backend.GetBook("1")
	if _, ok := library.GetCustomValue(b.CustomValuesStore, cbzconvert.WrittenHashKey); !ok {
		t.Error("written book has no written hash")
	}
}
