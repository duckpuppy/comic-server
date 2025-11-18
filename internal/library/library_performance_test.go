package library

import (
	"path/filepath"
	"testing"
	"time"
)

const testLibraryPath = "../../testdata/library/ComicDb.xml"

// TestSaveLibraryPerformance ensures SaveLibrary performance doesn't regress
// Uses testdata library (10 books) - save should complete in < 1ms
func TestSaveLibraryPerformance(t *testing.T) {
	// Load test library
	lib, err := LoadLibrary(testLibraryPath)
	if err != nil {
		t.Skipf("Skipping performance test: testdata library not available: %v", err)
	}

	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "test.xml")

	// Warm up (first save creates file, subsequent saves overwrite)
	if err := SaveLibrary(libraryPath, lib); err != nil {
		t.Fatalf("Warmup save failed: %v", err)
	}

	// Measure performance
	start := time.Now()
	err = SaveLibrary(libraryPath, lib)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("SaveLibrary failed: %v", err)
	}

	// For small library (10 books), save should be < 1ms
	// Observed performance: ~87µs
	maxDuration := 1 * time.Millisecond
	if duration > maxDuration {
		t.Errorf("SaveLibrary too slow: %v (expected < %v for %d books)",
			duration, maxDuration, len(lib.Books))
	}

	t.Logf("SaveLibrary performance: %v for %d books", duration, len(lib.Books))
}

// TestLoadLibraryPerformance ensures LoadLibrary performance doesn't regress
// Uses testdata library (10 books) - load should complete in < 1ms
func TestLoadLibraryPerformance(t *testing.T) {
	// Measure performance
	start := time.Now()
	lib, err := LoadLibrary(testLibraryPath)
	duration := time.Since(start)

	if err != nil {
		t.Skipf("Skipping performance test: testdata library not available: %v", err)
	}

	// For small library (10 books), load should be < 1ms
	// Observed performance: ~176µs
	maxDuration := 1 * time.Millisecond
	if duration > maxDuration {
		t.Errorf("LoadLibrary too slow: %v (expected < %v for %d books)",
			duration, maxDuration, len(lib.Books))
	}

	t.Logf("LoadLibrary performance: %v for %d books", duration, len(lib.Books))
}

// TestLibraryCacheFlushPerformance ensures cache flush doesn't regress
// Uses testdata library (10 books) - flush should complete in < 1ms
func TestLibraryCacheFlushPerformance(t *testing.T) {
	// Load test library
	lib, err := LoadLibrary(testLibraryPath)
	if err != nil {
		t.Skipf("Skipping performance test: testdata library not available: %v", err)
	}

	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "test.xml")

	// Create cache with no auto-flush
	cache := NewLibraryCache(lib, libraryPath, 0)
	defer cache.Stop()

	// Mark all books dirty
	for i := range lib.Books {
		cache.MarkDirty(lib.Books[i].ID)
	}

	// Warm up
	_, err = cache.Flush()
	if err != nil {
		t.Fatalf("Warmup flush failed: %v", err)
	}

	// Mark dirty again for actual test
	for i := range lib.Books {
		cache.MarkDirty(lib.Books[i].ID)
	}

	// Measure flush performance
	start := time.Now()
	_, err = cache.Flush()
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Flush includes SaveLibrary overhead + cache bookkeeping
	// For small library (10 books), should be < 1ms
	// Observed performance: ~87µs
	maxDuration := 1 * time.Millisecond
	if duration > maxDuration {
		t.Errorf("Cache flush too slow: %v (expected < %v for %d books)",
			duration, maxDuration, len(lib.Books))
	}

	t.Logf("Cache flush performance: %v for %d books", duration, len(lib.Books))
}

// TestLibraryCacheBatchEfficiency ensures cache batching provides performance benefit
// Batching 10 saves should be faster than 10 individual saves
func TestLibraryCacheBatchEfficiency(t *testing.T) {
	// Load test library
	lib, err := LoadLibrary(testLibraryPath)
	if err != nil {
		t.Skipf("Skipping performance test: testdata library not available: %v", err)
	}

	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "test.xml")

	// Measure 10 individual saves (no caching)
	individualStart := time.Now()
	for i := 0; i < 10; i++ {
		if err := SaveLibrary(libraryPath, lib); err != nil {
			t.Fatalf("Individual save failed: %v", err)
		}
	}
	individualDuration := time.Since(individualStart)

	// Measure 10 cached saves (batched into 1)
	cache := NewLibraryCache(lib, libraryPath, 0) // No auto-flush
	defer cache.Stop()

	batchStart := time.Now()
	for i := 0; i < 10; i++ {
		// Simulate marking different books dirty
		bookIdx := i % len(lib.Books)
		cache.MarkDirty(lib.Books[bookIdx].ID)
	}
	// Flush once (batches all 10 marks into 1 save)
	if _, err := cache.Flush(); err != nil {
		t.Fatalf("Batch flush failed: %v", err)
	}
	batchDuration := time.Since(batchStart)

	// Batched approach should be significantly faster
	// Expect at least 5x improvement (conservative threshold)
	if batchDuration*5 > individualDuration {
		t.Errorf("Cache batching not efficient enough: individual=%v, batched=%v (expected >5x improvement)",
			individualDuration, batchDuration)
	}

	improvementRatio := float64(individualDuration) / float64(batchDuration)
	t.Logf("Cache batching efficiency: %v individual saves in %v, batched into 1 save in %v (%.1fx improvement)",
		10, individualDuration, batchDuration, improvementRatio)
}

// TestSmartListEvaluationPerformance ensures smart list evaluation doesn't regress
// Uses testdata library (10 books) - evaluation should be < 100µs
func TestSmartListEvaluationPerformance(t *testing.T) {
	// Load test library
	lib, err := LoadLibrary(testLibraryPath)
	if err != nil {
		t.Skipf("Skipping performance test: testdata library not available: %v", err)
	}

	// Find a smart list with matchers
	var smartList *ComicListItem
	for i := range lib.ComicLists {
		isSmartList := lib.ComicLists[i].Type == "ComicSmartListItem" ||
			lib.ComicLists[i].Type == "comicrack:ComicSmartListItem"
		if isSmartList && len(lib.ComicLists[i].Matchers) > 0 {
			smartList = &lib.ComicLists[i]
			break
		}
	}

	if smartList == nil {
		t.Skip("No smart lists with matchers found in test library")
	}

	// Measure smart list evaluation
	start := time.Now()
	matchedBooks, err := lib.MatchBooks(smartList)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("MatchBooks failed: %v", err)
	}

	// For small library (10 books), evaluation should be < 100µs
	// Observed performance: ~2.8µs
	maxDuration := 100 * time.Microsecond
	if duration > maxDuration {
		t.Errorf("Smart list evaluation too slow: %v (expected < %v for %d books with %d matchers)",
			duration, maxDuration, len(lib.Books), len(smartList.Matchers))
	}

	t.Logf("Smart list evaluation performance: %v for %d books, %d matchers, %d matches",
		duration, len(lib.Books), len(smartList.Matchers), len(matchedBooks))
}
