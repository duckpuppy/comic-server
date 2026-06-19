package comicvine

import (
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestBuildCompletenessMap(t *testing.T) {
	c := testCache(t)

	// Set up a synced volume with 5 issues
	c.UpsertVolume(&CachedVolume{
		CVID: 100, Name: "Batman", IssueCount: 5,
		SyncStatus: SyncStatusSynced, SyncedAt: time.Now(), IssuesSynced: true,
	})
	c.UpsertIssues([]CachedIssue{
		{CVID: 1001, VolumeCVID: 100, Number: "1"},
		{CVID: 1002, VolumeCVID: 100, Number: "2"},
		{CVID: 1003, VolumeCVID: 100, Number: "3"},
		{CVID: 1004, VolumeCVID: 100, Number: "4"},
		{CVID: 1005, VolumeCVID: 100, Number: "5"},
	})

	// Library books: own issues 1, 2, 3 of volume 100
	books := []library.ComicBook{
		{ID: "book-1", CustomValuesStore: ",comicvine_volume=100,comicvine_issue=1001"},
		{ID: "book-2", CustomValuesStore: ",comicvine_volume=100,comicvine_issue=1002"},
		{ID: "book-3", CustomValuesStore: ",comicvine_volume=100,comicvine_issue=1003"},
		{ID: "book-4", CustomValuesStore: ""},                                          // no CV data
		{ID: "book-5", CustomValuesStore: ",comicvine_volume=999,comicvine_issue=9999"}, // unknown volume
	}

	result, err := BuildCompletenessMap(books, c)
	if err != nil {
		t.Fatal(err)
	}

	// Books 1-3 should have completeness data
	for _, id := range []string{"book-1", "book-2", "book-3"} {
		cv := result[id]
		if cv == nil {
			t.Fatalf("%s: expected CV data, got nil", id)
		}
		if cv.TotalIssues != 5 {
			t.Errorf("%s: total=%d, want 5", id, cv.TotalIssues)
		}
		if cv.OwnedIssues != 3 {
			t.Errorf("%s: owned=%d, want 3", id, cv.OwnedIssues)
		}
		if cv.MissingCount != 2 {
			t.Errorf("%s: missing=%d, want 2", id, cv.MissingCount)
		}
		if cv.PercentOwned != 60 {
			t.Errorf("%s: percent=%d, want 60", id, cv.PercentOwned)
		}
		if cv.IsComplete != "No" {
			t.Errorf("%s: complete=%s, want No", id, cv.IsComplete)
		}
	}

	// Book 4 and 5 should not have data
	if result["book-4"] != nil {
		t.Error("book-4 (no CV tags) should not have CV data")
	}
	if result["book-5"] != nil {
		t.Error("book-5 (unknown volume) should not have CV data")
	}
}

func TestBuildCompletenessMap_FullyOwned(t *testing.T) {
	c := testCache(t)

	c.UpsertVolume(&CachedVolume{
		CVID: 200, Name: "Mini", IssueCount: 2,
		SyncStatus: SyncStatusSynced, SyncedAt: time.Now(), IssuesSynced: true,
	})
	c.UpsertIssues([]CachedIssue{
		{CVID: 2001, VolumeCVID: 200, Number: "1"},
		{CVID: 2002, VolumeCVID: 200, Number: "2"},
	})

	books := []library.ComicBook{
		{ID: "a", CustomValuesStore: ",comicvine_volume=200,comicvine_issue=2001"},
		{ID: "b", CustomValuesStore: ",comicvine_volume=200,comicvine_issue=2002"},
	}

	result, _ := BuildCompletenessMap(books, c)
	cv := result["a"]
	if cv == nil {
		t.Fatal("expected CV data")
	}
	if cv.IsComplete != "Yes" || cv.PercentOwned != 100 || cv.MissingCount != 0 {
		t.Errorf("got complete=%s percent=%d missing=%d", cv.IsComplete, cv.PercentOwned, cv.MissingCount)
	}
}

func TestExtractCVIssueID(t *testing.T) {
	tests := []struct {
		store string
		want  int
	}{
		{",comicvine_issue=133658,comicvine_volume=20784", 133658},
		{",comicvine_volume=45520,comicvine_issue=313502", 313502},
		{"", 0},
		{",comicvine_volume=100", 0},
	}
	for _, tt := range tests {
		if got := extractCVIssueID(tt.store); got != tt.want {
			t.Errorf("extractCVIssueID(%q) = %d, want %d", tt.store, got, tt.want)
		}
	}
}
