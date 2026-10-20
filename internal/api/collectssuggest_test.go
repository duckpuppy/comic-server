package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/collects"
	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/library"
)

func TestCollectsSuggestions_ListAcceptDismiss(t *testing.T) {
	books := []library.ComicBook{
		{ID: "a", Series: "Batman TPB", Number: "1", FilePath: "a.cbz", Summary: "Collects Batman (1940) #1-3 and Superman #5."},
		{ID: "b", Series: "Odd TPB", Number: "1", FilePath: "b.cbz", Summary: "Collects Unknown Thing #1-2"},
		{ID: "c", Series: "No Summary", FilePath: "c.cbz"},
		{ID: "d", Series: "Linked TPB", FilePath: "d.cbz", Summary: "Collects Batman (1940) #1-3"},
		{ID: "e", Series: "Placeholder", FilePath: "", Summary: "Collects Batman (1940) #1-3"},
		{ID: "f", Series: "Ambiguous TPB", FilePath: "f.cbz", Summary: "Collects Robin #1-4"},
	}
	collects.Set(&books[3], []collects.Entry{{Volume: 100, From: "1"}})
	s := newDuplicatesTestServer(t, t.TempDir(), books)
	cache, err := comicvine.OpenCache(filepath.Join(t.TempDir(), "cv.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cache.Close() })
	for _, v := range []comicvine.CachedVolume{
		{CVID: 100, Name: "Batman", StartYear: "1940"},
		{CVID: 101, Name: "Robin", StartYear: "1991"},
		{CVID: 102, Name: "Robin", StartYear: "2000"},
	} {
		v.SyncStatus, v.SyncedAt = comicvine.SyncStatusSynced, time.Now()
		cache.UpsertVolume(&v)
	}
	cache.UpsertIssues([]comicvine.CachedIssue{
		{CVID: 1001, VolumeCVID: 100, Number: "1"}, {CVID: 1002, VolumeCVID: 100, Number: "2"}, {CVID: 1003, VolumeCVID: 100, Number: "3"},
	})
	s.cvCache = cache

	list := func(query string) (int, []string, string) {
		w := httptest.NewRecorder()
		s.handleCollectsSuggestions(w, httptest.NewRequest(http.MethodGet, "/api/library/collects-suggestions"+query, nil))
		var out struct {
			Total       int `json:"total"`
			Suggestions []struct {
				BookID  string `json:"book_id"`
				Entries []struct {
					Volume     int    `json:"volume"`
					Series     string `json:"series"`
					Confidence string `json:"confidence"`
				} `json:"entries"`
			} `json:"suggestions"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v\n%s", err, w.Body.String())
		}
		var ids []string
		for _, sg := range out.Suggestions {
			ids = append(ids, sg.BookID)
		}
		return out.Total, ids, w.Body.String()
	}

	total, ids, body := list("")
	if total != 3 || strings.Join(ids, ",") != "a,f,b" {
		t.Fatalf("list = %d %v (placeholder, no summary and already linked books must be absent)\n%s", total, ids, body)
	}
	if !strings.Contains(body, `"volume":100`) || !strings.Contains(body, `"confidence":"high"`) {
		t.Errorf("book a should resolve to volume 100 with high confidence: %s", body)
	}
	if !strings.Contains(body, "named \\\"Robin\\\"") && !strings.Contains(body, "are named") {
		t.Errorf("book f should say why Robin is ambiguous: %s", body)
	}
	if _, ids, _ = list("?min=high"); strings.Join(ids, ",") != "a" {
		t.Errorf("min=high = %v", ids)
	}
	if _, ids, _ = list("?q=odd"); strings.Join(ids, ",") != "b" {
		t.Errorf("q=odd = %v", ids)
	}

	post := func(id, body string) int {
		w := httptest.NewRecorder()
		s.handleCollectsSuggestionAction(w, httptest.NewRequest(http.MethodPost, "/api/library/collects-suggestions/"+id, strings.NewReader(body)))
		return w.Code
	}
	if code := post("d", `{"action":"accept"}`); code != http.StatusConflict {
		t.Errorf("accept on a linked book: %d", code)
	}
	if code := post("a", `{"action":"bogus"}`); code != http.StatusBadRequest {
		t.Errorf("bad action: %d", code)
	}
	if code := post("a", `{"action":"accept","keep":[0]}`); code != http.StatusOK {
		t.Fatalf("accept: %d", code)
	}
	var d BookDetail
	getJSON(t, s, "/api/library/books/a", &d)
	if len(d.Collects) != 1 || d.Collects[0].Volume != 100 || d.Collects[0].From != "1" || d.Collects[0].To != "3" || d.Collects[0].Source != collects.SourceSuggested {
		t.Errorf("saved links = %+v", d.Collects)
	}
	if code := post("b", `{"action":"dismiss"}`); code != http.StatusOK {
		t.Fatalf("dismiss: %d", code)
	}
	if _, ids, _ = list(""); strings.Join(ids, ",") != "f" {
		t.Errorf("after accept and dismiss = %v", ids)
	}
	getJSON(t, s, "/api/library/books/b", &d)
	if len(d.Collects) != 0 {
		t.Errorf("dismiss must not write links: %+v", d.Collects)
	}
}
