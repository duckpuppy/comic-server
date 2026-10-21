package cbzconvert

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestExportCopy_WritesCopyAndLeavesSourceAlone(t *testing.T) {
	src := filepath.Join(t.TempDir(), "book.cbz")
	writeTestCBZ(t, src, map[string][]byte{"p1.jpg": []byte("one"), "p2.jpg": []byte("two")})
	before, _ := os.ReadFile(src)
	dest := filepath.Join(t.TempDir(), "out.cbz")

	book := &library.ComicBook{FilePath: src, Series: "S", Number: "1"}
	n, err := ExportCopy(book, src, dest, Options{}, false)
	if err != nil || n != 2 {
		t.Fatalf("ExportCopy = %d, %v", n, err)
	}
	entries := readZipEntries(t, dest)
	if !bytes.Equal(entries["p1.jpg"], []byte("one")) || entries["ComicInfo.xml"] == nil {
		t.Errorf("export entries wrong: %v", entries)
	}
	after, _ := os.ReadFile(src)
	if !bytes.Equal(before, after) {
		t.Error("the source file changed")
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(dest), ".export-*"))
	if len(left) != 0 {
		t.Errorf("temp files left behind: %v", left)
	}
}

func TestExportCopy_ExistingTargetNeedsOverwrite(t *testing.T) {
	src := filepath.Join(t.TempDir(), "book.cbz")
	writeTestCBZ(t, src, map[string][]byte{"p1.jpg": []byte("one")})
	dest := filepath.Join(t.TempDir(), "out.cbz")
	if err := os.WriteFile(dest, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	book := &library.ComicBook{FilePath: src}
	if _, err := ExportCopy(book, src, dest, Options{}, false); !errors.Is(err, ErrExportExists) {
		t.Fatalf("want ErrExportExists, got %v", err)
	}
	if got, _ := os.ReadFile(dest); string(got) != "old" {
		t.Error("existing target was changed without overwrite")
	}
	if _, err := ExportCopy(book, src, dest, Options{}, true); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	if got, _ := os.ReadFile(dest); string(got) == "old" {
		t.Error("overwrite did not replace the target")
	}
}

func TestExportCopy_RefusesSourceAsTarget(t *testing.T) {
	src := filepath.Join(t.TempDir(), "book.cbz")
	writeTestCBZ(t, src, map[string][]byte{"p1.jpg": []byte("one")})
	if _, err := ExportCopy(&library.ComicBook{FilePath: src}, src, src, Options{}, true); err == nil {
		t.Fatal("exporting onto the source must fail")
	}
}

func TestExportCopy_DropsDeletedPages(t *testing.T) {
	src := filepath.Join(t.TempDir(), "book.cbz")
	writeTestCBZ(t, src, map[string][]byte{"p1.jpg": []byte("one"), "p2.jpg": []byte("two")})
	dest := filepath.Join(t.TempDir(), "out.cbz")
	book := &library.ComicBook{FilePath: src, Pages: []library.ComicPageInfo{{Image: 1, Type: library.PageTypeDeleted}}}
	n, err := ExportCopy(book, src, dest, Options{}, false)
	if err != nil || n != 1 {
		t.Fatalf("ExportCopy = %d, %v", n, err)
	}
}
