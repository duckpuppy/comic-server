package comicvine

import (
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/library"
)

func seedVolume(t *testing.T, c *Cache) {
	t.Helper()
	c.UpsertVolume(&CachedVolume{CVID: 100, Name: "Batman", Publisher: "DC", StartYear: "1940",
		SyncStatus: SyncStatusSynced, SyncedAt: time.Now(), IssuesSynced: true})
	c.UpsertIssues([]CachedIssue{
		{CVID: 1001, VolumeCVID: 100, Number: "1"},
		{CVID: 1010, VolumeCVID: 100, Number: "10"},
		{CVID: 1002, VolumeCVID: 100, Number: "2"},
		{CVID: 1003, VolumeCVID: 100, Number: "3"},
	})
}

func TestBuildSeriesCompleteness(t *testing.T) {
	c := testCache(t)
	seedVolume(t, c)
	books := []library.ComicBook{
		{ID: "own1", FilePath: "a.cbz", CustomValuesStore: ",comicvine_volume=100,comicvine_issue=1001"},
		{ID: "own2", FilePath: "b.cbz", CustomValuesStore: ",comicvine_volume=100,comicvine_issue=1002"},
		{ID: "want3", FilePath: "", CustomValuesStore: ",comicvine_volume=100,comicvine_issue=1003"},
		{ID: "loose", FilePath: "c.cbz", CustomValuesStore: ",comicvine_volume=100"}, // volume only
		{ID: "other", FilePath: "d.cbz", CustomValuesStore: ",comicvine_volume=200,comicvine_issue=2001"},
	}

	sc, err := BuildSeriesCompleteness(books, c, 100)
	if err != nil || sc == nil {
		t.Fatalf("sc=%v err=%v", sc, err)
	}
	if sc.Total != 4 || sc.Owned != 2 || sc.Missing != 2 || sc.Wanted != 1 || sc.Percent != 50 || sc.Unmatched != 1 {
		t.Errorf("counts = %+v", sc)
	}
	var order []string
	for _, i := range sc.Issues {
		order = append(order, i.Number)
	}
	if got := order; len(got) != 4 || got[0] != "1" || got[1] != "2" || got[2] != "3" || got[3] != "10" {
		t.Errorf("order = %v, want numeric 1 2 3 10", got)
	}
	byNum := map[string]SeriesIssue{}
	for _, i := range sc.Issues {
		byNum[i.Number] = i
	}
	if !byNum["1"].Owned || byNum["1"].BookID != "own1" {
		t.Errorf("issue 1 = %+v", byNum["1"])
	}
	if byNum["3"].Owned || !byNum["3"].Wanted || byNum["3"].BookID != "want3" {
		t.Errorf("issue 3 = %+v", byNum["3"])
	}
	if byNum["10"].Owned || byNum["10"].Wanted {
		t.Errorf("issue 10 = %+v", byNum["10"])
	}
}

func TestBuildSeriesCompleteness_UnsyncedVolumeIsNil(t *testing.T) {
	c := testCache(t)
	c.UpsertVolume(&CachedVolume{CVID: 5, Name: "X", SyncStatus: SyncStatusPending})
	sc, err := BuildSeriesCompleteness(nil, c, 5)
	if err != nil || sc != nil {
		t.Errorf("sc=%v err=%v, want nil,nil", sc, err)
	}
}

func TestBuildVolumeSummaries_RankedByMissing(t *testing.T) {
	c := testCache(t)
	seedVolume(t, c)
	c.UpsertVolume(&CachedVolume{CVID: 200, Name: "Aquaman", SyncStatus: SyncStatusSynced, SyncedAt: time.Now(), IssuesSynced: true})
	c.UpsertIssues([]CachedIssue{{CVID: 2001, VolumeCVID: 200, Number: "1"}})
	books := []library.ComicBook{
		{ID: "a", FilePath: "a.cbz", CustomValuesStore: ",comicvine_volume=100,comicvine_issue=1001"},
		{ID: "b", FilePath: "b.cbz", CustomValuesStore: ",comicvine_volume=200,comicvine_issue=2001"},
	}
	got, err := BuildVolumeSummaries(books, c)
	if err != nil || len(got) != 2 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if got[0].VolumeID != 100 || got[0].Missing != 3 || got[0].Percent != 25 {
		t.Errorf("first = %+v, want Batman with 3 missing at 25%%", got[0])
	}
	if got[1].VolumeID != 200 || got[1].Missing != 0 || got[1].Percent != 100 {
		t.Errorf("second = %+v", got[1])
	}
}

func TestBuildCompletenessMap_WantedPlaceholderIsNotOwned(t *testing.T) {
	c := testCache(t)
	seedVolume(t, c)
	books := []library.ComicBook{
		{ID: "own", FilePath: "a.cbz", CustomValuesStore: ",comicvine_volume=100,comicvine_issue=1001"},
		{ID: "want", FilePath: "", CustomValuesStore: ",comicvine_volume=100,comicvine_issue=1002"},
	}
	m, err := BuildCompletenessMap(books, c)
	if err != nil {
		t.Fatal(err)
	}
	if cv := m["own"]; cv == nil || cv.OwnedIssues != 1 || cv.MissingCount != 3 {
		t.Errorf("completeness = %+v, want 1 owned / 3 missing", cv)
	}
}

func TestPhysicalCopyIsOwned(t *testing.T) {
	c := testCache(t)
	seedVolume(t, c)
	books := []library.ComicBook{
		{ID: "own", FilePath: "a.cbz", CustomValuesStore: ",comicvine_volume=100,comicvine_issue=1001"},
		{ID: "paper", BookCollectionStatus: "Physical", CustomValuesStore: ",comicvine_volume=100,comicvine_issue=1002"},
		{ID: "want", CustomValuesStore: ",comicvine_volume=100,comicvine_issue=1003"},
	}
	m, err := BuildCompletenessMap(books, c)
	if err != nil {
		t.Fatal(err)
	}
	if cv := m["own"]; cv == nil || cv.OwnedIssues != 2 || cv.MissingCount != 2 {
		t.Errorf("completeness = %+v, want 2 owned / 2 missing", cv)
	}
	sc, err := BuildSeriesCompleteness(books, c, 100)
	if err != nil || sc == nil {
		t.Fatal(err)
	}
	if sc.Owned != 2 || sc.Wanted != 1 {
		t.Errorf("series = owned %d wanted %d, want 2 / 1", sc.Owned, sc.Wanted)
	}
}
