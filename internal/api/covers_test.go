package api

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

// solidPNG returns a tiny uniform-color PNG, used as a fixture cover image.
func solidPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 20, B: 20, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// writeTestCBZ builds a single-page CBZ fixture at dir/name and returns its path.
func writeTestCBZ(t *testing.T, dir, name string, pageData []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	w, err := zw.Create("page001.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(pageData); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func newCoverTestServer(t *testing.T, books []library.ComicBook) *Server {
	t.Helper()
	lib := &library.ComicLibrary{Books: books}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	return &Server{backend: backend}
}

func TestHandleGetBookCover_ReturnsCoverImage(t *testing.T) {
	pageData := solidPNG(t)
	cbzPath := writeTestCBZ(t, t.TempDir(), "test.cbz", pageData)

	server := newCoverTestServer(t, []library.ComicBook{
		{ID: "book-1", FilePath: cbzPath, Series: "Batman"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/library/books/book-1/cover", nil)
	w := httptest.NewRecorder()
	server.handleBooksRouter(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("expected Content-Type image/png, got %q", ct)
	}
	if !bytes.Equal(w.Body.Bytes(), pageData) {
		t.Error("expected response body to be the cover page's exact bytes")
	}
}

func TestHandleGetBookCover_UnknownBookID(t *testing.T) {
	server := newCoverTestServer(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/library/books/does-not-exist/cover", nil)
	w := httptest.NewRecorder()
	server.handleBooksRouter(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown book, got %d", w.Code)
	}
}

func TestHandleGetBookCover_MissingFilePath(t *testing.T) {
	server := newCoverTestServer(t, []library.ComicBook{
		{ID: "book-1", FilePath: "", Series: "Batman"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/library/books/book-1/cover", nil)
	w := httptest.NewRecorder()
	server.handleBooksRouter(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for a book with no file path, got %d", w.Code)
	}
}

func TestHandleGetBookCover_UnreadableFile(t *testing.T) {
	server := newCoverTestServer(t, []library.ComicBook{
		{ID: "book-1", FilePath: filepath.Join(t.TempDir(), "does-not-exist.cbz"), Series: "Batman"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/library/books/book-1/cover", nil)
	w := httptest.NewRecorder()
	server.handleBooksRouter(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for a missing archive file, got %d", w.Code)
	}
}

func TestHandleGetBookCover_MethodNotAllowed(t *testing.T) {
	server := newCoverTestServer(t, []library.ComicBook{
		{ID: "book-1", FilePath: "irrelevant.cbz"},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/library/books/book-1/cover", nil)
	w := httptest.NewRecorder()
	server.handleBooksRouter(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleBooksRouter_UnknownSubPath(t *testing.T) {
	server := newCoverTestServer(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/library/books/book-1/nonsense", nil)
	w := httptest.NewRecorder()
	server.handleBooksRouter(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown sub-path, got %d", w.Code)
	}
}
