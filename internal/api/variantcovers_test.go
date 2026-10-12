package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
)

func pngBytes(t *testing.T, shade uint8) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = shade
	}
	_ = color.Gray{}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// variantFixture wires a server whose ComicVine client and image CDN are one
// local httptest server.
func variantFixture(t *testing.T, cbz config.CBZConvertConfig) (*Server, string, string) {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/issue/4000-777"):
			fmt.Fprintf(w, `{"status_code":1,"error":"OK","results":{"id":777,"name":"X",
				"image":{"original_url":"%[1]s/img/main.png"},
				"associated_images":[
					{"original_url":"%[1]s/img/21.png","id":21,"caption":null,"image_tags":"All Images"},
					{"original_url":"%[1]s/img/22.png","id":22,"caption":"Variant B","image_tags":"All Images"}]}}`, srv.URL)
		case strings.HasPrefix(r.URL.Path, "/img/21"):
			w.Write(pngBytes(t, 40))
		case strings.HasPrefix(r.URL.Path, "/img/22"):
			w.Write(pngBytes(t, 90))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	cbzPath := filepath.Join(dir, "book.cbz")
	f, _ := os.Create(cbzPath)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("001.png")
	w.Write(pngBytes(t, 10))
	zw.Close()
	f.Close()

	db, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	book := library.ComicBook{ID: "b1", Series: "S", Number: "1", FilePath: cbzPath, PageCount: 1,
		CustomValuesStore: ",comicvine_issue=777"}
	s := &Server{
		backend:        library.NewXMLBackendFromLibrary(&library.ComicLibrary{Books: []library.ComicBook{book}}, "", nil),
		configDB:       db,
		cvClient:       comicvine.NewClient("k", comicvine.WithBaseURL(srv.URL)),
		cvImageAnyHost: true,
		config: &config.Config{Server: config.ServerConfig{
			CBZConvert: cbz, TrashPath: t.TempDir(), TrashRetentionDays: 30,
		}},
	}
	return s, cbzPath, "b1"
}

func variantCall(s *Server, method, id string, body any) *httptest.ResponseRecorder {
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	w := httptest.NewRecorder()
	s.handleVariantCovers(w, httptest.NewRequest(method, "/api/library/variant-covers/"+id, rdr))
	return w
}

func TestVariantCovers_PreviewListsCandidates(t *testing.T) {
	s, _, id := variantFixture(t, config.CBZConvertConfig{Enabled: true})
	w := variantCall(s, http.MethodGet, id, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var info VariantCoversInfo
	_ = json.NewDecoder(w.Body).Decode(&info)
	if info.IssueID != 777 || len(info.Candidates) != 2 || !info.CanApply ||
		info.Candidates[1].ID != 22 || info.Candidates[1].Caption != "Variant B" || info.Candidates[0].Added {
		t.Errorf("info = %+v", info)
	}
}

func TestVariantCovers_PreviewExplainsWhyApplyIsBlocked(t *testing.T) {
	s, _, id := variantFixture(t, config.CBZConvertConfig{}) // cbz_convert off
	var info VariantCoversInfo
	_ = json.NewDecoder(variantCall(s, http.MethodGet, id, nil).Body).Decode(&info)
	if info.CanApply || info.Reason == "" || len(info.Candidates) != 2 {
		t.Errorf("info = %+v", info)
	}
	if w := variantCall(s, http.MethodPost, id, VariantCoversApplyRequest{ImageIDs: []int{21}}); w.Code != http.StatusServiceUnavailable {
		t.Errorf("apply while disabled = %d, want 503", w.Code)
	}
}

func TestVariantCovers_ApplyAppendsRecordsAndIsIdempotent(t *testing.T) {
	s, cbzPath, id := variantFixture(t, config.CBZConvertConfig{Enabled: true})

	w := variantCall(s, http.MethodPost, id, VariantCoversApplyRequest{ImageIDs: []int{21, 22, 999}})
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var res VariantCoversApplyResult
	_ = json.NewDecoder(w.Body).Decode(&res)
	if res.Added != 2 || res.PageCount != 3 || len(res.Errors) != 1 {
		t.Fatalf("result = %+v", res)
	}

	zr, err := zip.OpenReader(cbzPath)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	zr.Close()
	for _, want := range []string{"001.png", "zzzz-cv-variant-21.png", "zzzz-cv-variant-22.png", "ComicInfo.xml"} {
		if !names[want] {
			t.Errorf("archive missing %s (have %v)", want, names)
		}
	}

	b, _ := s.backend.GetBook(id)
	if b.PageCount != 3 || len(b.Pages) != 2 || b.Pages[0].Type != library.PageTypeFrontCover {
		t.Errorf("book = pages %d, %+v", b.PageCount, b.Pages)
	}
	if got := addedVariantIDs(b); !got[21] || !got[22] {
		t.Errorf("added ids = %v", got)
	}

	// The preview now marks them added, and a repeat adds nothing.
	var info VariantCoversInfo
	_ = json.NewDecoder(variantCall(s, http.MethodGet, id, nil).Body).Decode(&info)
	if !info.Candidates[0].Added || !info.Candidates[1].Added {
		t.Errorf("candidates not marked added: %+v", info.Candidates)
	}
	var again VariantCoversApplyResult
	_ = json.NewDecoder(variantCall(s, http.MethodPost, id, VariantCoversApplyRequest{ImageIDs: []int{21}}).Body).Decode(&again)
	if again.Added != 0 || again.Skipped != 1 {
		t.Errorf("repeat = %+v", again)
	}

	rows, total, _ := s.configDB.ListAudit(configdb.AuditFilter{Action: audit.ActionVariantCovers})
	if total != 1 || rows[0].BookID != id {
		t.Errorf("audit rows = %+v", rows)
	}
}

func TestVariantCovers_RefusesNonCBZAndUnscrapedBooks(t *testing.T) {
	s, _, id := variantFixture(t, config.CBZConvertConfig{Enabled: true})
	b, _ := s.backend.GetBook(id)
	cp := *b
	cp.FilePath = "x.cbr"
	_ = s.backend.UpdateBooks([]*library.ComicBook{&cp})
	if w := variantCall(s, http.MethodPost, id, VariantCoversApplyRequest{ImageIDs: []int{21}}); w.Code != http.StatusServiceUnavailable {
		t.Errorf("cbr apply = %d, want 503", w.Code)
	}

	cp.CustomValuesStore = ""
	_ = s.backend.UpdateBooks([]*library.ComicBook{&cp})
	if w := variantCall(s, http.MethodGet, id, nil); w.Code != http.StatusConflict {
		t.Errorf("unscraped = %d, want 409", w.Code)
	}
}
