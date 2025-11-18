package ratelimit

import (
	"sync"
	"time"
)

// IPLimiter tracks connection attempts per IP address using a sliding window algorithm
type IPLimiter struct {
	mu             sync.RWMutex
	attempts       map[string][]time.Time // IP -> list of attempt timestamps
	maxAttempts    int                    // Maximum attempts allowed per window
	windowDuration time.Duration          // Time window for rate limiting
	cleanupTicker  *time.Ticker           // Periodic cleanup of expired entries
	stopCleanup    chan struct{}          // Signal to stop cleanup goroutine
}

// NewIPLimiter creates a new IP rate limiter
// maxAttempts: maximum number of connection attempts allowed per window
// windowDuration: time window for counting attempts (e.g., 1 minute)
func NewIPLimiter(maxAttempts int, windowDuration time.Duration) *IPLimiter {
	limiter := &IPLimiter{
		attempts:       make(map[string][]time.Time),
		maxAttempts:    maxAttempts,
		windowDuration: windowDuration,
		cleanupTicker:  time.NewTicker(windowDuration),
		stopCleanup:    make(chan struct{}),
	}

	// Start background cleanup goroutine
	go limiter.cleanupLoop()

	return limiter
}

// Allow checks if a connection attempt from the given IP is allowed
// Returns true if allowed, false if rate limit exceeded
func (l *IPLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.windowDuration)

	// Get or create attempt list for this IP
	attempts, exists := l.attempts[ip]
	if !exists {
		attempts = make([]time.Time, 0, l.maxAttempts)
	}

	// Remove expired attempts (outside the sliding window)
	validAttempts := make([]time.Time, 0, len(attempts))
	for _, attempt := range attempts {
		if attempt.After(cutoff) {
			validAttempts = append(validAttempts, attempt)
		}
	}

	// Check if we're at the limit
	if len(validAttempts) >= l.maxAttempts {
		// Update the list even though we're rejecting (for accurate tracking)
		l.attempts[ip] = validAttempts
		return false
	}

	// Allow the attempt and record it
	validAttempts = append(validAttempts, now)
	l.attempts[ip] = validAttempts
	return true
}

// GetAttemptCount returns the current number of attempts for an IP within the window
func (l *IPLimiter) GetAttemptCount(ip string) int {
	l.mu.RLock()
	defer l.mu.RUnlock()

	attempts, exists := l.attempts[ip]
	if !exists {
		return 0
	}

	now := time.Now()
	cutoff := now.Add(-l.windowDuration)

	count := 0
	for _, attempt := range attempts {
		if attempt.After(cutoff) {
			count++
		}
	}

	return count
}

// Reset clears all rate limit data for an IP address
func (l *IPLimiter) Reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.attempts, ip)
}

// ResetAll clears all rate limit data
func (l *IPLimiter) ResetAll() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.attempts = make(map[string][]time.Time)
}

// GetTrackedIPs returns a list of all IPs currently being tracked
func (l *IPLimiter) GetTrackedIPs() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	ips := make([]string, 0, len(l.attempts))
	for ip := range l.attempts {
		ips = append(ips, ip)
	}

	return ips
}

// cleanupLoop periodically removes expired entries to prevent memory growth
func (l *IPLimiter) cleanupLoop() {
	for {
		select {
		case <-l.cleanupTicker.C:
			l.cleanup()
		case <-l.stopCleanup:
			return
		}
	}
}

// cleanup removes entries that have no valid attempts within the window
func (l *IPLimiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.windowDuration)

	// Remove IPs with no valid attempts
	for ip, attempts := range l.attempts {
		validAttempts := make([]time.Time, 0, len(attempts))
		for _, attempt := range attempts {
			if attempt.After(cutoff) {
				validAttempts = append(validAttempts, attempt)
			}
		}

		if len(validAttempts) == 0 {
			delete(l.attempts, ip)
		} else {
			l.attempts[ip] = validAttempts
		}
	}
}

// Stop stops the background cleanup goroutine
// Should be called when the limiter is no longer needed
func (l *IPLimiter) Stop() {
	close(l.stopCleanup)
	l.cleanupTicker.Stop()
}
