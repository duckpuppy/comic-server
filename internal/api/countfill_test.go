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

func TestCountFill_PreviewDoesNotWriteApplyDoes(t *testing.T) {
	books := []library.ComicBook{
		{ID: "1", Series: "Synced", FilePath: "a.cbz", CustomValuesStore: "comicvine_volume=100"},
		{ID: "2", Series: "Synced", FilePath: "b.cbz", CustomValuesStore: "comicvine_volume=100"},
		{ID: "3", Series: "Hand", Count: 3, FilePath: "c.cbz", CustomValuesStore: "comicvine_volume=100"},
		{ID: "4", Series: "Pending", FilePath: "d.cbz", CustomValuesStore: "comicvine_volume=200"},
		{ID: "5", Series: "Untagged", FilePath: "e.cbz"},
	}
	s := newCBZConvertTestServer(t, config.CBZConvertConfig{}, t.TempDir(), books)
	cache, err := comicvine.OpenCache(filepath.Join(t.TempDir(), "cv.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cache.Close() })
	cache.UpsertVolume(&comicvine.CachedVolume{CVID: 100, IssueCount: 12, SyncStatus: comicvine.SyncStatusSynced, SyncedAt: time.Now()})
	cache.UpsertVolume(&comicvine.CachedVolume{CVID: 200, IssueCount: 50, SyncStatus: comicvine.SyncStatusPending})
	s.cvCache = cache

	w := httptest.NewRecorder()
	s.handleCountFill(w, httptest.NewRequest(http.MethodGet, "/", nil))
	var pv CountFillPreview
	if err := json.NewDecoder(w.Body).Decode(&pv); err != nil {
		t.Fatal(err)
	}
	if pv.Total != 2 || pv.Volumes != 1 || pv.Waiting != 1 || len(pv.Sample) != 2 || pv.Sample[0].New != 12 {
		t.Fatalf("preview = %+v", pv)
	}
	if b, _ := s.backend.GetBook("1"); b.Count != 0 {
		t.Fatalf("preview wrote Count %d", b.Count)
	}

	w = httptest.NewRecorder()
	s.handleCountFill(w, httptest.NewRequest(http.MethodPost, "/", nil))
	var res CountFillResult
	_ = json.NewDecoder(w.Body).Decode(&res)
	if w.Code != http.StatusOK || res.Applied != 2 {
		t.Fatalf("apply = %d %+v", w.Code, res)
	}
	want := map[string]int{"1": 12, "2": 12, "3": 3, "4": 0, "5": 0}
	for id, n := range want {
		if b, _ := s.backend.GetBook(id); b.Count != n {
			t.Errorf("book %s Count = %d, want %d", id, b.Count, n)
		}
	}

	w = httptest.NewRecorder()
	s.handleCountFill(w, httptest.NewRequest(http.MethodPost, "/", nil))
	_ = json.NewDecoder(w.Body).Decode(&res)
	if res.Applied != 0 {
		t.Errorf("second apply = %d, want 0", res.Applied)
	}
}

func TestCountFill_NoCache(t *testing.T) {
	s := newCBZConvertTestServer(t, config.CBZConvertConfig{}, t.TempDir(), nil)
	w := httptest.NewRecorder()
	s.handleCountFill(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d", w.Code)
	}
}
