package cbzconvert

import (
	"encoding/xml"

	"github.com/duckpuppy/comic-server/internal/library"
)

// ComicBook.xml is ComicRackCE's superset of ComicInfo.xml (added in CE
// 0.9.184, PR #261): the book's full library record - read progress, custom
// values, catalog fields, page info - serialized with the same schema as a
// <Book> in ComicDb.xml but with the root element named after the type
// (ComicBook). It lives next to ComicInfo.xml, which always wins on
// shared fields when both are read. Off by default, matching ComicRack's
// own "Embed Library Info" export setting.
const comicBookEntryName = "ComicBook.xml"

// zeroGUID is what ComicRack's XmlSerializer writes for Guid.Empty.
const zeroGUID = "00000000-0000-0000-0000-000000000000"

// Options controls optional Convert behavior.
type Options struct {
	// EmbedComicBook also writes a ComicBook.xml entry (see above).
	EmbedComicBook bool
}

// BuildComicBookXML serializes book as ComicBook.xml bytes for the NEW
// archive, which has pageCount pages after Deleted pages were dropped.
//
// It mirrors ComicBook.Serialize in ComicRackCE: fields that belong to the
// library record or the file rather than the book's content (Id, File, file
// times and size, last-opened list, dirty flags, missing flag, dynamic page
// count) are blanked so they are neither embedded nor overwrite the
// receiving library's own values on read.
//
// Page-indexed data is re-based onto the new archive: the Deleted pages are
// removed from Pages, the remaining Image indexes are renumbered by position,
// and CurrentPage/LastPageRead/PreferredFrontCover are shifted the same way
// (clamped into range, as ComicRack's SetInfo does).
func BuildComicBookXML(book *library.ComicBook, pageCount int) ([]byte, error) {
	return buildComicBookXML(book, pageCount, true)
}

// buildComicBookXML is BuildComicBookXML with control over whether Deleted
// pages are dropped. Conversion drops them from the archive, so the record is
// re-based; an in-place metadata update keeps every page, so it must not be.
func buildComicBookXML(book *library.ComicBook, pageCount int, dropDeleted bool) ([]byte, error) {
	cb := *book

	deleted := map[int]bool{}
	if dropDeleted {
		deleted = deletedPageIndexes(book.Pages)
	}
	remap := func(old int) int {
		n := old
		for d := range deleted {
			if d < old {
				n--
			}
		}
		if n >= pageCount {
			n = pageCount - 1
		}
		if n < 0 {
			n = 0
		}
		return n
	}

	cb.Pages = nil
	for _, p := range book.Pages {
		if deleted[p.Image] {
			continue
		}
		p.Image = remap(p.Image)
		cb.Pages = append(cb.Pages, p)
	}
	cb.PageCount = pageCount
	cb.CurrentPage = remap(book.CurrentPage)
	cb.LastPageRead = remap(book.LastPageRead)
	cb.PreferredFrontCover = remap(book.PreferredFrontCover)

	cb.ID = zeroGUID
	cb.FilePath = ""
	cb.FileModifiedTime = library.ComicTime{}
	cb.FileCreationTime = library.ComicTime{}
	cb.FileSize = 0
	cb.LastOpenedFromListID = ""
	cb.ComicInfoIsDirty = false
	cb.FileIsMissing = false
	cb.NewPages = 0
	cb.EnableDynamicUpdate = true

	out, err := xml.MarshalIndent(&cb, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}
