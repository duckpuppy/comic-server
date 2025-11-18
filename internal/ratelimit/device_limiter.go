package ratelimit

import (
	"sync"
	"time"
)

// DeviceLimiter tracks request rates per device using a token bucket algorithm
// This provides smooth rate limiting and allows for bursts
type DeviceLimiter struct {
	mu             sync.RWMutex
	buckets        map[string]*tokenBucket // Device ID -> token bucket
	maxTokens      int                     // Maximum tokens (request capacity)
	refillRate     time.Duration           // Time per token refill
	cleanupTicker  *time.Ticker            // Periodic cleanup of inactive devices
	stopCleanup    chan struct{}           // Signal to stop cleanup goroutine
	inactiveExpiry time.Duration           // Time after which inactive devices are removed
}

// tokenBucket represents a token bucket for a single device
type tokenBucket struct {
	tokens     float64       // Current number of tokens
	maxTokens  int           // Maximum capacity
	refillRate time.Duration // Time to refill one token
	lastRefill time.Time     // Last time tokens were refilled
	lastAccess time.Time     // Last time this bucket was accessed (for cleanup)
}

// NewDeviceLimiter creates a new device rate limiter using token bucket algorithm
// maxRequests: maximum number of requests allowed per window
// window: time window (e.g., 1 minute for 100 requests/minute)
// The token bucket allows bursts up to maxRequests, then refills at a steady rate
func NewDeviceLimiter(maxRequests int, window time.Duration) *DeviceLimiter {
	limiter := &DeviceLimiter{
		buckets:        make(map[string]*tokenBucket),
		maxTokens:      maxRequests,
		refillRate:     window / time.Duration(maxRequests),
		cleanupTicker:  time.NewTicker(window),
		stopCleanup:    make(chan struct{}),
		inactiveExpiry: window * 2, // Remove inactive devices after 2 windows
	}

	// Start background cleanup goroutine
	go limiter.cleanupLoop()

	return limiter
}

// Allow checks if a request from the given device is allowed
// Returns true if allowed, false if rate limit exceeded
func (l *DeviceLimiter) Allow(deviceID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Get or create bucket for this device
	bucket, exists := l.buckets[deviceID]
	if !exists {
		bucket = &tokenBucket{
			tokens:     float64(l.maxTokens),
			maxTokens:  l.maxTokens,
			refillRate: l.refillRate,
			lastRefill: time.Now(),
			lastAccess: time.Now(),
		}
		l.buckets[deviceID] = bucket
	}

	// Refill tokens based on time elapsed
	bucket.refill()

	// Update last access time
	bucket.lastAccess = time.Now()

	// Check if we have tokens available
	if bucket.tokens >= 1.0 {
		bucket.tokens -= 1.0
		return true
	}

	// No tokens available - rate limited
	return false
}

// GetAvailableTokens returns the current number of tokens available for a device
func (l *DeviceLimiter) GetAvailableTokens(deviceID string) float64 {
	l.mu.RLock()
	defer l.mu.RUnlock()

	bucket, exists := l.buckets[deviceID]
	if !exists {
		return float64(l.maxTokens) // New device has full capacity
	}

	// Create a copy to avoid race conditions
	tempBucket := *bucket
	tempBucket.refill()
	return tempBucket.tokens
}

// Reset clears the rate limit data for a device
func (l *DeviceLimiter) Reset(deviceID string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.buckets, deviceID)
}

// ResetAll clears all rate limit data
func (l *DeviceLimiter) ResetAll() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.buckets = make(map[string]*tokenBucket)
}

// GetTrackedDevices returns a list of all devices currently being tracked
func (l *DeviceLimiter) GetTrackedDevices() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	devices := make([]string, 0, len(l.buckets))
	for deviceID := range l.buckets {
		devices = append(devices, deviceID)
	}

	return devices
}

// cleanupLoop periodically removes inactive devices to prevent memory growth
func (l *DeviceLimiter) cleanupLoop() {
	for {
		select {
		case <-l.cleanupTicker.C:
			l.cleanup()
		case <-l.stopCleanup:
			return
		}
	}
}

// cleanup removes buckets for devices that haven't been accessed recently
func (l *DeviceLimiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.inactiveExpiry)

	// Remove devices that haven't been accessed in a while
	for deviceID, bucket := range l.buckets {
		if bucket.lastAccess.Before(cutoff) {
			delete(l.buckets, deviceID)
		}
	}
}

// Stop stops the background cleanup goroutine
// Should be called when the limiter is no longer needed
func (l *DeviceLimiter) Stop() {
	close(l.stopCleanup)
	l.cleanupTicker.Stop()
}

// refill adds tokens to the bucket based on time elapsed
// Must be called with lock held
func (b *tokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(b.lastRefill)

	// Calculate how many tokens to add
	tokensToAdd := float64(elapsed) / float64(b.refillRate)

	if tokensToAdd > 0 {
		b.tokens += tokensToAdd
		if b.tokens > float64(b.maxTokens) {
			b.tokens = float64(b.maxTokens)
		}
		b.lastRefill = now
	}
}
