package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/library"
)

// relinkFixture: one folder holding the files named in present; each book in
// books gets FilePath = dir/<name>. Returns the server and the folder.
func relinkFixture(t *testing.T, present []string, books []library.ComicBook) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	for _, n := range present {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for i := range books {
		if books[i].FilePath != "" && !filepath.IsAbs(books[i].FilePath) {
			books[i].FilePath = filepath.Join(dir, books[i].FilePath)
		}
	}
	s := &Server{
		backend: library.NewXMLBackendFromLibrary(&library.ComicLibrary{Books: books}, "", nil),
		config:  &config.Config{},
	}
	return s, dir
}

func postJSONTo(s *Server, h http.HandlerFunc, url string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func TestRelink_PreviewThenApply(t *testing.T) {
	s, dir := relinkFixture(t, []string{"Batman 005.cbz"}, []library.ComicBook{
		{ID: "b1", Series: "Batman", Number: "5", FilePath: "Batman 005.cbr"},
	})

	w := postJSONTo(s, s.handleBooksRouter, "/api/library/books/b1/relink", map[string]any{"apply": false})
	var out RelinkOutcome
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 200 || out.Status != relinkFound || out.Applied || filepath.Base(out.NewPath) != "Batman 005.cbz" {
		t.Fatalf("preview = %d %+v", w.Code, out)
	}
	if got, _ := s.backend.GetBook("b1"); filepath.Ext(got.FilePath) != ".cbr" {
		t.Fatalf("preview changed the book: %s", got.FilePath)
	}

	w = postJSONTo(s, s.handleBooksRouter, "/api/library/books/b1/relink", map[string]any{"apply": true})
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if !out.Applied {
		t.Fatalf("apply = %d %+v", w.Code, out)
	}
	got, _ := s.backend.GetBook("b1")
	if got.FilePath != filepath.Join(dir, "Batman 005.cbz") || got.FileSize != 1 {
		t.Errorf("book = %+v", got)
	}
}

func TestRelink_SkipsFileAnotherBookUses(t *testing.T) {
	s, _ := relinkFixture(t, []string{"Batman 005.cbz"}, []library.ComicBook{
		{ID: "b1", Series: "Batman", Number: "5", FilePath: "Batman 005.cbr"},
		{ID: "b2", Series: "Batman", Number: "5", FilePath: "Batman 005.cbz"}, // owns the file
	})
	w := postJSONTo(s, s.handleBooksRouter, "/api/library/books/b1/relink", map[string]any{"apply": true})
	var out RelinkOutcome
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.Status != relinkNotFound || out.Applied {
		t.Errorf("out = %+v", out)
	}
}

func TestRelink_NotMissingAndFolderMissing(t *testing.T) {
	s, dir := relinkFixture(t, []string{"here.cbz"}, []library.ComicBook{
		{ID: "ok", Series: "A", Number: "1", FilePath: "here.cbz"},
		{ID: "nodir", Series: "B", Number: "1", FilePath: filepath.Join("/no/such/folder", "x.cbz")},
		{ID: "wanted", Series: "C", Number: "1"},
	})
	_ = dir
	for id, want := range map[string]string{"ok": relinkNotMissing, "nodir": relinkFolderMissing, "wanted": relinkNoFile} {
		w := postJSONTo(s, s.handleBooksRouter, "/api/library/books/"+id+"/relink", map[string]any{})
		var out RelinkOutcome
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		if out.Status != want {
			t.Errorf("%s: status %q, want %q", id, out.Status, want)
		}
	}
}

func TestRemoveRecord_OnlyWhenFileIsGone(t *testing.T) {
	s, _ := relinkFixture(t, []string{"here.cbz"}, []library.ComicBook{
		{ID: "present", Series: "A", Number: "1", FilePath: "here.cbz"},
		{ID: "gone", Series: "B", Number: "1", FilePath: "gone.cbz"},
		{ID: "wanted", Series: "C", Number: "1"},
	})
	for id, want := range map[string]int{"present": 409, "wanted": 409, "gone": 200} {
		w := postJSONTo(s, s.handleBooksRouter, "/api/library/books/"+id+"/remove-record", map[string]any{})
		if w.Code != want {
			t.Errorf("%s: status %d, want %d (%s)", id, w.Code, want, w.Body.String())
		}
	}
	if b, _ := s.backend.GetBook("gone"); b != nil {
		t.Error("record should be removed")
	}
	if b, _ := s.backend.GetBook("present"); b == nil {
		t.Error("record with a file on disk must stay")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(mustPath(t, s, "present")), "here.cbz")); err != nil {
		t.Errorf("file was touched: %v", err)
	}
}

func mustPath(t *testing.T, s *Server, id string) string {
	t.Helper()
	b, err := s.backend.GetBook(id)
	if err != nil || b == nil {
		t.Fatalf("no book %s", id)
	}
	return b.FilePath
}

func TestRelinkMissing_BulkPreviewAndApply(t *testing.T) {
	s, dir := relinkFixture(t, []string{"Batman 005.cbz", "Robin 001.cbz"}, []library.ComicBook{
		{ID: "b1", Series: "Batman", Number: "5", FilePath: "Batman 005.cbr"},
		{ID: "b2", Series: "Robin", Number: "1", FilePath: "gone.cbz"},
		{ID: "b3", Series: "Nobody", Number: "9", FilePath: "lost.cbz"},
	})
	// Pretend the background disk pass already found all three missing.
	s.disk.interval = time.Hour
	s.disk.checkedAt = time.Now()
	s.disk.missing = map[string]bool{"b1": true, "b2": true, "b3": true}

	w := postJSONTo(s, s.handleRelinkMissing, "/api/library/workflow/relink-missing", map[string]any{"apply": false})
	var resp RelinkMissingResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || w.Code != 200 {
		t.Fatalf("preview %d %s", w.Code, w.Body.String())
	}
	if resp.Total != 3 || resp.Counts[relinkFound] != 2 || resp.Counts[relinkNotFound] != 1 || resp.Applied {
		t.Fatalf("preview = %+v", resp)
	}
	if b, _ := s.backend.GetBook("b1"); filepath.Ext(b.FilePath) != ".cbr" {
		t.Fatal("preview must not change books")
	}

	w = postJSONTo(s, s.handleRelinkMissing, "/api/library/workflow/relink-missing", map[string]any{"apply": true})
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if w.Code != 200 || !resp.Applied {
		t.Fatalf("apply %d %s", w.Code, w.Body.String())
	}
	b1, _ := s.backend.GetBook("b1")
	b2, _ := s.backend.GetBook("b2")
	b3, _ := s.backend.GetBook("b3")
	if b1.FilePath != filepath.Join(dir, "Batman 005.cbz") || b2.FilePath != filepath.Join(dir, "Robin 001.cbz") {
		t.Errorf("b1=%s b2=%s", b1.FilePath, b2.FilePath)
	}
	if b3.FilePath != filepath.Join(dir, "lost.cbz") {
		t.Errorf("unmatched book changed: %s", b3.FilePath)
	}
	// Relinked books leave the missing set right away.
	if s.disk.missing["b1"] || s.disk.missing["b2"] || !s.disk.missing["b3"] {
		t.Errorf("missing = %v", s.disk.missing)
	}
}

func TestRelink_ManualPick(t *testing.T) {
	s, dir := relinkFixture(t, []string{"Odd Name.cbz", "notes.txt", "Owned.cbz"}, []library.ComicBook{
		{ID: "b1", Series: "Batman", Number: "5", FilePath: "gone.cbr"},
		{ID: "b2", Series: "Other", Number: "1", FilePath: "Owned.cbz"},
	})
	cases := map[string]string{
		filepath.Join(dir, "Odd Name.cbz"): relinkFound,
		filepath.Join(dir, "notes.txt"):    relinkPickBad,
		filepath.Join(dir, "nope.cbz"):     relinkPickBad,
		dir:                                relinkPickBad,
		filepath.Join(dir, "Owned.cbz"):    relinkPickTaken,
	}
	for pick, want := range cases {
		w := postJSONTo(s, s.handleBooksRouter, "/api/library/books/b1/relink", map[string]any{"apply": false, "file_path": pick})
		var out RelinkOutcome
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		if out.Status != want {
			t.Errorf("%s: status %q, want %q", filepath.Base(pick), out.Status, want)
		}
	}
	good := filepath.Join(dir, "Odd Name.cbz")
	w := postJSONTo(s, s.handleBooksRouter, "/api/library/books/b1/relink", map[string]any{"apply": true, "file_path": good})
	var out RelinkOutcome
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	got, _ := s.backend.GetBook("b1")
	if !out.Applied || got.FilePath != good || got.FileSize != 1 {
		t.Errorf("apply = %+v, book = %+v", out, got)
	}
}

func TestRelink_ManualPickKeepsWindowsStyleUnderRoot(t *testing.T) {
	s, dir := relinkFixture(t, nil, nil)
	s.config.Server.LibraryRoot = dir
	s.config.Server.LibrarySourceRoot = `G:\Comics`
	raw, ok := s.rawPathForPick(filepath.Join(dir, "sub", "x.cbz"), `G:\Comics\old\y.cbr`)
	if !ok || raw != `G:\Comics\sub\x.cbz` {
		t.Errorf("raw = %q ok=%v", raw, ok)
	}
	if _, ok := s.rawPathForPick("/elsewhere/x.cbz", `G:\Comics\y.cbr`); ok {
		t.Error("a pick outside library_root must be refused")
	}
}
