package comicvine

import (
	"bytes"
	"image/color"
	"path/filepath"
	"testing"
)

func TestReadAllPages_CBZ_SortedAndSkipsNonImages(t *testing.T) {
	img1 := gradientImage(t, 16, 16)
	img2 := solidImage(t, color.RGBA{R: 255, A: 255}, 16, 16)
	path := writeCBZ(t, t.TempDir(), "test.cbz", map[string][]byte{
		"page002.png":           img2,
		"page001.png":           img1,
		"AAAA_not_an_image.txt": []byte("hello"),
	})

	pages, err := ReadAllPages(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}
	if pages[0].Name != "page001.png" || !bytes.Equal(pages[0].Data, img1) {
		t.Errorf("page 0 = %q, want page001.png with img1 data", pages[0].Name)
	}
	if pages[1].Name != "page002.png" || !bytes.Equal(pages[1].Data, img2) {
		t.Errorf("page 1 = %q, want page002.png with img2 data", pages[1].Name)
	}
}

func TestReadAllPages_CBZ_NoImages(t *testing.T) {
	path := writeCBZ(t, t.TempDir(), "test.cbz", map[string][]byte{
		"ComicInfo.xml": []byte(`<ComicInfo/>`),
	})

	pages, err := ReadAllPages(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 0 {
		t.Errorf("expected 0 pages, got %d", len(pages))
	}
}

func TestReadAllPages_FileNotFound(t *testing.T) {
	_, err := ReadAllPages(filepath.Join(t.TempDir(), "nonexistent.cbz"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadAllPages_UnsupportedExtension(t *testing.T) {
	_, err := ReadAllPages("book.pdf")
	if err == nil {
		t.Fatal("expected error for unsupported extension")
	}
}

func TestReadAllPages_CB7(t *testing.T) {
	img1 := gradientImage(t, 16, 16)
	img2 := solidImage(t, color.RGBA{B: 255, A: 255}, 16, 16)
	path := writeCB7(t, t.TempDir(), "test.cb7", map[string][]byte{
		"page002.png": img2,
		"page001.png": img1,
	})

	pages, err := ReadAllPages(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}
	if pages[0].Name != "page001.png" || !bytes.Equal(pages[0].Data, img1) {
		t.Errorf("page 0 = %q, want page001.png with img1 data", pages[0].Name)
	}
	if pages[1].Name != "page002.png" || !bytes.Equal(pages[1].Data, img2) {
		t.Errorf("page 1 = %q, want page002.png with img2 data", pages[1].Name)
	}
}
