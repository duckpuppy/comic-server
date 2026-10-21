package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/library"
)

func TestExportFileName(t *testing.T) {
	cases := []struct {
		book   library.ComicBook
		naming string
		want   string
	}{
		{library.ComicBook{FilePath: `C:\Comics\Batman 001.cbr`}, "original", "Batman 001.cbz"},
		{library.ComicBook{FilePath: "/a/b/x.cbz"}, "", "x.cbz"},
		{library.ComicBook{Series: "Batman", Volume: 2, Number: "3", Year: 2019, FilePath: "z.cbz"}, "series", "Batman V2 #003 (2019).cbz"},
		{library.ComicBook{Series: "A/B: C?", Number: "1.5", FilePath: "z.cbz"}, "series", "A_B_ C_ #1.5.cbz"},
		{library.ComicBook{ID: "{id-1}", FilePath: ""}, "original", "{id-1}.cbz"},
		{library.ComicBook{FilePath: "z.cbz"}, "series", "z.cbz"}, // no series: fall back to the file name
		{library.ComicBook{FilePath: "/a/..cbz"}, "original", "book.cbz"},
	}
	for _, c := range cases {
		if got := exportFileName(&c.book, c.naming); got != c.want {
			t.Errorf("exportFileName(%+v, %q) = %q, want %q", c.book, c.naming, got, c.want)
		}
	}
	long := library.ComicBook{FilePath: strings.Repeat("é", 200) + ".cbz"}
	if got := exportFileName(&long, "original"); len(got) > maxExportNameBytes+4 {
		t.Errorf("long name not capped: %d bytes", len(got))
	}
}

func TestExportFolder(t *testing.T) {
	root := t.TempDir()
	if got, err := exportFolder(root, ""); err != nil || got != root {
		t.Errorf("empty subfolder: %q %v", got, err)
	}
	if got, err := exportFolder(root, " run1 "); err != nil || got != filepath.Join(root, "run1") {
		t.Errorf("subfolder: %q %v", got, err)
	}
	for _, bad := range []string{"..", ".", "a/b", `a\b`, "../x", "..."} {
		if _, err := exportFolder(root, bad); err == nil {
			t.Errorf("subfolder %q should be refused", bad)
		}
	}
}

func startExport(t *testing.T, s *Server, body string) (int, string) {
	t.Helper()
	w := httptest.NewRecorder()
	s.handleBookExportStart(w, httptest.NewRequest(http.MethodPost, "/api/library/browse/export-books", strings.NewReader(body)))
	return w.Code, w.Body.String()
}

func waitExport(t *testing.T, s *Server) BookExportJob {
	t.Helper()
	for i := 0; i < 200; i++ {
		s.exportJobMu.RLock()
		job := *s.exportJob
		s.exportJobMu.RUnlock()
		if job.Status == "completed" {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("export did not finish")
	return BookExportJob{}
}

func TestBookExport_OffWithoutExportPath(t *testing.T) {
	s := newCBZConvertTestServer(t, config.CBZConvertConfig{}, "", bulkEditBooks())
	if code, _ := startExport(t, s, `{"book_ids":["b1"]}`); code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", code)
	}
}

func TestBookExport_CopiesAndLeavesOriginals(t *testing.T) {
	libDir, exportDir := t.TempDir(), t.TempDir()
	var books []library.ComicBook
	for _, n := range []string{"1", "2"} {
		p := filepath.Join(libDir, "Batman "+n+".cbz")
		writeTestCBZFixture(t, p)
		books = append(books, library.ComicBook{ID: "b" + n, Series: "Batman", Number: n, FilePath: p})
	}
	books = append(books, library.ComicBook{ID: "b3", Series: "Placeholder"}) // no file
	s := newCBZConvertTestServer(t, config.CBZConvertConfig{}, "", books)
	s.config.Server.ExportPath = exportDir

	code, body := startExport(t, s, `{"book_ids":["b1","b2","b3"],"subfolder":"run1"}`)
	if code != http.StatusAccepted {
		t.Fatalf("start: %d %s", code, body)
	}
	job := waitExport(t, s)
	if job.Exported != 2 || job.Skipped != 1 || job.Failed != 0 {
		t.Fatalf("job = %+v", job)
	}
	for _, n := range []string{"Batman 1.cbz", "Batman 2.cbz"} {
		if _, err := os.Stat(filepath.Join(exportDir, "run1", n)); err != nil {
			t.Errorf("missing export %s: %v", n, err)
		}
	}
	for _, b := range books[:2] {
		if _, err := os.Stat(b.FilePath); err != nil {
			t.Errorf("original gone: %v", err)
		}
	}

	// A second run skips what is already there.
	if code, body := startExport(t, s, `{"book_ids":["b1","b2"],"subfolder":"run1"}`); code != http.StatusAccepted {
		t.Fatalf("second start: %d %s", code, body)
	}
	if job := waitExport(t, s); job.Exported != 0 || job.Skipped != 2 {
		t.Errorf("second job = %+v", job)
	}
}

func TestBookExport_SameNameInTwoFoldersGetsNumbered(t *testing.T) {
	libDir, exportDir := t.TempDir(), t.TempDir()
	var books []library.ComicBook
	for _, sub := range []string{"a", "b"} {
		_ = os.MkdirAll(filepath.Join(libDir, sub), 0o755)
		p := filepath.Join(libDir, sub, "same.cbz")
		writeTestCBZFixture(t, p)
		books = append(books, library.ComicBook{ID: sub, FilePath: p})
	}
	s := newCBZConvertTestServer(t, config.CBZConvertConfig{}, "", books)
	s.config.Server.ExportPath = exportDir
	if code, body := startExport(t, s, `{"book_ids":["a","b"]}`); code != http.StatusAccepted {
		t.Fatalf("start: %d %s", code, body)
	}
	if job := waitExport(t, s); job.Exported != 2 {
		t.Fatalf("job = %+v", job)
	}
	for _, n := range []string{"same.cbz", "same (2).cbz"} {
		if _, err := os.Stat(filepath.Join(exportDir, n)); err != nil {
			t.Errorf("missing %s: %v", n, err)
		}
	}
}

func TestBookExport_RefusesBadInput(t *testing.T) {
	s := newCBZConvertTestServer(t, config.CBZConvertConfig{}, "", bulkEditBooks())
	s.config.Server.ExportPath = t.TempDir()
	for body, want := range map[string]int{
		`{"book_ids":["b1"],"subfolder":"../x"}`: http.StatusBadRequest,
		`{"book_ids":["b1"],"naming":"weird"}`:   http.StatusBadRequest,
		`not json`:                               http.StatusBadRequest,
	} {
		if code, _ := startExport(t, s, body); code != want {
			t.Errorf("%s: status %d, want %d", body, code, want)
		}
	}
}
