package library

import (
	"testing"
	"time"
)

func TestListCache_GetCount(t *testing.T) {
	cache := NewListCache(5 * time.Minute)

	// Cache miss should return 0, false
	count, found := cache.GetCount("list-123")
	if found {
		t.Error("Expected cache miss for non-existent list")
	}
	if count != 0 {
		t.Errorf("Expected count 0, got %d", count)
	}
}

func TestListCache_SetAndGet(t *testing.T) {
	cache := NewListCache(5 * time.Minute)

	cache.SetCount("list-123", 2847)

	count, found := cache.GetCount("list-123")
	if !found {
		t.Error("Expected cache hit after SetCount")
	}
	if count != 2847 {
		t.Errorf("Expected count 2847, got %d", count)
	}
}

func TestListCache_Expiry(t *testing.T) {
	cache := NewListCache(100 * time.Millisecond)

	cache.SetCount("list-123", 2847)
	time.Sleep(150 * time.Millisecond)

	_, found := cache.GetCount("list-123")
	if found {
		t.Error("Expected cache miss after TTL expiry")
	}
}

// TestListCache_InvalidateKeepsStaleValue verifies Invalidate/InvalidateAll
// mark an entry stale (GetCounts misses) without discarding the last-known
// value, so GetStaleCounts can still serve it as an immediate fallback
// instead of forcing every post-invalidation request to block on a full
// recompute (comic-server-cg1).
func TestListCache_InvalidateKeepsStaleValue(t *testing.T) {
	cache := NewListCache(5 * time.Minute)
	cache.SetCounts("list-1", 42, 10)

	cache.Invalidate("list-1")

	if _, _, found := cache.GetCounts("list-1"); found {
		t.Error("expected GetCounts to miss after Invalidate")
	}
	count, unread, found := cache.GetStaleCounts("list-1")
	if !found || count != 42 || unread != 10 {
		t.Errorf("expected GetStaleCounts to still return (42, 10), got (%d, %d, %v)", count, unread, found)
	}
}

func TestListCache_InvalidateAllKeepsStaleValues(t *testing.T) {
	cache := NewListCache(5 * time.Minute)
	cache.SetCounts("list-1", 42, 10)
	cache.SetCounts("list-2", 7, 1)

	cache.InvalidateAll()

	for _, id := range []string{"list-1", "list-2"} {
		if _, _, found := cache.GetCounts(id); found {
			t.Errorf("expected GetCounts(%s) to miss after InvalidateAll", id)
		}
		if _, _, found := cache.GetStaleCounts(id); !found {
			t.Errorf("expected GetStaleCounts(%s) to still find a stale value after InvalidateAll", id)
		}
	}
}

func TestListCache_TryBeginRefresh(t *testing.T) {
	cache := NewListCache(5 * time.Minute)

	if !cache.TryBeginRefresh("list-1") {
		t.Fatal("expected the first TryBeginRefresh to win")
	}
	if cache.TryBeginRefresh("list-1") {
		t.Error("expected a second concurrent TryBeginRefresh for the same list to lose")
	}

	cache.EndRefresh("list-1")

	if !cache.TryBeginRefresh("list-1") {
		t.Error("expected TryBeginRefresh to succeed again after EndRefresh")
	}
}

func TestListCache_SizeSetGetAndMiss(t *testing.T) {
	cache := NewListCache(5 * time.Minute)

	if _, _, found := cache.GetSize("list-1"); found {
		t.Fatal("expected a miss before SetSize")
	}

	cache.SetSize("list-1", 1614, 2)
	bytes, unknown, found := cache.GetSize("list-1")
	if !found || bytes != 1614 || unknown != 2 {
		t.Errorf("GetSize = (%d, %d, %v), want (1614, 2, true)", bytes, unknown, found)
	}
}

func TestListCache_SizeExpires(t *testing.T) {
	cache := NewListCache(50 * time.Millisecond)
	cache.SetSize("list-1", 10, 0)
	time.Sleep(80 * time.Millisecond)

	if _, _, found := cache.GetSize("list-1"); found {
		t.Error("expected the size to expire with the TTL")
	}
}

func TestListCache_InvalidateClearsSize(t *testing.T) {
	cache := NewListCache(5 * time.Minute)
	cache.SetSize("list-1", 10, 0)
	cache.SetSize("list-2", 20, 0)

	cache.Invalidate("list-1")
	if _, _, found := cache.GetSize("list-1"); found {
		t.Error("Invalidate should drop the size of that list")
	}
	if _, _, found := cache.GetSize("list-2"); !found {
		t.Error("Invalidate should leave other lists alone")
	}

	cache.InvalidateAll()
	if _, _, found := cache.GetSize("list-2"); found {
		t.Error("InvalidateAll should drop every size")
	}
}

func TestListCache_ZeroValueSetSizeDoesNotPanic(t *testing.T) {
	var cache ListCache
	cache.SetSize("list-1", 5, 0)
	cache.InvalidateAll()
}
