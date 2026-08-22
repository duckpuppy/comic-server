package storage

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

// syntheticBookCount mirrors the real library scale referenced by
// comic-server-770/comic-server-cg1 (67,389 books).
const syntheticBookCount = 67389

// syntheticSeriesCount controls matcher selectivity: with this many
// distinct series names spread evenly over syntheticBookCount books, an
// equals-on-Series matcher matches roughly 1/syntheticSeriesCount of the
// library - realistic for a real smart list, and small enough that the SQL
// fast path's row-fetch reduction actually matters.
const syntheticSeriesCount = 500

// generateSyntheticLibrary builds a library of n books with varied Series/
// Publisher/Year and, on ~97.5% of books, tags and custom values - matching
// the real library's observed tag coverage (see the untagged-books memory)
// so the benchmark exercises the same batch-load path GetAllBooks/
// GetBooksWhere use in production, not an artificially tag-free shortcut.
func generateSyntheticLibrary(n int) *library.ComicLibrary {
	books := make([]library.ComicBook, n)
	for i := 0; i < n; i++ {
		publisher := "DC Comics"
		if i%2 == 1 {
			publisher = "Marvel Comics"
		}
		book := library.ComicBook{
			ID:        fmt.Sprintf("book-%d", i),
			FilePath:  fmt.Sprintf("/comics/series-%d/book-%d.cbz", i%syntheticSeriesCount, i),
			Title:     fmt.Sprintf("Issue #%d", i),
			Series:    fmt.Sprintf("Series %d", i%syntheticSeriesCount),
			Number:    fmt.Sprintf("%d", i%100),
			Publisher: publisher,
			Year:      1980 + i%45,
			PageCount: 20 + i%12,
			Rating:    float64(i % 6),
		}
		if i%40 != 0 { // ~97.5% tagged
			book.Tags = "hero, action"
			book.CustomValuesStore = ",comicvine_volume=1000,ReadingOrder=5"
		}
		books[i] = book
	}
	return &library.ComicLibrary{ID: "synthetic-library", Name: "Synthetic Library", Books: books}
}

func newSyntheticSQLiteBackend(b *testing.B) *SQLiteBackend {
	b.Helper()
	dir := b.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		b.Fatalf("Open: %v", err)
	}
	if _, err := db.Import(generateSyntheticLibrary(syntheticBookCount), ImportOptions{}); err != nil {
		b.Fatalf("Import: %v", err)
	}
	db.Close()

	backend, err := NewSQLiteBackend(dbPath, "")
	if err != nil {
		b.Fatalf("NewSQLiteBackend: %v", err)
	}
	b.Cleanup(func() { backend.Close() })
	return backend
}

func newSyntheticXMLBackend(b *testing.B) *library.XMLBackend {
	b.Helper()
	backend := library.NewXMLBackendFromLibrary(generateSyntheticLibrary(syntheticBookCount), "", nil)
	b.Cleanup(func() { backend.Close() })
	return backend
}

// simpleEqualsList is fully SQL-translatable (see matcher_sql.go): a single
// top-level Series equals matcher, matching ~1/syntheticSeriesCount books.
func simpleEqualsList() *library.ComicListItem {
	return &library.ComicListItem{
		ID: "list-simple", Type: "ComicSmartListItem", Name: "Series 1", MatcherMode: "And",
		Matchers: []library.ComicBookMatcher{
			{Type: "Series", MatchOperator: "0", MatchValue: "Series 1"},
		},
	}
}

// tagsList is NOT SQL-translatable (Tags lives in a separate table), so
// SQLiteBackend falls back to the full in-memory path for it - useful as
// this benchmark suite's own "before" baseline for the SQL fast path above,
// run against identical data and matcher selectivity.
func tagsList() *library.ComicListItem {
	return &library.ComicListItem{
		ID: "list-tags", Type: "ComicSmartListItem", Name: "Hero tag", MatcherMode: "And",
		Matchers: []library.ComicBookMatcher{
			{Type: "Tags", MatchOperator: "1", MatchValue: "hero"},
		},
	}
}

func BenchmarkSQLiteBackend_MatchBooks_SQLFastPath(b *testing.B) {
	backend := newSyntheticSQLiteBackend(b)
	list := simpleEqualsList()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := backend.MatchBooks(list); err != nil {
			b.Fatalf("MatchBooks: %v", err)
		}
	}
}

// BenchmarkSQLiteBackend_MatchBooks_FullScanFallback is this file's "before"
// baseline: an equally selective matcher that can't use the SQL fast path
// (Tags requires a join to a separate table), so SQLiteBackend falls back
// to GetAllBooks() + in-memory evaluation - the same code path every
// SQLiteBackend.MatchBooks call used before comic-server-770.
func BenchmarkSQLiteBackend_MatchBooks_FullScanFallback(b *testing.B) {
	backend := newSyntheticSQLiteBackend(b)
	list := tagsList()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := backend.MatchBooks(list); err != nil {
			b.Fatalf("MatchBooks: %v", err)
		}
	}
}

// BenchmarkXMLBackend_MatchBooks is the XML backend's cost for the same
// data and matcher as BenchmarkSQLiteBackend_MatchBooks_SQLFastPath, per
// comic-server-770's acceptance criteria ("measurable improvement over the
// XML backend's evaluation cost").
func BenchmarkXMLBackend_MatchBooks(b *testing.B) {
	backend := newSyntheticXMLBackend(b)
	list := simpleEqualsList()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := backend.MatchBooks(list); err != nil {
			b.Fatalf("MatchBooks: %v", err)
		}
	}
}

// BenchmarkGetAllBooks_Synthetic exercises the batch tag/custom-value
// loading fix on its own (no matcher involved), showing the reduction from
// O(books) extra round trips to O(books/batchLoadChunkSize).
func BenchmarkGetAllBooks_Synthetic(b *testing.B) {
	backend := newSyntheticSQLiteBackend(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := backend.GetAllBooks(); err != nil {
			b.Fatalf("GetAllBooks: %v", err)
		}
	}
}
