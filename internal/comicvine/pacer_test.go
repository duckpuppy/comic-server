package comicvine

import (
	"context"
	"testing"
	"time"
)

func TestPacer_AllowsImmediateFirstRequest(t *testing.T) {
	p := NewPacer(WithBurstSize(5), WithBurstPause(1*time.Hour))

	start := time.Now()
	if err := p.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("first request should be immediate")
	}
}

func TestPacer_PausesAfterBurst(t *testing.T) {
	p := NewPacer(
		WithBurstSize(3),
		WithBurstPause(100*time.Millisecond),
		WithWindowSize(100),
		WithWindowPause(1*time.Hour),
	)

	// Simulate 3 requests
	for i := 0; i < 3; i++ {
		p.RecordRequest()
	}

	start := time.Now()
	if err := p.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	if elapsed < 90*time.Millisecond {
		t.Fatalf("expected burst pause (~100ms), got %v", elapsed)
	}
}

func TestPacer_CancelledContextDuringPause(t *testing.T) {
	p := NewPacer(
		WithBurstSize(1),
		WithBurstPause(10*time.Second),
		WithWindowSize(100),
		WithWindowPause(1*time.Hour),
	)
	p.RecordRequest()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := p.Wait(ctx)
	if err == nil {
		t.Fatal("expected context error during pause")
	}
}

func TestPacer_WindowPauseTakesPrecedence(t *testing.T) {
	p := NewPacer(
		WithBurstSize(5),
		WithBurstPause(50*time.Millisecond),
		WithWindowSize(3),
		WithWindowPause(150*time.Millisecond),
	)

	// Fill window
	for i := 0; i < 3; i++ {
		p.RecordRequest()
	}

	start := time.Now()
	if err := p.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	// Window pause (150ms) should fire, not burst pause (50ms)
	if elapsed < 140*time.Millisecond {
		t.Fatalf("expected window pause (~150ms), got %v", elapsed)
	}
}

func TestPacer_ResetClearsCounters(t *testing.T) {
	p := NewPacer(WithBurstSize(2), WithBurstPause(1*time.Hour))

	p.RecordRequest()
	p.RecordRequest()
	p.Reset()

	// After reset, next Wait should be immediate (not paused)
	start := time.Now()
	if err := p.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("should be immediate after reset")
	}
}

func TestPacer_Stats(t *testing.T) {
	p := NewPacer(WithBurstSize(10), WithWindowSize(100))
	p.RecordRequest()
	p.RecordRequest()

	burst, window := p.Stats()
	if burst != 2 || window != 2 {
		t.Fatalf("stats: burst=%d window=%d, want 2,2", burst, window)
	}
}
