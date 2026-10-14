package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/comicvine"
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

func TestAttention_ComicVineAndSiblingFlags(t *testing.T) {
	books := []library.ComicBook{
		{ID: "1", Series: "Batman", Number: "1", Volume: 2011, Count: 10, FilePath: "a.cbz", CustomValuesStore: "comicvine_volume=100,comicvine_issue=1"},
		{ID: "2", Series: "Batman", Number: "5", Volume: 2011, Count: 12, FilePath: "b.cbz", CustomValuesStore: "comicvine_volume=100,comicvine_issue=2"},
		{ID: "3", Series: "Batman", Number: "3", Volume: 2011, Count: 12, FilePath: "c.cbz", CustomValuesStore: "comicvine_volume=100"},
		{ID: "4", Series: "Batman", Number: "4", Volume: 2011, FilePath: "d.cbz", CustomValuesStore: "comicvine_volume=999"},
	}
	s := newCBZConvertTestServer(t, config.CBZConvertConfig{}, t.TempDir(), books)
	cache, err := comicvine.OpenCache(filepath.Join(t.TempDir(), "cv.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cache.Close() })
	cache.UpsertVolume(&comicvine.CachedVolume{CVID: 100, IssueCount: 12, SyncStatus: comicvine.SyncStatusSynced, SyncedAt: time.Now()})
	cache.UpsertIssues([]comicvine.CachedIssue{{CVID: 1, VolumeCVID: 100, Number: "1"}, {CVID: 2, VolumeCVID: 100, Number: "2"}})
	s.cvCache = cache

	w := httptest.NewRecorder()
	s.handleAttentionSummary(w, httptest.NewRequest(http.MethodGet, "/api/library/attention", nil))
	var resp struct{ Flags []AttentionFlagSummary }
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, f := range resp.Flags {
		counts[f.Flag] = f.Count
	}
	// Book 1: count 10 vs 12. Book 2: number 5 vs CV's 2. Book 4: volume 999
	// against three siblings on 100.
	if counts["count_mismatch"] != 1 || counts["metadata_mismatch"] != 1 || counts["cv_volume_conflict"] != 1 {
		t.Fatalf("counts = %v", counts)
	}
}
