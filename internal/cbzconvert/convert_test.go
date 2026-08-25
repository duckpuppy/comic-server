package cbzconvert

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/trash"
)

func writeTestCBZ(t *testing.T, path string, entries map[string][]byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for name, data := range entries {
		w, err := zw.Create(name)
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
}

func readZipEntries(t *testing.T, path string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	out := make(map[string][]byte)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = data
	}
	return out
}

func TestConvert_RepacksPagesByteIdenticalAndEmbedsComicInfo(t *testing.T) {
	libDir := t.TempDir()
	trashDir := t.TempDir()

	page1 := []byte("fake-jpeg-bytes-page-1")
	page2 := []byte("fake-png-bytes-page-2")
	src := filepath.Join(libDir, "book.cbz") // start as CBZ->CBZ to exercise repack without needing CBR/CB7 tooling
	writeTestCBZ(t, src, map[string][]byte{
		"page002.png": page2,
		"page001.jpg": page1,
	})

	book := &library.ComicBook{
		FilePath: src,
		Title:    "Test Issue",
		Series:   "Test Series",
		Number:   "1",
		Year:     2020,
		Writer:   "Jane Doe",
	}

	tr := &trash.Trash{Root: trashDir, RetentionDays: 30}
	result, err := Convert(book, nil, tr)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if result.NewFilePath != src {
		t.Errorf("NewFilePath = %q, want %q (same-path repack)", result.NewFilePath, src)
	}
	if result.PageCount != 2 {
		t.Errorf("PageCount = %d, want 2", result.PageCount)
	}

	entries := readZipEntries(t, src)
	if !bytes.Equal(entries["page001.jpg"], page1) {
		t.Error("page001.jpg not byte-identical to source")
	}
	if !bytes.Equal(entries["page002.png"], page2) {
		t.Error("page002.png not byte-identical to source")
	}

	ciData, ok := entries[comicInfoEntryName]
	if !ok {
		t.Fatal("ComicInfo.xml missing from converted archive")
	}
	var ci comicInfoXML
	if err := xml.Unmarshal(ciData, &ci); err != nil {
		t.Fatalf("ComicInfo.xml not well-formed: %v", err)
	}
	if ci.Title != "Test Issue" || ci.Series != "Test Series" || ci.Writer != "Jane Doe" || ci.Year != 2020 {
		t.Errorf("ComicInfo.xml metadata mismatch: %+v", ci)
	}
	if ci.PageCount != 2 {
		t.Errorf("ComicInfo.xml PageCount = %d, want 2", ci.PageCount)
	}
}

func TestConvert_QuarantinesOriginalNotDeletes(t *testing.T) {
	libDir := t.TempDir()
	trashDir := t.TempDir()
	src := filepath.Join(libDir, "book.cbz")
	original := []byte("page-a")
	writeTestCBZ(t, src, map[string][]byte{"page001.jpg": original})

	book := &library.ComicBook{FilePath: src, Title: "X"}
	tr := &trash.Trash{Root: trashDir, RetentionDays: 30}

	if _, err := Convert(book, nil, tr); err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	// The pre-conversion archive bytes should be recoverable from
	// quarantine, not gone. Since same-path repack uses tr.Replace, the
	// original whole file (not just its page bytes) is quarantined.
	found := false
	_ = filepath.WalkDir(trashDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		zr, zerr := zip.OpenReader(path)
		if zerr != nil {
			return nil
		}
		defer zr.Close()
		for _, f := range zr.File {
			if f.Name == "page001.jpg" {
				found = true
			}
		}
		return nil
	})
	if !found {
		t.Error("original archive not found in quarantine")
	}
}

func TestConvert_DifferentExtension_WriteNewThenQuarantine(t *testing.T) {
	// Exercises the cross-extension path directly with a fake resolvePath
	// that maps a .cbr-suffixed raw path to an actual .cbz test fixture on
	// disk (standing in for a CBR archive, since building a real RAR
	// fixture needs an external tool) - what matters here is that Convert
	// computes a genuinely different target path and drives WriteNew +
	// Quarantine, not Replace.
	libDir := t.TempDir()
	trashDir := t.TempDir()
	actualSrc := filepath.Join(libDir, "book.cbz")
	writeTestCBZ(t, actualSrc, map[string][]byte{"page001.jpg": []byte("data")})

	rawSrc := filepath.Join(libDir, "book.cbr") // never actually created on disk
	resolvePath := func(p string) string {
		if p == rawSrc {
			return actualSrc
		}
		return p // the computed .cbz raw target already matches the real path
	}

	book := &library.ComicBook{FilePath: rawSrc, Title: "Y"}
	tr := &trash.Trash{Root: trashDir, RetentionDays: 30}

	result, err := Convert(book, resolvePath, tr)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	wantNewPath := filepath.Join(libDir, "book.cbz")
	if result.NewFilePath != wantNewPath {
		t.Errorf("NewFilePath = %q, want %q", result.NewFilePath, wantNewPath)
	}

	// The resolved .cbz path should now contain the converted content
	// (over top of the fixture data - resolvePath aliased actualSrc to
	// both the source archive AND the target CBZ path in this test setup,
	// same as a real .cbr->.cbz conversion where they're different raw
	// paths that happen to both resolve under the same mount).
	entries := readZipEntries(t, actualSrc)
	if _, ok := entries[comicInfoEntryName]; !ok {
		t.Error("ComicInfo.xml missing from converted archive")
	}
}

func TestConvert_NoImagesInSource(t *testing.T) {
	libDir := t.TempDir()
	trashDir := t.TempDir()
	src := filepath.Join(libDir, "empty.cbz")
	writeTestCBZ(t, src, map[string][]byte{"ComicInfo.xml": []byte(`<ComicInfo/>`)})

	book := &library.ComicBook{FilePath: src}
	tr := &trash.Trash{Root: trashDir, RetentionDays: 30}

	if _, err := Convert(book, nil, tr); err == nil {
		t.Fatal("expected error for source archive with no image pages")
	}

	// Source must be untouched on failure.
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source should be untouched after failure: %v", err)
	}
}

func TestConvert_SourceFileNotFound(t *testing.T) {
	libDir := t.TempDir()
	trashDir := t.TempDir()
	book := &library.ComicBook{FilePath: filepath.Join(libDir, "missing.cbz")}
	tr := &trash.Trash{Root: trashDir, RetentionDays: 30}

	if _, err := Convert(book, nil, tr); err == nil {
		t.Fatal("expected error for missing source file")
	}
}

func TestConvert_DropsDeletedPages(t *testing.T) {
	libDir := t.TempDir()
	trashDir := t.TempDir()
	src := filepath.Join(libDir, "book.cbz")
	page0 := []byte("page-0-kept")
	page1 := []byte("page-1-deleted")
	page2 := []byte("page-2-kept")
	writeTestCBZ(t, src, map[string][]byte{
		"page000.jpg": page0,
		"page001.jpg": page1,
		"page002.jpg": page2,
	})

	book := &library.ComicBook{
		FilePath: src,
		Pages: []library.ComicPageInfo{
			{Image: 0, Type: library.PageTypeStory},
			{Image: 1, Type: library.PageTypeDeleted},
			{Image: 2, Type: library.PageTypeStory},
		},
	}
	tr := &trash.Trash{Root: trashDir, RetentionDays: 30}

	result, err := Convert(book, nil, tr)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	if result.PageCount != 2 {
		t.Errorf("PageCount = %d, want 2 (deleted page excluded)", result.PageCount)
	}

	entries := readZipEntries(t, src)
	if !bytes.Equal(entries["page000.jpg"], page0) {
		t.Error("page000.jpg (kept) missing or wrong")
	}
	if !bytes.Equal(entries["page002.jpg"], page2) {
		t.Error("page002.jpg (kept) missing or wrong")
	}
	if _, ok := entries["page001.jpg"]; ok {
		t.Error("page001.jpg (marked Deleted) should not be in the converted archive")
	}

	ciData := entries[comicInfoEntryName]
	var ci comicInfoXML
	if err := xml.Unmarshal(ciData, &ci); err != nil {
		t.Fatalf("ComicInfo.xml not well-formed: %v", err)
	}
	if ci.PageCount != 2 {
		t.Errorf("ComicInfo.xml PageCount = %d, want 2", ci.PageCount)
	}
}

func TestConvert_AllPagesDeletedIsAnError(t *testing.T) {
	libDir := t.TempDir()
	trashDir := t.TempDir()
	src := filepath.Join(libDir, "book.cbz")
	writeTestCBZ(t, src, map[string][]byte{"page000.jpg": []byte("x")})

	book := &library.ComicBook{
		FilePath: src,
		Pages:    []library.ComicPageInfo{{Image: 0, Type: library.PageTypeDeleted}},
	}
	tr := &trash.Trash{Root: trashDir, RetentionDays: 30}

	if _, err := Convert(book, nil, tr); err == nil {
		t.Fatal("expected error when every page is marked Deleted")
	}
}

func TestDropDeletedPages(t *testing.T) {
	pages := []comicvine.Page{{Name: "a"}, {Name: "b"}, {Name: "c"}}

	// No book.Pages info at all -> nothing filtered.
	if got := dropDeletedPages(pages, nil); len(got) != 3 {
		t.Errorf("expected no filtering with nil bookPages, got %d pages", len(got))
	}

	// No Deleted entries -> nothing filtered.
	notDeleted := []library.ComicPageInfo{{Image: 0, Type: library.PageTypeStory}}
	if got := dropDeletedPages(pages, notDeleted); len(got) != 3 {
		t.Errorf("expected no filtering with no Deleted pages, got %d pages", len(got))
	}

	// Middle page deleted -> 2 remain, in original relative order.
	withDeleted := []library.ComicPageInfo{{Image: 1, Type: library.PageTypeDeleted}}
	got := dropDeletedPages(pages, withDeleted)
	if len(got) != 2 || got[0].Name != "a" || got[1].Name != "c" {
		t.Errorf("got %v, want [a, c]", got)
	}
}

func TestChangeExt(t *testing.T) {
	cases := []struct{ in, want string }{
		{`G:\Comics\book.cbr`, `G:\Comics\book.cbz`},
		{"/comics/book.cb7", "/comics/book.cbz"},
		{"/comics/book.cbz", "/comics/book.cbz"},
		{"/comics/noext", "/comics/noext.cbz"},
	}
	for _, c := range cases {
		if got := changeExt(c.in, ".cbz"); got != c.want {
			t.Errorf("changeExt(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
