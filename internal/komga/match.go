package komga

import (
	"context"
	"fmt"
	"path"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/pathmap"
)

// TranslatePath converts a comic-server library path into the equivalent
// path as Komga sees it. Thin wrapper kept for this package's existing call
// sites; the actual implementation (and its tests) live in internal/pathmap
// since covers.go and internal/sync need the identical translation for
// comic-server's OWN filesystem view, not just Komga's - see
// comic-server-64l/comic-server-ivq/comic-server-4n9.
func TranslatePath(localRoot, remoteRoot, localPath string) (string, error) {
	return pathmap.TranslatePath(localRoot, remoteRoot, localPath)
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

// BookReadStatus pairs a resolved Komga book ID with the source book's
// current read state, for pushing read/unread status independent of
// collection/read-list membership (which for a Collection target only
// tracks distinct series, not per-issue state).
type BookReadStatus struct {
	Book        *library.ComicBook
	KomgaBookID string
	Read        bool
}

// ResolveBookReadStatus translates each book's path and looks it up in the
// index, returning one BookReadStatus per resolved book (no deduplication -
// unlike ResolveReadListBooks/ResolveCollectionSeries, callers need every
// book's own read state, not just a set of Komga IDs) and any books that
// couldn't be resolved.
func (idx *Index) ResolveBookReadStatus(books []*library.ComicBook, localRoot, remoteRoot string) ([]BookReadStatus, []UnmatchedBook) {
	var matched []BookReadStatus
	var unmatched []UnmatchedBook

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

		matched = append(matched, BookReadStatus{Book: book, KomgaBookID: id, Read: !book.IsUnread()})
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
