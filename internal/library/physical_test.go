package library

import "testing"

func TestIsPhysical(t *testing.T) {
	cases := []struct {
		b        ComicBook
		physical bool
		wanted   bool
	}{
		{ComicBook{BookCollectionStatus: "Physical"}, true, false},
		{ComicBook{BookCollectionStatus: " physical "}, true, false},
		{ComicBook{}, false, true},
		{ComicBook{BookCollectionStatus: "Owned"}, false, true},
		{ComicBook{FilePath: "a.cbz", BookCollectionStatus: "Physical"}, false, false},
	}
	for i, c := range cases {
		if got := IsPhysical(&c.b); got != c.physical {
			t.Errorf("%d: IsPhysical = %v", i, got)
		}
		if got := IsWantedPlaceholder(&c.b); got != c.wanted {
			t.Errorf("%d: IsWantedPlaceholder = %v", i, got)
		}
	}
}
