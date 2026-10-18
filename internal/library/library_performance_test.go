package library

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const testLibraryPath = "../../testdata/library/ComicDb.xml"

// perfCeiling is the longest any of the small-library operations below may
// take. These are sanity ceilings, not speed tests: an operation on 3-10 books
// takes microseconds, so a result near this means something is badly wrong
// (an accidental O(n^2), a stuck lock), while ordinary CI noise (a busy shared
// runner, a GC pause) cannot reach it. The earlier limits (1ms, 100us, a
// time-ratio) failed releases at random (tracker #23). Real speed
// measurements are the Benchmark functions in performance_test.go and the
// *Large benchmarks below.
const perfCeiling = 2 * time.Second

// TestSaveLibraryPerformance ensures SaveLibrary does not become pathologically
// slow (see perfCeiling). Uses the testdata library.
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

	// Observed performance: ~87µs
	if duration > perfCeiling {
		t.Errorf("SaveLibrary too slow: %v (ceiling %v for %d books)",
			duration, perfCeiling, len(lib.Books))
	}

	t.Logf("SaveLibrary performance: %v for %d books", duration, len(lib.Books))
}

// TestLoadLibraryPerformance ensures LoadLibrary does not become pathologically
// slow (see perfCeiling). Uses the testdata library.
func TestLoadLibraryPerformance(t *testing.T) {
	// Measure performance
	start := time.Now()
	lib, err := LoadLibrary(testLibraryPath)
	duration := time.Since(start)

	if err != nil {
		t.Skipf("Skipping performance test: testdata library not available: %v", err)
	}

	// Observed performance: ~176µs
	if duration > perfCeiling {
		t.Errorf("LoadLibrary too slow: %v (ceiling %v for %d books)",
			duration, perfCeiling, len(lib.Books))
	}

	t.Logf("LoadLibrary performance: %v for %d books", duration, len(lib.Books))
}

// TestLibraryCacheFlushPerformance ensures cache flush does not become
// pathologically slow (see perfCeiling). Uses the testdata library.
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

	// Observed performance: ~87µs locally.
	if duration > perfCeiling {
		t.Errorf("Cache flush too slow: %v (ceiling %v for %d books)",
			duration, perfCeiling, len(lib.Books))
	}

	t.Logf("Cache flush performance: %v for %d books", duration, len(lib.Books))
}

// TestLibraryCacheBatchEfficiency checks that batching does what it is for:
// ten changes cost one write, not ten. It counts writes (the flush counter and
// the file appearing) instead of comparing times, which depended on how busy
// the machine was (tracker #23).
func TestLibraryCacheBatchEfficiency(t *testing.T) {
	lib, err := LoadLibrary(testLibraryPath)
	if err != nil {
		t.Skipf("Skipping test: testdata library not available: %v", err)
	}

	libraryPath := filepath.Join(t.TempDir(), "test.xml")
	cache := NewLibraryCache(lib, libraryPath, 0) // No auto-flush
	defer cache.Stop()

	flushes := func() float64 { return getCounterValue(cacheFlushesTotal.WithLabelValues("success")) }
	before := flushes()

	for i := 0; i < 10; i++ {
		cache.MarkDirty(lib.Books[i%len(lib.Books)].ID)
	}
	if got := flushes() - before; got != 0 {
		t.Fatalf("marking books dirty wrote %v times, want 0 (batching broken)", got)
	}
	if _, err := os.Stat(libraryPath); err == nil {
		t.Fatal("library file written before any Flush")
	}

	saved, err := cache.Flush()
	if err != nil || !saved {
		t.Fatalf("Flush = %v, %v; want true, nil", saved, err)
	}
	if got := flushes() - before; got != 1 {
		t.Errorf("10 changes caused %v writes, want exactly 1", got)
	}
	if _, err := os.Stat(libraryPath); err != nil {
		t.Errorf("library file missing after Flush: %v", err)
	}

	// Nothing is dirty any more, so another Flush must not write again.
	saved, err = cache.Flush()
	if err != nil || saved {
		t.Errorf("second Flush = %v, %v; want false, nil", saved, err)
	}
	if got := flushes() - before; got != 1 {
		t.Errorf("an empty Flush wrote again: %v writes total, want 1", got)
	}
}

// TestSmartListEvaluationPerformance ensures smart list evaluation does not
// become pathologically slow (see perfCeiling). Uses the testdata library.
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

	// Observed performance: ~2.8µs
	if duration > perfCeiling {
		t.Errorf("Smart list evaluation too slow: %v (ceiling %v for %d books with %d matchers)",
			duration, perfCeiling, len(lib.Books), len(smartList.Matchers))
	}

	t.Logf("Smart list evaluation performance: %v for %d books, %d matchers, %d matches",
		duration, len(lib.Books), len(smartList.Matchers), len(matchedBooks))
}

// --- Large Library Benchmarks ---
// These benchmarks use the production library (65K+ books) to measure real-world performance

const largeLibraryPath = "../../production_data/ComicDB.xml"

// BenchmarkSaveLibraryLarge benchmarks SaveLibrary with production library
func BenchmarkSaveLibraryLarge(b *testing.B) {
	// Load production library
	lib, err := LoadLibrary(largeLibraryPath)
	if err != nil {
		b.Skipf("Skipping large library benchmark: %v", err)
	}

	tempDir := b.TempDir()
	libraryPath := filepath.Join(tempDir, "test.xml")

	b.Logf("Benchmarking SaveLibrary with %d books", len(lib.Books))

	// Warm up
	if err := SaveLibrary(libraryPath, lib); err != nil {
		b.Fatalf("Warmup save failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := SaveLibrary(libraryPath, lib); err != nil {
			b.Fatalf("SaveLibrary failed: %v", err)
		}
	}
}

// BenchmarkLoadLibraryLarge benchmarks LoadLibrary with production library
func BenchmarkLoadLibraryLarge(b *testing.B) {
	// Check if library exists
	if _, err := LoadLibrary(largeLibraryPath); err != nil {
		b.Skipf("Skipping large library benchmark: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lib, err := LoadLibrary(largeLibraryPath)
		if err != nil {
			b.Fatalf("LoadLibrary failed: %v", err)
		}
		b.Logf("Loaded %d books", len(lib.Books))
		_ = lib // Use lib to avoid optimization
	}
}

// BenchmarkCacheFlushLarge benchmarks cache flush with production library
func BenchmarkCacheFlushLarge(b *testing.B) {
	// Load production library
	lib, err := LoadLibrary(largeLibraryPath)
	if err != nil {
		b.Skipf("Skipping large library benchmark: %v", err)
	}

	tempDir := b.TempDir()
	libraryPath := filepath.Join(tempDir, "test.xml")

	b.Logf("Benchmarking cache flush with %d books", len(lib.Books))

	// Create cache with no auto-flush
	cache := NewLibraryCache(lib, libraryPath, 0)
	defer cache.Stop()

	// Warm up - save once to create file
	for i := range lib.Books {
		cache.MarkDirty(lib.Books[i].ID)
	}
	if _, err := cache.Flush(); err != nil {
		b.Fatalf("Warmup flush failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Mark all books dirty
		for j := range lib.Books {
			cache.MarkDirty(lib.Books[j].ID)
		}

		// Flush
		b.StartTimer()
		if _, err := cache.Flush(); err != nil {
			b.Fatalf("Flush failed: %v", err)
		}
		b.StopTimer()
	}
}

// BenchmarkCacheBatchEfficiencyLarge benchmarks batch efficiency with production library
func BenchmarkCacheBatchEfficiencyLarge(b *testing.B) {
	// Load production library
	lib, err := LoadLibrary(largeLibraryPath)
	if err != nil {
		b.Skipf("Skipping large library benchmark: %v", err)
	}

	tempDir := b.TempDir()
	libraryPath := filepath.Join(tempDir, "test.xml")

	b.Logf("Benchmarking batch efficiency with %d books", len(lib.Books))

	b.Run("IndividualSaves", func(b *testing.B) {
		// Benchmark 10 individual saves
		for i := 0; i < b.N; i++ {
			for j := 0; j < 10; j++ {
				if err := SaveLibrary(libraryPath, lib); err != nil {
					b.Fatalf("Individual save failed: %v", err)
				}
			}
		}
	})

	b.Run("BatchedSaves", func(b *testing.B) {
		// Benchmark 10 saves batched into 1
		cache := NewLibraryCache(lib, libraryPath, 0)
		defer cache.Stop()

		for i := 0; i < b.N; i++ {
			for j := 0; j < 10; j++ {
				// Simulate marking different books dirty
				bookIdx := j % len(lib.Books)
				cache.MarkDirty(lib.Books[bookIdx].ID)
			}
			// Flush once (batches all 10 marks into 1 save)
			if _, err := cache.Flush(); err != nil {
				b.Fatalf("Batch flush failed: %v", err)
			}
		}
	})
}
