package comicvine

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// waits reports whether Wait blocks (does not return within d).
func waits(p *Pacer, ctx context.Context, d time.Duration) bool {
	c, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	return errors.Is(p.Wait(c), context.DeadlineExceeded)
}

func TestPacer_BackgroundStopsShortOfBurstLimit(t *testing.T) {
	p := NewPacer(WithBurstSize(6), WithBurstPause(time.Hour), WithWindowSize(1000),
		WithBackgroundHeadroom(2, 0, 0))
	for i := 0; i < 4; i++ { // 4 of 6: background limit is 6-2 = 4
		p.RecordRequest()
	}
	if !waits(p, WithBackground(context.Background()), 60*time.Millisecond) {
		t.Fatal("background should wait at burst-reserve")
	}
	if waits(p, context.Background(), 60*time.Millisecond) {
		t.Fatal("interactive should still have the reserved slots")
	}
	p.RecordRequest()
	p.RecordRequest() // the interactive caller used the reserve
	if !waits(p, context.Background(), 60*time.Millisecond) {
		t.Fatal("interactive waits once the full burst is used")
	}
}

func TestPacer_BackgroundBurstPauseServedThenContinues(t *testing.T) {
	p := NewPacer(WithBurstSize(4), WithBurstPause(80*time.Millisecond), WithWindowSize(1000),
		WithBackgroundHeadroom(1, 0, 0))
	for i := 0; i < 3; i++ {
		p.RecordRequest()
	}
	start := time.Now()
	if err := p.Wait(WithBackground(context.Background())); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d < 50*time.Millisecond {
		t.Fatalf("background did not pause (%s)", d)
	}
	if b, _ := p.Stats(); b != 0 {
		t.Errorf("burst count after the pause = %d, want 0", b)
	}
}

func TestPacer_BackgroundHourlyCap(t *testing.T) {
	p := NewPacer(WithBurstSize(1000), WithWindowSize(1000), WithBackgroundHeadroom(0, 0, 3))
	for i := 0; i < 3; i++ {
		p.RecordRequest()
	}
	if !waits(p, WithBackground(context.Background()), 60*time.Millisecond) {
		t.Fatal("background should wait at its hourly cap")
	}
	if waits(p, context.Background(), 60*time.Millisecond) {
		t.Fatal("the hourly cap is for background callers only")
	}
	// The wait runs until the oldest counted request is an hour old.
	d, why := p.pauseNeeded(true, time.Now())
	if d < 59*time.Minute || why == "" {
		t.Errorf("pause = %s (%q), want about an hour with a reason", d, why)
	}
	// An hour on, the budget is free again.
	if d, _ := p.pauseNeeded(true, time.Now().Add(61*time.Minute)); d != 0 {
		t.Errorf("pause after an hour = %s, want 0", d)
	}
}

func TestPacer_HourlyCapCountsRequestsBeforeARestart(t *testing.T) {
	store := memStore{}
	opts := []PacerOption{WithBurstSize(1000), WithWindowSize(1000), WithBackgroundHeadroom(0, 0, 2), WithPacerStore(store)}
	p1 := NewPacer(opts...)
	p1.RecordRequest()
	p1.RecordRequest()
	p2 := NewPacer(opts...)
	if !waits(p2, WithBackground(context.Background()), 60*time.Millisecond) {
		t.Fatal("a restart must not reset the background hourly budget")
	}
}

// The sync marks its own context as background, so its requests leave
// headroom without the caller having to remember to.
func TestSyncer_RunIsBackground(t *testing.T) {
	c := testCache(t)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"status_code":1,"results":{"id":100,"name":"V","count_of_issues":1}}`))
	}))
	t.Cleanup(srv.Close)
	pacer := NewPacer(WithBackgroundHeadroom(0, 0, 1))
	pacer.RecordRequest() // the background budget (1 an hour) is already used
	client := NewClient("k", WithBaseURL(srv.URL), WithPacer(pacer))
	syncer := NewSyncer(client, c)
	if _, err := syncer.SeedFromLibrary([]string{",comicvine_volume=100"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_ = syncer.Run(ctx, map[int]int{100: 1})
	if hits != 0 {
		t.Errorf("sync sent %d requests past its hourly budget", hits)
	}
	// The same budget does not hold back an interactive call.
	if _, err := client.FetchVolume(context.Background(), 100); err != nil {
		t.Errorf("interactive fetch: %v", err)
	}
}
