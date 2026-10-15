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

func threePageBook(t *testing.T) (*library.ComicBook, string, *trash.Trash) {
	t.Helper()
	src := filepath.Join(t.TempDir(), "book.cbz")
	writeTestCBZ(t, src, map[string][]byte{
		"001.jpg": []byte("A"), "002.jpg": []byte("B"), "003.jpg": []byte("C"), "004.jpg": []byte("D"),
		"ComicInfo.xml": []byte(`<ComicInfo><Title>Old</Title><Pages>` +
			`<Page Image="1" DoublePage="true"/><Page Image="3" Type="BackCover"/></Pages></ComicInfo>`),
	})
	book := &library.ComicBook{
		FilePath: src, Title: "T", Series: "S", PageCount: 4,
		CurrentPage: 2, LastPageRead: 3, PreferredFrontCover: 0,
		Pages: []library.ComicPageInfo{
			{Image: 0, Type: library.PageTypeFrontCover},
			{Image: 1, Bookmark: "mark"},
			{Image: 3, Type: library.PageTypeBackCover},
		},
	}
	return book, src, &trash.Trash{Root: t.TempDir(), RetentionDays: 30}
}

func orig(i ...int) []PageRef {
	var out []PageRef
	for _, n := range i {
		out = append(out, PageRef{Orig: n})
	}
	return out
}

func TestManagePages_ReorderDeleteAddRemapsEverything(t *testing.T) {
	book, src, tr := threePageBook(t)
	// New order: D, A, upload, B. C (index 2) is deleted.
	plan := PagePlan{
		Order:   append(append(orig(3, 0), PageRef{Orig: -1, Upload: 0}), orig(1)...),
		Uploads: []UploadedPage{{Ext: ".PNG", Data: []byte("NEW")}},
	}
	res, err := ManagePages(book, nil, tr, plan, Options{EmbedComicBook: true})
	if err != nil {
		t.Fatalf("ManagePages: %v", err)
	}
	if res.PageCount != 4 || res.Deleted != 1 || res.Added != 1 {
		t.Fatalf("result = %+v", res)
	}

	e := readZipEntries(t, src)
	want := map[string]string{"000.jpg": "D", "001.jpg": "A", "002.png": "NEW", "003.jpg": "B"}
	for name, data := range want {
		if string(e[name]) != data {
			t.Errorf("%s = %q, want %q (entries %v)", name, e[name], data, keys(e))
		}
	}
	if _, ok := e["001.jpg"]; !ok || len(e) != 6 { // 4 pages + ComicInfo + ComicBook
		t.Errorf("unexpected entries: %v", keys(e))
	}

	// Pages: A(cover) 0->1, B(bookmark) 1->3, D(back cover) 3->0; C had none.
	byImage := map[int]library.ComicPageInfo{}
	for _, p := range res.Pages {
		byImage[p.Image] = p
	}
	if byImage[1].Type != library.PageTypeFrontCover || byImage[3].Bookmark != "mark" || byImage[0].Type != library.PageTypeBackCover {
		t.Errorf("remapped pages = %+v", res.Pages)
	}
	if byImage[2].ImageSize != 3 {
		t.Errorf("upload page = %+v", byImage[2])
	}
	// CurrentPage 2 (C) was deleted: nearest kept before it is B (1) -> new 3.
	// LastPageRead 3 (D) -> 0. PreferredFrontCover 0 (A) -> 1.
	if res.CurrentPage != 3 || res.LastPageRead != 0 || res.PreferredFrontCover != 1 {
		t.Errorf("pointers = cur %d last %d cover %d", res.CurrentPage, res.LastPageRead, res.PreferredFrontCover)
	}

	var ci comicInfoXML
	if err := xml.Unmarshal(e[comicInfoEntryName], &ci); err != nil {
		t.Fatal(err)
	}
	if ci.PageCount != 4 {
		t.Errorf("PageCount = %d", ci.PageCount)
	}
	// The file-only DoublePage attribute follows page B (1 -> 3).
	var gotDouble bool
	for _, p := range ci.Pages.Page {
		if p.Image == 3 {
			for _, a := range p.Attrs {
				gotDouble = gotDouble || (a.Name.Local == "DoublePage" && a.Value == "true")
			}
		}
	}
	if !gotDouble {
		t.Errorf("DoublePage did not follow its page: %+v", ci.Pages.Page)
	}
	if !bytes.Contains(e[comicBookEntryName], []byte("mark")) {
		t.Error("ComicBook.xml missing remapped page info")
	}
}

func TestManagePages_RejectsBadPlans(t *testing.T) {
	book, _, tr := threePageBook(t)
	cases := map[string]PagePlan{
		"empty":     {},
		"range":     {Order: orig(0, 9)},
		"duplicate": {Order: orig(0, 0, 1)},
		"upload":    {Order: []PageRef{{Orig: -1, Upload: 0}}},
		"not image": {Order: append(orig(0), PageRef{Orig: -1}), Uploads: []UploadedPage{{Ext: ".exe", Data: []byte("x")}}},
	}
	for name, plan := range cases {
		var pe *PlanError
		if _, err := ManagePages(book, nil, tr, plan, Options{}); !errors.As(err, &pe) {
			t.Errorf("%s: err = %v, want PlanError", name, err)
		}
	}
}

func TestManagePages_NoChangeAndNonCBZ(t *testing.T) {
	book, src, tr := threePageBook(t)
	if _, err := ManagePages(book, nil, tr, PagePlan{Order: orig(0, 1, 2, 3)}, Options{}); !errors.Is(err, ErrNoPageChange) {
		t.Errorf("identity plan err = %v", err)
	}
	if e := readZipEntries(t, src); string(e["001.jpg"]) != "A" {
		t.Error("archive changed on a no-op plan")
	}
	if _, err := ManagePages(&library.ComicBook{FilePath: "x.cbr"}, nil, tr, PagePlan{Order: orig(0)}, Options{}); !errors.Is(err, ErrNotCBZ) {
		t.Errorf("cbr err = %v", err)
	}
}

func TestPageName(t *testing.T) {
	if got := PageName(7, 12, ".JPG"); got != "007.jpg" {
		t.Errorf("got %q", got)
	}
	if got := PageName(7, 1500, ".png"); got != "0007.png" {
		t.Errorf("got %q", got)
	}
}
