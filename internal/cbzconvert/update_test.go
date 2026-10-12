package cbzconvert

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/trash"
)

func TestUpdateMetadata_ReplacesOnlyMetadataEntries(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "book.cbz")
	p1, p2 := []byte("page-one"), []byte("page-two")
	writeTestCBZ(t, src, map[string][]byte{
		"001.jpg":           p1,
		"002.png":           p2,
		"sub/ComicInfo.xml": []byte("<ComicInfo><Title>Old</Title></ComicInfo>"),
		"extra/notes.txt":   []byte("keep me"),
	})

	book := &library.ComicBook{FilePath: src, Title: "New Title", Series: "S", PageCount: 2,
		Pages: []library.ComicPageInfo{{Image: 0}, {Image: 1, Type: library.PageTypeDeleted}}}
	tr := &trash.Trash{Root: t.TempDir(), RetentionDays: 30}

	res, err := UpdateMetadata(book, nil, tr, Options{EmbedComicBook: true})
	if err != nil {
		t.Fatalf("UpdateMetadata: %v", err)
	}
	if res.NewFilePath != src || res.PageCount != 2 {
		t.Errorf("result = %+v", res)
	}

	e := readZipEntries(t, src)
	if !bytes.Equal(e["001.jpg"], p1) || !bytes.Equal(e["002.png"], p2) {
		t.Error("page bytes changed")
	}
	if string(e["extra/notes.txt"]) != "keep me" {
		t.Error("unrelated entry lost")
	}
	if _, ok := e["sub/ComicInfo.xml"]; ok {
		t.Error("old nested ComicInfo.xml should be replaced, not kept")
	}
	var ci comicInfoXML
	if err := xml.Unmarshal(e[comicInfoEntryName], &ci); err != nil {
		t.Fatalf("ComicInfo.xml: %v", err)
	}
	if ci.Title != "New Title" {
		t.Errorf("Title = %q", ci.Title)
	}
	cb, ok := e[comicBookEntryName]
	if !ok {
		t.Fatal("ComicBook.xml missing")
	}
	// Deleted page is kept (not dropped) in an in-place update.
	if !bytes.Contains(cb, []byte("Deleted")) {
		t.Error("ComicBook.xml should keep the Deleted page record")
	}
}

func TestUpdateMetadata_QuarantinesOriginal(t *testing.T) {
	dir, trashDir := t.TempDir(), t.TempDir()
	src := filepath.Join(dir, "b.cbz")
	writeTestCBZ(t, src, map[string][]byte{"1.jpg": []byte("x")})
	tr := &trash.Trash{Root: trashDir, RetentionDays: 30}
	if _, err := UpdateMetadata(&library.ComicBook{FilePath: src, Title: "T"}, nil, tr, Options{}); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = filepath.Walk(trashDir, func(_ string, info os.FileInfo, _ error) error {
		if info != nil && !info.IsDir() {
			n++
		}
		return nil
	})
	if n != 1 {
		t.Errorf("quarantined files = %d, want 1", n)
	}
}

func TestUpdateMetadata_RejectsNonCBZ(t *testing.T) {
	tr := &trash.Trash{Root: t.TempDir(), RetentionDays: 30}
	_, err := UpdateMetadata(&library.ComicBook{FilePath: "/x/y.cbr"}, nil, tr, Options{})
	if err != ErrNotCBZ {
		t.Errorf("err = %v, want ErrNotCBZ", err)
	}
}

func TestIsStale_FingerprintLifecycle(t *testing.T) {
	book := &library.ComicBook{FilePath: "a.cbz", Title: "One", Series: "S"}

	if IsStale(book, false) {
		t.Fatal("never-written book with no dirty flag must not be stale")
	}
	MarkWritten(book, false)
	if IsStale(book, false) {
		t.Fatal("book must be clean right after MarkWritten")
	}
	// Recording the hash itself changes the custom values store; it must not
	// make the book stale.
	book.CustomValuesStore = library.SetCustomValue(book.CustomValuesStore, "comic_server_workflow_stage", "scrape")
	if IsStale(book, false) {
		t.Fatal("workflow stage bookkeeping must not make a book stale")
	}
	book.Title = "Two"
	if !IsStale(book, false) {
		t.Fatal("metadata edit must make the book stale")
	}
	MarkWritten(book, false)
	if IsStale(book, false) {
		t.Fatal("clean again after rewrite")
	}
}

func TestIsStale_DirtyFlagsAndFormats(t *testing.T) {
	book := &library.ComicBook{FilePath: "a.cbz", ComicInfoIsDirty: true}
	if !IsStale(book, false) {
		t.Error("ComicInfoIsDirty should be stale")
	}
	book = &library.ComicBook{FilePath: "a.cbz", ComicBookIsDirty: true}
	if IsStale(book, false) {
		t.Error("ComicBookIsDirty alone is not stale when ComicBook.xml is not embedded")
	}
	if !IsStale(book, true) {
		t.Error("ComicBookIsDirty should be stale when embedding")
	}
	cbr := &library.ComicBook{FilePath: "a.cbr", ComicInfoIsDirty: true}
	if IsStale(cbr, true) {
		t.Error("non-CBZ files are never stale (cannot be rewritten in place)")
	}
}

func TestMarkWritten_ClearsDirtyFlags(t *testing.T) {
	book := &library.ComicBook{FilePath: "a.cbz", ComicInfoIsDirty: true, ComicBookIsDirty: true}
	MarkWritten(book, false)
	if book.ComicInfoIsDirty || !book.ComicBookIsDirty {
		t.Errorf("without embed: info=%v book=%v, want false/true", book.ComicInfoIsDirty, book.ComicBookIsDirty)
	}
	MarkWritten(book, true)
	if book.ComicBookIsDirty {
		t.Error("with embed ComicBookIsDirty should clear")
	}
}
