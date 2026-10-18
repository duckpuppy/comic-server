package comicvine

import (
	"context"
	"encoding/json"
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

	// last and times are kept so a restart can carry the pacing over: ComicVine
	// counts requests on its side, so a restart must not give a fresh count.
	last  time.Time
	times []time.Time // request times of the last hour, oldest first
	store StateStore
}

// StateStore keeps small key-value state across restarts. *Cache satisfies it.
type StateStore interface {
	GetSyncState(key string) (string, error)
	SetSyncState(key, value string) error
}

const (
	pacerStateKey = "pacer_state"
	pacerHistory  = time.Hour
	pacerMaxTimes = 2000
)

type pacerState struct {
	Burst  int     `json:"burst"`
	Window int     `json:"window"`
	Times  []int64 `json:"times_ms"`
}

// WithPacerStore makes the pacer load its counters from store at creation and
// save them after every request.
func WithPacerStore(s StateStore) PacerOption {
	return func(p *Pacer) { p.store = s }
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
	p.load()
	return p
}

func (p *Pacer) load() {
	if p.store == nil {
		return
	}
	raw, err := p.store.GetSyncState(pacerStateKey)
	if err != nil || raw == "" {
		return
	}
	var st pacerState
	if json.Unmarshal([]byte(raw), &st) != nil {
		return
	}
	p.burstCount, p.windowCount = st.Burst, st.Window
	for _, ms := range st.Times {
		p.times = append(p.times, time.UnixMilli(ms))
	}
	p.prune(time.Now())
	if n := len(st.Times); n > 0 {
		p.last = time.UnixMilli(st.Times[n-1])
	}
}

// prune drops request times older than an hour. Caller holds mu (or is the constructor).
func (p *Pacer) prune(now time.Time) {
	i := 0
	for i < len(p.times) && now.Sub(p.times[i]) > pacerHistory {
		i++
	}
	p.times = p.times[i:]
	if len(p.times) > pacerMaxTimes {
		p.times = p.times[len(p.times)-pacerMaxTimes:]
	}
}

// save writes the counters out. Caller holds mu. A failed save is ignored: pacing still works in memory.
func (p *Pacer) save() {
	if p.store == nil {
		return
	}
	st := pacerState{Burst: p.burstCount, Window: p.windowCount}
	for _, t := range p.times {
		st.Times = append(st.Times, t.UnixMilli())
	}
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	_ = p.store.SetSyncState(pacerStateKey, string(b))
}

// RequestsLastHour returns how many requests went out in the last hour,
// counting those made before a restart.
func (p *Pacer) RequestsLastHour() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prune(time.Now())
	return len(p.times)
}

// Wait blocks until the next request is allowed, respecting burst and window limits.
// Returns an error if the context is cancelled while waiting.
func (p *Pacer) Wait(ctx context.Context) error {
	p.mu.Lock()
	var pause time.Duration

	if p.windowCount > 0 && p.windowCount%p.windowSize == 0 {
		pause = p.windowPause
	} else if p.burstCount > 0 && p.burstCount%p.burstSize == 0 {
		pause = p.burstPause
	}
	// The pause runs from the last request, so time already served (before a
	// restart, say) counts.
	if pause > 0 && !p.last.IsZero() {
		pause -= time.Since(p.last)
	}
	p.mu.Unlock()

	if pause > 0 {
		jobLog(ctx, JobLogInfo, "Waiting %s for the ComicVine rate limit", pause.Round(time.Second))
		select {
		case <-time.After(pause):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// The pause is over (or was never needed): start a fresh burst. This is
	// done after the wait so a restart during the pause does not lose it.
	p.mu.Lock()
	if p.burstCount > 0 && p.burstCount%p.burstSize == 0 {
		p.burstCount = 0
		p.save()
	} else if p.windowCount > 0 && p.windowCount%p.windowSize == 0 {
		p.burstCount = 0
		p.save()
	}
	p.mu.Unlock()

	return nil
}

// RecordRequest increments the burst and window counters. Call after a successful request.
func (p *Pacer) RecordRequest() {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	p.burstCount++
	p.windowCount++
	p.last = now
	p.times = append(p.times, now)
	p.prune(now)
	p.save()
}

// Reset clears all counters (e.g. after a long backoff period).
func (p *Pacer) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.burstCount = 0
	p.windowCount = 0
	p.last = time.Time{}
	p.times = nil
	p.save()
}

// Stats returns the current burst and window counts.
func (p *Pacer) Stats() (burstCount, windowCount int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.burstCount, p.windowCount
}
