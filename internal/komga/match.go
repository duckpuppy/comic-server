package komga

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// TranslatePath converts a comic-server library path (ComicBook.FilePath,
// using whatever separator style the ComicRack host's OS produced) into the
// equivalent path as Komga sees it, by swapping localRoot for remoteRoot -
// the same approach as the *Arr apps' Remote Path Mapping. Directory
// structure below the root is assumed identical between the two.
//
// The localRoot prefix match is case-insensitive (Windows paths typically
// are), but everything after the root is preserved verbatim, since Komga's
// filesystem is usually Linux and case-sensitive.
func TranslatePath(localRoot, remoteRoot, localPath string) (string, error) {
	normPath := normalizeSlashes(localPath)
	normRoot := strings.TrimSuffix(normalizeSlashes(localRoot), "/")

	if len(normPath) < len(normRoot) || !strings.EqualFold(normPath[:len(normRoot)], normRoot) {
		return "", fmt.Errorf("path %q is not rooted at local_root %q", localPath, localRoot)
	}

	suffix := strings.TrimPrefix(normPath[len(normRoot):], "/")
	remote := strings.TrimSuffix(normalizeSlashes(remoteRoot), "/")
	if suffix == "" {
		return remote, nil
	}
	return remote + "/" + suffix, nil
}

func normalizeSlashes(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

// Index is a snapshot of Komga's library keyed by file path, used to
// resolve comic-server books/series to their Komga IDs. Build once and
// reuse across multiple list pushes; rebuild periodically, since it drifts
// as Komga's own library scans run independently of comic-server's.
type Index struct {
	seriesByPath map[string]string // Komga path -> series ID
	booksByPath  map[string]string // Komga path -> book ID
}

// BuildIndex fetches every series and book from Komga and indexes them by
// path. For a library of comic-server's scale (tens of thousands of books)
// this is a slow, multi-page fetch - call it on its own schedule, not on
// every list push.
func BuildIndex(ctx context.Context, c *Client) (*Index, error) {
	series, err := c.ListAllSeries(ctx)
	if err != nil {
		return nil, fmt.Errorf("build komga index: %w", err)
	}
	books, err := c.ListAllBooks(ctx)
	if err != nil {
		return nil, fmt.Errorf("build komga index: %w", err)
	}

	idx := &Index{
		seriesByPath: make(map[string]string, len(series)),
		booksByPath:  make(map[string]string, len(books)),
	}
	for _, s := range series {
		idx.seriesByPath[s.URL] = s.ID
	}
	for _, b := range books {
		idx.booksByPath[b.URL] = b.ID
	}
	return idx, nil
}

// UnmatchedBook records a smart-list book that couldn't be resolved to a
// Komga entity, along with why. Intended for logging now; the acceptance
// criterion of surfacing these in the web UI is separate, later work.
type UnmatchedBook struct {
	Book   *library.ComicBook
	Reason string
}

// ResolveReadListBooks translates each book's path and looks it up in the
// index, returning matched Komga book IDs (in list order, duplicates
// removed) and any books that couldn't be resolved. Unmatched books are not
// an error - they're skipped so the rest of the list can still sync.
func (idx *Index) ResolveReadListBooks(books []*library.ComicBook, localRoot, remoteRoot string) ([]string, []UnmatchedBook) {
	var matched []string
	var unmatched []UnmatchedBook
	seen := make(map[string]bool, len(books))

	for _, book := range books {
		remotePath, err := TranslatePath(localRoot, remoteRoot, book.FilePath)
		if err != nil {
			unmatched = append(unmatched, UnmatchedBook{Book: book, Reason: err.Error()})
			continue
		}

		id, ok := idx.booksByPath[remotePath]
		if !ok {
			unmatched = append(unmatched, UnmatchedBook{Book: book, Reason: fmt.Sprintf("no Komga book at %q", remotePath)})
			continue
		}

		if !seen[id] {
			seen[id] = true
			matched = append(matched, id)
		}
	}
	return matched, unmatched
}

// ResolveCollectionSeries translates each book's path, takes its parent
// directory as the series path (comic-server's library is organized one
// directory per series, matching Komga's own layout), and looks that up in
// the index. Multiple books resolving to the same series are deduplicated,
// in first-seen order.
func (idx *Index) ResolveCollectionSeries(books []*library.ComicBook, localRoot, remoteRoot string) ([]string, []UnmatchedBook) {
	var matched []string
	var unmatched []UnmatchedBook
	seen := make(map[string]bool, len(books))

	for _, book := range books {
		remotePath, err := TranslatePath(localRoot, remoteRoot, book.FilePath)
		if err != nil {
			unmatched = append(unmatched, UnmatchedBook{Book: book, Reason: err.Error()})
			continue
		}

		seriesPath := path.Dir(remotePath)
		id, ok := idx.seriesByPath[seriesPath]
		if !ok {
			unmatched = append(unmatched, UnmatchedBook{Book: book, Reason: fmt.Sprintf("no Komga series at %q", seriesPath)})
			continue
		}

		if !seen[id] {
			seen[id] = true
			matched = append(matched, id)
		}
	}
	return matched, unmatched
}
