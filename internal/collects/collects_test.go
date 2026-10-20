package collects

import (
	"reflect"
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestRoundTripKeepsSeparators(t *testing.T) {
	in := []Entry{
		{Volume: 4050, Series: "Ghost Rider, Danny Ketch = 1+1", From: "1", To: "5"},
		{Series: "Star Wars (2015) | Annual", From: "Annual 1", Kind: KindBackup, Partial: true, Source: SourceSuggested},
	}
	enc := Encode(in)
	if strings.ContainsAny(enc, ", =") {
		t.Fatalf("encoded text has a custom-value separator: %q", enc)
	}
	got := Decode(enc)
	want := []Entry{in[0].Normalize(), in[1].Normalize()}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip\n got %+v\nwant %+v", got, want)
	}
}

func TestNoteRoundTrip(t *testing.T) {
	in := Entry{Volume: 9, From: "181", Partial: true, Note: "The Hulk story, part 1 = a|b+c"}
	enc := Encode([]Entry{in})
	if strings.ContainsAny(enc, ", =") {
		t.Fatalf("encoded text has a custom-value separator: %q", enc)
	}
	got := Decode(enc)
	if len(got) != 1 || got[0].Note != in.Note {
		t.Errorf("got %+v", got)
	}
	if n := strings.Count(Encode([]Entry{{Volume: 9, From: "1"}}), "|"); n != 6 {
		t.Errorf("an entry without a note must keep 7 fields, got %d separators", n)
	}
	if (Entry{Volume: 1, From: "1", Note: strings.Repeat("x", 201)}).Validate() == nil {
		t.Error("a note over the limit should be refused")
	}
}

func TestDecodeOldSevenFieldValue(t *testing.T) {
	got := Decode("12||1|5|main|1|manual")
	if len(got) != 1 || got[0].Note != "" || !got[0].Partial {
		t.Errorf("got %+v", got)
	}
}

func TestSetGetThroughCustomValues(t *testing.T) {
	b := &library.ComicBook{CustomValuesStore: ",other=1"}
	if !Set(b, []Entry{{Volume: 7, From: "3", To: "3"}}) {
		t.Fatal("first set should change the book")
	}
	if Set(b, []Entry{{Volume: 7, From: "3"}}) {
		t.Error("same links (To equal From) should be a no-op")
	}
	if !Has(b) || len(Get(b)) != 1 || Get(b)[0].To != "" {
		t.Errorf("got %+v", Get(b))
	}
	if v, _ := library.GetCustomValue(b.CustomValuesStore, "other"); v != "1" {
		t.Error("other custom values must survive")
	}
	if !Set(b, nil) || Has(b) {
		t.Error("empty list should remove the value")
	}
	if Set(b, nil) {
		t.Error("removing nothing is a no-op")
	}
}

func TestValidate(t *testing.T) {
	bad := []Entry{
		{From: "1"},                             // no volume or series
		{Volume: 1},                             // no first issue
		{Volume: -1, From: "1"},                 // negative volume
		{Volume: 1, From: "1", Kind: "weird"},   // unknown kind
		{Volume: 1, From: "1", Source: "robot"}, // unknown source
		{Volume: 1, From: strings.Repeat("9", 201)},
	}
	for i, e := range bad {
		if e.Validate() == nil {
			t.Errorf("case %d should be invalid: %+v", i, e)
		}
	}
	if err := (Entry{Series: "X", From: "1"}).Validate(); err != nil {
		t.Errorf("series-only entry should be valid: %v", err)
	}
	if ValidateAll(make([]Entry, MaxEntries+1)) == nil {
		t.Error("too many entries should be refused")
	}
}

func TestDecodeSkipsDamagedEntries(t *testing.T) {
	got := Decode("garbage+12||1|5|main|0|manual+x||1||main|0|manual")
	if len(got) != 1 || got[0].Volume != 12 {
		t.Errorf("got %+v", got)
	}
}
