package cbzconvert

import (
	"bytes"
	"encoding/xml"
	"errors"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/trash"
)

func TestAddVariantCovers_AppendsMarksFrontCoverAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "book.cbz")
	p1, p2 := []byte("page-one"), []byte("page-two")
	writeTestCBZ(t, src, map[string][]byte{
		"001.jpg": p1, "002.jpg": p2,
		"ComicInfo.xml": []byte("<ComicInfo><Title>Old</Title></ComicInfo>"),
	})
	book := &library.ComicBook{FilePath: src, Title: "T", Series: "S", PageCount: 2}
	tr := &trash.Trash{Root: t.TempDir(), RetentionDays: 30}
	covers := []CoverImage{
		{ID: 11, Ext: ".jpg", Data: []byte("variant-b")},
		{ID: 12, Ext: ".PNG", Data: []byte("variant-c")},
	}

	res, err := AddVariantCovers(book, nil, tr, covers, Options{EmbedComicBook: true})
	if err != nil {
		t.Fatalf("AddVariantCovers: %v", err)
	}
	if res.PageCount != 4 || res.FirstIndex != 2 || len(res.AddedIDs) != 2 || len(res.NewPages) != 2 {
		t.Fatalf("result = %+v", res)
	}
	if res.NewPages[0].Image != 2 || res.NewPages[1].Image != 3 || res.NewPages[0].Type != library.PageTypeFrontCover {
		t.Errorf("NewPages = %+v", res.NewPages)
	}

	e := readZipEntries(t, src)
	if !bytes.Equal(e["001.jpg"], p1) || !bytes.Equal(e["002.jpg"], p2) {
		t.Error("original pages changed")
	}
	if string(e["zzzz-cv-variant-11.jpg"]) != "variant-b" || string(e["zzzz-cv-variant-12.png"]) != "variant-c" {
		t.Errorf("variant entries missing: %v", keys(e))
	}

	var ci comicInfoXML
	if err := xml.Unmarshal(e[comicInfoEntryName], &ci); err != nil {
		t.Fatal(err)
	}
	if ci.PageCount != 4 {
		t.Errorf("PageCount = %d, want 4", ci.PageCount)
	}
	var covers0, covers2, covers3 bool
	for _, pg := range ci.Pages.Page {
		if pg.Type == library.PageTypeFrontCover {
			covers0 = covers0 || pg.Image == 0
			covers2 = covers2 || pg.Image == 2
			covers3 = covers3 || pg.Image == 3
		}
	}
	if !covers0 || !covers2 || !covers3 {
		t.Errorf("ComicInfo Pages = %+v, want FrontCover at 0, 2, 3", ci.Pages.Page)
	}
	if !bytes.Contains(e[comicBookEntryName], []byte("FrontCover")) {
		t.Error("ComicBook.xml should carry the new FrontCover pages")
	}

	// Second run with the same images is a no-op, not a rewrite.
	if _, err := AddVariantCovers(book, nil, tr, covers, Options{}); !errors.Is(err, ErrNothingToAdd) {
		t.Errorf("repeat err = %v, want ErrNothingToAdd", err)
	}
	// A new image on top of the existing ones appends after them.
	res2, err := AddVariantCovers(book, nil, tr, []CoverImage{{ID: 13, Ext: ".jpg", Data: []byte("v-d")}}, Options{})
	if err != nil || res2.FirstIndex != 4 || res2.PageCount != 5 {
		t.Errorf("third add = %+v err=%v", res2, err)
	}
}

func TestAddVariantCovers_RejectsNonCBZ(t *testing.T) {
	tr := &trash.Trash{Root: t.TempDir(), RetentionDays: 30}
	_, err := AddVariantCovers(&library.ComicBook{FilePath: "x.cbr"}, nil, tr, []CoverImage{{ID: 1, Ext: ".jpg", Data: []byte("x")}}, Options{})
	if !errors.Is(err, ErrNotCBZ) {
		t.Errorf("err = %v, want ErrNotCBZ", err)
	}
}

func keys(m map[string][]byte) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
