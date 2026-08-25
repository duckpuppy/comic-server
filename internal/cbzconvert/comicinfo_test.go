package cbzconvert

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestBuildComicInfoXML_MapsFields(t *testing.T) {
	book := &library.ComicBook{
		Title:       "Issue Title",
		Series:      "Series Name",
		Number:      "5",
		Year:        2019,
		Month:       3,
		Publisher:   "Test Publisher",
		Writer:      "Writer Name",
		Genre:       "Superhero",
		LanguageISO: "en",
	}

	data, err := BuildComicInfoXML(book, 22)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(string(data), xml.Header) {
		t.Error("expected output to start with the XML declaration")
	}

	var ci comicInfoXML
	if err := xml.Unmarshal(data, &ci); err != nil {
		t.Fatalf("output is not well-formed XML: %v", err)
	}

	if ci.Title != book.Title || ci.Series != book.Series || ci.Number != book.Number ||
		ci.Year != book.Year || ci.Month != book.Month || ci.Publisher != book.Publisher ||
		ci.Writer != book.Writer || ci.Genre != book.Genre || ci.LanguageISO != book.LanguageISO {
		t.Errorf("field mismatch: %+v", ci)
	}
	if ci.PageCount != 22 {
		t.Errorf("PageCount = %d, want 22", ci.PageCount)
	}
}

func TestBuildComicInfoXML_FrontCoverPageMarker(t *testing.T) {
	data, err := BuildComicInfoXML(&library.ComicBook{}, 5)
	if err != nil {
		t.Fatal(err)
	}
	var ci comicInfoXML
	if err := xml.Unmarshal(data, &ci); err != nil {
		t.Fatal(err)
	}
	if len(ci.Pages.Page) != 1 || ci.Pages.Page[0].Image != 0 || ci.Pages.Page[0].Type != library.PageTypeFrontCover {
		t.Errorf("expected a single Image=0 FrontCover page marker, got %+v", ci.Pages.Page)
	}
}

func TestBuildComicInfoXML_ZeroPagesNoPageMarker(t *testing.T) {
	data, err := BuildComicInfoXML(&library.ComicBook{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var ci comicInfoXML
	if err := xml.Unmarshal(data, &ci); err != nil {
		t.Fatal(err)
	}
	if len(ci.Pages.Page) != 0 {
		t.Errorf("expected no page markers for pageCount=0, got %+v", ci.Pages.Page)
	}
}
