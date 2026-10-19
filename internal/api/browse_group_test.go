package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func stackBooks() []*library.ComicBook {
	return []*library.ComicBook{
		{ID: "a3", Series: "The X-Men", Volume: 1, Number: "3", Publisher: "Marvel", Year: 1963},
		{ID: "a1", Series: "X-Men", Volume: 1, Number: "1", Publisher: "Marvel", Year: 1963},
		{ID: "b1", Series: "X-Men", Volume: 2, Number: "1", Publisher: "Marvel", Year: 1991},
		{ID: "c1", Series: "Saga", Number: "1", Publisher: "Image", Year: 2012},
		{ID: "n1", Number: "1"},
	}
}

func TestBuildStacks_SeriesAndVolumeKeepApart(t *testing.T) {
	st := buildStacks(stackBooks(), "series", "", false)
	if len(st) != 4 {
		t.Fatalf("stacks = %d, want 4: %+v", len(st), st)
	}
	// "The X-Men" and "X-Men" volume 1 are one stack; the lowest issue is the cover.
	var x1 *BrowseStack
	for i := range st {
		if st[i].Sub == "Vol 1" {
			x1 = &st[i]
		}
	}
	if x1 == nil || x1.Count != 2 || x1.CoverID != "a1" {
		t.Fatalf("X-Men vol 1 stack wrong: %+v", x1)
	}
	if st[len(st)-1].Label != "(no series)" {
		t.Fatalf("empty series should sort last: %+v", st)
	}
}

func TestBuildStacks_CountSortAndDesc(t *testing.T) {
	st := buildStacks(stackBooks(), "publisher", "count", true)
	if st[0].Label != "Marvel" || st[0].Count != 3 {
		t.Fatalf("largest publisher first expected: %+v", st)
	}
}

func TestFilterToStack_MatchesBuildStacksKey(t *testing.T) {
	books := stackBooks()
	for _, g := range []string{"series", "publisher", "year", "format"} {
		for _, s := range buildStacks(books, g, "", false) {
			if got := len(filterToStack(books, g, s.Key)); got != s.Count {
				t.Errorf("%s %q: filter gives %d, stack count %d", g, s.Key, got, s.Count)
			}
		}
	}
}

func benchLibrary(n int) []*library.ComicBook {
	books := make([]*library.ComicBook, n)
	for i := range books {
		books[i] = &library.ComicBook{
			ID:        fmt.Sprintf("id-%d", i),
			Series:    fmt.Sprintf("The Series Number %d", i/25),
			Volume:    i % 3,
			Number:    fmt.Sprint(i % 25),
			Publisher: fmt.Sprintf("Publisher %d", i%200),
			Year:      1950 + i%75,
		}
	}
	return books
}

func BenchmarkBuildStacksSeries67k(b *testing.B) {
	books := benchLibrary(67000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildStacks(books, "series", "", false)
	}
}

func BenchmarkFilterToStackSeries67k(b *testing.B) {
	books := benchLibrary(67000)
	key, _, _ := stackKey("series", books[30000])
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filterToStack(books, "series", key)
	}
}

func browseStackRequest(t *testing.T, s *Server, body string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/library/browse", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleBrowse(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestHandleBrowse_StacksThenOpenStack(t *testing.T) {
	s := newBrowseTestServer(t, []library.ComicBook{
		{ID: "1", Series: "Batman", Number: "10"},
		{ID: "2", Series: "Batman", Number: "2"},
		{ID: "3", Series: "Superman", Number: "1"},
	})
	out := browseStackRequest(t, s, `{"show_all":true,"group_by":"series"}`)
	stacks, _ := out["stacks"].([]any)
	if len(stacks) != 2 || out["total"].(float64) != 2 || out["total_books"].(float64) != 3 {
		t.Fatalf("stacks response wrong: %v", out)
	}
	key := stacks[0].(map[string]any)["key"].(string)
	if key == "" {
		t.Fatal("empty stack key")
	}

	// Opening the stack returns its books in issue order (2 before 10).
	out = browseStackRequest(t, s, `{"show_all":true,"group_by":"series","group_key":"`+key+`"}`)
	comics, _ := out["comics"].([]any)
	if len(comics) != 2 || comics[0].(map[string]any)["number"] != "2" {
		t.Fatalf("opened stack wrong: %v", out)
	}

	// The same scope for bulk edit / Data Manager sees only that stack.
	got, err := s.resolveBrowseCandidates(BrowseRequest{ShowAll: true, GroupBy: "series", GroupKey: key})
	if err != nil || len(got) != 2 {
		t.Fatalf("scope = %d books, err %v; want 2", len(got), err)
	}
}

func TestHandleBrowse_StacksPageByStack(t *testing.T) {
	var books []library.ComicBook
	for i := 0; i < 5; i++ {
		books = append(books, library.ComicBook{ID: fmt.Sprint("a", i), Series: "A", Number: fmt.Sprint(i)}, library.ComicBook{ID: fmt.Sprint("b", i), Series: "B"})
	}
	s := newBrowseTestServer(t, books)
	req := httptest.NewRequest(http.MethodPost, "/api/library/browse?limit=1&offset=1", strings.NewReader(`{"show_all":true,"group_by":"series"}`))
	w := httptest.NewRecorder()
	s.handleBrowse(w, req)
	var out map[string]any
	_ = json.NewDecoder(w.Body).Decode(&out)
	st, _ := out["stacks"].([]any)
	if len(st) != 1 || st[0].(map[string]any)["label"] != "B" || out["has_more"] != false {
		t.Fatalf("second page wrong: %v", out)
	}
}
