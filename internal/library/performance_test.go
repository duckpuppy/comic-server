package library_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

// BenchmarkLoadLibrary measures library loading performance
func BenchmarkLoadLibrary(b *testing.B) {
	testdataPath := filepath.Join("..", "..", "testdata", "ComicDB.xml")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := library.LoadLibrary(testdataPath)
		if err != nil {
			b.Fatalf("Failed to load library: %v", err)
		}
	}
}

// BenchmarkSaveLibrary measures library saving performance
func BenchmarkSaveLibrary(b *testing.B) {
	testdataPath := filepath.Join("..", "..", "testdata", "ComicDB.xml")

	// Load library once
	lib, err := library.LoadLibrary(testdataPath)
	if err != nil {
		b.Fatalf("Failed to load library: %v", err)
	}

	// Create temp directory for saves
	tempDir := b.TempDir()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		savePath := filepath.Join(tempDir, "test.xml")
		err := library.SaveLibrary(savePath, lib)
		if err != nil {
			b.Fatalf("Failed to save library: %v", err)
		}
	}
}

// BenchmarkLoadThenSave measures round-trip performance
func BenchmarkLoadThenSave(b *testing.B) {
	testdataPath := filepath.Join("..", "..", "testdata", "ComicDB.xml")
	tempDir := b.TempDir()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Load
		lib, err := library.LoadLibrary(testdataPath)
		if err != nil {
			b.Fatalf("Failed to load library: %v", err)
		}

		// Save
		savePath := filepath.Join(tempDir, "test.xml")
		err = library.SaveLibrary(savePath, lib)
		if err != nil {
			b.Fatalf("Failed to save library: %v", err)
		}
	}
}

// BenchmarkGetBook measures book lookup performance
func BenchmarkGetBook(b *testing.B) {
	testdataPath := filepath.Join("..", "..", "testdata", "ComicDB.xml")

	lib, err := library.LoadLibrary(testdataPath)
	if err != nil {
		b.Fatalf("Failed to load library: %v", err)
	}

	// Get a valid book ID from the library
	var testBookID string
	if len(lib.Books) > 0 {
		testBookID = lib.Books[0].ID
	} else {
		b.Skip("No books in library")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		book := lib.GetBook(testBookID)
		if book == nil {
			b.Fatal("Book not found")
		}
	}
}

// BenchmarkSmartListEvaluation measures smart list matcher performance
func BenchmarkSmartListEvaluation(b *testing.B) {
	testdataPath := filepath.Join("..", "..", "testdata", "ComicDB.xml")

	lib, err := library.LoadLibrary(testdataPath)
	if err != nil {
		b.Fatalf("Failed to load library: %v", err)
	}

	// Find a smart list to benchmark
	var smartList *library.ComicListItem
	for i := range lib.ComicLists {
		if lib.ComicLists[i].Type == "ComicSmartListItem" ||
			lib.ComicLists[i].Type == "comicrack:ComicSmartListItem" {
			smartList = &lib.ComicLists[i]
			break
		}
	}

	if smartList == nil {
		b.Skip("No smart lists in library")
	}

	b.ResetTimer()
	b.Run("EvaluateSmartList", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := lib.MatchBooks(smartList)
			if err != nil {
				b.Fatalf("MatchBooks failed: %v", err)
			}
		}
	})
}

// BenchmarkSmartListEvaluationByComplexity benchmarks matchers of varying complexity
func BenchmarkSmartListEvaluationByComplexity(b *testing.B) {
	testdataPath := filepath.Join("..", "..", "testdata", "ComicDB.xml")

	lib, err := library.LoadLibrary(testdataPath)
	if err != nil {
		b.Fatalf("Failed to load library: %v", err)
	}

	if len(lib.Books) == 0 {
		b.Skip("No books in library")
	}

	// Simple matcher (1 condition)
	b.Run("Simple_1Matcher", func(b *testing.B) {
		smartList := &library.ComicListItem{
			Type:        "ComicSmartListItem",
			MatcherMode: "And",
			Matchers: []library.ComicBookMatcher{
				{Type: "Series", MatchOperator: "0", MatchValue: "Batman"},
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := lib.MatchBooks(smartList)
			if err != nil {
				b.Fatalf("MatchBooks failed: %v", err)
			}
		}
	})

	// Medium complexity (3 conditions)
	b.Run("Medium_3Matchers", func(b *testing.B) {
		smartList := &library.ComicListItem{
			Type:        "ComicSmartListItem",
			MatcherMode: "And",
			Matchers: []library.ComicBookMatcher{
				{Type: "Series", MatchOperator: "2", MatchValue: "Batman"},
				{Type: "Year", MatchOperator: "2", MatchValue: "2020"},
				{Type: "Publisher", MatchOperator: "0", MatchValue: "DC Comics"},
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := lib.MatchBooks(smartList)
			if err != nil {
				b.Fatalf("MatchBooks failed: %v", err)
			}
		}
	})

	// Complex (6 conditions with OR logic)
	b.Run("Complex_6Matchers_OR", func(b *testing.B) {
		smartList := &library.ComicListItem{
			Type:        "ComicSmartListItem",
			MatcherMode: "Or",
			Matchers: []library.ComicBookMatcher{
				{Type: "Series", MatchOperator: "2", MatchValue: "Batman"},
				{Type: "Series", MatchOperator: "2", MatchValue: "Superman"},
				{Type: "Series", MatchOperator: "2", MatchValue: "Wonder Woman"},
				{Type: "Publisher", MatchOperator: "0", MatchValue: "DC Comics"},
				{Type: "Publisher", MatchOperator: "0", MatchValue: "Marvel"},
				{Type: "Year", MatchOperator: "2", MatchValue: "2020"},
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := lib.MatchBooks(smartList)
			if err != nil {
				b.Fatalf("MatchBooks failed: %v", err)
			}
		}
	})
}

// BenchmarkLibraryIteration measures raw iteration performance
func BenchmarkLibraryIteration(b *testing.B) {
	testdataPath := filepath.Join("..", "..", "testdata", "ComicDB.xml")

	lib, err := library.LoadLibrary(testdataPath)
	if err != nil {
		b.Fatalf("Failed to load library: %v", err)
	}

	b.Run("IterateAllBooks", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			count := 0
			for range lib.Books {
				count++
			}
		}
	})

	b.Run("IterateAndAccessTitle", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, book := range lib.Books {
				_ = book.Title
			}
		}
	})

	b.Run("IterateAndAccessMultipleFields", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, book := range lib.Books {
				_ = book.Title
				_ = book.Series
				_ = book.Publisher
				_ = book.Year
				_ = book.PageCount
			}
		}
	})
}

// BenchmarkUpdateSingleBook measures the cost of updating one book and saving
func BenchmarkUpdateSingleBook(b *testing.B) {
	testdataPath := filepath.Join("..", "..", "testdata", "ComicDB.xml")

	lib, err := library.LoadLibrary(testdataPath)
	if err != nil {
		b.Fatalf("Failed to load library: %v", err)
	}

	if len(lib.Books) == 0 {
		b.Skip("No books in library")
	}

	tempDir := b.TempDir()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Update one book
		lib.Books[0].CurrentPage = 10
		lib.Books[0].LastPageRead = 9
		lib.Books[0].OpenCount++

		// Save entire library (this is what reverse sync does)
		savePath := filepath.Join(tempDir, "test.xml")
		err := library.SaveLibrary(savePath, lib)
		if err != nil {
			b.Fatalf("Failed to save library: %v", err)
		}
	}
}

// BenchmarkMemoryUsage measures memory footprint
func BenchmarkMemoryUsage(b *testing.B) {
	testdataPath := filepath.Join("..", "..", "testdata", "ComicDB.xml")

	b.Run("LoadLibrary", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := library.LoadLibrary(testdataPath)
			if err != nil {
				b.Fatalf("Failed to load library: %v", err)
			}
		}
	})

	b.Run("SaveLibrary", func(b *testing.B) {
		lib, err := library.LoadLibrary(testdataPath)
		if err != nil {
			b.Fatalf("Failed to load library: %v", err)
		}

		tempDir := b.TempDir()

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			savePath := filepath.Join(tempDir, "test.xml")
			err := library.SaveLibrary(savePath, lib)
			if err != nil {
				b.Fatalf("Failed to save library: %v", err)
			}
		}
	})
}

// BenchmarkFileSize measures the impact of file size
func BenchmarkFileSize(b *testing.B) {
	testdataPath := filepath.Join("..", "..", "testdata", "ComicDB.xml")

	// Get file size
	fileInfo, err := os.Stat(testdataPath)
	if err != nil {
		b.Fatalf("Failed to stat file: %v", err)
	}

	b.Logf("Library file size: %.2f MB", float64(fileInfo.Size())/(1024*1024))

	lib, err := library.LoadLibrary(testdataPath)
	if err != nil {
		b.Fatalf("Failed to load library: %v", err)
	}

	b.Logf("Books in library: %d", len(lib.Books))
	b.Logf("Lists in library: %d", len(lib.ComicLists))
}
