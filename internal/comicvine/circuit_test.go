package comicvine

import (
	"testing"
	"time"
)

func TestCircuitBreaker_StartsClosedAndAllows(t *testing.T) {
	cb := NewCircuitBreaker()
	if cb.State() != CircuitClosed {
		t.Fatalf("expected closed, got %s", cb.State())
	}
	if !cb.Allow() {
		t.Fatal("closed circuit should allow requests")
	}
}

func TestCircuitBreaker_OpensOnFailure(t *testing.T) {
	cb := NewCircuitBreaker(WithMinBackoff(100 * time.Millisecond))
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Fatalf("expected open, got %s", cb.State())
	}
	if cb.Allow() {
		t.Fatal("open circuit should not allow requests before backoff expires")
	}
	if cb.BackoffUntil().IsZero() {
		t.Fatal("backoff time should be set")
	}
}

func TestCircuitBreaker_TransitionsToHalfOpenAfterBackoff(t *testing.T) {
	cb := NewCircuitBreaker(WithMinBackoff(50 * time.Millisecond))
	cb.RecordFailure()

	time.Sleep(60 * time.Millisecond)

	if !cb.Allow() {
		t.Fatal("should allow after backoff expires")
	}
	if cb.State() != CircuitHalfOpen {
		t.Fatalf("expected half-open, got %s", cb.State())
	}
}

func TestCircuitBreaker_ClosesOnSuccessAfterHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(WithMinBackoff(10 * time.Millisecond))
	cb.RecordFailure()

	time.Sleep(20 * time.Millisecond)
	cb.Allow() // triggers half-open
	cb.RecordSuccess()

	if cb.State() != CircuitClosed {
		t.Fatalf("expected closed after half-open success, got %s", cb.State())
	}
}

func TestCircuitBreaker_ExponentialBackoff(t *testing.T) {
	cb := NewCircuitBreaker(
		WithMinBackoff(100*time.Millisecond),
		WithMaxBackoff(1*time.Second),
	)

	// First failure: 100ms backoff
	cb.RecordFailure()
	first := cb.BackoffUntil()

	time.Sleep(110 * time.Millisecond)
	cb.Allow() // half-open

	// Second failure in half-open: 200ms backoff
	cb.RecordFailure()
	second := cb.BackoffUntil()

	if !second.After(first) {
		t.Fatal("second backoff should be longer than first")
	}
}

func TestCircuitBreaker_MaxBackoffCap(t *testing.T) {
	cb := NewCircuitBreaker(
		WithMinBackoff(10*time.Millisecond),
		WithMaxBackoff(50*time.Millisecond),
	)

	// Trigger multiple failures to exceed max backoff
	for i := 0; i < 10; i++ {
		cb.RecordFailure()
		time.Sleep(15 * time.Millisecond)
		cb.Allow() // half-open
	}
	cb.RecordFailure()

	// Backoff should be capped
	until := cb.BackoffUntil()
	maxExpected := time.Now().Add(60 * time.Millisecond) // 50ms cap + some margin
	if until.After(maxExpected) {
		t.Fatalf("backoff %v exceeds max cap", until)
	}
}

func TestCircuitBreaker_ResetToClosedState(t *testing.T) {
	cb := NewCircuitBreaker(WithMinBackoff(1 * time.Hour))
	cb.RecordFailure()

	cb.Reset()
	if cb.State() != CircuitClosed {
		t.Fatalf("expected closed after reset, got %s", cb.State())
	}
	if !cb.Allow() {
		t.Fatal("should allow after reset")
	}
}

func TestCircuitBreaker_StateChangeCallback(t *testing.T) {
	var transitions []string
	cb := NewCircuitBreaker(
		WithMinBackoff(10*time.Millisecond),
		WithOnStateChange(func(from, to CircuitState) {
			transitions = append(transitions, from.String()+"→"+to.String())
		}),
	)

	cb.RecordFailure() // closed→open
	time.Sleep(15 * time.Millisecond)
	cb.Allow()         // open→half-open
	cb.RecordSuccess() // half-open→closed

	expected := []string{"closed→open", "open→half-open", "half-open→closed"}
	if len(transitions) != len(expected) {
		t.Fatalf("got %d transitions, want %d: %v", len(transitions), len(expected), transitions)
	}
	for i, exp := range expected {
		if transitions[i] != exp {
			t.Errorf("transition[%d] = %s, want %s", i, transitions[i], exp)
		}
	}
}
