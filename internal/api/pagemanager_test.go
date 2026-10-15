package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
)

// pageFixture is a server with one 3-page CBZ (distinct shades per page).
func pageFixture(t *testing.T, cbz config.CBZConvertConfig) (*Server, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "book.cbz")
	f, _ := os.Create(path)
	zw := zip.NewWriter(f)
	for i, name := range []string{"001.png", "002.png", "003.png"} {
		w, _ := zw.Create(name)
		w.Write(pngBytes(t, uint8(30+i*60)))
	}
	zw.Close()
	f.Close()
	book := library.ComicBook{ID: "b1", Series: "S", Number: "1", FilePath: path, PageCount: 3, CurrentPage: 2,
		Pages: []library.ComicPageInfo{{Image: 0, Type: library.PageTypeFrontCover}}}
	s := &Server{
		backend: library.NewXMLBackendFromLibrary(&library.ComicLibrary{Books: []library.ComicBook{book}}, "", nil),
		config: &config.Config{Server: config.ServerConfig{
			CBZConvert: cbz, TrashPath: t.TempDir(), TrashRetentionDays: 30,
		}},
	}
	return s, path
}

func pagesRoute(s *Server, method, url string, body io.Reader, ctype string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, body)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	w := httptest.NewRecorder()
	s.handleBooksRouter(w, req)
	return w
}

func pagePlanBody(t *testing.T, plan string, uploads ...[]byte) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("plan", plan)
	for _, u := range uploads {
		fw, _ := mw.CreateFormFile("upload", "new.png")
		fw.Write(u)
	}
	mw.Close()
	return &buf, mw.FormDataContentType()
}

func TestPageManager_ListAndThumbnail(t *testing.T) {
	s, _ := pageFixture(t, config.CBZConvertConfig{Enabled: true})
	w := pagesRoute(s, http.MethodGet, "/api/library/books/b1/pages", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var info BookPagesInfo
	_ = json.NewDecoder(w.Body).Decode(&info)
	if len(info.Pages) != 3 || !info.CanEdit || info.Version == "" || info.Pages[0].Type != library.PageTypeFrontCover {
		t.Errorf("info = %+v", info)
	}
	img := pagesRoute(s, http.MethodGet, "/api/library/books/b1/pages/1?w=100", nil, "")
	if img.Code != http.StatusOK || img.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("thumbnail = %d %s", img.Code, img.Header().Get("Content-Type"))
	}
	if miss := pagesRoute(s, http.MethodGet, "/api/library/books/b1/pages/9", nil, ""); miss.Code != http.StatusNotFound {
		t.Errorf("missing page = %d", miss.Code)
	}
}

func TestPageManager_BlockedWhenCBZConvertOff(t *testing.T) {
	s, _ := pageFixture(t, config.CBZConvertConfig{})
	var info BookPagesInfo
	_ = json.NewDecoder(pagesRoute(s, http.MethodGet, "/api/library/books/b1/pages", nil, "").Body).Decode(&info)
	if info.CanEdit || info.Reason == "" {
		t.Errorf("info = %+v", info)
	}
	body, ct := pagePlanBody(t, `{"order":[{"page":1},{"page":0}]}`)
	if w := pagesRoute(s, http.MethodPost, "/api/library/books/b1/pages", body, ct); w.Code != http.StatusServiceUnavailable {
		t.Errorf("apply while disabled = %d", w.Code)
	}
}

func TestPageManager_ApplyReorderDeleteAdd(t *testing.T) {
	s, path := pageFixture(t, config.CBZConvertConfig{Enabled: true})
	var info BookPagesInfo
	_ = json.NewDecoder(pagesRoute(s, http.MethodGet, "/api/library/books/b1/pages", nil, "").Body).Decode(&info)

	// Order: last page, upload, first page. Middle page deleted.
	plan := `{"version":"` + info.Version + `","order":[{"page":2},{"upload":0},{"page":0}]}`
	body, ct := pagePlanBody(t, plan, pngBytes(t, 200))
	w := pagesRoute(s, http.MethodPost, "/api/library/books/b1/pages", body, ct)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var res PageManagerResult
	_ = json.NewDecoder(w.Body).Decode(&res)
	if res.PageCount != 3 || res.Deleted != 1 || res.Added != 1 || res.Version == info.Version {
		t.Fatalf("result = %+v", res)
	}

	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, n := range []string{"000.png", "001.png", "002.png", "ComicInfo.xml"} {
		if !names[n] {
			t.Errorf("missing entry %s in %v", n, names)
		}
	}

	book, _ := s.backend.GetBook("b1")
	// Old page 0 (FrontCover) is now page 2; CurrentPage 2 -> 0.
	if book.PageCount != 3 || book.CurrentPage != 0 || len(book.Pages) == 0 {
		t.Fatalf("book = %+v", book)
	}
	var cover bool
	for _, p := range book.Pages {
		cover = cover || (p.Image == 2 && p.Type == library.PageTypeFrontCover)
	}
	if !cover {
		t.Errorf("Pages = %+v", book.Pages)
	}

	// The same plan against the old version is refused.
	body, ct = pagePlanBody(t, plan, pngBytes(t, 200))
	if w := pagesRoute(s, http.MethodPost, "/api/library/books/b1/pages", body, ct); w.Code != http.StatusConflict {
		t.Errorf("stale version = %d, want 409", w.Code)
	}
}

func TestPageManager_RejectsBadInput(t *testing.T) {
	s, _ := pageFixture(t, config.CBZConvertConfig{Enabled: true})
	cases := map[string]struct {
		plan    string
		uploads [][]byte
		want    int
	}{
		"out of range":   {`{"order":[{"page":7}]}`, nil, http.StatusBadRequest},
		"no pages":       {`{"order":[]}`, nil, http.StatusBadRequest},
		"both fields":    {`{"order":[{"page":0,"upload":0}]}`, nil, http.StatusBadRequest},
		"not an image":   {`{"order":[{"page":0},{"upload":0}]}`, [][]byte{[]byte("not an image at all")}, http.StatusBadRequest},
		"identity plan":  {`{"order":[{"page":0},{"page":1},{"page":2}]}`, nil, http.StatusConflict},
		"missing upload": {`{"order":[{"upload":3}]}`, nil, http.StatusBadRequest},
	}
	for name, c := range cases {
		body, ct := pagePlanBody(t, c.plan, c.uploads...)
		if w := pagesRoute(s, http.MethodPost, "/api/library/books/b1/pages", body, ct); w.Code != c.want {
			t.Errorf("%s: status %d (%s), want %d", name, w.Code, w.Body.String(), c.want)
		}
	}
}

// The trash path lives in config.db after migration (config.yaml's in-memory
// copy is cleared); the blocker must read the effective value.
func TestPageManager_TrashPathFromConfigDB(t *testing.T) {
	s, _ := pageFixture(t, config.CBZConvertConfig{Enabled: true})
	s.config.Server.TrashPath = ""
	db, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s.configDB = db

	var info BookPagesInfo
	_ = json.NewDecoder(pagesRoute(s, http.MethodGet, "/api/library/books/b1/pages", nil, "").Body).Decode(&info)
	if info.CanEdit || info.Reason != "trash_path is not configured" {
		t.Fatalf("nothing configured: %+v", info)
	}
	if err := db.UpsertTrashSettings(configdb.TrashSettings{Path: t.TempDir(), RetentionDays: 30}); err != nil {
		t.Fatal(err)
	}
	info = BookPagesInfo{}
	_ = json.NewDecoder(pagesRoute(s, http.MethodGet, "/api/library/books/b1/pages", nil, "").Body).Decode(&info)
	if !info.CanEdit {
		t.Errorf("trash set in config.db should allow edits: %+v", info)
	}
}
