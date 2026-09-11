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

// TestParseComicInfoXML_RoundTripsBuildComicInfoXML is the regression test
// for comic-server-chh's watch-folder metadata seeding: parsing back what
// BuildComicInfoXML wrote must recover every field it mapped, so a book
// created from an archive's ComicInfo.xml doesn't silently lose data
// relative to a book synced through the normal export/import cycle.
func TestParseComicInfoXML_RoundTripsBuildComicInfoXML(t *testing.T) {
	original := &library.ComicBook{
		Title:               "Issue Title",
		Series:              "Series Name",
		Number:              "5",
		Year:                2019,
		Month:               3,
		Publisher:           "Test Publisher",
		Writer:              "Writer Name",
		Genre:               "Superhero",
		LanguageISO:         "en",
		MainCharacterOrTeam: "Spider-Man",
		Rating:              4.5,
		CommunityRating:     3.75,
		Review:              "A fine issue.",
	}

	data, err := BuildComicInfoXML(original, 22)
	if err != nil {
		t.Fatal(err)
	}

	parsed, ok := ParseComicInfoXML(data)
	if !ok {
		t.Fatal("ParseComicInfoXML returned ok=false for valid data")
	}
	if parsed.Title != original.Title || parsed.Series != original.Series ||
		parsed.Number != original.Number || parsed.Year != original.Year ||
		parsed.Month != original.Month || parsed.Publisher != original.Publisher ||
		parsed.Writer != original.Writer || parsed.Genre != original.Genre ||
		parsed.LanguageISO != original.LanguageISO ||
		parsed.MainCharacterOrTeam != original.MainCharacterOrTeam ||
		parsed.Rating != original.Rating ||
		parsed.CommunityRating != original.CommunityRating ||
		parsed.Review != original.Review {
		t.Errorf("parsed book = %+v, want fields matching original %+v", parsed, original)
	}
	if parsed.PageCount != 22 {
		t.Errorf("PageCount = %d, want 22", parsed.PageCount)
	}
}

func TestParseComicInfoXML_InvalidDataReturnsNotOK(t *testing.T) {
	if _, ok := ParseComicInfoXML([]byte("not xml")); ok {
		t.Error("expected ok=false for invalid XML")
	}
}
