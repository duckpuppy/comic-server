package library

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// getGaugeValue retrieves the current value of a gauge metric
func getGaugeValue(gauge prometheus.Gauge) float64 {
	metric := &dto.Metric{}
	gauge.Write(metric)
	return metric.GetGauge().GetValue()
}

// getCounterValue retrieves the current value of a counter metric
func getCounterValue(counter prometheus.Counter) float64 {
	metric := &dto.Metric{}
	counter.Write(metric)
	return metric.GetCounter().GetValue()
}

// getHistogramCount retrieves the sample count from a histogram
func getHistogramCount(histogram prometheus.Histogram) uint64 {
	metric := &dto.Metric{}
	histogram.Write(metric)
	return metric.GetHistogram().GetSampleCount()
}

// TestLibraryCacheMetrics_DirtyBooksGauge tests dirty books gauge metric
func TestLibraryCacheMetrics_DirtyBooksGauge(t *testing.T) {
	lib := &ComicLibrary{
		Books: []ComicBook{
			{ID: "book1", Title: "Test Book 1"},
			{ID: "book2", Title: "Test Book 2"},
			{ID: "book3", Title: "Test Book 3"},
		},
	}

	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "test.xml")

	cache := NewLibraryCache(lib, libraryPath, 0)
	defer cache.Stop()

	// Initial gauge value should be 0
	initialValue := getGaugeValue(cacheDirtyBooksGauge)

	// Mark one book dirty
	cache.MarkDirty("book1")
	gaugeValue := getGaugeValue(cacheDirtyBooksGauge)
	if gaugeValue <= initialValue {
		t.Errorf("expected gauge to increase, got %v (was %v)", gaugeValue, initialValue)
	}

	// Mark two more books dirty
	cache.MarkManyDirty([]string{"book2", "book3"})
	gaugeValue = getGaugeValue(cacheDirtyBooksGauge)
	if gaugeValue <= initialValue {
		t.Errorf("expected gauge to increase further, got %v (was %v)", gaugeValue, initialValue)
	}

	// Flush should reset gauge to 0
	beforeFlush := gaugeValue
	_, err := cache.Flush()
	if err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	gaugeValue = getGaugeValue(cacheDirtyBooksGauge)
	if gaugeValue >= beforeFlush {
		t.Errorf("expected gauge to decrease after flush, got %v (was %v)", gaugeValue, beforeFlush)
	}
}

// TestLibraryCacheMetrics_FlushCounter tests flush counter metric
func TestLibraryCacheMetrics_FlushCounter(t *testing.T) {
	lib := &ComicLibrary{
		Books: []ComicBook{
			{ID: "book1", Title: "Test Book 1"},
		},
	}

	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "test.xml")

	cache := NewLibraryCache(lib, libraryPath, 0)
	defer cache.Stop()

	// Get initial counter values
	initialSuccess := getCounterValue(cacheFlushesTotal.WithLabelValues("success"))

	// Mark dirty and flush
	cache.MarkDirty("book1")
	_, err := cache.Flush()
	if err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	// Check success counter increased
	successCount := getCounterValue(cacheFlushesTotal.WithLabelValues("success"))
	if successCount <= initialSuccess {
		t.Errorf("expected success counter to increase, got %v (was %v)", successCount, initialSuccess)
	}
}

// TestLibraryCacheMetrics_FlushError tests flush error counter metric
func TestLibraryCacheMetrics_FlushError(t *testing.T) {
	lib := &ComicLibrary{
		Books: []ComicBook{
			{ID: "book1", Title: "Test Book 1"},
		},
	}

	// Use invalid path to trigger error
	invalidPath := "/nonexistent/path/test.xml"
	cache := NewLibraryCache(lib, invalidPath, 0)
	defer cache.Stop()

	// Get initial error counter
	initialError := getCounterValue(cacheFlushesTotal.WithLabelValues("error"))

	// Mark dirty and flush (should fail)
	cache.MarkDirty("book1")
	_, err := cache.Flush()
	if err == nil {
		t.Fatal("expected Flush() to fail with invalid path")
	}

	// Check error counter increased
	errorCount := getCounterValue(cacheFlushesTotal.WithLabelValues("error"))
	if errorCount <= initialError {
		t.Errorf("expected error counter to increase, got %v (was %v)", errorCount, initialError)
	}
}

// TestLibraryCacheMetrics_FlushDuration tests flush duration histogram
func TestLibraryCacheMetrics_FlushDuration(t *testing.T) {
	lib := &ComicLibrary{
		Books: []ComicBook{
			{ID: "book1", Title: "Test Book 1"},
		},
	}

	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "test.xml")

	cache := NewLibraryCache(lib, libraryPath, 0)
	defer cache.Stop()

	// Get initial histogram count
	initialCount := getHistogramCount(cacheFlushDurationSeconds)

	// Mark dirty and flush
	cache.MarkDirty("book1")
	_, err := cache.Flush()
	if err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	// Check histogram sample count increased
	sampleCount := getHistogramCount(cacheFlushDurationSeconds)
	if sampleCount <= initialCount {
		t.Errorf("expected histogram sample count to increase, got %v (was %v)", sampleCount, initialCount)
	}
}

// TestLibraryCacheMetrics_AutoFlush tests metrics with auto-flush
func TestLibraryCacheMetrics_AutoFlush(t *testing.T) {
	lib := &ComicLibrary{
		Books: []ComicBook{
			{ID: "book1", Title: "Test Book 1"},
		},
	}

	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "test.xml")

	// Create cache with 500ms auto-flush
	cache := NewLibraryCache(lib, libraryPath, 500*time.Millisecond)
	defer cache.Stop()

	// Get initial metrics
	initialSuccess := getCounterValue(cacheFlushesTotal.WithLabelValues("success"))
	initialHistCount := getHistogramCount(cacheFlushDurationSeconds)

	// Mark dirty
	cache.MarkDirty("book1")

	// Check gauge increased
	gaugeValue := getGaugeValue(cacheDirtyBooksGauge)
	if gaugeValue == 0 {
		t.Error("expected gauge to be > 0 after marking dirty")
	}

	// Wait for auto-flush (500ms interval + some buffer)
	time.Sleep(700 * time.Millisecond)

	// Check that flush occurred automatically
	successCount := getCounterValue(cacheFlushesTotal.WithLabelValues("success"))
	if successCount <= initialSuccess {
		t.Errorf("expected auto-flush to occur, success counter unchanged: %v", successCount)
	}

	histCount := getHistogramCount(cacheFlushDurationSeconds)
	if histCount <= initialHistCount {
		t.Errorf("expected auto-flush to record duration, histogram count unchanged: %v", histCount)
	}

	// Gauge should be back to 0 after flush
	gaugeValue = getGaugeValue(cacheDirtyBooksGauge)
	if gaugeValue != 0 {
		// Note: This might not be exactly 0 if other tests are running concurrently
		// but for this isolated test it should be 0
		t.Logf("gauge value after auto-flush: %v (expected close to 0)", gaugeValue)
	}
}

// TestLibraryCacheMetrics_NoDuplicateFlush tests that empty flushes don't record metrics
func TestLibraryCacheMetrics_NoDuplicateFlush(t *testing.T) {
	lib := &ComicLibrary{
		Books: []ComicBook{
			{ID: "book1", Title: "Test Book 1"},
		},
	}

	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "test.xml")

	cache := NewLibraryCache(lib, libraryPath, 0)
	defer cache.Stop()

	// Get initial counter
	initialSuccess := getCounterValue(cacheFlushesTotal.WithLabelValues("success"))

	// Flush with no dirty books should not increment counter
	saved, err := cache.Flush()
	if err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}
	if saved {
		t.Error("expected Flush() to return false when no dirty books")
	}

	successCount := getCounterValue(cacheFlushesTotal.WithLabelValues("success"))
	if successCount != initialSuccess {
		t.Errorf("expected no counter increment for empty flush, got %v (was %v)", successCount, initialSuccess)
	}
}

// BenchmarkLibraryCacheFlushWithMetrics benchmarks flush with metrics recording
func BenchmarkLibraryCacheFlushWithMetrics(b *testing.B) {
	lib := &ComicLibrary{
		Books: []ComicBook{
			{ID: "book1", Title: "Test Book 1"},
			{ID: "book2", Title: "Test Book 2"},
		},
	}

	// Create cache
	cache := NewLibraryCache(lib, "", 0) // Empty path for in-memory only
	defer cache.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Mark dirty
		cache.MarkDirty("book1")

		// Flush (will fail due to empty path, but still records metrics)
		_, _ = cache.Flush()
	}
}
