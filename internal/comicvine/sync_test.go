package comicvine

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestExtractCVVolumeID(t *testing.T) {
	tests := []struct {
		store string
		want  int
	}{
		{",comicvine_issue=133658,comicvine_volume=20784", 20784},
		{",comicvine_volume=45520,comicvine_issue=313502", 45520},
		{",comicvine_volume=12345", 12345},
		{"comicvine_volume=99", 99},
		{"", 0},
		{",other_key=123", 0},
		{",comicvine_volume=", 0},
		{",comicvine_volume=abc", 0},
	}
	for _, tt := range tests {
		t.Run(tt.store, func(t *testing.T) {
			if got := extractCVVolumeID(tt.store); got != tt.want {
				t.Errorf("extractCVVolumeID(%q) = %d, want %d", tt.store, got, tt.want)
			}
		})
	}
}

func TestBuildOwnedCounts(t *testing.T) {
	stores := []string{
		",comicvine_volume=100,comicvine_issue=1",
		",comicvine_volume=100,comicvine_issue=2",
		",comicvine_volume=100,comicvine_issue=3",
		",comicvine_volume=200,comicvine_issue=10",
		"", // no CV data
	}
	counts := BuildOwnedCounts(stores)
	if counts[100] != 3 {
		t.Errorf("volume 100: got %d, want 3", counts[100])
	}
	if counts[200] != 1 {
		t.Errorf("volume 200: got %d, want 1", counts[200])
	}
	if counts[300] != 0 {
		t.Errorf("volume 300: got %d, want 0", counts[300])
	}
}

func TestSeedFromLibrary(t *testing.T) {
	c := testCache(t)
	syncer := NewSyncer(nil, c) // no client needed for seeding

	stores := []string{
		",comicvine_volume=100,comicvine_issue=1",
		",comicvine_volume=100,comicvine_issue=2",
		",comicvine_volume=200,comicvine_issue=10",
		"", // no CV data
	}

	seeded, err := syncer.SeedFromLibrary(stores)
	if err != nil {
		t.Fatal(err)
	}
	if seeded != 2 {
		t.Errorf("seeded %d, want 2 unique volumes", seeded)
	}

	// Verify volumes exist as pending
	v, _ := c.GetVolume(100)
	if v == nil || v.SyncStatus != SyncStatusPending {
		t.Errorf("volume 100: %+v", v)
	}
	v, _ = c.GetVolume(200)
	if v == nil || v.SyncStatus != SyncStatusPending {
		t.Errorf("volume 200: %+v", v)
	}
}

// A volume ComicVine no longer has is marked missing and left alone, instead
// of failing and being tried again on every pass.
func TestSyncer_MissingVolumeIsNotRetried(t *testing.T) {
	c := testCache(t)
	calls := map[string]int{}
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls[r.URL.Path]++
		switch {
		case strings.HasPrefix(r.URL.Path, "/volume/4050-100"):
			_, _ = w.Write([]byte(`{"status_code":101,"error":"Object Not Found","results":[]}`))
		case strings.HasPrefix(r.URL.Path, "/volume/4050-200"):
			_, _ = w.Write([]byte(`{"status_code":1,"results":{"id":200,"name":"Real","start_year":"2001","count_of_issues":1}}`))
		case strings.HasPrefix(r.URL.Path, "/issues"):
			_, _ = w.Write([]byte(`{"status_code":1,"number_of_total_results":0,"number_of_page_results":0,"results":[]}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	syncer := NewSyncer(client, c)
	if _, err := syncer.SeedFromLibrary([]string{",comicvine_volume=100", ",comicvine_volume=200"}); err != nil {
		t.Fatal(err)
	}

	if err := syncer.Run(context.Background(), map[int]int{100: 5, 200: 1}); err != nil {
		t.Fatalf("Run did not finish cleanly: %v", err)
	}
	gone, _ := c.GetVolume(100)
	if gone == nil || gone.SyncStatus != SyncStatusMissing {
		t.Fatalf("volume 100 = %+v, want missing", gone)
	}
	if ok, _ := c.GetVolume(200); ok == nil || ok.SyncStatus != SyncStatusSynced {
		t.Errorf("volume 200 = %+v, want synced", ok)
	}
	if n, _ := c.MissingVolumeCount(); n != 1 {
		t.Errorf("missing count = %d, want 1", n)
	}
	if ids, _ := c.PendingVolumeIDs(map[int]int{}); len(ids) != 0 {
		t.Errorf("pending = %v, want none (missing is not retried)", ids)
	}
	if calls["/volume/4050-100/"] != 1 {
		t.Errorf("volume 100 fetched %d times, want 1", calls["/volume/4050-100/"])
	}
	// A failed volume, unlike a missing one, is still retried.
	if _, err := c.db.Exec(`UPDATE cv_volumes SET sync_status='failed', issues_synced=0 WHERE cv_id=200`); err != nil {
		t.Fatal(err)
	}
	if ids, _ := c.PendingVolumeIDs(map[int]int{}); len(ids) != 1 || ids[0] != 200 {
		t.Errorf("failed volume should stay pending-for-retry, got %v", ids)
	}
}

// While ComicVine says "slow down", the sync must stop after one refusal and
// leave every volume as it was, instead of failing the whole queue.
func TestSyncer_RateLimitPausesInsteadOfFailingVolumes(t *testing.T) {
	c := testCache(t)
	hits := 0
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(420)
		_, _ = w.Write([]byte(`{"error":"Rate limit exceeded.  Slow down cowboy.","status_code":107,"results":[]}`))
	})
	syncer := NewSyncer(client, c)
	stores := []string{",comicvine_volume=1", ",comicvine_volume=2", ",comicvine_volume=3", ",comicvine_volume=4"}
	if _, err := syncer.SeedFromLibrary(stores); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	err := syncer.Run(ctx, map[int]int{1: 4, 2: 3, 3: 2, 4: 1})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run err = %v, want it to be waiting out the pause until the context ended", err)
	}
	if hits != 1 {
		t.Errorf("ComicVine saw %d requests, want 1", hits)
	}
	synced, pending, failed, _ := c.SyncStats()
	if synced != 0 || pending != 4 || failed != 0 {
		t.Errorf("synced=%d pending=%d failed=%d, want 0/4/0 (a rate limit is not a failure)", synced, pending, failed)
	}
}
