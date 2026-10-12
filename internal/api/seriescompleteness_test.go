package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
)

func newCompletenessServer(t *testing.T) *Server {
	t.Helper()
	cache, err := comicvine.OpenCache(filepath.Join(t.TempDir(), "cv.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cache.Close() })
	cache.UpsertVolume(&comicvine.CachedVolume{CVID: 100, Name: "Batman", Publisher: "DC", StartYear: "1940",
		SyncStatus: comicvine.SyncStatusSynced, SyncedAt: time.Now(), IssuesSynced: true})
	cache.UpsertIssues([]comicvine.CachedIssue{
		{CVID: 1001, VolumeCVID: 100, Number: "1", CoverDate: "1940-04-25"},
		{CVID: 1002, VolumeCVID: 100, Number: "2", CoverDate: "1940-06-01"},
		{CVID: 1003, VolumeCVID: 100, Number: "3", Name: "Third"},
	})
	db, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	books := []library.ComicBook{
		{ID: "own1", Series: "Batman", Number: "1", FilePath: "a.cbz", CustomValuesStore: ",comicvine_volume=100,comicvine_issue=1001"},
	}
	return &Server{
		backend:  library.NewXMLBackendFromLibrary(&library.ComicLibrary{Books: books}, "", nil),
		configDB: db,
		cvCache:  cache,
	}
}

func TestSeriesCompleteness_ListAndDetail(t *testing.T) {
	s := newCompletenessServer(t)

	w := httptest.NewRecorder()
	s.handleSeriesCompleteness(w, httptest.NewRequest(http.MethodGet, "/api/library/series-completeness", nil))
	var list struct {
		Volumes []comicvine.VolumeSummary `json:"volumes"`
		Total   int                       `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || list.Volumes[0].VolumeID != 100 || list.Volumes[0].Owned != 1 || list.Volumes[0].Missing != 2 {
		t.Fatalf("list = %+v", list)
	}

	w = httptest.NewRecorder()
	s.handleSeriesCompletenessDetail(w, httptest.NewRequest(http.MethodGet, "/api/library/series-completeness/100", nil))
	var sc comicvine.SeriesCompleteness
	if err := json.NewDecoder(w.Body).Decode(&sc); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || sc.Total != 3 || sc.Owned != 1 || sc.Missing != 2 || sc.Percent != 33 {
		t.Errorf("detail = %d %+v", w.Code, sc)
	}

	w = httptest.NewRecorder()
	s.handleSeriesCompletenessDetail(w, httptest.NewRequest(http.MethodGet, "/api/library/series-completeness/999", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("unsynced volume status = %d, want 404", w.Code)
	}
}

func TestSeriesCompleteness_AddMissingToWanted(t *testing.T) {
	s := newCompletenessServer(t)
	post := func(ids ...int) MissingToWantedResult {
		body, _ := json.Marshal(MissingToWantedRequest{IssueIDs: ids})
		w := httptest.NewRecorder()
		s.handleSeriesCompletenessDetail(w, httptest.NewRequest(http.MethodPost, "/api/library/series-completeness/100/wanted", bytes.NewReader(body)))
		if w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		var r MissingToWantedResult
		_ = json.NewDecoder(w.Body).Decode(&r)
		return r
	}

	// 1001 is owned (skipped), 1002/1003 are missing, 424242 is not in the volume.
	res := post(1001, 1002, 1003, 424242)
	if res.Added != 2 || res.Skipped != 2 {
		t.Fatalf("result = %+v", res)
	}

	wanted, _ := s.wantedBooks()
	if len(wanted) != 2 {
		t.Fatalf("wanted = %d, want 2", len(wanted))
	}
	for _, b := range wanted {
		if b.Series != "Batman" || b.Publisher != "DC" || b.FilePath != "" {
			t.Errorf("wanted book = %+v", b)
		}
		if got := comicvine.ExtractCVIssueID(b.CustomValuesStore); got != 1002 && got != 1003 {
			t.Errorf("issue id = %d", got)
		}
	}
	for _, b := range wanted {
		if b.Number == "2" && b.Year != 1940 {
			t.Errorf("year = %d, want 1940 from cover date", b.Year)
		}
	}

	// A second request for the same issues adds nothing, and the wanted
	// placeholders do not count as owned.
	if again := post(1002, 1003); again.Added != 0 || again.Skipped != 2 {
		t.Errorf("repeat = %+v", again)
	}
	books, _ := s.backend.GetAllBooks()
	sc, _ := comicvine.BuildSeriesCompleteness(books, s.cvCache, 100)
	if sc.Owned != 1 || sc.Missing != 2 || sc.Wanted != 2 {
		t.Errorf("after wanted: owned=%d missing=%d wanted=%d", sc.Owned, sc.Missing, sc.Wanted)
	}

	rows, total, _ := s.configDB.ListAudit(configdb.AuditFilter{Action: audit.ActionWantedFromMissing})
	if total != 2 || rows[0].BookID == "" {
		t.Errorf("audit rows = %d %+v", total, rows)
	}
}

func TestSeriesCompleteness_NotConfigured(t *testing.T) {
	s := &Server{backend: library.NewXMLBackendFromLibrary(&library.ComicLibrary{}, "", nil)}
	w := httptest.NewRecorder()
	s.handleSeriesCompleteness(w, httptest.NewRequest(http.MethodGet, "/api/library/series-completeness", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}
