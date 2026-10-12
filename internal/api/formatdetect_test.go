package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/formatdetect"
	"github.com/duckpuppy/comic-server/internal/library"
)

func TestFormatDetect_PreviewDoesNotWriteApplyDoes(t *testing.T) {
	books := []library.ComicBook{
		{ID: "1", Series: "Mini", Count: 4, FilePath: "a.cbz"},
		{ID: "2", Series: "Long", Count: 80, FilePath: "b.cbz"},
		{ID: "3", Series: "Hand", Count: 4, Format: "Hardcover", FilePath: "c.cbz"},
		{ID: "4", Series: "Wanted", Count: 4},
	}
	s := newCBZConvertTestServer(t, config.CBZConvertConfig{}, t.TempDir(), books)

	w := httptest.NewRecorder()
	s.handleFormatDetect(w, httptest.NewRequest(http.MethodGet, "/", nil))
	var pv FormatDetectPreview
	if err := json.NewDecoder(w.Body).Decode(&pv); err != nil {
		t.Fatal(err)
	}
	if pv.Total != 2 || pv.ByFormat[formatdetect.FormatLimited] != 1 || pv.ByFormat[formatdetect.FormatSeries] != 1 {
		t.Fatalf("preview = %+v", pv)
	}
	if b, _ := s.backend.GetBook("1"); b.Format != "" {
		t.Fatalf("preview wrote Format %q", b.Format)
	}

	w = httptest.NewRecorder()
	s.handleFormatDetect(w, httptest.NewRequest(http.MethodPost, "/", nil))
	var res FormatDetectResult
	_ = json.NewDecoder(w.Body).Decode(&res)
	if w.Code != http.StatusOK || res.Applied != 2 {
		t.Fatalf("apply = %d %+v", w.Code, res)
	}
	want := map[string]string{"1": formatdetect.FormatLimited, "2": formatdetect.FormatSeries, "3": "Hardcover", "4": ""}
	for id, f := range want {
		if b, _ := s.backend.GetBook(id); b.Format != f {
			t.Errorf("book %s Format = %q, want %q", id, b.Format, f)
		}
	}

	// Second apply is a no-op.
	w = httptest.NewRecorder()
	s.handleFormatDetect(w, httptest.NewRequest(http.MethodPost, "/", nil))
	_ = json.NewDecoder(w.Body).Decode(&res)
	if res.Applied != 0 {
		t.Errorf("second apply = %d, want 0", res.Applied)
	}
}
