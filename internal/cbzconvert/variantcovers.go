package cbzconvert

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/trash"
)

// Variant covers (comic-server-0u48): append already-downloaded cover images
// to an existing CBZ as extra pages marked FrontCover.
//
// Design record:
//   - Appended, not inserted after the cover. Inserting would renumber every
//     later page and break bookmarks, reading progress and page-indexed
//     library data; appending leaves all existing page indexes untouched.
//   - Entry names are zzzz-cv-variant-<imageID>.<ext>: deterministic (so the
//     same image is recognised on a re-run) and sorting after typical page
//     names, because readers order pages by entry name.
//   - Page order is never re-sorted by Type (see the cbzconvert notes), so a
//     FrontCover page at the end is just a page that happens to be tagged.

// CoverImage is one downloaded image to append.
type CoverImage struct {
	ID   int // ComicVine associated-image id
	Ext  string
	Data []byte
}

// VariantEntryName is the archive entry name used for a cover image.
func VariantEntryName(id int, ext string) string {
	return fmt.Sprintf("zzzz-cv-variant-%d%s", id, strings.ToLower(ext))
}

// ErrNothingToAdd means every image was already present in the archive.
var ErrNothingToAdd = errors.New("cbzconvert: no new cover images to add")

// CoversResult describes a successful AddVariantCovers.
type CoversResult struct {
	PageCount  int                     // total image pages after the write
	AddedIDs   []int                   // image ids actually appended
	NewPages   []library.ComicPageInfo // FrontCover page records for the library
	FirstIndex int                     // page index of the first appended cover
}

// AddVariantCovers appends covers to book's CBZ. Images whose entry name is
// already in the archive are skipped, so a repeat call is a no-op that
// returns ErrNothingToAdd. Existing entries are raw-copied; ComicInfo.xml
// (and ComicBook.xml with opts.EmbedComicBook) are rewritten so the new pages
// are tagged FrontCover. The original goes through tr.Replace. Like Convert
// it never touches the library record.
func AddVariantCovers(book *library.ComicBook, resolvePath func(string) string, tr *trash.Trash, covers []CoverImage, opts Options) (CoversResult, error) {
	if resolvePath == nil {
		resolvePath = func(p string) string { return p }
	}
	if !isZipFile(book.FilePath) {
		return CoversResult{}, ErrNotCBZ
	}
	target := resolvePath(book.FilePath)

	existing, err := zipEntryNames(target)
	if err != nil {
		return CoversResult{}, fmt.Errorf("cbzconvert: read %s: %w", target, err)
	}
	imageCount, err := countImageEntries(target)
	if err != nil {
		return CoversResult{}, fmt.Errorf("cbzconvert: read %s: %w", target, err)
	}
	if imageCount == 0 {
		return CoversResult{}, fmt.Errorf("cbzconvert: no image pages found in %s", target)
	}

	var res CoversResult
	res.FirstIndex = imageCount
	var extra []metadataEntry
	for _, c := range covers {
		name := VariantEntryName(c.ID, c.Ext)
		if existing[strings.ToLower(name)] || len(c.Data) == 0 {
			continue
		}
		existing[strings.ToLower(name)] = true
		idx := imageCount + len(extra)
		extra = append(extra, metadataEntry{name: name, data: c.Data, store: true})
		res.AddedIDs = append(res.AddedIDs, c.ID)
		res.NewPages = append(res.NewPages, library.ComicPageInfo{Image: idx, Type: library.PageTypeFrontCover, ImageSize: int64(len(c.Data))})
	}
	if len(extra) == 0 {
		return CoversResult{}, ErrNothingToAdd
	}
	res.PageCount = imageCount + len(extra)

	frontCovers := make([]int, 0, len(res.NewPages))
	for _, p := range res.NewPages {
		frontCovers = append(frontCovers, p.Image)
	}
	comicInfo, err := buildComicInfoXML(book, res.PageCount, frontCovers)
	if err != nil {
		return CoversResult{}, fmt.Errorf("cbzconvert: build ComicInfo.xml: %w", err)
	}
	entries := append(extra, metadataEntry{name: comicInfoEntryName, data: comicInfo})
	replace := map[string]bool{strings.ToLower(comicInfoEntryName): true}

	if opts.EmbedComicBook {
		updated := *book
		updated.PageCount = res.PageCount
		updated.Pages = append(append([]library.ComicPageInfo(nil), book.Pages...), res.NewPages...)
		comicBook, err := buildComicBookXML(&updated, res.PageCount, false)
		if err != nil {
			return CoversResult{}, fmt.Errorf("cbzconvert: build ComicBook.xml: %w", err)
		}
		entries = append(entries, metadataEntry{name: comicBookEntryName, data: comicBook})
		replace[strings.ToLower(comicBookEntryName)] = true
	}

	write := func(tmpPath string) error { return copyZipWithMetadata(target, tmpPath, replace, entries) }
	validate := func(tmpPath string) error { return validateCBZ(tmpPath, res.PageCount, opts.EmbedComicBook) }
	if err := tr.Replace(target, write, validate); err != nil {
		return CoversResult{}, fmt.Errorf("cbzconvert: add covers to %s: %w", target, err)
	}
	return res, nil
}

// zipEntryNames returns the lower-cased base names of every entry.
func zipEntryNames(zipPath string) (map[string]bool, error) {
	if _, err := os.Stat(zipPath); err != nil {
		return nil, err
	}
	names := map[string]bool{}
	zr, err := openZip(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	for _, f := range zr.File {
		names[strings.ToLower(path.Base(f.Name))] = true
	}
	return names, nil
}
