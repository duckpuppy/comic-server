package library

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLibraryCacheNoAutoFlush tests cache without background flusher
func TestLibraryCacheNoAutoFlush(t *testing.T) {
	// Create test library
	lib := &ComicLibrary{
		Books: []ComicBook{
			{ID: "book1", Title: "Test Book 1"},
			{ID: "book2", Title: "Test Book 2"},
		},
	}

	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "test.xml")

	// Create cache with no auto-flush (flushInterval = 0)
	cache := NewLibraryCache(lib, libraryPath, 0)
	defer cache.Stop()

	// Mark a book as dirty
	cache.MarkDirty("book1")

	// Check dirty count
	if cache.DirtyCount() != 1 {
		t.Errorf("expected 1 dirty book, got %d", cache.DirtyCount())
	}

	// Flush manually
	saved, err := cache.Flush()
	if err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}
	if !saved {
		t.Error("expected library to be saved")
	}

	// Check dirty count after flush
	if cache.DirtyCount() != 0 {
		t.Errorf("expected 0 dirty books after flush, got %d", cache.DirtyCount())
	}

	// Verify file was written
	if _, err := os.Stat(libraryPath); os.IsNotExist(err) {
		t.Error("library file was not created")
	}
}

// TestLibraryCacheAutoFlush tests cache with background flusher
func TestLibraryCacheAutoFlush(t *testing.T) {
	lib := &ComicLibrary{
		Books: []ComicBook{
			{ID: "book1", Title: "Test Book 1"},
			{ID: "book2", Title: "Test Book 2"},
		},
	}

	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "test.xml")

	// Create cache with auto-flush every 500ms
	cache := NewLibraryCache(lib, libraryPath, 500*time.Millisecond)
	defer cache.Stop()

	// Mark a book as dirty
	cache.MarkDirty("book1")

	// Wait for auto-flush to occur
	time.Sleep(700 * time.Millisecond)

	// Check that dirty count is 0 (flushed)
	if cache.DirtyCount() != 0 {
		t.Errorf("expected 0 dirty books after auto-flush, got %d", cache.DirtyCount())
	}

	// Verify file was written
	if _, err := os.Stat(libraryPath); os.IsNotExist(err) {
		t.Error("library file was not created by auto-flush")
	}
}

// TestLibraryCacheMarkManyDirty tests marking multiple books as dirty
func TestLibraryCacheMarkManyDirty(t *testing.T) {
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

	// Mark multiple books as dirty
	cache.MarkManyDirty([]string{"book1", "book2", "book3"})

	// Check dirty count
	if cache.DirtyCount() != 3 {
		t.Errorf("expected 3 dirty books, got %d", cache.DirtyCount())
	}

	// Flush
	saved, err := cache.Flush()
	if err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}
	if !saved {
		t.Error("expected library to be saved")
	}

	// Check dirty count after flush
	if cache.DirtyCount() != 0 {
		t.Errorf("expected 0 dirty books after flush, got %d", cache.DirtyCount())
	}
}

// TestLibraryCacheFlushWhenNoDirtyBooks tests flush with no dirty books
func TestLibraryCacheFlushWhenNoDirtyBooks(t *testing.T) {
	lib := &ComicLibrary{
		Books: []ComicBook{
			{ID: "book1", Title: "Test Book 1"},
		},
	}

	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "test.xml")

	cache := NewLibraryCache(lib, libraryPath, 0)
	defer cache.Stop()

	// Flush without marking anything dirty
	saved, err := cache.Flush()
	if err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}
	if saved {
		t.Error("expected library NOT to be saved when no dirty books")
	}

	// Verify file was NOT created
	if _, err := os.Stat(libraryPath); !os.IsNotExist(err) {
		t.Error("library file should not be created when no dirty books")
	}
}

// TestLibraryCacheStop tests graceful shutdown with final flush
func TestLibraryCacheStop(t *testing.T) {
	lib := &ComicLibrary{
		Books: []ComicBook{
			{ID: "book1", Title: "Test Book 1"},
		},
	}

	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "test.xml")

	// Create cache with auto-flush (very long interval so it won't trigger)
	cache := NewLibraryCache(lib, libraryPath, 10*time.Minute)

	// Mark a book as dirty
	cache.MarkDirty("book1")

	// Check dirty count
	if cache.DirtyCount() != 1 {
		t.Errorf("expected 1 dirty book, got %d", cache.DirtyCount())
	}

	// Stop should perform final flush
	err := cache.Stop()
	if err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}

	// Check dirty count after stop
	if cache.DirtyCount() != 0 {
		t.Errorf("expected 0 dirty books after stop, got %d", cache.DirtyCount())
	}

	// Verify file was written during stop
	if _, err := os.Stat(libraryPath); os.IsNotExist(err) {
		t.Error("library file was not created during stop")
	}
}

// TestLibraryCacheGetLibrary tests read access to library
func TestLibraryCacheGetLibrary(t *testing.T) {
	lib := &ComicLibrary{
		Books: []ComicBook{
			{ID: "book1", Title: "Test Book 1"},
			{ID: "book2", Title: "Test Book 2"},
		},
	}

	cache := NewLibraryCache(lib, "", 0)
	defer cache.Stop()

	// Get library
	retrievedLib := cache.GetLibrary()

	// Verify it's the same library
	if retrievedLib != lib {
		t.Error("GetLibrary() returned different library instance")
	}

	// Verify books are accessible
	if len(retrievedLib.Books) != 2 {
		t.Errorf("expected 2 books, got %d", len(retrievedLib.Books))
	}
}

// TestLibraryCacheLastSaveTime tests last save time tracking
func TestLibraryCacheLastSaveTime(t *testing.T) {
	lib := &ComicLibrary{
		Books: []ComicBook{
			{ID: "book1", Title: "Test Book 1"},
		},
	}

	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "test.xml")

	cache := NewLibraryCache(lib, libraryPath, 0)
	defer cache.Stop()

	// Record initial last save time
	initialTime := cache.LastSaveTime()

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Mark dirty and flush
	cache.MarkDirty("book1")
	_, err := cache.Flush()
	if err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	// Check that last save time was updated
	newTime := cache.LastSaveTime()
	if !newTime.After(initialTime) {
		t.Error("last save time was not updated after flush")
	}
}

// TestLibraryCacheDuplicateDirtyMarks tests that duplicate dirty marks don't cause issues
func TestLibraryCacheDuplicateDirtyMarks(t *testing.T) {
	lib := &ComicLibrary{
		Books: []ComicBook{
			{ID: "book1", Title: "Test Book 1"},
		},
	}

	cache := NewLibraryCache(lib, "", 0)
	defer cache.Stop()

	// Mark the same book dirty multiple times
	cache.MarkDirty("book1")
	cache.MarkDirty("book1")
	cache.MarkDirty("book1")

	// Should only count as 1 dirty book (map deduplication)
	if cache.DirtyCount() != 1 {
		t.Errorf("expected 1 dirty book, got %d", cache.DirtyCount())
	}
}

// TestLibraryCacheConcurrentDirtyMarks tests concurrent dirty marking
func TestLibraryCacheConcurrentDirtyMarks(t *testing.T) {
	lib := &ComicLibrary{
		Books: []ComicBook{},
	}

	// Create 100 books
	for i := 0; i < 100; i++ {
		lib.Books = append(lib.Books, ComicBook{
			ID:    string(rune('A' + i)),
			Title: "Book " + string(rune('A'+i)),
		})
	}

	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "test.xml")

	cache := NewLibraryCache(lib, libraryPath, 0)
	defer cache.Stop()

	// Mark books dirty concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(offset int) {
			for j := 0; j < 10; j++ {
				bookID := string(rune('A' + offset + j))
				cache.MarkDirty(bookID)
			}
			done <- true
		}(i * 10)
	}

	// Wait for all goroutines to finish
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have marked 100 books as dirty
	if cache.DirtyCount() != 100 {
		t.Errorf("expected 100 dirty books, got %d", cache.DirtyCount())
	}

	// Flush should succeed
	saved, err := cache.Flush()
	if err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}
	if !saved {
		t.Error("expected library to be saved")
	}

	// Should have 0 dirty books after flush
	if cache.DirtyCount() != 0 {
		t.Errorf("expected 0 dirty books after flush, got %d", cache.DirtyCount())
	}
}

// BenchmarkLibraryCacheMarkDirty benchmarks marking books dirty
func BenchmarkLibraryCacheMarkDirty(b *testing.B) {
	lib := &ComicLibrary{
		Books: []ComicBook{
			{ID: "book1", Title: "Test Book 1"},
		},
	}

	cache := NewLibraryCache(lib, "", 0)
	defer cache.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.MarkDirty("book1")
	}
}

// BenchmarkLibraryCacheFlush benchmarks flushing the cache
func BenchmarkLibraryCacheFlush(b *testing.B) {
	lib := &ComicLibrary{
		Books: []ComicBook{
			{ID: "book1", Title: "Test Book 1"},
			{ID: "book2", Title: "Test Book 2"},
		},
	}

	tempDir := b.TempDir()
	libraryPath := filepath.Join(tempDir, "test.xml")

	cache := NewLibraryCache(lib, libraryPath, 0)
	defer cache.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Mark dirty before each flush
		cache.MarkDirty("book1")

		// Flush
		_, err := cache.Flush()
		if err != nil {
			b.Fatalf("Flush() failed: %v", err)
		}
	}
}
