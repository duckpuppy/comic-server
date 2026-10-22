package comicvine

import (
	"context"
	"strings"
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

func TestPacer_ReportsPauseToJobLog(t *testing.T) {
	p := NewPacer(WithBurstSize(1), WithBurstPause(10*time.Millisecond))
	p.RecordRequest()
	var got []string
	ctx := withJobLog(context.Background(), "job-1", func(jobID, level, msg string) {
		got = append(got, jobID+": "+msg)
	})
	if err := p.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !strings.Contains(got[0], "job-1: Waiting") || !strings.Contains(got[0], "rate limit") {
		t.Errorf("job log = %q", got)
	}
}

func TestPacer_ListsWaitingCallersWhileTheyWait(t *testing.T) {
	p := NewPacer(WithBurstSize(1), WithBurstPause(time.Hour), WithWindowSize(100), WithWindowPause(time.Hour))
	p.RecordRequest() // burst used up: the next caller must pause

	if got := p.Waiters(); len(got) != 0 {
		t.Fatalf("nobody waits yet, got %+v", got)
	}

	ctx, cancel := context.WithCancel(WithTask(WithBackground(context.Background()), "Test job"))
	done := make(chan error, 1)
	go func() { done <- p.Wait(ctx) }()

	var got []Waiter
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got = p.Waiters(); len(got) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 waiter, got %+v", got)
	}
	w := got[0]
	if w.Label != "Test job" || !w.Background || w.Reason != "burst" {
		t.Errorf("waiter = %+v", w)
	}
	if d := time.Until(w.RunsAt); d < 50*time.Minute || d > time.Hour+time.Minute {
		t.Errorf("RunsAt is %v away, want about an hour", d)
	}

	cancel()
	if err := <-done; err == nil {
		t.Error("Wait should return the context error")
	}
	if got := p.Waiters(); len(got) != 0 {
		t.Errorf("a cancelled caller must leave the list, got %+v", got)
	}
}

func TestPacer_UnnamedWaiterGetsDefaultLabelAndNoWaitMeansNoEntry(t *testing.T) {
	p := NewPacer(WithBurstSize(5))
	if err := p.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := p.Waiters(); len(got) != 0 {
		t.Errorf("a request that did not wait must not be listed: %+v", got)
	}
	if taskLabel(context.Background()) != "ComicVine request" {
		t.Error("default label changed")
	}
}

func TestPacer_ErrPaceWaitCallerIsNotListed(t *testing.T) {
	p := NewPacer(WithBurstSize(1), WithBurstPause(time.Hour), WithWindowSize(100), WithWindowPause(time.Hour))
	p.RecordRequest()
	err := p.Wait(WithMaxWait(context.Background(), time.Second))
	if _, ok := err.(ErrPaceWait); !ok {
		t.Fatalf("want ErrPaceWait, got %v", err)
	}
	if got := p.Waiters(); len(got) != 0 {
		t.Errorf("a caller that gave up at once must not be listed: %+v", got)
	}
}
