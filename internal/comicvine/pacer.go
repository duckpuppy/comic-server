package comicvine

import (
	"context"
	"sync"
	"time"
)

const (
	defaultBurstSize   = 20
	defaultBurstPause  = 5 * time.Minute
	defaultWindowSize  = 200
	defaultWindowPause = 30 * time.Minute
)

// Pacer implements burst-and-pause rate limiting to avoid triggering ComicVine's
// velocity detection. Rather than a steady drip of requests, it allows short bursts
// followed by pauses.
//
// Pattern: send BurstSize requests, pause BurstPause, repeat. After WindowSize
// total requests, take a longer WindowPause before continuing.
type Pacer struct {
	mu          sync.Mutex
	burstSize   int
	burstPause  time.Duration
	windowSize  int
	windowPause time.Duration
	burstCount  int // requests in current burst
	windowCount int // requests in current window
}

// PacerOption configures a Pacer.
type PacerOption func(*Pacer)

func WithBurstSize(n int) PacerOption {
	return func(p *Pacer) { p.burstSize = n }
}

func WithBurstPause(d time.Duration) PacerOption {
	return func(p *Pacer) { p.burstPause = d }
}

func WithWindowSize(n int) PacerOption {
	return func(p *Pacer) { p.windowSize = n }
}

func WithWindowPause(d time.Duration) PacerOption {
	return func(p *Pacer) { p.windowPause = d }
}

// NewPacer creates a Pacer with the given options.
func NewPacer(opts ...PacerOption) *Pacer {
	p := &Pacer{
		burstSize:   defaultBurstSize,
		burstPause:  defaultBurstPause,
		windowSize:  defaultWindowSize,
		windowPause: defaultWindowPause,
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Wait blocks until the next request is allowed, respecting burst and window limits.
// Returns an error if the context is cancelled while waiting.
func (p *Pacer) Wait(ctx context.Context) error {
	p.mu.Lock()
	var pause time.Duration

	if p.windowCount > 0 && p.windowCount%p.windowSize == 0 {
		pause = p.windowPause
		p.burstCount = 0
	} else if p.burstCount > 0 && p.burstCount%p.burstSize == 0 {
		pause = p.burstPause
		p.burstCount = 0
	}
	p.mu.Unlock()

	if pause > 0 {
		select {
		case <-time.After(pause):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

// RecordRequest increments the burst and window counters. Call after a successful request.
func (p *Pacer) RecordRequest() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.burstCount++
	p.windowCount++
}

// Reset clears all counters (e.g. after a long backoff period).
func (p *Pacer) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.burstCount = 0
	p.windowCount = 0
}

// Stats returns the current burst and window counts.
func (p *Pacer) Stats() (burstCount, windowCount int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.burstCount, p.windowCount
}
