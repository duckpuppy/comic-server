package comicvine

import (
	"sync"
	"time"
)

// CircuitState represents the state of the circuit breaker.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Normal operation
	CircuitOpen                         // Backing off after failure
	CircuitHalfOpen                     // Testing with a single request
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

const (
	defaultMinBackoff = 1 * time.Hour
	defaultMaxBackoff = 24 * time.Hour
)

// CircuitBreaker implements the circuit breaker pattern with exponential backoff.
// When any API call fails, the circuit opens immediately and waits at least
// MinBackoff before retrying. Repeated failures double the backoff up to MaxBackoff.
type CircuitBreaker struct {
	mu              sync.Mutex
	state           CircuitState
	backoffDuration time.Duration
	backoffUntil    time.Time
	minBackoff      time.Duration
	maxBackoff      time.Duration
	consecutiveOK   int
	onStateChange   func(from, to CircuitState)
}

// CircuitBreakerOption configures a CircuitBreaker.
type CircuitBreakerOption func(*CircuitBreaker)

func WithMinBackoff(d time.Duration) CircuitBreakerOption {
	return func(cb *CircuitBreaker) { cb.minBackoff = d }
}

func WithMaxBackoff(d time.Duration) CircuitBreakerOption {
	return func(cb *CircuitBreaker) { cb.maxBackoff = d }
}

func WithOnStateChange(fn func(from, to CircuitState)) CircuitBreakerOption {
	return func(cb *CircuitBreaker) { cb.onStateChange = fn }
}

// NewCircuitBreaker creates a circuit breaker with the given options.
func NewCircuitBreaker(opts ...CircuitBreakerOption) *CircuitBreaker {
	cb := &CircuitBreaker{
		state:      CircuitClosed,
		minBackoff: defaultMinBackoff,
		maxBackoff: defaultMaxBackoff,
	}
	for _, o := range opts {
		o(cb)
	}
	cb.backoffDuration = cb.minBackoff
	return cb
}

// Allow reports whether a request is allowed right now.
// Returns false when the circuit is open and the backoff hasn't elapsed.
// When the backoff expires, transitions to half-open and returns true for one probe request.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitHalfOpen:
		return true
	case CircuitOpen:
		if time.Now().Before(cb.backoffUntil) {
			return false
		}
		cb.transition(CircuitHalfOpen)
		return true
	}
	return false
}

// BackoffUntil returns when the current backoff expires (zero if not backing off).
func (cb *CircuitBreaker) BackoffUntil() time.Time {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.backoffUntil
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// RecordSuccess signals a successful API call.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveOK++
	if cb.state == CircuitHalfOpen {
		cb.backoffDuration = cb.minBackoff
		cb.backoffUntil = time.Time{}
		cb.transition(CircuitClosed)
	}
}

// RecordFailure signals a failed API call. The circuit opens immediately.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveOK = 0

	if cb.state == CircuitHalfOpen {
		cb.backoffDuration *= 2
		if cb.backoffDuration > cb.maxBackoff {
			cb.backoffDuration = cb.maxBackoff
		}
	} else if cb.state == CircuitClosed {
		cb.backoffDuration = cb.minBackoff
	}

	cb.backoffUntil = time.Now().Add(cb.backoffDuration)
	cb.transition(CircuitOpen)
}

// Reset forces the circuit back to closed state (e.g. on server startup with fresh state).
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.backoffDuration = cb.minBackoff
	cb.backoffUntil = time.Time{}
	cb.consecutiveOK = 0
	cb.transition(CircuitClosed)
}

func (cb *CircuitBreaker) transition(to CircuitState) {
	if cb.state == to {
		return
	}
	from := cb.state
	cb.state = to
	if cb.onStateChange != nil {
		cb.onStateChange(from, to)
	}
}
