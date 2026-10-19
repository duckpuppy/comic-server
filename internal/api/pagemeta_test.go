package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
)

func postPageMeta(s *Server, body string) int {
	w := pagesRoute(s, http.MethodPost, "/api/library/books/b1/pages/meta", strings.NewReader(body), "application/json")
	return w.Code
}

func TestPageMeta_SetsTypesAndBookmarksAndMarksDirty(t *testing.T) {
	s, path := pageFixture(t, config.CBZConvertConfig{Enabled: true})
	v := fileVersion(path)

	body := `{"version":"` + v + `","pages":[
		{"index":0,"type":"","bookmark":"Start"},
		{"index":2,"type":"Advertisement","bookmark":""}]}`
	if code := postPageMeta(s, body); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	b, _ := s.backend.GetBook("b1")
	if len(b.Pages) != 2 || b.Pages[0].Image != 0 || b.Pages[1].Image != 2 {
		t.Fatalf("pages = %+v", b.Pages)
	}
	if b.Pages[0].Type != "" || b.Pages[0].Bookmark != "Start" || b.Pages[1].Type != "Advertisement" {
		t.Errorf("values wrong: %+v", b.Pages)
	}
	if !b.ComicInfoIsDirty || !b.ComicBookIsDirty {
		t.Error("file should be marked out of date")
	}

	// The page list reports the new values and the type menu.
	w := pagesRoute(s, http.MethodGet, "/api/library/books/b1/pages", nil, "")
	var info BookPagesInfo
	_ = json.Unmarshal(w.Body.Bytes(), &info)
	if info.Pages[2].Type != "Advertisement" || info.Pages[0].Bookmark != "Start" || len(info.PageTypes) < 8 {
		t.Errorf("listing = %+v types=%v", info.Pages, info.PageTypes)
	}
}

func TestPageMeta_Refusals(t *testing.T) {
	s, path := pageFixture(t, config.CBZConvertConfig{Enabled: true})
	v := fileVersion(path)
	cases := []struct {
		name, body string
		want       int
	}{
		{"bad type", `{"pages":[{"index":1,"type":"Cover"}]}`, http.StatusBadRequest},
		{"out of range", `{"pages":[{"index":3,"type":"Story"}]}`, http.StatusBadRequest},
		{"negative", `{"pages":[{"index":-1,"type":"Story"}]}`, http.StatusBadRequest},
		{"duplicate", `{"pages":[{"index":1,"type":"Story"},{"index":1,"type":"Other"}]}`, http.StatusBadRequest},
		{"long bookmark", `{"pages":[{"index":1,"bookmark":"` + strings.Repeat("x", 201) + `"}]}`, http.StatusBadRequest},
		{"empty", `{"pages":[]}`, http.StatusBadRequest},
		{"stale version", `{"version":"1-1","pages":[{"index":1,"type":"Story"}]}`, http.StatusConflict},
		{"no change", `{"version":"` + v + `","pages":[{"index":0,"type":"FrontCover"}]}`, http.StatusConflict},
		{"clearing what is not there", `{"version":"` + v + `","pages":[{"index":2,"type":"","bookmark":""}]}`, http.StatusConflict},
	}
	for _, c := range cases {
		if got := postPageMeta(s, c.body); got != c.want {
			t.Errorf("%s: status = %d, want %d", c.name, got, c.want)
		}
		if b, _ := s.backend.GetBook("b1"); len(b.Pages) != 1 {
			t.Fatalf("after %q the stored book has %d page records", c.name, len(b.Pages))
		}
	}
	if b, _ := s.backend.GetBook("b1"); b.ComicInfoIsDirty || len(b.Pages) != 1 {
		t.Errorf("a refused request changed the book: %+v", b)
	}
}
