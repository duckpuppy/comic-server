package storage

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/duckpuppy/comic-server/internal/library"
)

// ExportResult holds the reconstructed library plus any warnings produced
// while building it (currently: CV-only matchers dropped from smart lists
// because real ComicRackCE doesn't understand them - see
// dropCVOnlyMatchers).
type ExportResult struct {
	Library  *library.ComicLibrary
	Warnings []string
}

// Export reconstructs a full library.ComicLibrary (every book, every
// smart/reading list with matchers) from the SQLite database, suitable for
// writing back out via library.SaveLibrary. This is the reverse of Import.
//
// comic-server's own extension matcher types
// (ComicServerCVSeriesCompleteMatcher, ComicServerCVMissingCountMatcher,
// ComicServerCVPercentOwnedMatcher) aren't recognized by real ComicRackCE,
// so they're dropped from any smart list that contains them - the rest of
// that list's matchers/structure is preserved. Each drop is reported in
// ExportResult.Warnings rather than silently discarded.
func (db *DB) Export() (*ExportResult, error) {
	books, err := db.GetAllBooks()
	if err != nil {
		return nil, fmt.Errorf("get all books: %w", err)
	}

	lists, err := db.GetAllLists()
	if err != nil {
		return nil, fmt.Errorf("get all lists: %w", err)
	}

	var warnings []string
	for i := range lists {
		dropCVOnlyMatchers(&lists[i], &warnings)
	}

	libID, libName, err := db.GetLibraryMetadata()
	if err != nil {
		return nil, fmt.Errorf("get library metadata: %w", err)
	}

	lib := &library.ComicLibrary{
		ID:         libID,
		Name:       libName,
		Books:      books,
		ComicLists: lists,
	}

	return &ExportResult{Library: lib, Warnings: warnings}, nil
}

// GetLibraryMetadata returns the library ID/name recorded by the most
// recent import (see storeLibraryMetadata). Both are empty strings if no
// import has ever populated library_metadata.
func (db *DB) GetLibraryMetadata() (id string, name string, err error) {
	row := db.QueryRow("SELECT value FROM library_metadata WHERE key = 'library_id'")
	if scanErr := row.Scan(&id); scanErr != nil && !errors.Is(scanErr, sql.ErrNoRows) {
		return "", "", scanErr
	}

	row = db.QueryRow("SELECT value FROM library_metadata WHERE key = 'library_name'")
	if scanErr := row.Scan(&name); scanErr != nil && !errors.Is(scanErr, sql.ErrNoRows) {
		return "", "", scanErr
	}

	return id, name, nil
}

// dropCVOnlyMatchers recursively removes any comic-server-only CV matcher
// (see library.IsCVOnlyMatcherType) from list's Matchers tree (including
// nested group matchers) and from every child list (folders nest lists),
// appending a warning per dropped matcher. The rest of each matcher
// group's structure/mode is preserved - only the offending matcher itself
// is removed, not its siblings.
func dropCVOnlyMatchers(list *library.ComicListItem, warnings *[]string) {
	list.Matchers = filterCVOnlyMatchers(list.Matchers, list.Name, warnings)
	for i := range list.ChildItems {
		dropCVOnlyMatchers(&list.ChildItems[i], warnings)
	}
}

func filterCVOnlyMatchers(matchers []library.ComicBookMatcher, listName string, warnings *[]string) []library.ComicBookMatcher {
	if len(matchers) == 0 {
		return matchers
	}

	kept := make([]library.ComicBookMatcher, 0, len(matchers))
	for i := range matchers {
		m := matchers[i]
		if library.IsCVOnlyMatcherType(m.Type) {
			*warnings = append(*warnings, fmt.Sprintf(
				"list %q: dropped comic-server-only matcher %s (not supported by ComicRackCE)",
				listName, m.Type))
			continue
		}
		m.Matchers = filterCVOnlyMatchers(m.Matchers, listName, warnings)
		kept = append(kept, m)
	}
	return kept
}
