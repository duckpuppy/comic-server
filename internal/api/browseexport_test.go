package api

import (
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func exportRequest(t *testing.T, s *Server, body string) (*httptest.ResponseRecorder, [][]string) {
	t.Helper()
	w := httptest.NewRecorder()
	s.handleBrowseExportCSV(w, httptest.NewRequest(http.MethodPost, "/api/library/browse/export-csv", strings.NewReader(body)))
	if w.Code != http.StatusOK {
		return w, nil
	}
	raw := strings.TrimPrefix(w.Body.String(), "\xEF\xBB\xBF")
	rows, err := csv.NewReader(strings.NewReader(raw)).ReadAll()
	if err != nil {
		t.Fatalf("not valid CSV: %v\n%s", err, raw)
	}
	return w, rows
}

func TestCSVCell_GuardsFormulas(t *testing.T) {
	for in, want := range map[string]string{
		"Batman": "Batman", "": "", "-1": "-1", "0.5": "0.5",
		"=1+2": "'=1+2", "+cmd": "'+cmd", "@SUM(A1)": "'@SUM(A1)", "-cmd|x": "'-cmd|x", "\tx": "'\tx",
	} {
		if got := csvCell(in); got != want {
			t.Errorf("csvCell(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExportColumns_DefaultsAreKnownAndUnique(t *testing.T) {
	seen := map[string]bool{}
	defaults := 0
	for _, c := range exportColumns {
		if seen[c.Key] {
			t.Errorf("duplicate column %q", c.Key)
		}
		seen[c.Key] = true
		if c.get == nil {
			t.Errorf("column %q has no getter", c.Key)
		}
		if c.Default {
			defaults++
		}
	}
	if defaults < 5 {
		t.Errorf("only %d default columns", defaults)
	}
	// Every library-field column must name a real field.
	for _, c := range exportColumns {
		b := &library.ComicBook{}
		_ = c.get(b)
	}
}

func TestBrowseExportCSV_ScopeColumnsAndEscaping(t *testing.T) {
	s := newBrowseTestServer(t, []library.ComicBook{
		{ID: "1", Series: "Batman", Number: "2", Year: 1940, Title: `He said "hi", twice`, FilePath: `G:\Comics\Batman 002.cbz`, CustomValuesStore: ",comicvine_volume=77,mine=x"},
		{ID: "2", Series: "Batman", Number: "10", Title: "=HYPERLINK(\"http://x\")"},
		{ID: "3", Series: "Superman", Number: "1"},
	})

	// Default columns, Batman only, issue order.
	w, rows := exportRequest(t, s, `{"browse":{"show_all":true,"query":"Batman","sort":"number"}}`)
	if rows == nil {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Errorf("headers: %q %q", ct, w.Header().Get("Content-Disposition"))
	}
	if !strings.HasPrefix(w.Body.String(), "\xEF\xBB\xBF") {
		t.Error("missing byte order mark")
	}
	if len(rows) != 3 || rows[1][0] != "Batman" || rows[1][1] != "2" || rows[2][1] != "10" {
		t.Fatalf("rows = %v", rows)
	}

	// Chosen columns, a custom value, quoting, formula guard, file name from a Windows path.
	_, rows = exportRequest(t, s, `{"browse":{"show_all":true,"query":"Batman","sort":"number"},"columns":["Title","FileName","ComicVineVolume","custom:mine","Year"]}`)
	if rows[0][0] != "Title" || rows[0][3] != "mine" {
		t.Fatalf("header = %v", rows[0])
	}
	r1, r2 := rows[1], rows[2]
	if r1[0] != `He said "hi", twice` || r1[1] != "Batman 002.cbz" || r1[2] != "77" || r1[3] != "x" || r1[4] != "1940" {
		t.Errorf("row 1 = %v", r1)
	}
	if !strings.HasPrefix(r2[0], "'=") || r2[4] != "" {
		t.Errorf("row 2 = %v (formula guard, unset year empty)", r2)
	}

	// The whole library, not a page.
	_, rows = exportRequest(t, s, `{"browse":{"show_all":true}}`)
	if len(rows) != 4 {
		t.Errorf("show_all rows = %d, want 4", len(rows))
	}
}

func TestBrowseExportCSV_Refusals(t *testing.T) {
	s := newBrowseTestServer(t, []library.ComicBook{{ID: "1", Series: "Batman"}})
	for name, c := range map[string]struct {
		body string
		want int
	}{
		"unknown column":  {`{"browse":{"show_all":true},"columns":["Nope"]}`, http.StatusBadRequest},
		"bad custom key":  {`{"browse":{"show_all":true},"columns":["custom:a\"b"]}`, http.StatusBadRequest},
		"no matchers":     {`{"browse":{}}`, http.StatusConflict},
		"nothing matches": {`{"browse":{"show_all":true,"query":"Zzzz"}}`, http.StatusConflict},
		"bad body":        {`nope`, http.StatusBadRequest},
	} {
		if w, _ := exportRequest(t, s, c.body); w.Code != c.want {
			t.Errorf("%s: status = %d, want %d", name, w.Code, c.want)
		}
	}
}
