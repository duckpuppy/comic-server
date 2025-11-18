package library

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/duckpuppy/comic-server/internal/log"
)

// Prometheus metrics for library cache
var (
	cacheDirtyBooksGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "comic_server_library_cache_dirty_books",
			Help: "Current number of dirty books in library cache waiting to be flushed",
		},
	)
	cacheFlushesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "comic_server_library_cache_flushes_total",
			Help: "Total number of library cache flush operations by status",
		},
		[]string{"status"}, // "success" or "error"
	)
	cacheFlushDurationSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "comic_server_library_cache_flush_duration_seconds",
			Help:    "Duration of library cache flush operations in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 2.0, 5.0}, // 1ms to 5s
		},
	)
)

// LibraryCache provides an in-memory cache for library with periodic flushing to disk
// This reduces the frequency of expensive full library saves by batching updates.
type LibraryCache struct {
	mu            sync.RWMutex
	library       *ComicLibrary
	libraryPath   string
	dirtyBooks    map[string]bool // Track which books have changed
	lastSave      time.Time
	flushInterval time.Duration
	autoFlush     bool
	stopChan      chan struct{}
	wg            sync.WaitGroup
}

// NewLibraryCache creates a new library cache
// If flushInterval > 0, a background goroutine will periodically flush dirty books.
// If flushInterval == 0, flushing only happens on explicit Flush() calls or when MarkDirty() is called.
func NewLibraryCache(library *ComicLibrary, libraryPath string, flushInterval time.Duration) *LibraryCache {
	c := &LibraryCache{
		library:       library,
		libraryPath:   libraryPath,
		dirtyBooks:    make(map[string]bool),
		lastSave:      time.Now(),
		flushInterval: flushInterval,
		autoFlush:     flushInterval > 0,
		stopChan:      make(chan struct{}),
	}

	// Start background flusher if auto-flush is enabled
	if c.autoFlush {
		c.wg.Add(1)
		go c.backgroundFlusher()
	}

	return c
}

// GetLibrary returns the library (read-only access)
func (c *LibraryCache) GetLibrary() *ComicLibrary {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.library
}

// MarkDirty marks a book as changed
func (c *LibraryCache) MarkDirty(bookID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dirtyBooks[bookID] = true

	// Update metrics
	cacheDirtyBooksGauge.Set(float64(len(c.dirtyBooks)))

	// Auto-flush if enough time has passed and auto-flush is disabled
	// (when auto-flush is enabled, backgroundFlusher handles it)
	if !c.autoFlush && c.shouldFlush() {
		c.flushLocked()
	}
}

// MarkManyDirty marks multiple books as changed
func (c *LibraryCache) MarkManyDirty(bookIDs []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, bookID := range bookIDs {
		c.dirtyBooks[bookID] = true
	}

	// Update metrics
	cacheDirtyBooksGauge.Set(float64(len(c.dirtyBooks)))

	// Auto-flush if enough time has passed and auto-flush is disabled
	if !c.autoFlush && c.shouldFlush() {
		c.flushLocked()
	}
}

// Flush writes dirty books to disk if any changes exist
// Returns true if library was saved, false if no changes
func (c *LibraryCache) Flush() (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.flushLocked()
}

// flushLocked performs the actual flush (must be called with lock held)
func (c *LibraryCache) flushLocked() (bool, error) {
	if len(c.dirtyBooks) == 0 {
		return false, nil // No changes to save
	}

	log.Info().
		Int("dirty_books", len(c.dirtyBooks)).
		Str("library_path", c.libraryPath).
		Msg("Flushing library cache to disk")

	// Record flush start time for metrics
	startTime := time.Now()

	// Save entire library
	err := SaveLibrary(c.libraryPath, c.library)

	// Record flush duration
	duration := time.Since(startTime)
	cacheFlushDurationSeconds.Observe(duration.Seconds())

	if err != nil {
		log.Error().Err(err).Msg("Failed to flush library cache")
		cacheFlushesTotal.WithLabelValues("error").Inc()
		return false, err
	}

	// Clear dirty tracking
	c.dirtyBooks = make(map[string]bool)
	c.lastSave = time.Now()

	// Update metrics
	cacheDirtyBooksGauge.Set(0) // All books flushed
	cacheFlushesTotal.WithLabelValues("success").Inc()

	log.Info().Msg("Library cache flushed successfully")
	return true, nil
}

// shouldFlush checks if enough time has passed since last save
// Must be called with lock held
func (c *LibraryCache) shouldFlush() bool {
	if len(c.dirtyBooks) == 0 {
		return false
	}

	if c.flushInterval == 0 {
		return false // Auto-flush disabled
	}

	return time.Since(c.lastSave) >= c.flushInterval
}

// backgroundFlusher runs in a goroutine and periodically flushes dirty books
func (c *LibraryCache) backgroundFlusher() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

	log.Info().
		Dur("flush_interval", c.flushInterval).
		Msg("Library cache background flusher started")

	for {
		select {
		case <-ticker.C:
			c.mu.Lock()
			if len(c.dirtyBooks) > 0 {
				_, err := c.flushLocked()
				if err != nil {
					log.Error().Err(err).Msg("Background flush failed")
				}
			}
			c.mu.Unlock()

		case <-c.stopChan:
			log.Info().Msg("Library cache background flusher stopped")
			return
		}
	}
}

// Stop stops the background flusher and performs final flush
func (c *LibraryCache) Stop() error {
	if c.autoFlush {
		close(c.stopChan)
		c.wg.Wait()
	}

	// Final flush to ensure all changes are saved
	log.Info().Msg("Performing final library cache flush before shutdown")
	_, err := c.Flush()
	return err
}

// DirtyCount returns the number of dirty books
func (c *LibraryCache) DirtyCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.dirtyBooks)
}

// LastSaveTime returns the time of the last save
func (c *LibraryCache) LastSaveTime() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastSave
}
