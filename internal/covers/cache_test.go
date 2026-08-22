package covers

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// bigImage returns a large uniform-color image encoded as JPEG - large
// enough that resizing down actually changes its dimensions, so tests can
// tell a resized thumbnail from a passthrough.
func bigImage(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeCBZFixture(t *testing.T, dir, name string, pageData []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	w, err := zw.Create("page001.jpg")
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

func decodedSize(t *testing.T, data []byte) (int, int) {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode result image: %v", err)
	}
	b := img.Bounds()
	return b.Dx(), b.Dy()
}

func TestCache_Get_ResizesLargeCover(t *testing.T) {
	cover := bigImage(t, 1200, 1800)
	cbzDir := t.TempDir()
	cbzPath := writeCBZFixture(t, cbzDir, "test.cbz", cover)

	cache, err := NewCache(t.TempDir(), 300)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}

	data, err := cache.Get("book-1", cbzPath)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	w, h := decodedSize(t, data)
	if w != 300 {
		t.Errorf("expected thumbnail width 300, got %d", w)
	}
	if h != 450 { // 1800 * 300 / 1200
		t.Errorf("expected thumbnail height 450 (aspect ratio preserved), got %d", h)
	}
}

func TestCache_Get_DoesNotUpscaleSmallCover(t *testing.T) {
	cover := bigImage(t, 100, 150)
	cbzDir := t.TempDir()
	cbzPath := writeCBZFixture(t, cbzDir, "test.cbz", cover)

	cache, err := NewCache(t.TempDir(), 300)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}

	data, err := cache.Get("book-1", cbzPath)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	w, h := decodedSize(t, data)
	if w != 100 || h != 150 {
		t.Errorf("expected original dimensions 100x150 (no upscale), got %dx%d", w, h)
	}
}

// TestCache_Get_ServesCachedCopyOnSecondCall proves a second Get for an
// unmodified source is actually served from disk, not re-extracted: after
// the first Get populates the cache, the source archive is made unreadable
// (chmod 000) without touching its mtime/size, so a real re-extraction
// attempt would fail. A second Get still succeeding with byte-identical
// output is only possible if it read the cached copy.
func TestCache_Get_ServesCachedCopyOnSecondCall(t *testing.T) {
	cbzDir := t.TempDir()
	cbzPath := writeCBZFixture(t, cbzDir, "test.cbz", bigImage(t, 400, 600))
	cacheDir := t.TempDir()

	cache, err := NewCache(cacheDir, 300)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}

	first, err := cache.Get("book-1", cbzPath)
	if err != nil {
		t.Fatalf("Get (first): %v", err)
	}

	if err := os.Chmod(cbzPath, 0000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(cbzPath, 0644) }) // let TempDir cleanup remove it

	second, err := cache.Get("book-1", cbzPath)
	if err != nil {
		t.Fatalf("Get (second, source now unreadable): %v - a real re-extraction would have failed, so this means it wasn't served from cache", err)
	}
	if !bytes.Equal(first, second) {
		t.Error("expected second Get to return byte-identical cached thumbnail")
	}

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected exactly 1 cached file after two Gets of the same book, got %d", len(entries))
	}
}

func TestCache_Get_InvalidatesOnSourceChange(t *testing.T) {
	cbzDir := t.TempDir()
	cbzPath := writeCBZFixture(t, cbzDir, "test.cbz", bigImage(t, 400, 600))
	cache, err := NewCache(t.TempDir(), 300)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}

	if _, err := cache.Get("book-1", cbzPath); err != nil {
		t.Fatalf("Get (first): %v", err)
	}

	// Rewrite the archive with a different cover and a different mtime -
	// simulates re-tagging FrontCover or changing which file sorts first,
	// which always rewrites the archive (see comic-server-0y6's design notes).
	time.Sleep(10 * time.Millisecond) // ensure a distinguishable mtime
	os.Remove(cbzPath)
	writeCBZFixture(t, cbzDir, "test.cbz", bigImage(t, 800, 1200))

	data, err := cache.Get("book-1", cbzPath)
	if err != nil {
		t.Fatalf("Get (after change): %v", err)
	}
	w, _ := decodedSize(t, data)
	if w != 300 {
		t.Errorf("expected the new cover to be re-extracted and resized to 300 wide, got %d", w)
	}
}

func TestCache_Invalidate_ForcesReExtraction(t *testing.T) {
	cbzDir := t.TempDir()
	cbzPath := writeCBZFixture(t, cbzDir, "test.cbz", bigImage(t, 400, 600))
	cacheDir := t.TempDir()
	cache, err := NewCache(cacheDir, 300)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}

	if _, err := cache.Get("book-1", cbzPath); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if entries, _ := os.ReadDir(cacheDir); len(entries) != 1 {
		t.Fatalf("expected 1 cached file before Invalidate, got %d", len(entries))
	}

	if err := cache.Invalidate("book-1"); err != nil {
		t.Fatalf("Invalidate: %v", err)
	}
	if entries, _ := os.ReadDir(cacheDir); len(entries) != 0 {
		t.Errorf("expected 0 cached files after Invalidate, got %d", len(entries))
	}
}

func TestCache_Invalidate_NoOpWhenNothingCached(t *testing.T) {
	cache, err := NewCache(t.TempDir(), 300)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	if err := cache.Invalidate("never-cached"); err != nil {
		t.Errorf("expected Invalidate on an uncached book to be a no-op, got error: %v", err)
	}
}

func TestCache_Get_MissingSourceFileErrors(t *testing.T) {
	cache, err := NewCache(t.TempDir(), 300)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	if _, err := cache.Get("book-1", filepath.Join(t.TempDir(), "does-not-exist.cbz")); err == nil {
		t.Error("expected an error for a missing source file")
	}
}

func TestSanitizeForFilename(t *testing.T) {
	got := sanitizeForFilename("{a-guid-1234}")
	want := "_a-guid-1234_"
	if got != want {
		t.Errorf("sanitizeForFilename() = %q, want %q", got, want)
	}
}
