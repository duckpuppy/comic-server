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

func TestBuildComicInfoXMLKeeping_CarriesUnknownElements(t *testing.T) {
	existing := []byte(`<?xml version="1.0"?>
<ComicInfo>
  <Series>Old</Series>
  <Summary>file-only summary</Summary>
  <Issue>3</Issue>
  <writer>Mairghread Scott</writer>
  <penciller>Someone</penciller>
  <HasBeenRead>True</HasBeenRead>
  <Custom attr="x"><Inner>v</Inner></Custom>
</ComicInfo>`)
	book := &library.ComicBook{Series: "New", Penciller: "Library Artist"}

	data, err := BuildComicInfoXMLKeeping(book, 2, existing)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	for _, want := range []string{"<Issue>3</Issue>", "<writer>Mairghread Scott</writer>", "<HasBeenRead>True</HasBeenRead>", `<Custom attr="x"><Inner>v</Inner></Custom>`, "<Series>New</Series>", "<Penciller>Library Artist</Penciller>"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
	// Schema fields come from the library: an empty one is not kept from the
	// file, and a case-twin of a written field is dropped.
	for _, gone := range []string{"file-only summary", "<penciller>"} {
		if strings.Contains(out, gone) {
			t.Errorf("should not contain %s:\n%s", gone, out)
		}
	}
	if _, ok := ParseComicInfoXML(data); !ok {
		t.Error("output is not valid ComicInfo.xml")
	}
}

func TestBuildComicInfoXMLKeeping_BadOrEmptyExisting(t *testing.T) {
	book := &library.ComicBook{Series: "S"}
	plain, _ := BuildComicInfoXML(book, 1)
	for _, existing := range [][]byte{nil, []byte("<not xml")} {
		got, err := BuildComicInfoXMLKeeping(book, 1, existing)
		if err != nil || string(got) != string(plain) {
			t.Errorf("existing %q: got %s, err %v", existing, got, err)
		}
	}
}

func pagesOf(t *testing.T, data []byte) []comicInfoPageXML {
	t.Helper()
	var ci comicInfoXML
	if err := xml.Unmarshal(data, &ci); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return ci.Pages.Page
}

func TestBuildComicInfoXML_PagesFromLibrary(t *testing.T) {
	book := &library.ComicBook{Series: "S", Pages: []library.ComicPageInfo{
		{Image: 0, Type: library.PageTypeFrontCover, ImageWidth: 1000, ImageHeight: 1500, ImageSize: 12345},
		{Image: 1, Type: library.PageTypeDeleted},
		{Image: 2, Bookmark: "Chapter 2", ImageWidth: 2000, ImageHeight: 1500, Key: "p3.jpg"},
		{Image: 9}, // beyond the archive
	}}
	existing := []byte(`<ComicInfo><Pages><Page Image="2" DoublePage="True"/></Pages></ComicInfo>`)

	// Convert: Deleted page dropped, later pages renumbered, file-only attrs follow the page.
	got := pagesOf(t, mustBuild(t, func() ([]byte, error) { return BuildComicInfoXMLKeeping(book, 2, existing) }))
	if len(got) != 2 {
		t.Fatalf("pages = %+v, want 2 entries", got)
	}
	if got[0].Image != 0 || got[0].Type != library.PageTypeFrontCover || got[0].ImageWidth != 1000 || got[0].ImageSize != 12345 {
		t.Errorf("page 0 = %+v", got[0])
	}
	if got[1].Image != 1 || got[1].Bookmark != "Chapter 2" || got[1].ImageWidth != 2000 || got[1].Key != "p3.jpg" {
		t.Errorf("page 1 = %+v", got[1])
	}
	if len(got[1].Attrs) != 1 || got[1].Attrs[0].Name.Local != "DoublePage" {
		t.Errorf("page 1 attrs = %+v, want DoublePage kept", got[1].Attrs)
	}

	// In place: every page keeps its index, Deleted included.
	got = pagesOf(t, mustBuild(t, func() ([]byte, error) { return buildComicInfoXML(book, 3, nil, existing, false) }))
	if len(got) != 3 || got[1].Type != library.PageTypeDeleted || got[2].Image != 2 {
		t.Errorf("in-place pages = %+v", got)
	}
}

func TestBuildComicInfoXML_PagesFromFileWhenLibraryHasNone(t *testing.T) {
	book := &library.ComicBook{Series: "S"}
	existing := []byte(`<ComicInfo><Pages><Page Image="0" Type="FrontCover" ImageWidth="900"/><Page Image="1" Type="Story" ImageHeight="1400"/></Pages></ComicInfo>`)
	got := pagesOf(t, mustBuild(t, func() ([]byte, error) { return buildComicInfoXML(book, 2, []int{1}, existing, false) }))
	if len(got) != 2 || got[0].ImageWidth != 900 || got[1].ImageHeight != 1400 || got[1].Type != library.PageTypeFrontCover {
		t.Errorf("pages = %+v, want file pages with page 1 marked FrontCover", got)
	}
}

func TestBuildComicInfoXML_NoPageDataKeepsCoverOnlyBlock(t *testing.T) {
	book := &library.ComicBook{Series: "S"}
	got := pagesOf(t, mustBuild(t, func() ([]byte, error) { return buildComicInfoXML(book, 5, []int{4}, nil, false) }))
	if len(got) != 2 || got[0].Image != 0 || got[0].Type != library.PageTypeFrontCover || got[1].Image != 4 || got[1].Type != library.PageTypeFrontCover {
		t.Errorf("pages = %+v, want page 0 and 4 FrontCover", got)
	}
}

func mustBuild(t *testing.T, f func() ([]byte, error)) []byte {
	t.Helper()
	data, err := f()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return data
}
