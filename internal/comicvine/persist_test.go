package comicvine

import (
	"context"
	"testing"
	"time"
)

type memStore map[string]string

func (m memStore) GetSyncState(k string) (string, error) { return m[k], nil }
func (m memStore) SetSyncState(k, v string) error        { m[k] = v; return nil }

func TestPacer_RestartKeepsBurstPause(t *testing.T) {
	store := memStore{}
	opts := []PacerOption{WithBurstSize(3), WithBurstPause(300 * time.Millisecond), WithWindowSize(100), WithWindowPause(time.Hour), WithPacerStore(store)}
	p1 := NewPacer(opts...)
	for i := 0; i < 3; i++ {
		p1.RecordRequest()
	}

	p2 := NewPacer(opts...) // "restart"
	if got := p2.RequestsLastHour(); got != 3 {
		t.Fatalf("requests last hour = %d, want 3", got)
	}
	start := time.Now()
	if err := p2.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d < 200*time.Millisecond {
		t.Fatalf("restarted pacer did not wait, waited %s", d)
	}
}

func TestPacer_RestartAfterPauseServedDoesNotWait(t *testing.T) {
	store := memStore{}
	opts := []PacerOption{WithBurstSize(3), WithBurstPause(50 * time.Millisecond), WithPacerStore(store)}
	p1 := NewPacer(opts...)
	for i := 0; i < 3; i++ {
		p1.RecordRequest()
	}
	time.Sleep(80 * time.Millisecond)
	p2 := NewPacer(opts...)
	start := time.Now()
	if err := p2.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 30*time.Millisecond {
		t.Fatal("pause already served, should not wait")
	}
}

func TestCircuit_RestartKeepsOpenState(t *testing.T) {
	store := memStore{}
	cb1 := NewCircuitBreaker(WithCircuitStore(store), WithMinBackoff(time.Hour))
	cb1.RecordFailure()

	cb2 := NewCircuitBreaker(WithCircuitStore(store), WithMinBackoff(time.Hour))
	if cb2.Allow() {
		t.Fatal("circuit should stay open after restart")
	}
	if cb2.BackoffUntil().Before(time.Now().Add(50 * time.Minute)) {
		t.Fatal("backoff end not carried over")
	}
}

func TestCircuit_RestartAfterBackoffProbesAndKeepsDoubling(t *testing.T) {
	store := memStore{}
	cb1 := NewCircuitBreaker(WithCircuitStore(store), WithMinBackoff(10*time.Millisecond), WithMaxBackoff(time.Hour))
	cb1.RecordFailure()
	time.Sleep(20 * time.Millisecond)

	cb2 := NewCircuitBreaker(WithCircuitStore(store), WithMinBackoff(10*time.Millisecond), WithMaxBackoff(time.Hour))
	if !cb2.Allow() || cb2.State() != CircuitHalfOpen {
		t.Fatal("expected a half-open probe")
	}
	cb2.RecordFailure() // probe fails: backoff doubles
	if got := cb2.BackoffUntil().Sub(time.Now()); got < 12*time.Millisecond {
		t.Fatalf("backoff did not double, %s", got)
	}
}

func TestCircuit_SuccessClearsSavedState(t *testing.T) {
	store := memStore{}
	cb := NewCircuitBreaker(WithCircuitStore(store), WithMinBackoff(time.Millisecond))
	cb.RecordFailure()
	time.Sleep(3 * time.Millisecond)
	cb.Allow()
	cb.RecordSuccess()
	if !NewCircuitBreaker(WithCircuitStore(store)).Allow() {
		t.Fatal("closed circuit should load closed")
	}
}
