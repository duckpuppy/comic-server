package library

import (
	"sync"
	"time"
)

// ListCache caches smart list book counts to avoid expensive re-evaluation.
//
// Invalidate/InvalidateAll don't delete entries - they mark them stale (see
// cacheEntry.timestamp) so GetCounts correctly reports a miss, while
// GetStaleCounts can still hand back the last-known value as an immediate
// fallback. Combined with TryBeginRefresh/EndRefresh, this lets a caller
// serve a possibly-stale count instantly and recompute in the background
// instead of blocking the request on a full recompute every time the cache
// is invalidated (e.g. on every library reload) - see comic-server-cg1.
type ListCache struct {
	counts map[string]*cacheEntry
	mu     sync.RWMutex
	ttl    time.Duration

	refreshMu  sync.Mutex
	refreshing map[string]bool // listIDs with a background refresh in flight
}

type cacheEntry struct {
	count     int
	unread    int
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

// SetCount stores a total count, preserving any cached unread count.
func (c *ListCache) SetCount(listID string, count int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	unread := 0
	if existing, ok := c.counts[listID]; ok {
		unread = existing.unread
	}
	c.counts[listID] = &cacheEntry{
		count:     count,
		unread:    unread,
		timestamp: time.Now(),
	}
}

// GetCounts retrieves both total and unread counts if cached and unexpired.
func (c *ListCache) GetCounts(listID string) (count, unread int, found bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.counts[listID]
	if !exists || time.Since(entry.timestamp) > c.ttl {
		return 0, 0, false
	}
	return entry.count, entry.unread, true
}

// SetCounts stores both total and unread counts.
func (c *ListCache) SetCounts(listID string, count, unread int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.counts[listID] = &cacheEntry{
		count:     count,
		unread:    unread,
		timestamp: time.Now(),
	}
}

// GetStaleCounts returns the last-known counts for listID regardless of TTL
// expiry - even one just marked stale by Invalidate/InvalidateAll. Use as an
// immediate fallback when GetCounts reports a miss, paired with
// TryBeginRefresh to trigger exactly one background recompute rather than
// blocking the caller.
func (c *ListCache) GetStaleCounts(listID string) (count, unread int, found bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.counts[listID]
	if !exists {
		return 0, 0, false
	}
	return entry.count, entry.unread, true
}

// TryBeginRefresh marks listID as having a background refresh in flight,
// returning true if the caller won the race and should perform the refresh
// (calling EndRefresh when done), or false if one is already running and
// the caller should skip starting a duplicate.
func (c *ListCache) TryBeginRefresh(listID string) bool {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	if c.refreshing == nil {
		c.refreshing = make(map[string]bool)
	}
	if c.refreshing[listID] {
		return false
	}
	c.refreshing[listID] = true
	return true
}

// EndRefresh clears the in-flight marker set by TryBeginRefresh.
func (c *ListCache) EndRefresh(listID string) {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()
	delete(c.refreshing, listID)
}

// Invalidate marks a specific list's cached count stale (GetCounts will miss
// on it) while keeping the last-known value available via GetStaleCounts.
func (c *ListCache) Invalidate(listID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.counts[listID]; ok {
		entry.timestamp = time.Time{}
	}
}

// InvalidateAll marks every cached count stale (GetCounts will miss on all
// of them) while keeping last-known values available via GetStaleCounts.
func (c *ListCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, entry := range c.counts {
		entry.timestamp = time.Time{}
	}
}
