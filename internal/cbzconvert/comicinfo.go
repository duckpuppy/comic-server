package cbzconvert

import (
	"encoding/xml"
	"reflect"
	"sort"
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
	Image       int    `xml:"Image,attr"`
	Type        string `xml:"Type,attr,omitempty"`
	Bookmark    string `xml:"Bookmark,attr,omitempty"`
	ImageSize   int64  `xml:"ImageSize,attr,omitempty"`
	ImageWidth  int    `xml:"ImageWidth,attr,omitempty"`
	ImageHeight int    `xml:"ImageHeight,attr,omitempty"`
	Key         string `xml:"Key,attr,omitempty"`
	// Attrs holds page attributes the library does not store (another
	// tagger's DoublePage, ComicRack's Rotation/PagePosition), kept from the
	// file's existing ComicInfo.xml.
	Attrs []xml.Attr `xml:",any,attr"`
}

// BuildComicInfoXML serializes book's current library metadata as
// ComicInfo.xml bytes, with PageCount and the Pages block reflecting the
// NEW archive's page count (pageCount) rather than whatever book.PageCount
// happened to say before conversion - the two can differ if the source
// archive's actual image count didn't match the library record. The Pages
// block is built by comicInfoPages (comic-server-mgac): one entry per page the
// library knows, as ComicRackCE's PackedStorageProvider writes it, with
// Deleted pages dropped and the rest renumbered, since Convert has already
// dropped them from pageCount (see convert.go).
func BuildComicInfoXML(book *library.ComicBook, pageCount int) ([]byte, error) {
	return buildComicInfoXML(book, pageCount, nil, nil, true)
}

// BuildComicInfoXMLKeeping is BuildComicInfoXML for a file that already has a
// ComicInfo.xml (existing): elements outside the schema are carried over, so
// a write never drops data the library cannot hold. Schema fields always come
// from the library, so a field cleared in the library is cleared in the file.
// An unknown element whose name matches a written schema field apart from
// case (<writer> next to <Writer>) is dropped, so readers never see two
// conflicting values. existing may be nil or invalid XML; nothing is kept then.
func BuildComicInfoXMLKeeping(book *library.ComicBook, pageCount int, existing []byte) ([]byte, error) {
	return buildComicInfoXML(book, pageCount, nil, existing, true)
}

// buildComicInfoXML is BuildComicInfoXMLKeeping plus extra FrontCover page
// indexes (comic-server-0u48's appended variant covers) and control over
// whether Deleted pages are dropped from the Pages block: Convert drops them
// from the archive, an in-place write (Write Metadata, Variant covers) keeps
// every page, so its indexes must not move.
func buildComicInfoXML(book *library.ComicBook, pageCount int, extraFrontCovers []int, existing []byte, dropDeleted bool) ([]byte, error) {
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

	var old *comicInfoXML
	if len(existing) > 0 {
		var parsed comicInfoXML
		if xml.Unmarshal(existing, &parsed) == nil {
			old = &parsed
		}
	}
	var oldPages []comicInfoPageXML
	if old != nil {
		oldPages = old.Pages.Page
	}
	ci.Pages.Page = comicInfoPages(book.Pages, oldPages, pageCount, extraFrontCovers, dropDeleted)
	ci.Extra = keptExtras(&ci, old)

	out, err := xml.MarshalIndent(ci, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}

// keptExtras returns existing's unknown elements, minus any whose name equals
// a schema field ci writes, ignoring case.
func keptExtras(ci *comicInfoXML, old *comicInfoXML) []extraElement {
	if old == nil || len(old.Extra) == 0 {
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

// comicInfoPages builds the Pages block for an archive of pageCount pages
// (comic-server-mgac). The library's page info is the source, like
// ComicRackCE's export; when the library has none, the file's own Pages block
// is kept. Attributes the library does not store are copied from the file's
// entry for the same page. With dropDeleted, Deleted pages are left out and the
// rest renumbered by position. extraFrontCovers are marked FrontCover. With no
// page data at all, the block is the page-0 FrontCover entry written before.
// Entries outside the archive are dropped; none are written when pageCount is
// 0 (the fingerprint build).
func comicInfoPages(libPages []library.ComicPageInfo, filePages []comicInfoPageXML, pageCount int, extraFrontCovers []int, dropDeleted bool) []comicInfoPageXML {
	if pageCount <= 0 {
		return nil
	}
	fileByIndex := make(map[int]comicInfoPageXML, len(filePages))
	for _, p := range filePages {
		if _, dup := fileByIndex[p.Image]; !dup {
			fileByIndex[p.Image] = p
		}
	}

	var src []comicInfoPageXML
	if len(libPages) > 0 {
		for _, p := range libPages {
			src = append(src, comicInfoPageXML{
				Image: p.Image, Type: p.Type, Bookmark: p.Bookmark, ImageSize: p.ImageSize,
				ImageWidth: p.ImageWidth, ImageHeight: p.ImageHeight, Key: p.Key,
				Attrs: fileByIndex[p.Image].Attrs,
			})
		}
	} else {
		src = append(src, filePages...)
	}

	deleted := map[int]bool{}
	if dropDeleted {
		for _, p := range src {
			if p.Type == library.PageTypeDeleted {
				deleted[p.Image] = true
			}
		}
	}
	byIndex := map[int]comicInfoPageXML{}
	for _, p := range src {
		if deleted[p.Image] {
			continue
		}
		n := p.Image
		for d := range deleted {
			if d < p.Image {
				n--
			}
		}
		if n < 0 || n >= pageCount {
			continue
		}
		if _, dup := byIndex[n]; dup {
			continue
		}
		p.Image = n
		byIndex[n] = p
	}

	if len(byIndex) == 0 {
		byIndex[0] = comicInfoPageXML{Image: 0, Type: library.PageTypeFrontCover}
	}
	for _, idx := range extraFrontCovers {
		if idx < 0 || idx >= pageCount {
			continue
		}
		p := byIndex[idx]
		p.Image = idx
		p.Type = library.PageTypeFrontCover
		byIndex[idx] = p
	}

	out := make([]comicInfoPageXML, 0, len(byIndex))
	for _, p := range byIndex {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Image < out[j].Image })
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
