package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/library"
)

func getJSON(t *testing.T, s *Server, path string, out any) int {
	t.Helper()
	w := httptest.NewRecorder()
	s.handleBooksRouter(w, httptest.NewRequest(http.MethodGet, path, nil))
	if w.Code == http.StatusOK && out != nil {
		if err := json.NewDecoder(w.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
	return w.Code
}

func TestBookDetail(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.cbz")
	writeTestCBZWithComicInfo(t, p, `<?xml version="1.0"?><ComicInfo><Series>Batman</Series><Number>1</Number><Year>1999</Year><Pages><Page Image="0"/></Pages></ComicInfo>`)
	books := []library.ComicBook{
		{ID: "b1", Series: "Batman", Number: "1", Year: 2011, PageCount: 2, FilePath: p,
			CustomValuesStore: "comicvine_volume=42,comicvine_issue=7"},
		{ID: "w1", Series: "Batman", Number: "2"},
	}
	s := newCBZConvertTestServer(t, config.CBZConvertConfig{}, t.TempDir(), books)

	var d BookDetail
	if code := getJSON(t, s, "/api/library/books/b1", &d); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if d.Book.Series != "Batman" || d.File == nil || !d.File.Exists || d.File.Ext != ".cbz" {
		t.Fatalf("detail = %+v file = %+v", d.Book, d.File)
	}
	if d.ComicVine == nil || d.ComicVine.VolumeID != 42 || d.ComicVine.IssueURL != "https://comicvine.gamespot.com/issue/4000-7/" {
		t.Fatalf("comicvine = %+v", d.ComicVine)
	}
	if d.CustomValues["comicvine_volume"] != "42" {
		t.Errorf("custom values = %v", d.CustomValues)
	}
	hasFlag := false
	for _, f := range d.Flags {
		if f.Flag == "missing_format" {
			hasFlag = true
		}
	}
	if !hasFlag {
		t.Errorf("flags = %+v", d.Flags)
	}

	var ci ComicInfoComparison
	if code := getJSON(t, s, "/api/library/books/b1/comicinfo", &ci); code != http.StatusOK {
		t.Fatalf("comicinfo status %d", code)
	}
	if !ci.Present || ci.Raw == "" || ci.Same == 0 {
		t.Fatalf("comicinfo = %+v", ci)
	}
	diff := map[string]ComicInfoField{}
	for _, f := range ci.Diff {
		diff[f.Field] = f
	}
	if diff["Year"].File != "1999" || diff["Year"].Library != "2011" {
		t.Errorf("Year diff = %+v (all %+v)", diff["Year"], ci.Diff)
	}
	if _, ok := diff["Series"]; ok {
		t.Errorf("Series should agree: %+v", ci.Diff)
	}
	if _, ok := diff["Pages"]; ok {
		t.Error("Pages must be skipped")
	}

	// Fileless book: no file block, comicinfo explains.
	d = BookDetail{}
	getJSON(t, s, "/api/library/books/w1", &d)
	if d.File != nil {
		t.Errorf("fileless book has file %+v", d.File)
	}
	ci = ComicInfoComparison{}
	getJSON(t, s, "/api/library/books/w1/comicinfo", &ci)
	if ci.Present || ci.Error == "" {
		t.Errorf("fileless comicinfo = %+v", ci)
	}

	if code := getJSON(t, s, "/api/library/books/nope", nil); code != http.StatusNotFound {
		t.Errorf("missing book status %d", code)
	}
	if code := getJSON(t, s, "/api/library/books/b1/other", nil); code != http.StatusNotFound {
		t.Errorf("unknown suffix status %d", code)
	}
}
