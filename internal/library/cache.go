package library

import (
	"sync"
	"time"
)

// ListCache caches smart list book counts to avoid expensive re-evaluation
type ListCache struct {
	counts map[string]*cacheEntry
	mu     sync.RWMutex
	ttl    time.Duration
}

type cacheEntry struct {
	count     int
	timestamp time.Time
}

// NewListCache creates a new list cache with the given TTL
func NewListCache(ttl time.Duration) *ListCache {
	return &ListCache{
		counts: make(map[string]*cacheEntry),
		ttl:    ttl,
	}
}

// GetCount retrieves a cached count if it exists and hasn't expired
func (c *ListCache) GetCount(listID string) (int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.counts[listID]
	if !exists {
		return 0, false
	}

	// Check if expired
	if time.Since(entry.timestamp) > c.ttl {
		return 0, false
	}

	return entry.count, true
}

// SetCount sets a count in the cache with current timestamp
func (c *ListCache) SetCount(listID string, count int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.counts[listID] = &cacheEntry{
		count:     count,
		timestamp: time.Now(),
	}
}

// Invalidate removes a specific list from cache
func (c *ListCache) Invalidate(listID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.counts, listID)
}

// InvalidateAll clears the entire cache
func (c *ListCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.counts = make(map[string]*cacheEntry)
}
