package cbzconvert

import (
	"bytes"
	"encoding/xml"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/library"
)

func pngBytes(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sizeOf(t *testing.T, data []byte) (int, int, string) {
	t.Helper()
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Width, cfg.Height, format
}

func TestPageOptionsValidate(t *testing.T) {
	good := []PageOptions{
		{},
		{Format: PageFormatJPEG, Quality: 90},
		{Resize: PageResizeWidth, Width: 1200},
		{Resize: PageResizeHeight, Height: 1800},
		{Resize: PageResizeFit, Width: 1000, Height: 1500, DontEnlarge: true},
	}
	for _, o := range good {
		if err := o.Validate(); err != nil {
			t.Errorf("%+v refused: %v", o, err)
		}
	}
	bad := []PageOptions{
		{Format: "webp"},
		{Format: PageFormatJPEG, Quality: 101},
		{Quality: -1},
		{Resize: "stretch", Width: 10},
		{Resize: PageResizeWidth},
		{Resize: PageResizeHeight, Width: 100},
		{Resize: PageResizeFit, Width: 100},
		{Resize: PageResizeWidth, Width: MaxPageDimension + 1},
	}
	for _, o := range bad {
		if err := o.Validate(); err == nil {
			t.Errorf("%+v should be refused", o)
		}
	}
}

func TestTargetSize(t *testing.T) {
	cases := []struct {
		w, h   int
		o      PageOptions
		ww, hh int
	}{
		{1000, 1500, PageOptions{}, 1000, 1500},
		{1000, 1500, PageOptions{Resize: PageResizeWidth, Width: 500}, 500, 750},
		{1000, 1500, PageOptions{Resize: PageResizeHeight, Height: 300}, 200, 300},
		{1000, 1500, PageOptions{Resize: PageResizeFit, Width: 500, Height: 500}, 333, 500},
		{1000, 1500, PageOptions{Resize: PageResizeFit, Width: 2000, Height: 2000}, 1333, 2000},
		{1000, 1500, PageOptions{Resize: PageResizeWidth, Width: 2000, DontEnlarge: true}, 1000, 1500},
		{1000, 1500, PageOptions{Resize: PageResizeWidth, Width: 2000}, 2000, 3000},
		{10, 1000, PageOptions{Resize: PageResizeHeight, Height: 10}, 1, 10}, // never below 1 pixel
	}
	for _, c := range cases {
		if w, h := targetSize(c.w, c.h, c.o); w != c.ww || h != c.hh {
			t.Errorf("targetSize(%d,%d,%+v) = %dx%d, want %dx%d", c.w, c.h, c.o, w, h, c.ww, c.hh)
		}
	}
}

func TestTransformPages_NoOptionsKeepsEverything(t *testing.T) {
	in := []comicvine.Page{{Name: "z/a.jpg", Data: []byte("raw")}}
	out, stats, err := TransformPages(in, PageOptions{})
	if err != nil || out[0].Name != "z/a.jpg" || string(out[0].Data) != "raw" || stats[0].Changed {
		t.Fatalf("out=%+v stats=%+v err=%v", out, stats, err)
	}
}

func TestTransformPages_ResizeAndReencode(t *testing.T) {
	j := jpegBytes(t, 400, 600)
	p := pngBytes(t, 400, 600, color.RGBA{200, 10, 10, 255})
	in := []comicvine.Page{{Name: "b.jpg", Data: j}, {Name: "a.png", Data: p}}

	// Resize only: each page keeps its format, bytes change, sizes follow.
	out, stats, err := TransformPages(in, PageOptions{Resize: PageResizeWidth, Width: 200})
	if err != nil {
		t.Fatal(err)
	}
	if w, h, f := sizeOf(t, out[0].Data); w != 200 || h != 300 || f != "jpeg" {
		t.Errorf("page 0 = %dx%d %s", w, h, f)
	}
	if w, h, f := sizeOf(t, out[1].Data); w != 200 || h != 300 || f != "png" {
		t.Errorf("page 1 = %dx%d %s", w, h, f)
	}
	if out[0].Name != "000.jpg" || out[1].Name != "001.png" {
		t.Errorf("names = %q %q, want a zero-padded sequence in input order", out[0].Name, out[1].Name)
	}
	if !stats[0].Changed || stats[0].Width != 200 || stats[0].Height != 300 || stats[0].Size != int64(len(out[0].Data)) {
		t.Errorf("stats[0] = %+v", stats[0])
	}

	// Format only: PNG page becomes JPEG, JPEG page of the same size is kept as is.
	out, stats, err = TransformPages(in, PageOptions{Format: PageFormatJPEG, Quality: 70})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out[0].Data, j) || stats[0].Changed {
		t.Error("a JPEG page that needs no change must keep its exact bytes")
	}
	if _, _, f := sizeOf(t, out[1].Data); f != "jpeg" || out[1].Name != "001.jpg" {
		t.Errorf("PNG page: format %s name %s", f, out[1].Name)
	}
}

func TestTransformPages_DontEnlargeKeepsBytes(t *testing.T) {
	j := jpegBytes(t, 100, 150)
	out, stats, err := TransformPages([]comicvine.Page{{Name: "a.jpg", Data: j}}, PageOptions{Resize: PageResizeFit, Width: 1000, Height: 1000, DontEnlarge: true})
	if err != nil || !bytes.Equal(out[0].Data, j) || stats[0].Changed {
		t.Fatalf("small page was touched: %+v %v", stats[0], err)
	}
}

func TestTransformPages_TransparentPNGToJPEGGoesWhite(t *testing.T) {
	clear := pngBytes(t, 8, 8, color.RGBA{0, 0, 0, 0})
	out, _, err := TransformPages([]comicvine.Page{{Name: "a.png", Data: clear}}, PageOptions{Format: PageFormatJPEG})
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(out[0].Data))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(4, 4).RGBA()
	if r>>8 < 240 || g>>8 < 240 || b>>8 < 240 {
		t.Errorf("transparent pixel became %d,%d,%d, want white", r>>8, g>>8, b>>8)
	}
}

func TestTransformPages_BadImageNamesThePage(t *testing.T) {
	_, _, err := TransformPages([]comicvine.Page{{Name: "ok.png", Data: pngBytes(t, 4, 4, color.White)}, {Name: "broken.jpg", Data: []byte("not an image")}}, PageOptions{Format: PageFormatPNG})
	if err == nil || !strings.Contains(err.Error(), "page 2") || !strings.Contains(err.Error(), "broken.jpg") {
		t.Fatalf("err = %v", err)
	}
}

func TestExportCopy_ResizesAndFixesPageSizesInComicInfo(t *testing.T) {
	src := filepath.Join(t.TempDir(), "book.cbz")
	writeTestCBZ(t, src, map[string][]byte{
		"p0.png": pngBytes(t, 400, 600, color.White),
		"p1.png": pngBytes(t, 400, 600, color.Black), // library marks this one Deleted
		"p2.png": pngBytes(t, 400, 600, color.White),
	})
	dest := filepath.Join(t.TempDir(), "out.cbz")
	book := &library.ComicBook{FilePath: src, Series: "S", Pages: []library.ComicPageInfo{
		{Image: 0, ImageWidth: 400, ImageHeight: 600, ImageSize: 111},
		{Image: 1, Type: library.PageTypeDeleted},
		{Image: 2, ImageWidth: 400, ImageHeight: 600, ImageSize: 333},
	}}
	n, err := ExportCopy(book, src, dest, Options{}, PageOptions{Resize: PageResizeWidth, Width: 100}, false)
	if err != nil || n != 2 {
		t.Fatalf("ExportCopy = %d, %v", n, err)
	}
	entries := readZipEntries(t, dest)
	if w, h, _ := sizeOf(t, entries["000.png"]); w != 100 || h != 150 {
		t.Errorf("page 000 = %dx%d", w, h)
	}
	if _, ok := entries["002.png"]; ok {
		t.Error("deleted page was exported")
	}
	var ci comicInfoXML
	if err := xml.Unmarshal(entries["ComicInfo.xml"], &ci); err != nil {
		t.Fatal(err)
	}
	if len(ci.Pages.Page) != 2 {
		t.Fatalf("pages in ComicInfo = %+v", ci.Pages.Page)
	}
	for i, p := range ci.Pages.Page {
		if p.ImageWidth != 100 || p.ImageHeight != 150 || p.ImageSize == 111 || p.ImageSize == 333 || p.ImageSize == 0 {
			t.Errorf("ComicInfo page %d = %+v, want the new size", i, p)
		}
	}
	if book.Pages[0].ImageSize != 111 || book.Pages[2].ImageWidth != 400 {
		t.Error("the library's own page list was changed")
	}
}
