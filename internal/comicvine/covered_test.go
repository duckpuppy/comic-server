package comicvine

import (
	"testing"

	"github.com/duckpuppy/comic-server/internal/collects"
	"github.com/duckpuppy/comic-server/internal/library"
)

func collectedBook(id, file string, entries ...collects.Entry) library.ComicBook {
	b := library.ComicBook{ID: id, FilePath: file}
	collects.Set(&b, entries)
	return b
}

func TestBuildSeriesCompleteness_CoveredByCollection(t *testing.T) {
	c := testCache(t)
	seedVolume(t, c) // volume 100: issues 1, 2, 3, 10
	books := []library.ComicBook{
		{ID: "own2", FilePath: "b.cbz", CustomValuesStore: ",comicvine_volume=100,comicvine_issue=1002"},
		collectedBook("tpb", "t.cbz", collects.Entry{Volume: 100, From: "1", To: "3"}),
	}
	sc, err := BuildSeriesCompleteness(books, c, 100)
	if err != nil || sc == nil {
		t.Fatalf("sc=%v err=%v", sc, err)
	}
	if sc.Owned != 1 || sc.Covered != 2 || sc.Missing != 1 || sc.Percent != 25 {
		t.Errorf("counts = %+v", sc)
	}
	by := map[string]SeriesIssue{}
	for _, i := range sc.Issues {
		by[i.Number] = i
	}
	if !by["1"].Covered || by["1"].CoveredBy != "tpb" || by["1"].Owned {
		t.Errorf("issue 1 = %+v", by["1"])
	}
	if !by["2"].Owned || by["2"].Covered {
		t.Errorf("owned wins over covered: %+v", by["2"])
	}
	if by["10"].Covered || by["10"].Owned {
		t.Errorf("issue 10 = %+v", by["10"])
	}
}

func TestBuildSeriesCompleteness_RunsThatCoverNothing(t *testing.T) {
	c := testCache(t)
	seedVolume(t, c)
	books := []library.ComicBook{
		collectedBook("backup", "a.cbz", collects.Entry{Volume: 100, From: "1", Kind: collects.KindBackup}),
		collectedBook("partial", "b.cbz", collects.Entry{Volume: 100, From: "2", Partial: true}),
		collectedBook("nameonly", "c.cbz", collects.Entry{Series: "Batman", From: "3"}),
		collectedBook("backwards", "d.cbz", collects.Entry{Volume: 100, From: "3", To: "1"}),
		collectedBook("absent", "e.cbz", collects.Entry{Volume: 100, From: "50", To: "60"}),
		collectedBook("wanted", "", collects.Entry{Volume: 100, From: "10"}), // placeholder holds nothing
	}
	sc, err := BuildSeriesCompleteness(books, c, 100)
	if err != nil || sc == nil {
		t.Fatalf("sc=%v err=%v", sc, err)
	}
	if sc.Covered != 0 || sc.Missing != 4 {
		t.Errorf("counts = %+v, want nothing covered", sc)
	}
}

func TestBuildSeriesCompleteness_SingleIssueRun(t *testing.T) {
	c := testCache(t)
	seedVolume(t, c)
	books := []library.ComicBook{collectedBook("tpb", "t.cbz", collects.Entry{Volume: 100, From: "10"})}
	sc, _ := BuildSeriesCompleteness(books, c, 100)
	if sc == nil || sc.Covered != 1 || sc.Missing != 3 {
		t.Errorf("counts = %+v", sc)
	}
}

func TestBuildVolumeSummaries_CollectionOnlyVolumeAppears(t *testing.T) {
	c := testCache(t)
	seedVolume(t, c)
	books := []library.ComicBook{collectedBook("tpb", "t.cbz", collects.Entry{Volume: 100, From: "1", To: "2"})}
	got, err := BuildVolumeSummaries(books, c)
	if err != nil || len(got) != 1 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if v := got[0]; v.Owned != 0 || v.Covered != 2 || v.Missing != 2 {
		t.Errorf("summary = %+v", v)
	}
}

func TestCollectionVolumeIDsSeedAndCount(t *testing.T) {
	b := collectedBook("tpb", "t.cbz",
		collects.Entry{Volume: 100, From: "1", To: "2"},
		collects.Entry{Volume: 100, From: "5"},
		collects.Entry{Volume: 200, From: "1"},
		collects.Entry{Series: "No volume", From: "1"})
	ids := collectionVolumeIDs(b.CustomValuesStore)
	if len(ids) != 2 || ids[0] != 100 || ids[1] != 200 {
		t.Errorf("ids = %v", ids)
	}
	counts := BuildOwnedCounts([]string{b.CustomValuesStore})
	if counts[100] != 1 || counts[200] != 1 {
		t.Errorf("counts = %v", counts)
	}
}

func TestCountCoveredAsOwned_SeriesSummariesAndMatcherMap(t *testing.T) {
	c := testCache(t)
	seedVolume(t, c) // volume 100: issues 1, 2, 3, 10
	books := []library.ComicBook{
		{ID: "own2", FilePath: "b.cbz", CustomValuesStore: ",comicvine_volume=100,comicvine_issue=1002"},
		collectedBook("tpb", "t.cbz", collects.Entry{Volume: 100, From: "1", To: "3"}),
	}
	on := CompletenessOptions{CountCoveredAsOwned: true}

	sc, err := BuildSeriesCompletenessOpts(books, c, 100, on)
	if err != nil || sc == nil {
		t.Fatalf("sc=%v err=%v", sc, err)
	}
	// Issues 1 and 3 are covered, 2 is owned: all three count, 10 is missing.
	if !sc.CoveredAsOwned || sc.Owned != 3 || sc.Covered != 2 || sc.Missing != 1 || sc.Percent != 75 {
		t.Errorf("series = %+v", sc)
	}
	off, _ := BuildSeriesCompleteness(books, c, 100)
	if off.CoveredAsOwned || off.Owned != 1 || off.Percent != 25 {
		t.Errorf("default must stay owned-only: %+v", off)
	}

	sums, _ := BuildVolumeSummariesOpts(books, c, on)
	if len(sums) != 1 || sums[0].Owned != 3 || sums[0].Missing != 1 || sums[0].Percent != 75 || !sums[0].CoveredAsOwned {
		t.Errorf("summaries = %+v", sums)
	}

	// The matcher map: the owned single issue's book sees the collection's issues too.
	m, _ := BuildCompletenessMapOpts(books, c, on)
	if cv := m["own2"]; cv == nil || cv.OwnedIssues != 3 || cv.MissingCount != 1 || cv.IsComplete != "No" {
		t.Errorf("map on = %+v", cv)
	}
	m, _ = BuildCompletenessMap(books, c)
	if cv := m["own2"]; cv == nil || cv.OwnedIssues != 1 || cv.MissingCount != 3 {
		t.Errorf("map off = %+v (collections must be ignored by default)", cv)
	}
}
