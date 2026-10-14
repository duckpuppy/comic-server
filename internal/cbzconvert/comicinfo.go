package cbzconvert

import (
	"encoding/xml"
	"reflect"
	"strings"

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

	Characters          string `xml:"Characters,omitempty"`
	Teams               string `xml:"Teams,omitempty"`
	Locations           string `xml:"Locations,omitempty"`
	MainCharacterOrTeam string `xml:"MainCharacterOrTeam,omitempty"`

	ScanInformation string `xml:"ScanInformation,omitempty"`
	StoryArc        string `xml:"StoryArc,omitempty"`
	SeriesGroup     string `xml:"SeriesGroup,omitempty"`
	AgeRating       string `xml:"AgeRating,omitempty"`

	Rating          float64 `xml:"Rating,omitempty"`
	CommunityRating float64 `xml:"CommunityRating,omitempty"`
	Review          string  `xml:"Review,omitempty"`

	Pages struct {
		Page []comicInfoPageXML `xml:"Page"`
	} `xml:"Pages"`

	// Extra holds elements outside the schema above (e.g. a tagger's
	// lowercase <writer> or <HasBeenRead>). The library cannot store them,
	// so a write copies them over from the file's existing ComicInfo.xml
	// instead of dropping them (comic-server-y95y).
	Extra []extraElement `xml:",any"`
}

// extraElement is one unknown ComicInfo.xml element, kept verbatim.
type extraElement struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Inner   string     `xml:",innerxml"`
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
	return buildComicInfoXML(book, pageCount, nil, nil)
}

// BuildComicInfoXMLKeeping is BuildComicInfoXML for a file that already has a
// ComicInfo.xml (existing): elements outside the schema are carried over, so
// a write never drops data the library cannot hold. Schema fields always come
// from the library, so a field cleared in the library is cleared in the file.
// An unknown element whose name matches a written schema field apart from
// case (<writer> next to <Writer>) is dropped, so readers never see two
// conflicting values. existing may be nil or invalid XML; nothing is kept then.
func BuildComicInfoXMLKeeping(book *library.ComicBook, pageCount int, existing []byte) ([]byte, error) {
	return buildComicInfoXML(book, pageCount, nil, existing)
}

// buildComicInfoXML is BuildComicInfoXMLKeeping plus extra FrontCover page
// indexes (comic-server-0u48's appended variant covers) written into the Pages
// block after the usual page-0 cover.
func buildComicInfoXML(book *library.ComicBook, pageCount int, extraFrontCovers []int, existing []byte) ([]byte, error) {
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

		Characters:          book.Characters,
		Teams:               book.Teams,
		Locations:           book.Locations,
		MainCharacterOrTeam: book.MainCharacterOrTeam,

		ScanInformation: book.ScanInformation,
		StoryArc:        book.StoryArc,
		SeriesGroup:     book.SeriesGroup,
		AgeRating:       book.AgeRating,

		Rating:          book.Rating,
		CommunityRating: book.CommunityRating,
		Review:          book.Review,
	}

	if pageCount > 0 {
		ci.Pages.Page = []comicInfoPageXML{{Image: 0, Type: library.PageTypeFrontCover}}
		for _, idx := range extraFrontCovers {
			ci.Pages.Page = append(ci.Pages.Page, comicInfoPageXML{Image: idx, Type: library.PageTypeFrontCover})
		}
	}
	ci.Extra = keptExtras(&ci, existing)

	out, err := xml.MarshalIndent(ci, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}

// keptExtras returns existing's unknown elements, minus any whose name equals
// a schema field ci writes, ignoring case.
func keptExtras(ci *comicInfoXML, existing []byte) []extraElement {
	if len(existing) == 0 {
		return nil
	}
	var old comicInfoXML
	if err := xml.Unmarshal(existing, &old); err != nil || len(old.Extra) == 0 {
		return nil
	}
	written := map[string]bool{}
	v := reflect.ValueOf(*ci)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("xml"), ",")
		if name == "" || name == "-" || v.Field(i).IsZero() {
			continue
		}
		written[strings.ToLower(name)] = true
	}
	var out []extraElement
	for _, e := range old.Extra {
		if !written[strings.ToLower(e.XMLName.Local)] {
			out = append(out, e)
		}
	}
	return out
}

// ParseComicInfoXML is BuildComicInfoXML's inverse: unmarshals an archive's
// embedded ComicInfo.xml bytes into a partial *library.ComicBook (every
// field ComicInfo.xml can carry; FilePath, ID, and anything else specific
// to a library record are left for the caller to fill in). Returns
// ok=false if data isn't valid ComicInfo.xml - used by the watch-folder
// "start processing" action (comic-server-chh) to seed a new book from
// the archive's own tags, which take priority over a filename-only guess.
func ParseComicInfoXML(data []byte) (*library.ComicBook, bool) {
	var ci comicInfoXML
	if err := xml.Unmarshal(data, &ci); err != nil {
		return nil, false
	}

	book := &library.ComicBook{
		Title:           ci.Title,
		Series:          ci.Series,
		Number:          ci.Number,
		Count:           ci.Count,
		Volume:          ci.Volume,
		AlternateSeries: ci.AlternateSeries,
		AlternateNumber: ci.AlternateNumber,
		AlternateCount:  ci.AlternateCount,
		Summary:         ci.Summary,
		Notes:           ci.Notes,
		Year:            ci.Year,
		Month:           ci.Month,
		Day:             ci.Day,

		Writer:      ci.Writer,
		Penciller:   ci.Penciller,
		Inker:       ci.Inker,
		Colorist:    ci.Colorist,
		Letterer:    ci.Letterer,
		CoverArtist: ci.CoverArtist,
		Editor:      ci.Editor,
		Translator:  ci.Translator,

		Publisher:   ci.Publisher,
		Imprint:     ci.Imprint,
		Genre:       ci.Genre,
		Tags:        ci.Tags,
		Web:         ci.Web,
		PageCount:   ci.PageCount,
		LanguageISO: ci.LanguageISO,
		Format:      ci.Format,

		BlackAndWhite: ci.BlackAndWhite,
		Manga:         ci.Manga,

		Characters:          ci.Characters,
		Teams:               ci.Teams,
		Locations:           ci.Locations,
		MainCharacterOrTeam: ci.MainCharacterOrTeam,

		ScanInformation: ci.ScanInformation,
		StoryArc:        ci.StoryArc,
		SeriesGroup:     ci.SeriesGroup,
		AgeRating:       ci.AgeRating,

		Rating:          ci.Rating,
		CommunityRating: ci.CommunityRating,
		Review:          ci.Review,
	}
	return book, true
}
