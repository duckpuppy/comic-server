package comicvine

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// solidImage returns a uniform-color test image encoded as JPEG.
func solidImage(t *testing.T, c color.RGBA, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// gradientImage returns a left-to-right brightness gradient, encoded as PNG.
func gradientImage(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetGray(x, y, color.Gray{Y: uint8(255 * x / w)})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestComputeDHash_IdenticalImagesMatch(t *testing.T) {
	data := gradientImage(t, 64, 64)
	h1, err := ComputeDHash(data)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := ComputeDHash(data)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Errorf("h1 = %x, h2 = %x, want equal", h1, h2)
	}
	if sim := h1.Similarity(h2); sim != 1.0 {
		t.Errorf("similarity = %v, want 1.0", sim)
	}
}

func TestComputeDHash_DifferentImagesDiffer(t *testing.T) {
	black := solidImage(t, color.RGBA{R: 0, G: 0, B: 0, A: 255}, 64, 64)
	gradient := gradientImage(t, 64, 64)

	h1, err := ComputeDHash(black)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := ComputeDHash(gradient)
	if err != nil {
		t.Fatal(err)
	}
	if sim := h1.Similarity(h2); sim >= 0.9 {
		t.Errorf("similarity = %v, want clearly less than 1.0 for very different images", sim)
	}
}

func TestComputeDHash_InvalidData(t *testing.T) {
	_, err := ComputeDHash([]byte("not an image"))
	if err == nil {
		t.Fatal("expected error for invalid image data")
	}
}

func TestCoverHash_Similarity(t *testing.T) {
	tests := []struct {
		name string
		a, b CoverHash
		want float64
	}{
		{"identical", 0b1010, 0b1010, 1.0},
		{"all bits differ (64)", 0x0, 0xFFFFFFFFFFFFFFFF, 0.0},
		{"one bit differs", 0b0000, 0b0001, 1.0 - 1.0/64.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Similarity(tt.b); got != tt.want {
				t.Errorf("Similarity(%b, %b) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// writeCBZ builds a zip archive at dir/name.cbz from the given entries and
// returns its path.
func writeCBZ(t *testing.T, dir, name string, entries map[string][]byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for entryName, data := range entries {
		w, err := zw.Create(entryName)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractCoverFromCBZ_FirstImageAlphabetically(t *testing.T) {
	img1 := gradientImage(t, 16, 16)
	img2 := solidImage(t, color.RGBA{R: 255, A: 255}, 16, 16)
	path := writeCBZ(t, t.TempDir(), "test.cbz", map[string][]byte{
		"page002.png": img2,
		"page001.png": img1,
	})

	got, err := ExtractCoverFromCBZ(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, img1) {
		t.Error("expected page001.png (alphabetically first) as cover")
	}
}

func TestExtractCoverFromCBZ_FrontCoverFromComicInfo(t *testing.T) {
	img0 := gradientImage(t, 16, 16)
	img1 := solidImage(t, color.RGBA{G: 255, A: 255}, 16, 16)
	comicInfo := []byte(`<?xml version="1.0"?>
<ComicInfo>
  <Pages>
    <Page Image="0" Type="Story" />
    <Page Image="1" Type="FrontCover" />
  </Pages>
</ComicInfo>`)
	path := writeCBZ(t, t.TempDir(), "test.cbz", map[string][]byte{
		"page000.jpg":   img0,
		"page001.jpg":   img1,
		"ComicInfo.xml": comicInfo,
	})

	got, err := ExtractCoverFromCBZ(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, img1) {
		t.Error("expected page001.jpg (marked FrontCover) as cover, not the alphabetically-first page")
	}
}

func TestExtractCoverFromCBZ_SkipsNonImageFiles(t *testing.T) {
	img := gradientImage(t, 16, 16)
	path := writeCBZ(t, t.TempDir(), "test.cbz", map[string][]byte{
		"AAAA_not_an_image.txt": []byte("hello"),
		"page001.jpg":           img,
	})

	got, err := ExtractCoverFromCBZ(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, img) {
		t.Error("expected page001.jpg, non-image files should be skipped")
	}
}

func TestExtractCoverFromCBZ_NoImages(t *testing.T) {
	path := writeCBZ(t, t.TempDir(), "test.cbz", map[string][]byte{
		"ComicInfo.xml": []byte(`<ComicInfo/>`),
	})

	_, err := ExtractCoverFromCBZ(path)
	if err == nil {
		t.Fatal("expected error for archive with no images")
	}
}

func TestExtractCoverFromCBZ_FileNotFound(t *testing.T) {
	_, err := ExtractCoverFromCBZ(filepath.Join(t.TempDir(), "nonexistent.cbz"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestDownloadCoverHash(t *testing.T) {
	data := gradientImage(t, 32, 32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer srv.Close()

	client := NewClient("test-key")
	hash, err := client.DownloadCoverHash(t.Context(), srv.URL+"/cover.png")
	if err != nil {
		t.Fatal(err)
	}
	want, err := ComputeDHash(data)
	if err != nil {
		t.Fatal(err)
	}
	if hash != want {
		t.Errorf("hash = %x, want %x", hash, want)
	}
}

func TestDownloadCoverHash_EmptyURL(t *testing.T) {
	client := NewClient("test-key")
	if _, err := client.DownloadCoverHash(t.Context(), ""); err == nil {
		t.Fatal("expected error for empty url")
	}
}

func TestDownloadCoverHash_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClient("test-key")
	if _, err := client.DownloadCoverHash(t.Context(), srv.URL+"/missing.jpg"); err == nil {
		t.Fatal("expected error for HTTP 404")
	}
}

// writeCB7 builds a 7z archive at dir/name from the given entries by
// shelling out to the system 7z binary (there's no pure-Go 7z writer this
// project depends on), and returns its path. Skips the test if 7z isn't
// installed.
func writeCB7(t *testing.T, dir, name string, entries map[string][]byte) string {
	t.Helper()
	sevenZipBin, err := exec.LookPath("7z")
	if err != nil {
		t.Skip("7z binary not found in PATH, skipping CB7 test")
	}

	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	for entryName, data := range entries {
		full := filepath.Join(srcDir, entryName)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	archivePath := filepath.Join(dir, name)
	cmd := exec.Command(sevenZipBin, "a", "-y", archivePath, ".")
	cmd.Dir = srcDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("7z a failed: %v\n%s", err, out)
	}
	return archivePath
}

func TestExtractCoverFromCB7_FirstImageAlphabetically(t *testing.T) {
	img1 := gradientImage(t, 16, 16)
	img2 := solidImage(t, color.RGBA{R: 255, A: 255}, 16, 16)
	path := writeCB7(t, t.TempDir(), "test.cb7", map[string][]byte{
		"page002.png": img2,
		"page001.png": img1,
	})

	got, err := extractCoverFromCB7(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, img1) {
		t.Error("expected page001.png (alphabetically first) as cover")
	}
}

func TestExtractCoverFromCB7_FrontCoverFromComicInfo(t *testing.T) {
	img0 := gradientImage(t, 16, 16)
	img1 := solidImage(t, color.RGBA{G: 255, A: 255}, 16, 16)
	comicInfo := []byte(`<?xml version="1.0"?>
<ComicInfo>
  <Pages>
    <Page Image="0" Type="Story" />
    <Page Image="1" Type="FrontCover" />
  </Pages>
</ComicInfo>`)
	path := writeCB7(t, t.TempDir(), "test.cb7", map[string][]byte{
		"page000.jpg":   img0,
		"page001.jpg":   img1,
		"ComicInfo.xml": comicInfo,
	})

	got, err := extractCoverFromCB7(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, img1) {
		t.Error("expected page001.jpg (marked FrontCover) as cover, not the alphabetically-first page")
	}
}

func TestExtractCoverFromCB7_SkipsNonImageFiles(t *testing.T) {
	img := gradientImage(t, 16, 16)
	path := writeCB7(t, t.TempDir(), "test.cb7", map[string][]byte{
		"AAAA_not_an_image.txt": []byte("hello"),
		"page001.jpg":           img,
	})

	got, err := extractCoverFromCB7(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, img) {
		t.Error("expected page001.jpg, non-image files should be skipped")
	}
}

func TestExtractCoverFromCB7_NoImages(t *testing.T) {
	path := writeCB7(t, t.TempDir(), "test.cb7", map[string][]byte{
		"ComicInfo.xml": []byte(`<ComicInfo/>`),
	})

	_, err := extractCoverFromCB7(path)
	if err == nil {
		t.Fatal("expected error for archive with no images")
	}
}

func TestExtractCoverFromCB7_FileNotFound(t *testing.T) {
	_, err := extractCoverFromCB7(filepath.Join(t.TempDir(), "nonexistent.cb7"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestExtractCover_DispatchesToZip(t *testing.T) {
	img := gradientImage(t, 16, 16)
	path := writeCBZ(t, t.TempDir(), "test.cbz", map[string][]byte{"page001.png": img})

	got, err := ExtractCover(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, img) {
		t.Error("ExtractCover(.cbz) did not return the expected cover")
	}
}

func TestExtractCover_DispatchesToSevenZip(t *testing.T) {
	img := gradientImage(t, 16, 16)
	path := writeCB7(t, t.TempDir(), "test.cb7", map[string][]byte{"page001.png": img})

	got, err := ExtractCover(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, img) {
		t.Error("ExtractCover(.cb7) did not return the expected cover")
	}
}

// TestExtractCover_DispatchesByExtension confirms each extension routes to
// the matching extractor, using deliberately invalid archive content: we
// only need to observe which "open <format>: ..." error comes back, not
// perform a real extraction. Real-archive round-trips for .cbz/.cb7 are
// covered above; there's no RAR encoder available to build a .cbr fixture
// with in this environment, so the RAR path is exercised at the dispatch/
// error-handling level only.
func TestExtractCover_DispatchesByExtension(t *testing.T) {
	tests := []struct {
		ext           string
		wantErrSubstr string
	}{
		{".cbr", "open cbr"},
		{".rar", "open cbr"},
		{".cb7", "open cb7"},
		{".7z", "open cb7"},
	}
	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "test"+tt.ext)
			if err := os.WriteFile(path, []byte("not a real archive"), 0644); err != nil {
				t.Fatal(err)
			}
			_, err := ExtractCover(path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErrSubstr) {
				t.Errorf("ExtractCover(%s) error = %v, want substring %q", tt.ext, err, tt.wantErrSubstr)
			}
		})
	}
}

func TestExtractCover_UnsupportedExtension(t *testing.T) {
	_, err := ExtractCover("/tmp/comic.pdf")
	if err == nil || !strings.Contains(err.Error(), "unsupported archive format") {
		t.Errorf("err = %v, want unsupported archive format error", err)
	}
}
