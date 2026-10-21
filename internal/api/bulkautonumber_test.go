package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
)

func TestNaturalLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"Issue 2", "Issue 10", true},
		{"Issue 10", "Issue 2", false},
		{"issue 2", "Issue 3", true}, // case is ignored
		{"a 02", "a 2", false},       // leading zeros are the same number
		{"a", "ab", true},
		{"b", "a1", false},
	}
	for _, c := range cases {
		if got := naturalLess(c.a, c.b); got != c.want {
			t.Errorf("naturalLess(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestAutonumberFormat(t *testing.T) {
	a := Autonumber{Start: 5, Step: 5, Pad: 3}
	if a.format(0) != "005" || a.format(2) != "015" || a.format(30) != "155" {
		t.Errorf("got %s %s %s", a.format(0), a.format(2), a.format(30))
	}
	if (Autonumber{Start: 1, Step: 1, Pad: 2}).format(99) != "100" {
		t.Error("padding must not cut a longer number")
	}
	bad := []Autonumber{{Order: "x"}, {Order: "date", Step: -1}, {Order: "date", Start: -1}, {Order: "date", Pad: 11}}
	for i, b := range bad {
		if b.validate() == nil {
			t.Errorf("case %d should be invalid: %+v", i, b)
		}
	}
	ok := Autonumber{Order: "file"}
	if err := ok.validate(); err != nil || ok.Step != 1 {
		t.Errorf("default step: %v %+v", err, ok)
	}
}

func TestPlanAutonumberOrders(t *testing.T) {
	books := []*library.ComicBook{
		{ID: "a", Title: "Zed 10", FilePath: "/x/c 10.cbz", Year: 1990, Month: 5},
		{ID: "b", Title: "Zed 2", FilePath: "/x/c 2.cbz"},
		{ID: "c", Title: "Alpha", FilePath: "/x/c 1.cbz", Year: 1990, Month: 1},
	}
	order := func(o string) string {
		nums, _ := planAutonumber(books, Autonumber{Order: o, Start: 1, Step: 1})
		return nums["a"] + nums["b"] + nums["c"]
	}
	if got := order("current"); got != "123" {
		t.Errorf("current = %s", got)
	}
	if got := order("date"); got != "231" { // c (Jan) < a (May) < b (undated, last)
		t.Errorf("date = %s", got)
	}
	if got := order("title"); got != "321" { // Alpha, Zed 2, Zed 10
		t.Errorf("title = %s", got)
	}
	if got := order("file"); got != "321" { // c 1, c 2, c 10
		t.Errorf("file = %s", got)
	}
}

func TestPlanAutonumberSkipsComicVineBooks(t *testing.T) {
	books := []*library.ComicBook{
		{ID: "a", Series: "S", Number: "7", CustomValuesStore: "comicvine_issue=123"},
		{ID: "b", Series: "S"},
	}
	nums, skipped := planAutonumber(books, Autonumber{Order: "current", Start: 1, Step: 1})
	if _, has := nums["a"]; has || nums["b"] != "1" {
		t.Errorf("nums = %v", nums)
	}
	if len(skipped) != 1 || skipped[0].ID != "a" || skipped[0].Number != "7" {
		t.Errorf("skipped = %+v", skipped)
	}
}

func autonumberBooks() []library.ComicBook {
	return []library.ComicBook{
		{ID: "w1", Series: "Web", Number: "9", Title: "Ep 2", FilePath: "w2.cbz"},
		{ID: "w2", Series: "Web", Number: "", Title: "Ep 10", FilePath: "w10.cbz"},
		{ID: "w3", Series: "Web", Number: "4", Title: "Ep 1", FilePath: "w1.cbz"},
		{ID: "cv", Series: "Web", Number: "3", Title: "Ep 0", FilePath: "w0.cbz", CustomValuesStore: "comicvine_issue=55"},
	}
}

func TestBulkEdit_AutonumberPreviewAndApply(t *testing.T) {
	s := newDuplicatesTestServer(t, t.TempDir(), autonumberBooks())
	edit := `"edit":{"autonumber":{"order":"title","start":1,"step":1,"pad":2}}`
	ids := `"book_ids":["w1","w2","w3","cv"],"expected_count":4`
	code, resp, raw := postBulkEdit(t, s, `{`+ids+`,`+edit+`}`)
	if code != http.StatusOK {
		t.Fatalf("preview: %d %s", code, raw)
	}
	if resp.Applied || resp.Total != 4 || resp.Changed != 3 || resp.Skipped != 1 || resp.Fields["Number"] != 3 {
		t.Errorf("preview resp = %+v", resp)
	}
	if len(resp.SkippedBooks) != 1 || resp.SkippedBooks[0].ID != "cv" {
		t.Errorf("skipped books = %+v", resp.SkippedBooks)
	}
	if b, _ := s.backend.GetBook("w1"); b.Number != "9" || b.ComicBookIsDirty {
		t.Errorf("preview wrote: %q dirty=%v", b.Number, b.ComicBookIsDirty)
	}
	if rows, _, _ := s.configDB.ListAudit(configdb.AuditFilter{Limit: 10}); len(rows) != 0 {
		t.Errorf("preview was audited: %d rows", len(rows))
	}

	if code, _, raw = postBulkEdit(t, s, `{`+ids+`,"apply":true,`+edit+`}`); code != http.StatusOK {
		t.Fatalf("apply: %d %s", code, raw)
	}
	// Titles: "Ep 1" < "Ep 2" < "Ep 10".
	for id, want := range map[string]string{"w3": "01", "w1": "02", "w2": "03", "cv": "3"} {
		if b, _ := s.backend.GetBook(id); b.Number != want {
			t.Errorf("%s Number = %q, want %q", id, b.Number, want)
		}
	}
	if rows, _, _ := s.configDB.ListAudit(configdb.AuditFilter{Limit: 10}); len(rows) != 3 {
		t.Errorf("audit rows = %d, want 3", len(rows))
	}
	// A second run changes nothing.
	_, again, _ := postBulkEdit(t, s, `{`+ids+`,`+edit+`}`)
	if again.Changed != 0 || again.Unchanged != 3 || again.Skipped != 1 {
		t.Errorf("repeat resp = %+v", again)
	}
}

func TestBulkEdit_AutonumberRules(t *testing.T) {
	s := newDuplicatesTestServer(t, t.TempDir(), autonumberBooks())
	bad := []string{
		`"edit":{"autonumber":{"order":"nope"}}`,
		`"edit":{"autonumber":{"order":"date","step":-2}}`,
		`"edit":{"autonumber":{"order":"date"},"fields":{"Publisher":"X"}}`,
		`"edit":{"autonumber":{"order":"date"},"read":"read"}`,
	}
	for _, e := range bad {
		if code, _, raw := postBulkEdit(t, s, `{"book_ids":["w1"],"expected_count":1,`+e+`}`); code != http.StatusBadRequest {
			t.Errorf("%s: status %d %s", e, code, raw)
		}
	}
	// The single-book edit refuses it.
	book, _ := s.backend.GetBook("w1")
	if _, err := applyBookEdit(book, BookEditRequest{Autonumber: &Autonumber{Order: "date"}}, time.Now()); err == nil {
		t.Error("single-book edit should refuse autonumber")
	}
}
