package ratelimit

import (
	"testing"
	"time"
)

func TestDeviceLimiter_Allow(t *testing.T) {
	// Create limiter: 5 requests per second (token bucket)
	limiter := NewDeviceLimiter(5, 1*time.Second)
	defer limiter.Stop()

	deviceID := "device-123"

	// Should allow initial burst of 5 requests
	for i := 1; i <= 5; i++ {
		if !limiter.Allow(deviceID) {
			t.Errorf("Request %d should be allowed (initial burst)", i)
		}
	}

	// 6th request should be denied (bucket empty)
	if limiter.Allow(deviceID) {
		t.Error("6th request should be denied (bucket empty)")
	}

	// Wait for one token to refill (1 second / 5 tokens = 200ms per token)
	time.Sleep(250 * time.Millisecond)

	// Should allow 1 more request now
	if !limiter.Allow(deviceID) {
		t.Error("Request after token refill should be allowed")
	}

	// Should be denied again (bucket empty)
	if limiter.Allow(deviceID) {
		t.Error("Next request should be denied (bucket empty again)")
	}
}

func TestDeviceLimiter_MultipleDevices(t *testing.T) {
	limiter := NewDeviceLimiter(3, 1*time.Second)
	defer limiter.Stop()

	device1 := "device-1"
	device2 := "device-2"

	// Both devices should get their own buckets
	for i := 1; i <= 3; i++ {
		if !limiter.Allow(device1) {
			t.Errorf("Device 1 request %d should be allowed", i)
		}
		if !limiter.Allow(device2) {
			t.Errorf("Device 2 request %d should be allowed", i)
		}
	}

	// Both should be rate limited
	if limiter.Allow(device1) {
		t.Error("Device 1 should be rate limited")
	}
	if limiter.Allow(device2) {
		t.Error("Device 2 should be rate limited")
	}
}

func TestDeviceLimiter_TokenRefill(t *testing.T) {
	// Create limiter: 10 requests per second (100ms per token)
	limiter := NewDeviceLimiter(10, 1*time.Second)
	defer limiter.Stop()

	deviceID := "device-123"

	// Use all tokens
	for i := 1; i <= 10; i++ {
		if !limiter.Allow(deviceID) {
			t.Errorf("Request %d should be allowed", i)
		}
	}

	// Should be denied
	if limiter.Allow(deviceID) {
		t.Error("Should be rate limited after using all tokens")
	}

	// Wait for tokens to refill (500ms = ~5 tokens)
	time.Sleep(550 * time.Millisecond)

	// Should allow about 5 more requests
	allowed := 0
	for i := 0; i < 7; i++ {
		if limiter.Allow(deviceID) {
			allowed++
		}
	}

	// Should have allowed roughly 5 requests (give or take 1 for timing)
	if allowed < 4 || allowed > 6 {
		t.Errorf("Expected ~5 requests after 500ms refill, got %d", allowed)
	}
}

func TestDeviceLimiter_GetAvailableTokens(t *testing.T) {
	limiter := NewDeviceLimiter(5, 1*time.Second)
	defer limiter.Stop()

	deviceID := "device-123"

	// Initially should have full capacity
	tokens := limiter.GetAvailableTokens(deviceID)
	if tokens != 5.0 {
		t.Errorf("Expected 5 tokens initially, got %f", tokens)
	}

	// Use 3 tokens
	for i := 0; i < 3; i++ {
		limiter.Allow(deviceID)
	}

	// Should have 2 tokens left
	tokens = limiter.GetAvailableTokens(deviceID)
	if tokens < 1.9 || tokens > 2.1 {
		t.Errorf("Expected ~2 tokens after using 3, got %f", tokens)
	}

	// Use remaining tokens
	limiter.Allow(deviceID)
	limiter.Allow(deviceID)

	// Should have close to 0 tokens
	tokens = limiter.GetAvailableTokens(deviceID)
	if tokens > 0.2 {
		t.Errorf("Expected ~0 tokens after using all, got %f", tokens)
	}

	// Wait for refill
	time.Sleep(250 * time.Millisecond)

	// Should have some tokens refilled
	tokens = limiter.GetAvailableTokens(deviceID)
	if tokens < 0.5 || tokens > 2.0 {
		t.Errorf("Expected ~1 token after 200ms, got %f", tokens)
	}
}

func TestDeviceLimiter_Reset(t *testing.T) {
	limiter := NewDeviceLimiter(3, 1*time.Second)
	defer limiter.Stop()

	deviceID := "device-123"

	// Use all tokens
	limiter.Allow(deviceID)
	limiter.Allow(deviceID)
	limiter.Allow(deviceID)

	// Should be denied
	if limiter.Allow(deviceID) {
		t.Error("Should be rate limited")
	}

	// Reset this device
	limiter.Reset(deviceID)

	// Should have full capacity again
	tokens := limiter.GetAvailableTokens(deviceID)
	if tokens != 3.0 {
		t.Errorf("Expected 3 tokens after reset, got %f", tokens)
	}

	// Should be allowed
	if !limiter.Allow(deviceID) {
		t.Error("Should be allowed after reset")
	}
}

func TestDeviceLimiter_ResetAll(t *testing.T) {
	limiter := NewDeviceLimiter(2, 1*time.Second)
	defer limiter.Stop()

	device1 := "device-1"
	device2 := "device-2"

	// Use all tokens for both devices
	limiter.Allow(device1)
	limiter.Allow(device1)
	limiter.Allow(device2)
	limiter.Allow(device2)

	// Both should be denied
	if limiter.Allow(device1) {
		t.Error("Device 1 should be rate limited")
	}
	if limiter.Allow(device2) {
		t.Error("Device 2 should be rate limited")
	}

	// Reset all
	limiter.ResetAll()

	// Both should have full capacity
	if !limiter.Allow(device1) {
		t.Error("Device 1 should be allowed after reset all")
	}
	if !limiter.Allow(device2) {
		t.Error("Device 2 should be allowed after reset all")
	}
}

func TestDeviceLimiter_GetTrackedDevices(t *testing.T) {
	limiter := NewDeviceLimiter(5, 1*time.Second)
	defer limiter.Stop()

	// Initially empty
	if devices := limiter.GetTrackedDevices(); len(devices) != 0 {
		t.Errorf("Expected 0 tracked devices, got %d", len(devices))
	}

	// Make some requests
	limiter.Allow("device-1")
	limiter.Allow("device-2")
	limiter.Allow("device-1")

	devices := limiter.GetTrackedDevices()
	if len(devices) != 2 {
		t.Errorf("Expected 2 tracked devices, got %d", len(devices))
	}

	// Check that the devices are in the list
	deviceMap := make(map[string]bool)
	for _, device := range devices {
		deviceMap[device] = true
	}

	if !deviceMap["device-1"] {
		t.Error("Expected to find device-1 in tracked devices")
	}
	if !deviceMap["device-2"] {
		t.Error("Expected to find device-2 in tracked devices")
	}
}

func TestDeviceLimiter_Cleanup(t *testing.T) {
	// Create limiter with 1 second window, 2 second inactivity expiry
	limiter := NewDeviceLimiter(5, 1*time.Second)
	defer limiter.Stop()

	deviceID := "device-123"

	// Make a request
	limiter.Allow(deviceID)

	// Should be tracked
	if len(limiter.GetTrackedDevices()) != 1 {
		t.Error("Device should be tracked")
	}

	// Manually set last access to past (simulate inactivity)
	limiter.mu.Lock()
	limiter.buckets[deviceID].lastAccess = time.Now().Add(-3 * time.Second)
	limiter.mu.Unlock()

	// Trigger cleanup
	limiter.cleanup()

	// Device should be removed
	if len(limiter.GetTrackedDevices()) != 0 {
		t.Error("Inactive device should be cleaned up")
	}
}

func TestDeviceLimiter_ConcurrentAccess(t *testing.T) {
	limiter := NewDeviceLimiter(100, 1*time.Second)
	defer limiter.Stop()

	done := make(chan bool)

	// Spawn multiple goroutines trying to access the limiter
	for i := 0; i < 10; i++ {
		go func(id int) {
			deviceID := "device-123"
			for j := 0; j < 10; j++ {
				limiter.Allow(deviceID)
				limiter.GetAvailableTokens(deviceID)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic or deadlock
	// Should have used all 100 tokens
	tokens := limiter.GetAvailableTokens("device-123")
	if tokens > 10 {
		t.Errorf("Expected most tokens to be used, but %f remain", tokens)
	}
}

func TestDeviceLimiter_BurstBehavior(t *testing.T) {
	// Test that token bucket allows bursts up to capacity
	limiter := NewDeviceLimiter(10, 1*time.Second)
	defer limiter.Stop()

	deviceID := "device-123"

	// Wait a bit to ensure full bucket
	time.Sleep(100 * time.Millisecond)

	// Should allow immediate burst of 10 requests
	start := time.Now()
	allowed := 0
	for i := 0; i < 15; i++ {
		if limiter.Allow(deviceID) {
			allowed++
		}
	}
	elapsed := time.Since(start)

	// Should have allowed exactly 10 requests in a very short time
	if allowed != 10 {
		t.Errorf("Expected burst of 10 requests, got %d", allowed)
	}

	// Burst should complete very quickly (much less than 1 second)
	if elapsed > 100*time.Millisecond {
		t.Errorf("Burst should be fast, took %v", elapsed)
	}
}

func TestDeviceLimiter_SteadyState(t *testing.T) {
	// Test that token bucket maintains steady rate after burst
	limiter := NewDeviceLimiter(5, 1*time.Second) // 200ms per token
	defer limiter.Stop()

	deviceID := "device-123"

	// Use initial burst
	for i := 0; i < 5; i++ {
		limiter.Allow(deviceID)
	}

	// Now should get ~1 request per 200ms
	time.Sleep(250 * time.Millisecond)
	if !limiter.Allow(deviceID) {
		t.Error("Should allow 1 request after 200ms refill")
	}

	time.Sleep(250 * time.Millisecond)
	if !limiter.Allow(deviceID) {
		t.Error("Should allow 1 request after another 200ms refill")
	}
}
