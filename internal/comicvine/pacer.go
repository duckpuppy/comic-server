package comicvine

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

const (
	defaultBurstSize   = 20
	defaultBurstPause  = 5 * time.Minute
	defaultWindowSize  = 200
	defaultWindowPause = 30 * time.Minute

	// Background callers (the volume sync) leave headroom so a person who
	// starts a scrape or a match is not queued behind them: they stop short of
	// the burst and window limits and share only part of the hourly budget
	// (ComicVine allows about 200 requests an hour).
	defaultBackgroundBurstReserve  = 5
	defaultBackgroundWindowReserve = 20
	defaultBackgroundHourlyCap     = 120
)

type backgroundKey struct{}

// WithBackground marks ctx as belonging to a background job. Its requests
// are paced with headroom (see defaultBackgroundHourlyCap); anything else is
// treated as interactive and may use the full limits.
func WithBackground(ctx context.Context) context.Context {
	return context.WithValue(ctx, backgroundKey{}, true)
}

type maxWaitKey struct{}

// WithMaxWait limits how long a request may wait for pacing. A caller that
// answers a web request cannot hold it for minutes (a reverse proxy gives up
// after about a minute and the person sees a bare 504); with a limit, a longer
// pause comes back at once as ErrPaceWait carrying the time to wait.
func WithMaxWait(ctx context.Context, d time.Duration) context.Context {
	return context.WithValue(ctx, maxWaitKey{}, d)
}

// ErrPaceWait is returned instead of waiting when the pacing pause is longer
// than the caller's WithMaxWait.
type ErrPaceWait struct {
	After  time.Duration // how long until the next request is allowed
	Reason string        // "burst", "window" or "hourly"
}

func (e ErrPaceWait) Error() string {
	return fmt.Sprintf("ComicVine pacing (%s limit): the next request is allowed in %s", e.Reason, e.After.Round(time.Second))
}

func isBackground(ctx context.Context) bool {
	v, _ := ctx.Value(backgroundKey{}).(bool)
	return v
}

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

	bgBurstReserve  int
	bgWindowReserve int
	bgHourlyCap     int

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

// WithBackgroundHeadroom sets how many burst and window slots, and how many
// requests an hour, background callers leave to everyone else.
func WithBackgroundHeadroom(burstReserve, windowReserve, hourlyCap int) PacerOption {
	return func(p *Pacer) {
		p.bgBurstReserve, p.bgWindowReserve, p.bgHourlyCap = burstReserve, windowReserve, hourlyCap
	}
}

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

		bgBurstReserve:  defaultBackgroundBurstReserve,
		bgWindowReserve: defaultBackgroundWindowReserve,
		bgHourlyCap:     defaultBackgroundHourlyCap,
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

// Wait blocks until the next request is allowed, respecting burst, window and
// (for background callers) hourly limits. Returns an error if the context is
// cancelled while waiting.
func (p *Pacer) Wait(ctx context.Context) error {
	bg := isBackground(ctx)
	for {
		pause, reason := p.pauseNeeded(bg, time.Now())
		if pause <= 0 {
			return nil
		}
		if limit, ok := ctx.Value(maxWaitKey{}).(time.Duration); ok && pause > limit {
			jobLog(ctx, JobLogInfo, "Not waiting %s for the ComicVine rate limit (%s limit); try again later", pause.Round(time.Second), reason)
			return ErrPaceWait{After: pause, Reason: reason}
		}
		why := ""
		if reason == "hourly" {
			why = " (the background sync keeps part of the hourly budget free)"
		}
		jobLog(ctx, JobLogInfo, "Waiting %s for the ComicVine rate limit%s", pause.Round(time.Second), why)
		select {
		case <-time.After(pause):
		case <-ctx.Done():
			return ctx.Err()
		}
		// Look again: the counters may have moved while waiting.
	}
}

// pauseNeeded returns how long to wait before the next request, 0 for none.
// A limit whose pause has already been served (measured from the last request,
// so time before a restart counts) is cleared here.
func (p *Pacer) pauseNeeded(bg bool, now time.Time) (time.Duration, string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	burstLimit, windowLimit := p.burstSize, p.windowSize
	if bg {
		burstLimit = max(p.burstSize-p.bgBurstReserve, 1)
		windowLimit = max(p.windowSize-p.bgWindowReserve, 1)
	}
	served := func(pause time.Duration) time.Duration {
		if p.last.IsZero() {
			return 0
		}
		return pause - now.Sub(p.last)
	}

	if p.windowCount >= windowLimit {
		if d := served(p.windowPause); d > 0 {
			return d, "window"
		}
		p.windowCount, p.burstCount = 0, 0
		p.save()
	}
	if p.burstCount >= burstLimit {
		if d := served(p.burstPause); d > 0 {
			return d, "burst"
		}
		p.burstCount = 0
		p.save()
	}
	if bg && p.bgHourlyCap > 0 {
		p.prune(now)
		if n := len(p.times); n >= p.bgHourlyCap {
			// Wait until enough of the last hour's requests have aged out.
			d := p.times[n-p.bgHourlyCap].Add(pacerHistory).Sub(now)
			return max(d, time.Second), "hourly"
		}
	}
	return 0, ""
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
