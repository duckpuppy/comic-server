package comicvine

import "testing"

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
