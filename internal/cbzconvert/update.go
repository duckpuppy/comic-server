package cbzconvert

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/trash"
)

// WrittenHashKey is the custom value that records a fingerprint of the
// metadata last embedded in a book's CBZ. The "comic_server_" prefix marks
// comic-server's own bookkeeping keys (see also the workflow stage key); they
// are excluded from the fingerprint so recording it never makes a book stale.
const WrittenHashKey = "comic_server_written_hash"

const internalKeyPrefix = "comic_server_"

// ErrNotCBZ is returned when a book's file isn't a CBZ/ZIP archive; only
// those can be updated in place.
var ErrNotCBZ = errors.New("cbzconvert: file is not a CBZ/ZIP archive")

// IsZipPath reports whether p has an extension UpdateMetadata can rewrite.
func IsZipPath(p string) bool { return isZipFile(p) }

func isZipFile(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".cbz", ".zip":
		return true
	}
	return false
}

// Fingerprint hashes the book metadata that would be embedded in its
// archive. With embedComicBook it also covers the ComicBook-only data that
// ComicRackCE tracks with ComicBookIsDirty (released time, series-complete,
// catalog fields, custom values) - but not reading progress, which changes on
// every read and which ComicRack does not track either.
func Fingerprint(book *library.ComicBook, embedComicBook bool) string {
	h := sha256.New()
	// pageCount 0: the page count of the file is not part of the metadata.
	if data, err := BuildComicInfoXML(book, 0); err == nil {
		h.Write(data)
	}
	if embedComicBook {
		fmt.Fprintf(h, "|%v|%s|%f|%s|%s|%s|%s|%s|%s|%s|%s|%s|",
			book.ReleasedTime.Time, book.SeriesComplete, book.BookPrice,
			book.BookAge, book.BookCondition, book.BookStore, book.BookOwner,
			book.BookCollectionStatus, book.BookNotes, book.BookLocation,
			book.ISBN, externalCustomValues(book.CustomValuesStore))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// externalCustomValues returns store without comic-server's own bookkeeping
// pairs.
func externalCustomValues(store string) string {
	var kept []string
	for _, pair := range strings.Split(store, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" || strings.HasPrefix(pair, internalKeyPrefix) {
			continue
		}
		kept = append(kept, pair)
	}
	return strings.Join(kept, ",")
}

// IsStale reports whether book's CBZ is out of date relative to the library
// record: ComicRack's own dirty flags are set, or a fingerprint recorded at the
// last write no longer matches the current metadata. A book comic-server has
// never written (no recorded fingerprint) is not considered stale unless a
// dirty flag says so - there is no baseline to compare against.
func IsStale(book *library.ComicBook, embedComicBook bool) bool {
	if book.FilePath == "" || !isZipFile(book.FilePath) {
		return false
	}
	if book.ComicInfoIsDirty || (embedComicBook && book.ComicBookIsDirty) {
		return true
	}
	recorded, ok := library.GetCustomValue(book.CustomValuesStore, WrittenHashKey)
	if !ok {
		return false
	}
	return recorded != Fingerprint(book, embedComicBook)
}

// MarkWritten records that book's archive now carries its current metadata:
// stores the fingerprint and clears the dirty flags this write covers.
func MarkWritten(book *library.ComicBook, embedComicBook bool) {
	book.ComicInfoIsDirty = false
	if embedComicBook {
		book.ComicBookIsDirty = false
	}
	book.CustomValuesStore = library.SetCustomValue(book.CustomValuesStore, WrittenHashKey, Fingerprint(book, embedComicBook))
}

// UpdateMetadata rewrites book's CBZ so its embedded ComicInfo.xml (and
// ComicBook.xml with opts.EmbedComicBook) matches the library record. Page
// entries are copied byte-for-byte without recompression and nothing is
// dropped or reordered; only the metadata entries change. The original is
// retired through tr, same as Convert. Like Convert it never touches the
// library record - callers apply MarkWritten and persist.
func UpdateMetadata(book *library.ComicBook, resolvePath func(string) string, tr *trash.Trash, opts Options) (Result, error) {
	if resolvePath == nil {
		resolvePath = func(p string) string { return p }
	}
	if !isZipFile(book.FilePath) {
		return Result{}, ErrNotCBZ
	}
	target := resolvePath(book.FilePath)

	imageCount, err := countImageEntries(target)
	if err != nil {
		return Result{}, fmt.Errorf("cbzconvert: read %s: %w", target, err)
	}
	if imageCount == 0 {
		return Result{}, fmt.Errorf("cbzconvert: no image pages found in %s", target)
	}
	// The library's own page count wins when known: it can count formats the
	// validation's image-extension list doesn't.
	pageCount := book.PageCount
	if pageCount <= 0 {
		pageCount = imageCount
	}

	comicInfo, err := BuildComicInfoXML(book, pageCount)
	if err != nil {
		return Result{}, fmt.Errorf("cbzconvert: build ComicInfo.xml: %w", err)
	}
	metadata := []metadataEntry{{name: comicInfoEntryName, data: comicInfo}}
	replace := map[string]bool{strings.ToLower(comicInfoEntryName): true}
	if opts.EmbedComicBook {
		comicBook, err := buildComicBookXML(book, pageCount, false)
		if err != nil {
			return Result{}, fmt.Errorf("cbzconvert: build ComicBook.xml: %w", err)
		}
		metadata = append(metadata, metadataEntry{name: comicBookEntryName, data: comicBook})
		replace[strings.ToLower(comicBookEntryName)] = true
	}

	write := func(tmpPath string) error { return copyZipWithMetadata(target, tmpPath, replace, metadata) }
	validate := func(tmpPath string) error { return validateCBZ(tmpPath, imageCount, opts.EmbedComicBook) }
	if err := tr.Replace(target, write, validate); err != nil {
		return Result{}, fmt.Errorf("cbzconvert: update %s: %w", target, err)
	}
	return Result{NewFilePath: book.FilePath, PageCount: pageCount}, nil
}

func countImageEntries(zipPath string) (int, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, err
	}
	defer zr.Close()
	n := 0
	for _, f := range zr.File {
		if imageExtensions[strings.ToLower(filepath.Ext(f.Name))] {
			n++
		}
	}
	return n, nil
}

// copyZipWithMetadata writes dst as a copy of src with every entry whose base
// name is in replace removed and the metadata entries appended. Surviving
// entries are copied raw (no decompress/recompress).
func copyZipWithMetadata(src, dst string, replace map[string]bool, metadata []metadataEntry) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	for _, f := range zr.File {
		if replace[strings.ToLower(path.Base(f.Name))] {
			continue
		}
		if err := zw.Copy(f); err != nil {
			zw.Close()
			return err
		}
	}

	now := time.Now()
	for _, m := range metadata {
		method := zip.Deflate
		if m.store {
			method = zip.Store // already-compressed image data
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: m.name, Method: method, Modified: now})
		if err != nil {
			zw.Close()
			return err
		}
		if _, err := w.Write(m.data); err != nil {
			zw.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return out.Close()
}

func openZip(p string) (*zip.ReadCloser, error) { return zip.OpenReader(p) }
