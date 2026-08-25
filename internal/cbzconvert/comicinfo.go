package cbzconvert

import (
	"encoding/xml"

	"github.com/duckpuppy/comic-server/internal/library"
)

// comicInfoXML mirrors the standard ComicInfo.xml v2 schema (the format
// ComicRack, ComicTagger, and most other comic readers/taggers read) -
// deliberately a separate type from library.ComicBook, since ComicDb.xml
// (what ComicBook's own xml tags serialize) and ComicInfo.xml are
// different document schemas that happen to share many field names but
// aren't interchangeable (see ComicRackCE's own PackedStorageProvider,
// which explicitly forces its export type back to a plain ComicInfo before
// serializing, for the same reason - see comic-server-pkk.2's research).
type comicInfoXML struct {
	XMLName xml.Name `xml:"ComicInfo"`

	Title           string `xml:"Title,omitempty"`
	Series          string `xml:"Series,omitempty"`
	Number          string `xml:"Number,omitempty"`
	Count           int    `xml:"Count,omitempty"`
	Volume          int    `xml:"Volume,omitempty"`
	AlternateSeries string `xml:"AlternateSeries,omitempty"`
	AlternateNumber string `xml:"AlternateNumber,omitempty"`
	AlternateCount  int    `xml:"AlternateCount,omitempty"`
	Summary         string `xml:"Summary,omitempty"`
	Notes           string `xml:"Notes,omitempty"`
	Year            int    `xml:"Year,omitempty"`
	Month           int    `xml:"Month,omitempty"`
	Day             int    `xml:"Day,omitempty"`

	Writer      string `xml:"Writer,omitempty"`
	Penciller   string `xml:"Penciller,omitempty"`
	Inker       string `xml:"Inker,omitempty"`
	Colorist    string `xml:"Colorist,omitempty"`
	Letterer    string `xml:"Letterer,omitempty"`
	CoverArtist string `xml:"CoverArtist,omitempty"`
	Editor      string `xml:"Editor,omitempty"`
	Translator  string `xml:"Translator,omitempty"`

	Publisher   string `xml:"Publisher,omitempty"`
	Imprint     string `xml:"Imprint,omitempty"`
	Genre       string `xml:"Genre,omitempty"`
	Tags        string `xml:"Tags,omitempty"`
	Web         string `xml:"Web,omitempty"`
	PageCount   int    `xml:"PageCount,omitempty"`
	LanguageISO string `xml:"LanguageISO,omitempty"`
	Format      string `xml:"Format,omitempty"`

	BlackAndWhite string `xml:"BlackAndWhite,omitempty"`
	Manga         string `xml:"Manga,omitempty"`

	Characters string `xml:"Characters,omitempty"`
	Teams      string `xml:"Teams,omitempty"`
	Locations  string `xml:"Locations,omitempty"`

	ScanInformation string `xml:"ScanInformation,omitempty"`
	StoryArc        string `xml:"StoryArc,omitempty"`
	SeriesGroup     string `xml:"SeriesGroup,omitempty"`
	AgeRating       string `xml:"AgeRating,omitempty"`

	CommunityRating float64 `xml:"CommunityRating,omitempty"`

	Pages struct {
		Page []comicInfoPageXML `xml:"Page"`
	} `xml:"Pages"`
}

type comicInfoPageXML struct {
	Image int    `xml:"Image,attr"`
	Type  string `xml:"Type,attr,omitempty"`
}

// BuildComicInfoXML serializes book's current library metadata as
// ComicInfo.xml bytes, with PageCount and the Pages block reflecting the
// NEW archive's page count (pageCount) rather than whatever book.PageCount
// happened to say before conversion - the two can differ if the source
// archive's actual image count didn't match the library record. Page-level
// per-image metadata (bookmarks, per-page Type beyond the front cover) is
// intentionally not carried over - out of scope per comic-server-43b
// (matches ComicRack's own default EmbedComicInfo behavior, which embeds
// current book metadata, not a full page-info dump - that's the separate,
// off-by-default EmbedComicBook). Deleted pages are already excluded from
// pageCount/pages by dropDeletedPages before this is called - see
// convert.go.
func BuildComicInfoXML(book *library.ComicBook, pageCount int) ([]byte, error) {
	ci := comicInfoXML{
		Title:           book.Title,
		Series:          book.Series,
		Number:          book.Number,
		Count:           book.Count,
		Volume:          book.Volume,
		AlternateSeries: book.AlternateSeries,
		AlternateNumber: book.AlternateNumber,
		AlternateCount:  book.AlternateCount,
		Summary:         book.Summary,
		Notes:           book.Notes,
		Year:            book.Year,
		Month:           book.Month,
		Day:             book.Day,

		Writer:      book.Writer,
		Penciller:   book.Penciller,
		Inker:       book.Inker,
		Colorist:    book.Colorist,
		Letterer:    book.Letterer,
		CoverArtist: book.CoverArtist,
		Editor:      book.Editor,
		Translator:  book.Translator,

		Publisher:   book.Publisher,
		Imprint:     book.Imprint,
		Genre:       book.Genre,
		Tags:        book.Tags,
		Web:         book.Web,
		PageCount:   pageCount,
		LanguageISO: book.LanguageISO,
		Format:      book.Format,

		BlackAndWhite: book.BlackAndWhite,
		Manga:         book.Manga,

		Characters: book.Characters,
		Teams:      book.Teams,
		Locations:  book.Locations,

		ScanInformation: book.ScanInformation,
		StoryArc:        book.StoryArc,
		SeriesGroup:     book.SeriesGroup,
		AgeRating:       book.AgeRating,

		CommunityRating: book.CommunityRating,
	}

	if pageCount > 0 {
		ci.Pages.Page = []comicInfoPageXML{{Image: 0, Type: library.PageTypeFrontCover}}
	}

	out, err := xml.MarshalIndent(ci, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}
