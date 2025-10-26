package ratelimit

import (
	"testing"
	"time"
)

func TestIPLimiter_Allow(t *testing.T) {
	// Create limiter: 3 attempts per second
	limiter := NewIPLimiter(3, 1*time.Second)
	defer limiter.Stop()

	ip := "192.168.1.100"

	// First 3 attempts should be allowed
	for i := 1; i <= 3; i++ {
		if !limiter.Allow(ip) {
			t.Errorf("Attempt %d should be allowed", i)
		}
	}

	// 4th attempt should be denied
	if limiter.Allow(ip) {
		t.Error("4th attempt should be denied (rate limit exceeded)")
	}

	// Wait for window to expire
	time.Sleep(1100 * time.Millisecond)

	// Should be allowed again
	if !limiter.Allow(ip) {
		t.Error("Attempt after window expiry should be allowed")
	}
}

func TestIPLimiter_MultipleIPs(t *testing.T) {
	limiter := NewIPLimiter(2, 1*time.Second)
	defer limiter.Stop()

	ip1 := "192.168.1.100"
	ip2 := "192.168.1.101"

	// Both IPs should get 2 attempts
	if !limiter.Allow(ip1) {
		t.Error("IP1 attempt 1 should be allowed")
	}
	if !limiter.Allow(ip2) {
		t.Error("IP2 attempt 1 should be allowed")
	}
	if !limiter.Allow(ip1) {
		t.Error("IP1 attempt 2 should be allowed")
	}
	if !limiter.Allow(ip2) {
		t.Error("IP2 attempt 2 should be allowed")
	}

	// Both should be rate limited
	if limiter.Allow(ip1) {
		t.Error("IP1 should be rate limited")
	}
	if limiter.Allow(ip2) {
		t.Error("IP2 should be rate limited")
	}
}

func TestIPLimiter_SlidingWindow(t *testing.T) {
	// Create limiter: 2 attempts per 500ms
	limiter := NewIPLimiter(2, 500*time.Millisecond)
	defer limiter.Stop()

	ip := "192.168.1.100"

	// Use both attempts
	if !limiter.Allow(ip) {
		t.Error("Attempt 1 should be allowed")
	}
	if !limiter.Allow(ip) {
		t.Error("Attempt 2 should be allowed")
	}

	// Should be denied now
	if limiter.Allow(ip) {
		t.Error("Attempt 3 should be denied")
	}

	// Wait for first attempt to expire (250ms + some buffer)
	time.Sleep(300 * time.Millisecond)

	// Still should be denied (only 1 slot freed)
	if limiter.Allow(ip) {
		t.Error("Attempt 4 should still be denied (sliding window)")
	}

	// Wait for second attempt to expire
	time.Sleep(300 * time.Millisecond)

	// Now should be allowed
	if !limiter.Allow(ip) {
		t.Error("Attempt 5 should be allowed after window slides")
	}
}

func TestIPLimiter_GetAttemptCount(t *testing.T) {
	limiter := NewIPLimiter(5, 1*time.Second)
	defer limiter.Stop()

	ip := "192.168.1.100"

	// Initially zero
	if count := limiter.GetAttemptCount(ip); count != 0 {
		t.Errorf("Expected 0 attempts, got %d", count)
	}

	// Make some attempts
	limiter.Allow(ip)
	limiter.Allow(ip)
	limiter.Allow(ip)

	if count := limiter.GetAttemptCount(ip); count != 3 {
		t.Errorf("Expected 3 attempts, got %d", count)
	}

	// Wait for window to expire
	time.Sleep(1100 * time.Millisecond)

	// Count should be zero (expired)
	if count := limiter.GetAttemptCount(ip); count != 0 {
		t.Errorf("Expected 0 attempts after expiry, got %d", count)
	}
}

func TestIPLimiter_Reset(t *testing.T) {
	limiter := NewIPLimiter(2, 1*time.Second)
	defer limiter.Stop()

	ip := "192.168.1.100"

	// Use up limit
	limiter.Allow(ip)
	limiter.Allow(ip)

	// Should be denied
	if limiter.Allow(ip) {
		t.Error("Should be rate limited")
	}

	// Reset this IP
	limiter.Reset(ip)

	// Should be allowed again
	if !limiter.Allow(ip) {
		t.Error("Should be allowed after reset")
	}
}

func TestIPLimiter_ResetAll(t *testing.T) {
	limiter := NewIPLimiter(1, 1*time.Second)
	defer limiter.Stop()

	ip1 := "192.168.1.100"
	ip2 := "192.168.1.101"

	// Use up limits for both
	limiter.Allow(ip1)
	limiter.Allow(ip2)

	// Both should be denied
	if limiter.Allow(ip1) {
		t.Error("IP1 should be rate limited")
	}
	if limiter.Allow(ip2) {
		t.Error("IP2 should be rate limited")
	}

	// Reset all
	limiter.ResetAll()

	// Both should be allowed
	if !limiter.Allow(ip1) {
		t.Error("IP1 should be allowed after reset all")
	}
	if !limiter.Allow(ip2) {
		t.Error("IP2 should be allowed after reset all")
	}
}

func TestIPLimiter_GetTrackedIPs(t *testing.T) {
	limiter := NewIPLimiter(5, 1*time.Second)
	defer limiter.Stop()

	// Initially empty
	if ips := limiter.GetTrackedIPs(); len(ips) != 0 {
		t.Errorf("Expected 0 tracked IPs, got %d", len(ips))
	}

	// Make some attempts
	limiter.Allow("192.168.1.100")
	limiter.Allow("192.168.1.101")
	limiter.Allow("192.168.1.100")

	ips := limiter.GetTrackedIPs()
	if len(ips) != 2 {
		t.Errorf("Expected 2 tracked IPs, got %d", len(ips))
	}

	// Check that the IPs are in the list
	ipMap := make(map[string]bool)
	for _, ip := range ips {
		ipMap[ip] = true
	}

	if !ipMap["192.168.1.100"] {
		t.Error("Expected to find 192.168.1.100 in tracked IPs")
	}
	if !ipMap["192.168.1.101"] {
		t.Error("Expected to find 192.168.1.101 in tracked IPs")
	}
}

func TestIPLimiter_Cleanup(t *testing.T) {
	// Create limiter with short window for faster testing
	limiter := NewIPLimiter(3, 100*time.Millisecond)
	defer limiter.Stop()

	ip := "192.168.1.100"

	// Make some attempts
	limiter.Allow(ip)
	limiter.Allow(ip)

	// Should be tracked
	if len(limiter.GetTrackedIPs()) != 1 {
		t.Error("IP should be tracked")
	}

	// Wait for window to expire and cleanup to run
	time.Sleep(250 * time.Millisecond)

	// Manually trigger cleanup to ensure it runs
	limiter.cleanup()

	// IP should be removed (no valid attempts within window)
	if len(limiter.GetTrackedIPs()) != 0 {
		t.Error("IP should be cleaned up after expiry")
	}
}

func TestIPLimiter_ConcurrentAccess(t *testing.T) {
	limiter := NewIPLimiter(100, 1*time.Second)
	defer limiter.Stop()

	done := make(chan bool)

	// Spawn multiple goroutines trying to access the limiter
	for i := 0; i < 10; i++ {
		go func(id int) {
			ip := "192.168.1.100"
			for j := 0; j < 10; j++ {
				limiter.Allow(ip)
				limiter.GetAttemptCount(ip)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic or deadlock
	// Total attempts = 10 goroutines * 10 attempts = 100
	count := limiter.GetAttemptCount("192.168.1.100")
	if count != 100 {
		t.Errorf("Expected 100 attempts, got %d", count)
	}
}

func TestIPLimiter_ZeroAttempts(t *testing.T) {
	// Limiter that allows no attempts
	limiter := NewIPLimiter(0, 1*time.Second)
	defer limiter.Stop()

	ip := "192.168.1.100"

	// Should always deny
	if limiter.Allow(ip) {
		t.Error("Should deny when max attempts is 0")
	}
}

func TestIPLimiter_LargeWindow(t *testing.T) {
	// Test with a larger time window
	limiter := NewIPLimiter(5, 10*time.Second)
	defer limiter.Stop()

	ip := "192.168.1.100"

	// Make all 5 attempts
	for i := 1; i <= 5; i++ {
		if !limiter.Allow(ip) {
			t.Errorf("Attempt %d should be allowed", i)
		}
	}

	// 6th should be denied
	if limiter.Allow(ip) {
		t.Error("6th attempt should be denied")
	}

	// Should still be tracked
	if len(limiter.GetTrackedIPs()) != 1 {
		t.Error("IP should still be tracked")
	}
}
