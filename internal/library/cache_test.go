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
