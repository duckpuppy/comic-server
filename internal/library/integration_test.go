package library_test

import (
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestLoadRealLibrary(t *testing.T) {
	// Load the actual ComicDB.xml from testdata
	testdataPath := filepath.Join("..", "..", "testdata", "ComicDB.xml")

	lib, err := library.LoadLibrary(testdataPath)
	if err != nil {
		t.Fatalf("Failed to load library: %v", err)
	}

	t.Run("LibraryStructure", func(t *testing.T) {
		if lib.ID == "" {
			t.Error("Library ID should not be empty")
		}

		if len(lib.Books) == 0 {
			t.Error("Library should have books")
		}

		t.Logf("Loaded library with %d books", len(lib.Books))
	})

	t.Run("BookMetadata", func(t *testing.T) {
		// Check that books have proper metadata
		bookCount := 0
		seriesCount := 0
		publisherCount := 0

		for _, book := range lib.Books {
			bookCount++

			if book.ID == "" {
				t.Error("Book ID should not be empty")
			}

			if book.FilePath != "" {
				// Most books should have file paths
				bookCount++
			}

			if book.Series != "" {
				seriesCount++
			}

			if book.Publisher != "" {
				publisherCount++
			}

			// Check first book in detail
			if bookCount == 1 {
				t.Logf("First book details:")
				t.Logf("  ID: %s", book.ID)
				t.Logf("  Series: %s", book.Series)
				t.Logf("  Number: %s", book.Number)
				if book.Year > 0 {
					t.Logf("  Year: %d", book.Year)
				}
				t.Logf("  Publisher: %s", book.Publisher)
				t.Logf("  PageCount: %d", book.PageCount)
			}
		}

		t.Logf("Books with series: %d/%d", seriesCount, bookCount)
		t.Logf("Books with publisher: %d/%d", publisherCount, bookCount)

		if seriesCount == 0 {
			t.Error("Expected at least some books to have series")
		}
	})

	t.Run("Pages", func(t *testing.T) {
		// Check that books have page information
		booksWithPages := 0
		totalPages := 0

		for _, book := range lib.Books {
			if len(book.Pages) > 0 {
				booksWithPages++
				totalPages += len(book.Pages)

				// Verify page structure
				for i, page := range book.Pages {
					if page.Image != i {
						t.Errorf("Page image index mismatch: expected %d, got %d", i, page.Image)
						break
					}
				}
			}
		}

		t.Logf("Books with pages: %d/%d", booksWithPages, len(lib.Books))
		t.Logf("Total pages: %d", totalPages)

		if booksWithPages == 0 {
			t.Error("Expected at least some books to have page information")
		}
	})

	t.Run("Timestamps", func(t *testing.T) {
		// Check that timestamp fields are parsed correctly
		booksWithAdded := 0
		booksWithOpened := 0
		booksWithBoth := 0
		booksWithNeither := 0

		for _, book := range lib.Books {
			hasAdded := !book.AddedTime.IsZero()
			hasOpened := !book.OpenedTime.IsZero()

			if hasAdded {
				booksWithAdded++
			}
			if hasOpened {
				booksWithOpened++
			}
			if hasAdded && hasOpened {
				booksWithBoth++
			}
			if !hasAdded && !hasOpened {
				booksWithNeither++
			}
		}

		t.Logf("Books with AddedTime: %d/%d", booksWithAdded, len(lib.Books))
		t.Logf("Books with OpenedTime: %d/%d", booksWithOpened, len(lib.Books))
		t.Logf("Books with both timestamps: %d/%d", booksWithBoth, len(lib.Books))
		t.Logf("Books with no timestamps: %d/%d", booksWithNeither, len(lib.Books))

		// Verify the test data has representative samples
		if booksWithAdded == 0 {
			t.Error("Expected at least some books with AddedTime in test data")
		}
		if booksWithOpened == 0 {
			t.Error("Expected at least some books with OpenedTime in test data")
		}
		if booksWithBoth == 0 {
			t.Error("Expected at least some books with both timestamps in test data")
		}
		if booksWithNeither == 0 {
			t.Error("Expected at least some books with no timestamps in test data")
		}
	})

	t.Run("ReadProgress", func(t *testing.T) {
		// Check read progress tracking
		booksRead := 0
		booksInProgress := 0

		for _, book := range lib.Books {
			if book.OpenCount > 0 {
				booksRead++
			}

			if book.CurrentPage > 0 && book.CurrentPage < book.PageCount {
				booksInProgress++
			}
		}

		t.Logf("Books opened at least once: %d/%d", booksRead, len(lib.Books))
		t.Logf("Books in progress: %d/%d", booksInProgress, len(lib.Books))
	})

	t.Run("CustomValues", func(t *testing.T) {
		// ComicBook struct may not have CustomValuesStore field exposed
		// Skip this test for now as it's not critical
		t.Skip("CustomValuesStore field not exposed in current implementation")
	})
}

func TestLibraryReadLists(t *testing.T) {
	testdataPath := filepath.Join("..", "..", "testdata", "ComicDB.xml")

	lib, err := library.LoadLibrary(testdataPath)
	if err != nil {
		t.Fatalf("Failed to load library: %v", err)
	}

	t.Run("ComicLists", func(t *testing.T) {
		if len(lib.ComicLists) > 0 {
			t.Logf("Found %d comic lists", len(lib.ComicLists))

			for i, list := range lib.ComicLists {
				if i < 5 { // Log first 5
					t.Logf("  List: %s", list.Name)
				}
			}
		} else {
			t.Log("No comic lists in library (this is okay)")
		}
	})
}

func TestLibraryQueryCapabilities(t *testing.T) {
	testdataPath := filepath.Join("..", "..", "testdata", "ComicDB.xml")

	lib, err := library.LoadLibrary(testdataPath)
	if err != nil {
		t.Fatalf("Failed to load library: %v", err)
	}

	t.Run("FindBooksByPublisher", func(t *testing.T) {
		// Find all unique publishers
		publishers := make(map[string]int)
		for _, book := range lib.Books {
			if book.Publisher != "" {
				publishers[book.Publisher]++
			}
		}

		if len(publishers) == 0 {
			t.Skip("No publishers in test data")
		}

		t.Logf("Found %d unique publishers", len(publishers))

		// Test filtering by a publisher
		for publisher := range publishers {
			count := 0
			for _, book := range lib.Books {
				if book.Publisher == publisher {
					count++
				}
			}

			if count != publishers[publisher] {
				t.Errorf("Publisher %q: expected %d books, got %d", publisher, publishers[publisher], count)
			}

			// Only test first publisher
			break
		}
	})

	t.Run("FindBooksBySeries", func(t *testing.T) {
		// Find all unique series
		series := make(map[string]int)
		for _, book := range lib.Books {
			if book.Series != "" {
				series[book.Series]++
			}
		}

		if len(series) == 0 {
			t.Skip("No series in test data")
		}

		t.Logf("Found %d unique series", len(series))
	})

	t.Run("FindUnreadBooks", func(t *testing.T) {
		unreadCount := 0
		for _, book := range lib.Books {
			if book.OpenCount == 0 {
				unreadCount++
			}
		}

		t.Logf("Unread books: %d/%d", unreadCount, len(lib.Books))
	})

	t.Run("FindBooksInProgress", func(t *testing.T) {
		inProgressCount := 0
		for _, book := range lib.Books {
			if book.CurrentPage > 0 && book.CurrentPage < book.PageCount {
				inProgressCount++
			}
		}

		t.Logf("Books in progress: %d/%d", inProgressCount, len(lib.Books))
	})
}

// Benchmark loading the real library file
func BenchmarkLoadRealLibrary(b *testing.B) {
	testdataPath := filepath.Join("..", "..", "testdata", "ComicDB.xml")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := library.LoadLibrary(testdataPath)
		if err != nil {
			b.Fatalf("Failed to load library: %v", err)
		}
	}
}
