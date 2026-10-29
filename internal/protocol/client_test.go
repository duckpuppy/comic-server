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

// UseSlowLink (tracker #57): a relay that moves under 1 MB/s must not fail
// a large book, and its message timeouts are loosened.
func TestUseSlowLink_ScalesDataTimeout(t *testing.T) {
	const book = 90 * 1024 * 1024 // a typical 90 MB book

	normal := NewClient("127.0.0.1", 7614)
	if got := normal.dataTimeout(book); got != 90*time.Second {
		t.Errorf("normal dataTimeout = %v, want 90s", got)
	}

	slow := NewClient("127.0.0.1", 7614)
	slow.UseSlowLink()
	if got := slow.dataTimeout(book); got != 720*time.Second {
		t.Errorf("slow dataTimeout = %v, want 720s", got)
	}
	if got := slow.dataTimeout(1024); got != slowLinkIOTimeout {
		t.Errorf("slow small-message timeout = %v, want %v", got, slowLinkIOTimeout)
	}
	if slow.connectionTimeout != slowLinkConnectionTimeout {
		t.Errorf("connection timeout = %v", slow.connectionTimeout)
	}
	if slow.retries != normal.retries {
		t.Errorf("UseSlowLink must not raise client retries (a retried WriteFile resends the whole file): %d vs %d", slow.retries, normal.retries)
	}
}
