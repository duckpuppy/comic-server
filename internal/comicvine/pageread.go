package comicvine

import (
	"archive/zip"
	"fmt"

	"github.com/bodgit/sevenzip"
	rardecode "github.com/nwaples/rardecode/v2"
)

// ListPageNames returns the entry names of a comic archive's image pages in
// page order (the order ReadAllPages and ExtractCover use), without reading
// any page data. Dispatches on the real container like ReadAllPages.
// comic-server-65o8's reader uses it to learn the page count cheaply.
func ListPageNames(path string) ([]string, error) {
	switch dispatchExt(path) {
	case ".cbz", ".zip":
		zr, err := zip.OpenReader(path)
		if err != nil {
			return nil, openFailure("cbz", path, err)
		}
		defer zr.Close()
		var names []string
		for _, f := range sortedImageFiles(zr.File) {
			names = append(names, f.Name)
		}
		return names, nil
	case ".cbr", ".rar":
		files, err := rardecode.List(path)
		if err != nil {
			return nil, openFailure("cbr", path, err)
		}
		var names []string
		for _, f := range sortedRARFiles(files) {
			names = append(names, f.Name)
		}
		return names, nil
	case ".cb7", ".7z":
		rc, err := sevenzip.OpenReader(path)
		if err != nil {
			return nil, openFailure("cb7", path, err)
		}
		defer rc.Close()
		var names []string
		for _, f := range sortedSevenZipFiles(rc.File) {
			names = append(names, f.Name)
		}
		return names, nil
	default:
		return nil, fmt.Errorf("unsupported archive format: %s", path)
	}
}

// ReadPage returns the raw bytes of page n (0-based, page order) of a comic
// archive, reading only that entry.
func ReadPage(path string, n int) ([]byte, error) {
	if n < 0 {
		return nil, fmt.Errorf("page %d out of range", n)
	}
	switch dispatchExt(path) {
	case ".cbz", ".zip":
		zr, err := zip.OpenReader(path)
		if err != nil {
			return nil, openFailure("cbz", path, err)
		}
		defer zr.Close()
		files := sortedImageFiles(zr.File)
		if n >= len(files) {
			return nil, fmt.Errorf("page %d out of range (%d pages)", n, len(files))
		}
		return readZipFile(files[n])
	case ".cbr", ".rar":
		files, err := rardecode.List(path)
		if err != nil {
			return nil, openFailure("cbr", path, err)
		}
		images := sortedRARFiles(files)
		if n >= len(images) {
			return nil, fmt.Errorf("page %d out of range (%d pages)", n, len(images))
		}
		return readRARFile(images[n])
	case ".cb7", ".7z":
		rc, err := sevenzip.OpenReader(path)
		if err != nil {
			return nil, openFailure("cb7", path, err)
		}
		defer rc.Close()
		images := sortedSevenZipFiles(rc.File)
		if n >= len(images) {
			return nil, fmt.Errorf("page %d out of range (%d pages)", n, len(images))
		}
		return readSevenZipFile(images[n])
	default:
		return nil, fmt.Errorf("unsupported archive format: %s", path)
	}
}
