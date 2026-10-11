package library

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestComicBookIsDirty_XMLRoundTrip(t *testing.T) {
	var b ComicBook
	if err := xml.Unmarshal([]byte(`<Book Id="x"><Series>A</Series><ComicBookIsDirty>true</ComicBookIsDirty></Book>`), &b); err != nil {
		t.Fatal(err)
	}
	if !b.ComicBookIsDirty {
		t.Fatal("ComicBookIsDirty not read from XML")
	}
	out, err := xml.Marshal(&b)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<ComicBookIsDirty>true</ComicBookIsDirty>") {
		t.Errorf("ComicBookIsDirty not written back: %s", out)
	}

	b.ComicBookIsDirty = false
	out, _ = xml.Marshal(&b)
	if strings.Contains(string(out), "ComicBookIsDirty") {
		t.Errorf("false flag should be omitted (ComicRack default): %s", out)
	}
}

func TestModifiedLibraryInfoMatcher(t *testing.T) {
	dirty := &ComicBook{ComicBookIsDirty: true}
	clean := &ComicBook{}
	infoDirty := &ComicBook{ComicInfoIsDirty: true} // the sibling flag must not leak in

	tests := []struct {
		name     string
		operator string // YesNo: 0 = yes, 1 = no
		book     *ComicBook
		want     bool
	}{
		{"yes matches dirty", "0", dirty, true},
		{"yes skips clean", "0", clean, false},
		{"no matches clean", "1", clean, true},
		{"no skips dirty", "1", dirty, false},
		{"separate from ComicInfoIsDirty", "0", infoDirty, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := NewMatcherFromXML(&ComicBookMatcher{Type: "ComicBookModifiedLibraryInfoMatcher", MatchOperator: tt.operator})
			if err != nil {
				t.Fatal(err)
			}
			if got := m.Match(tt.book); got != tt.want {
				t.Errorf("Match = %v, want %v", got, tt.want)
			}
		})
	}
}
