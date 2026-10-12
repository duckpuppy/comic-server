package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/library"
)

func TestAttention_SummaryAndDrillIn(t *testing.T) {
	books := []library.ComicBook{
		{ID: "1", Series: "S", Count: 5, Publisher: "P", Format: "Series", FilePath: "a.pdf", PageCount: 20},
		{ID: "2", Series: "", Publisher: "P", Format: "Series", FilePath: "b.cbz", PageCount: 20},
		{ID: "3", Series: "", Publisher: "P", Format: "Series", FilePath: "c.cbz", PageCount: 20},
	}
	s := newCBZConvertTestServer(t, config.CBZConvertConfig{}, t.TempDir(), books)

	w := httptest.NewRecorder()
	s.handleAttentionSummary(w, httptest.NewRequest(http.MethodGet, "/api/library/attention", nil))
	var sum struct{ Flags []AttentionFlagSummary }
	if err := json.NewDecoder(w.Body).Decode(&sum); err != nil {
		t.Fatal(err)
	}
	if len(sum.Flags) != 2 || sum.Flags[0].Flag != "no_series" || sum.Flags[0].Count != 2 || sum.Flags[1].Flag != "is_pdf" {
		t.Fatalf("flags = %+v", sum.Flags)
	}

	w = httptest.NewRecorder()
	s.handleAttentionFlagBooks(w, httptest.NewRequest(http.MethodGet, "/api/library/attention/is_pdf/books", nil))
	var page struct {
		Comics []ComicPreview
		Total  int
	}
	if err := json.NewDecoder(w.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Comics) != 1 || page.Comics[0].ID != "1" {
		t.Errorf("page = %+v", page)
	}

	w = httptest.NewRecorder()
	s.handleAttentionFlagBooks(w, httptest.NewRequest(http.MethodGet, "/api/library/attention/nope/books", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown flag status = %d, want 404", w.Code)
	}
}
