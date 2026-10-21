package cbzconvert

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/library"
)

// ErrExportExists is returned by ExportCopy when destPath is already there and
// overwrite is off.
var ErrExportExists = errors.New("cbzconvert: export target already exists")

// ExportCopy writes a CBZ copy of book to destPath (tracker #7, ComicRack's
// Export Books). Unlike Convert it never touches the source: no trash, no
// library change. Pages are raw-copied (Deleted pages dropped), with a fresh
// ComicInfo.xml (and ComicBook.xml when opts.EmbedComicBook). The copy is built
// in a temp file next to destPath, validated, then renamed into place.
//
// resolvedSrc is the readable path of the source archive. ExportCopy refuses a
// destPath that is the source itself. With overwrite off an existing target
// gives ErrExportExists; with it on, the earlier export is replaced.
//
// pageOpts can re-encode or resize the pages (see PageOptions). The zero value
// copies them byte for byte. When it is active the book's per-page sizes in the
// new ComicInfo.xml follow the pages, and the source's own ComicInfo.xml is not
// merged in (its page sizes would be stale, and its unknown elements are not
// carried over into a re-encoded copy).
func ExportCopy(book *library.ComicBook, resolvedSrc, destPath string, opts Options, pageOpts PageOptions, overwrite bool) (int, error) {
	if err := pageOpts.Validate(); err != nil {
		return 0, fmt.Errorf("cbzconvert: page options: %w", err)
	}
	if sameFile(resolvedSrc, destPath) {
		return 0, fmt.Errorf("cbzconvert: export target is the source file")
	}
	if !overwrite {
		if _, err := os.Lstat(destPath); err == nil {
			return 0, ErrExportExists
		}
	}

	pages, err := comicvine.ReadAllPages(resolvedSrc)
	if err != nil {
		return 0, fmt.Errorf("cbzconvert: read source pages: %w", err)
	}
	deleted := deletedPageIndexes(book.Pages)
	var origIndex []int // library page index of each kept page
	for i := range pages {
		if !deleted[i] {
			origIndex = append(origIndex, i)
		}
	}
	pages = dropDeletedPages(pages, book.Pages)
	if len(pages) == 0 {
		return 0, fmt.Errorf("cbzconvert: no image pages found in %s", resolvedSrc)
	}

	var existingInfo []byte
	if pageOpts.Active() {
		var stats []PageStat
		pages, stats, err = TransformPages(pages, pageOpts)
		if err != nil {
			return 0, fmt.Errorf("cbzconvert: %w", err)
		}
		// Work on a copy: book.Pages is shared with the live library.
		b := *book
		b.Pages = append([]library.ComicPageInfo(nil), book.Pages...)
		for k, st := range stats {
			if !st.Changed {
				continue
			}
			for j := range b.Pages {
				if b.Pages[j].Image == origIndex[k] {
					b.Pages[j].ImageSize, b.Pages[j].ImageWidth, b.Pages[j].ImageHeight = st.Size, st.Width, st.Height
				}
			}
		}
		book = &b
	} else {
		existingInfo, _ = comicvine.ReadComicInfoXMLBytes(resolvedSrc)
	}
	info, err := BuildComicInfoXMLKeeping(book, len(pages), existingInfo)
	if err != nil {
		return 0, fmt.Errorf("cbzconvert: build ComicInfo.xml: %w", err)
	}
	metadata := []metadataEntry{{name: comicInfoEntryName, data: info}}
	if opts.EmbedComicBook {
		cb, err := BuildComicBookXML(book, len(pages))
		if err != nil {
			return 0, fmt.Errorf("cbzconvert: build ComicBook.xml: %w", err)
		}
		metadata = append(metadata, metadataEntry{name: comicBookEntryName, data: cb})
	}

	tmp, err := os.CreateTemp(filepath.Dir(destPath), ".export-*.tmp")
	if err != nil {
		return 0, fmt.Errorf("cbzconvert: create temp export file: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	// tmpPath is our own half-written scratch file, never a comic.
	fail := func(err error) (int, error) { os.Remove(tmpPath); return 0, err }

	if err := writeCBZ(tmpPath, pages, metadata); err != nil {
		return fail(fmt.Errorf("cbzconvert: write export: %w", err))
	}
	if err := validateCBZ(tmpPath, len(pages), opts.EmbedComicBook); err != nil {
		return fail(fmt.Errorf("cbzconvert: validate export: %w", err))
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return fail(err)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fail(fmt.Errorf("cbzconvert: move export into place: %w", err))
	}
	return len(pages), nil
}

func sameFile(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ai, err1 := os.Stat(a)
	bi, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(ai, bi)
}
