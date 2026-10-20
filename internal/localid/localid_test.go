package localid

import (
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestSetGetClear(t *testing.T) {
	b := &library.ComicBook{ID: "a", CustomValuesStore: ",comicvine_x=1"}
	id := Identity{Volume: NewVolumeKey(), Issue: NewIssueKey()}
	if !Set(b, id) || Set(b, id) {
		t.Fatal("Set should change once, then be a no-op")
	}
	got, ok := Get(b)
	if !ok || got != id {
		t.Errorf("Get = %+v %v, want %+v", got, ok, id)
	}
	if !Has(b) || !strings.Contains(b.CustomValuesStore, "comicvine_x=1") {
		t.Errorf("store = %q", b.CustomValuesStore)
	}
	if !Clear(b) || Clear(b) || Has(b) {
		t.Error("Clear should remove once")
	}
}

func TestGetRejectsDamagedValue(t *testing.T) {
	for _, v := range []string{"", "lv-1", "x|y", "lv-1|zz"} {
		b := &library.ComicBook{CustomValuesStore: library.SetCustomValue("", Key, v)}
		if Has(b) {
			t.Errorf("%q should not be an identity", v)
		}
	}
}

func TestKeysAreUnique(t *testing.T) {
	if NewVolumeKey() == NewVolumeKey() || NewIssueKey() == NewIssueKey() {
		t.Error("keys repeat")
	}
}

func TestSiblings(t *testing.T) {
	self := library.ComicBook{ID: "1", Series: "Ghost Rider  TPB", FilePath: "a"}
	books := []library.ComicBook{
		self,
		{ID: "2", Series: "ghost rider tpb", FilePath: "b"},
		{ID: "3", Series: "Ghost Rider TPB", FilePath: ""},                                            // placeholder
		{ID: "4", Series: "Ghost Rider TPB", FilePath: "d", CustomValuesStore: ",comicvine_volume=9"}, // has a CV volume
		{ID: "5", Series: "Other", FilePath: "e"},
	}
	sib := Siblings(books, &books[0])
	if len(sib) != 1 || sib[0].ID != "2" {
		t.Errorf("siblings = %v", sib)
	}
	Set(sib[0], Identity{Volume: "lv-bbb", Issue: "li-1"})
	if got := ExistingVolumeKey(sib); got != "lv-bbb" {
		t.Errorf("existing key = %q", got)
	}
	if Siblings(books, &library.ComicBook{ID: "x"}) != nil {
		t.Error("a book with no series has no siblings")
	}
}
