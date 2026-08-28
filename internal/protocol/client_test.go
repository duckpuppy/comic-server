package protocol

import (
	"testing"
	"time"
)

// TestDataTimeout_ScalesWithSize is the regression test for a real bug
// found live 2026-08-27: WriteFile/ReadFile used the flat default
// receiveTimeout (5s) as an absolute deadline for the ENTIRE data
// transfer, not just the small protocol messages (a command byte, a
// filename) it was sized for. A real comic book averaged ~90MB in one
// observed sync - nowhere near transferable in 5 seconds over any WiFi,
// entirely independent of actual flakiness. dataTimeout now scales the
// deadline with the payload size instead.
func TestDataTimeout_ScalesWithSize(t *testing.T) {
	c := &Client{receiveTimeout: 5 * time.Second}

	if got := c.dataTimeout(1024); got != 5*time.Second {
		t.Errorf("a small payload should use the default receiveTimeout, got %v", got)
	}

	// 90MB at the 1MB/s conservative floor needs ~90s - far more than the
	// 5s default, and the whole point of this fix.
	got := c.dataTimeout(90 * 1024 * 1024)
	if got < 89*time.Second {
		t.Errorf("a 90MB payload should get roughly 90s, got %v", got)
	}
}
