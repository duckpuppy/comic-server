package datamanager

import (
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestComicBookIsDirtyField_ReadsAndWritesRealFlag(t *testing.T) {
	book := &library.ComicBook{}
	if v, ok := GetFieldString(book, "ComicBookIsDirty"); !ok || v != "no" {
		t.Fatalf("clean book: got %q, %v, want no", v, ok)
	}
	if err := SetFieldString(book, "ComicBookIsDirty", "yes"); err != nil {
		t.Fatalf("SetFieldString: %v", err)
	}
	if !book.ComicBookIsDirty {
		t.Fatal("flag not written to the book")
	}
	if v, _ := GetFieldString(book, "ComicBookIsDirty"); v != "yes" {
		t.Errorf("got %q after set, want yes", v)
	}
	if book.ComicInfoIsDirty {
		t.Error("writing ComicBookIsDirty must not touch ComicInfoIsDirty")
	}
}
