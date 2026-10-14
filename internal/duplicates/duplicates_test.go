package duplicates

import (
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func bk(id, series, num, file string, pages int) *library.ComicBook {
	return &library.ComicBook{ID: id, Series: series, Number: num, FilePath: file, PageCount: pages}
}

func hasReason(g Group, r Reason) bool {
	for _, x := range g.Reasons {
		if x == r {
			return true
		}
	}
	return false
}

func TestDetect_SeriesNumberIgnoresLeadingZerosAndArticles(t *testing.T) {
	books := []*library.ComicBook{
		bk("a", "The Batman", "007", "/x/a.cbz", 22),
		bk("b", "Batman", "7", "/y/b.cbz", 24),
		bk("c", "Batman", "8", "/y/c.cbz", 24),
	}
	groups := Detect(books, Options{})
	if len(groups) != 1 || len(groups[0].Books) != 2 {
		t.Fatalf("groups = %+v", groups)
	}
	if !hasReason(groups[0], ReasonSeriesNumber) {
		t.Errorf("reasons = %v", groups[0].Reasons)
	}
}

func TestDetect_DifferentVolumeOrFormatNotDuplicate(t *testing.T) {
	a := bk("a", "Batman", "1", "/a.cbz", 22)
	b := bk("b", "Batman", "1", "/b.cbz", 22)
	b.Volume = 2
	c := bk("c", "Batman", "1", "/c.cbz", 22)
	c.Format = "TPB"
	if g := Detect([]*library.ComicBook{a, b, c}, Options{}); len(g) != 0 {
		t.Fatalf("expected none, got %+v", g)
	}
}

func TestDetect_FilelessNeverGroupsWithFile(t *testing.T) {
	digital := bk("d", "Batman", "1", "/d.cbz", 22)
	digital.CustomValuesStore = ",comicvine_issue=555"
	physical := bk("p", "Batman", "1", "", 0)
	physical.CustomValuesStore = ",comicvine_issue=555"
	if g := Detect([]*library.ComicBook{digital, physical}, Options{}); len(g) != 0 {
		t.Fatalf("physical + digital grouped: %+v", g)
	}

	// Two fileless entries for one issue are still duplicates of each other.
	physical2 := bk("p2", "Batman", "001", "", 0)
	g := Detect([]*library.ComicBook{digital, physical, physical2}, Options{})
	if len(g) != 1 || len(g[0].Books) != 2 || g[0].Books[0].FilePath != "" || g[0].Books[1].FilePath != "" {
		t.Fatalf("groups = %+v", g)
	}
}

func TestDetect_ComicVineIssue(t *testing.T) {
	a := bk("a", "Foo", "1", "/a.cbz", 20)
	a.CustomValuesStore = ",comicvine_issue=555"
	b := bk("b", "Totally Different", "9", "/b.cbz", 20)
	b.CustomValuesStore = ",comicvine_volume=1,comicvine_issue=555"
	groups := Detect([]*library.ComicBook{a, b}, Options{})
	if len(groups) != 1 || !hasReason(groups[0], ReasonComicVineIssue) {
		t.Fatalf("groups = %+v", groups)
	}
}

func TestDetect_PageCountGuard(t *testing.T) {
	issue := bk("a", "Batman", "1", "/a.cbz", 22)
	tpb := bk("b", "Batman", "1", "/b.cbz", 180)
	if g := Detect([]*library.ComicBook{issue, tpb}, Options{}); len(g) != 0 {
		t.Fatalf("3x guard should drop group, got %+v", g)
	}
	ok := bk("c", "Batman", "1", "/c.cbz", 60) // 60 <= 3*22
	if g := Detect([]*library.ComicBook{issue, ok}, Options{}); len(g) != 1 {
		t.Fatalf("within ratio should group, got %+v", g)
	}
}

func TestDetect_FilenameCopy(t *testing.T) {
	books := []*library.ComicBook{
		bk("a", "Alpha", "1", "/lib/Comic X.cbz", 20),
		bk("b", "Beta", "2", "/lib/Comic X (1).cbz", 20),
		bk("c", "Gamma", "3", "/lib/Other (2011).cbz", 20),
		bk("d", "Delta", "4", "/lib/Other.cbz", 20),
	}
	groups := Detect(books, Options{})
	if len(groups) != 1 || !hasReason(groups[0], ReasonFilenameCopy) {
		t.Fatalf("groups = %+v", groups)
	}
	if len(groups[0].Books) != 2 || groups[0].Books[0].ID == "c" {
		t.Errorf("wrong members: %+v", groups[0].Books)
	}
}

func TestDetect_CoverAnchorDoesNotChain(t *testing.T) {
	// Hashes: A=0, B differs by 4 bits, C differs from B by 4 more bits
	// (8 from A). B matches anchor A; C must NOT chain through B.
	hashes := map[string]uint64{"a": 0, "b": 0xF, "c": 0xFF}
	books := []*library.ComicBook{
		bk("a", "Same Series", "1", "/a.cbz", 20),
		bk("b", "Same Series", "2", "/b.cbz", 20),
		bk("c", "Same Series", "3", "/c.cbz", 20),
	}
	groups := Detect(books, Options{CoverHash: func(b *library.ComicBook) (uint64, bool) {
		h, ok := hashes[b.ID]
		return h, ok
	}})
	if len(groups) != 1 || len(groups[0].Books) != 2 || !hasReason(groups[0], ReasonCover) {
		t.Fatalf("groups = %+v", groups)
	}
	for _, b := range groups[0].Books {
		if b.ID == "c" {
			t.Errorf("c chained into group")
		}
	}
}

func TestGroupKeyOrderIndependent(t *testing.T) {
	if GroupKey([]string{"a", "b", "c"}) != GroupKey([]string{"c", "a", "b"}) {
		t.Error("key depends on order")
	}
	if GroupKey([]string{"a", "b"}) == GroupKey([]string{"a", "b", "c"}) {
		t.Error("key ignores membership")
	}
}

func TestNormalizeNumber(t *testing.T) {
	cases := map[string]string{"007": "7", "0": "0", "000": "0", "0.5": "0.5", "12": "12", " 01a ": "1a", "": ""}
	for in, want := range cases {
		if got := normalizeNumber(in); got != want {
			t.Errorf("normalizeNumber(%q) = %q, want %q", in, got, want)
		}
	}
}
