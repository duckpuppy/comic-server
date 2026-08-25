// Package cbzconvert implements comic-server's port of ComicRack's native
// "Convert to CBZ" feature: repack a comic archive's pages as-is into a new
// CBZ (zip) file and embed a ComicInfo.xml built from the book's current
// library metadata. See comic-server-pkk.2 for the research this is based
// on and comic-server-1up/comic-server-rhe (internal/trash) for the
// safe-replace mechanism this package builds on - this is comic-server's
// first feature that writes to and retires a comic archive file.
package cbzconvert

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/trash"
)

const comicInfoEntryName = "ComicInfo.xml"

// Result reports the outcome of converting one book to CBZ. Convert
// doesn't modify book or touch the library backend - callers apply
// NewFilePath/PageCount to the library record themselves and persist it,
// same division of responsibility as internal/scaninfo.DetectTag.
type Result struct {
	// NewFilePath is book.FilePath with its extension changed to .cbz,
	// in the same path style (separators, root) as the input - i.e. still
	// a RAW library path, not run through resolvePath. Equal to the
	// original FilePath if the source was already a .cbz (repack in
	// place).
	NewFilePath string
	PageCount   int
}

// Convert repacks book's source archive into a new CBZ, embeds a
// ComicInfo.xml built from the book's current metadata, and safely
// retires the original via tr.
//
// resolvePath translates a raw library FilePath into a path this process
// can actually open (see config.Config.ResolveLibraryFilePath); pass nil
// to use raw paths unchanged (the common case where comic-server runs on
// the same host/filesystem that wrote the library).
//
// When the source is already a .cbz (source and target path are
// identical), the old and new content occupy the same path, so this uses
// tr.Replace - the original is quarantined and the new file swapped in as
// one atomic operation. When the extension changes (the common case:
// .cbr/.cb7 -> .cbz), source and target are different paths - ComicRack's
// own export pipeline does the equivalent by writing the new file, then
// separately retiring the old one (see comic-server-pkk.2's research) - so
// this uses tr.WriteNew for the new file followed by tr.Quarantine for the
// old one. If the Quarantine step fails after WriteNew already succeeded,
// Convert removes the newly-written file before returning the error, so a
// partial failure never leaves an orphan file with the library record
// still unchanged.
func Convert(book *library.ComicBook, resolvePath func(string) string, tr *trash.Trash) (Result, error) {
	if resolvePath == nil {
		resolvePath = func(p string) string { return p }
	}

	rawSrc := book.FilePath
	resolvedSrc := resolvePath(rawSrc)

	pages, err := comicvine.ReadAllPages(resolvedSrc)
	if err != nil {
		return Result{}, fmt.Errorf("cbzconvert: read source pages: %w", err)
	}
	pages = dropDeletedPages(pages, book.Pages)
	if len(pages) == 0 {
		return Result{}, fmt.Errorf("cbzconvert: no image pages found in %s", resolvedSrc)
	}

	comicInfoBytes, err := BuildComicInfoXML(book, len(pages))
	if err != nil {
		return Result{}, fmt.Errorf("cbzconvert: build ComicInfo.xml: %w", err)
	}

	rawTarget := changeExt(rawSrc, ".cbz")
	resolvedTarget := resolvePath(rawTarget)

	write := func(tmpPath string) error { return writeCBZ(tmpPath, pages, comicInfoBytes) }
	validate := func(tmpPath string) error { return validateCBZ(tmpPath, len(pages)) }

	if filepath.Clean(resolvedTarget) == filepath.Clean(resolvedSrc) {
		if err := tr.Replace(resolvedTarget, write, validate); err != nil {
			return Result{}, fmt.Errorf("cbzconvert: replace %s: %w", resolvedTarget, err)
		}
	} else {
		if err := tr.WriteNew(resolvedTarget, write, validate); err != nil {
			return Result{}, fmt.Errorf("cbzconvert: write %s: %w", resolvedTarget, err)
		}
		if err := tr.Quarantine(resolvedSrc); err != nil {
			os.Remove(resolvedTarget) // avoid leaving an orphan new file behind
			return Result{}, fmt.Errorf("cbzconvert: quarantine original %s: %w", resolvedSrc, err)
		}
	}

	return Result{NewFilePath: rawTarget, PageCount: len(pages)}, nil
}

// dropDeletedPages removes any page whose library index is marked
// PageTypeDeleted in bookPages, matching ComicRackCE's own default export
// behavior (StorageProvider.GetImages: `if setting.RemovePages &&
// cpi.IsTypeOf(setting.RemovePageFilter) return no bytes for this page` -
// RemovePages/RemovePageFilter default to true/Deleted). pages is assumed
// sorted in the same order as the library's own page indices (both are
// "sorted image order" - the standard assumption ComicPageInfo.Image
// indices are defined against). Remaining pages are implicitly renumbered
// by their new position in the returned slice, matching how ComicRackCE's
// PackedStorageProvider.OnStore assigns a fresh sequential index (num++)
// only to pages actually written.
func dropDeletedPages(pages []comicvine.Page, bookPages []library.ComicPageInfo) []comicvine.Page {
	if len(bookPages) == 0 {
		return pages
	}
	deleted := make(map[int]bool, len(bookPages))
	for _, p := range bookPages {
		if strings.EqualFold(p.Type, library.PageTypeDeleted) {
			deleted[p.Image] = true
		}
	}
	if len(deleted) == 0 {
		return pages
	}

	kept := make([]comicvine.Page, 0, len(pages))
	for i, p := range pages {
		if deleted[i] {
			continue
		}
		kept = append(kept, p)
	}
	return kept
}

// changeExt returns rawPath with its extension replaced by newExt.
// filepath.Ext works correctly even on a foreign-OS raw path (e.g. a
// Windows path read on Linux) as long as it contains no '/' - it looks
// backward from the end for the last '.', which is unaffected by '\'
// separators it doesn't recognize as path boundaries.
func changeExt(rawPath, newExt string) string {
	return strings.TrimSuffix(rawPath, filepath.Ext(rawPath)) + newExt
}

// writeCBZ creates a new zip at tmpPath containing pages (as-is, no
// re-encoding - matches ComicRack's default repack behavior) plus a
// ComicInfo.xml entry.
func writeCBZ(tmpPath string, pages []comicvine.Page, comicInfoBytes []byte) error {
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)

	for _, page := range pages {
		// Images are already compressed formats (jpg/png/gif/webp); Store
		// avoids paying deflate's CPU cost for essentially no size benefit.
		w, err := zw.CreateHeader(&zip.FileHeader{
			Name:   filepath.Base(page.Name),
			Method: zip.Store,
		})
		if err != nil {
			zw.Close()
			return err
		}
		if _, err := w.Write(page.Data); err != nil {
			zw.Close()
			return err
		}
	}

	ciw, err := zw.Create(comicInfoEntryName)
	if err != nil {
		zw.Close()
		return err
	}
	if _, err := ciw.Write(comicInfoBytes); err != nil {
		zw.Close()
		return err
	}

	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}

// validateCBZ sanity-checks a just-written CBZ before internal/trash is
// allowed to let it replace anything: it must open as a valid zip, contain
// exactly wantPages image entries, and have a well-formed ComicInfo.xml
// entry.
func validateCBZ(path string, wantPages int) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("open written zip: %w", err)
	}
	defer zr.Close()

	imageCount := 0
	haveComicInfo := false
	for _, f := range zr.File {
		if strings.EqualFold(f.Name, comicInfoEntryName) {
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("open %s: %w", comicInfoEntryName, err)
			}
			var ci comicInfoXML
			decErr := xml.NewDecoder(rc).Decode(&ci)
			rc.Close()
			if decErr != nil {
				return fmt.Errorf("%s is not well-formed: %w", comicInfoEntryName, decErr)
			}
			haveComicInfo = true
			continue
		}
		if imageExtensions[strings.ToLower(filepath.Ext(f.Name))] {
			imageCount++
		}
	}

	if !haveComicInfo {
		return fmt.Errorf("%s missing from written zip", comicInfoEntryName)
	}
	if imageCount != wantPages {
		return fmt.Errorf("written zip has %d image pages, want %d", imageCount, wantPages)
	}
	return nil
}

var imageExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
}
