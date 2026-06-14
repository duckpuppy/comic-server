package library

import (
	"testing"
	"time"
)

func makeBook(series string, volume int, number string, year int, pageCount, lastPageRead int, openCount int, rating, communityRating float64, count int, seriesComplete string, addedTime, openedTime time.Time) *ComicBook {
	b := &ComicBook{
		Series:          series,
		Volume:          volume,
		Number:          number,
		Year:            year,
		PageCount:       pageCount,
		LastPageRead:    lastPageRead,
		OpenCount:       openCount,
		Rating:          rating,
		CommunityRating: communityRating,
		Count:           count,
		SeriesComplete:  seriesComplete,
	}
	b.AddedTime = ComicTime{addedTime}
	b.OpenedTime = ComicTime{openedTime}
	return b
}

func TestBuildSeriesStats_Count(t *testing.T) {
	books := []*ComicBook{
		makeBook("X-Men", 1, "1", 1990, 22, 0, 0, 0, 0, 0, "", time.Time{}, time.Time{}),
		makeBook("X-Men", 1, "2", 1990, 22, 0, 0, 0, 0, 0, "", time.Time{}, time.Time{}),
		makeBook("X-Men", 1, "3", 1991, 22, 0, 0, 0, 0, 0, "", time.Time{}, time.Time{}),
		makeBook("Batman", 1, "1", 2000, 24, 0, 0, 0, 0, 0, "", time.Time{}, time.Time{}),
	}
	stats := buildSeriesStats(books)
	k := seriesKey{Series: "x-men", Volume: 1}
	s := stats[k]
	if s == nil {
		t.Fatal("no stats for X-Men v1")
	}
	if s.Count != 3 {
		t.Errorf("Count = %d, want 3", s.Count)
	}
	bk := seriesKey{Series: "batman", Volume: 1}
	if stats[bk].Count != 1 {
		t.Errorf("Batman count = %d, want 1", stats[bk].Count)
	}
}

func TestBuildSeriesStats_ReadCount(t *testing.T) {
	books := []*ComicBook{
		// read (OpenCount > 0 and LastPageRead >= PageCount-1)
		{Series: "X", Volume: 1, PageCount: 22, LastPageRead: 21, OpenCount: 1},
		// unread (never opened)
		{Series: "X", Volume: 1, PageCount: 22, LastPageRead: 0, OpenCount: 0},
		// partially read
		{Series: "X", Volume: 1, PageCount: 22, LastPageRead: 10, OpenCount: 2},
	}
	stats := buildSeriesStats(books)
	k := seriesKey{Series: "x", Volume: 1}
	s := stats[k]
	if s.ReadCount != 1 {
		t.Errorf("ReadCount = %d, want 1", s.ReadCount)
	}
	if s.ReadPercentage != 33 { // 1/3 * 100 = 33
		t.Errorf("ReadPercentage = %d, want 33", s.ReadPercentage)
	}
}

func TestBuildSeriesStats_PageCount(t *testing.T) {
	books := []*ComicBook{
		{Series: "X", Volume: 1, PageCount: 22, LastPageRead: 10},
		{Series: "X", Volume: 1, PageCount: 30, LastPageRead: 5},
	}
	stats := buildSeriesStats(books)
	s := stats[seriesKey{Series: "x", Volume: 1}]
	if s.PageCount != 52 {
		t.Errorf("PageCount = %d, want 52", s.PageCount)
	}
	if s.PageReadCount != 15 {
		t.Errorf("PageReadCount = %d, want 15", s.PageReadCount)
	}
}

func TestBuildSeriesStats_AverageRating(t *testing.T) {
	books := []*ComicBook{
		{Series: "X", Volume: 1, Rating: 4.0},
		{Series: "X", Volume: 1, Rating: 2.0},
		{Series: "X", Volume: 1, Rating: 0}, // unrated — excluded from average
	}
	stats := buildSeriesStats(books)
	s := stats[seriesKey{Series: "x", Volume: 1}]
	if s.AverageRating != 3.0 {
		t.Errorf("AverageRating = %v, want 3.0", s.AverageRating)
	}
}

func TestBuildSeriesStats_YearRange(t *testing.T) {
	books := []*ComicBook{
		{Series: "X", Volume: 1, Year: 1990},
		{Series: "X", Volume: 1, Year: 1995},
		{Series: "X", Volume: 1, Year: 1992},
	}
	stats := buildSeriesStats(books)
	s := stats[seriesKey{Series: "x", Volume: 1}]
	if s.MinYear != 1990 {
		t.Errorf("MinYear = %d, want 1990", s.MinYear)
	}
	if s.MaxYear != 1995 {
		t.Errorf("MaxYear = %d, want 1995", s.MaxYear)
	}
	if s.RunningTimeYears != 5 {
		t.Errorf("RunningTimeYears = %d, want 5", s.RunningTimeYears)
	}
}

func TestBuildSeriesStats_NumberRange(t *testing.T) {
	books := []*ComicBook{
		{Series: "X", Volume: 1, Number: "3"},
		{Series: "X", Volume: 1, Number: "1"},
		{Series: "X", Volume: 1, Number: "7"},
	}
	stats := buildSeriesStats(books)
	s := stats[seriesKey{Series: "x", Volume: 1}]
	if s.FirstNumber != 1 {
		t.Errorf("FirstNumber = %v, want 1", s.FirstNumber)
	}
	if s.LastNumber != 7 {
		t.Errorf("LastNumber = %v, want 7", s.LastNumber)
	}
}

func TestBuildSeriesStats_Gaps(t *testing.T) {
	// Issues 1,2,3 then jump to 7,8 — gap is 3→7 (size 4)
	books := []*ComicBook{
		{Series: "X", Volume: 1, Number: "1"},
		{Series: "X", Volume: 1, Number: "2"},
		{Series: "X", Volume: 1, Number: "3"},
		{Series: "X", Volume: 1, Number: "7"},
		{Series: "X", Volume: 1, Number: "8"},
	}
	stats := buildSeriesStats(books)
	s := stats[seriesKey{Series: "x", Volume: 1}]
	if s.GapCount != 1 {
		t.Errorf("GapCount = %d, want 1", s.GapCount)
	}
	if s.MaxGapSize != 4 {
		t.Errorf("MaxGapSize = %v, want 4", s.MaxGapSize)
	}
	if !s.GapStarts[3] {
		t.Error("GapStarts should contain 3")
	}
	if !s.GapEnds[7] {
		t.Error("GapEnds should contain 7")
	}
}

func TestBuildSeriesStats_NoGaps(t *testing.T) {
	books := []*ComicBook{
		{Series: "X", Volume: 1, Number: "1"},
		{Series: "X", Volume: 1, Number: "2"},
		{Series: "X", Volume: 1, Number: "3"},
	}
	stats := buildSeriesStats(books)
	s := stats[seriesKey{Series: "x", Volume: 1}]
	if s.GapCount != 0 {
		t.Errorf("GapCount = %d, want 0", s.GapCount)
	}
}

func TestBuildSeriesStats_MultipleGaps(t *testing.T) {
	// Issues 1, 3, 7 → gaps at 1→3 (size 2) and 3→7 (size 4)
	books := []*ComicBook{
		{Series: "X", Volume: 1, Number: "1"},
		{Series: "X", Volume: 1, Number: "3"},
		{Series: "X", Volume: 1, Number: "7"},
	}
	stats := buildSeriesStats(books)
	s := stats[seriesKey{Series: "x", Volume: 1}]
	if s.GapCount != 2 {
		t.Errorf("GapCount = %d, want 2", s.GapCount)
	}
	if s.MaxGapSize != 4 {
		t.Errorf("MaxGapSize = %v, want 4", s.MaxGapSize)
	}
}

func TestBuildSeriesStats_AllComplete(t *testing.T) {
	books := []*ComicBook{
		{Series: "X", Volume: 1, SeriesComplete: "Yes"},
		{Series: "X", Volume: 1, SeriesComplete: "Yes"},
	}
	stats := buildSeriesStats(books)
	s := stats[seriesKey{Series: "x", Volume: 1}]
	if s.AllComplete != "Yes" {
		t.Errorf("AllComplete = %q, want Yes", s.AllComplete)
	}

	// Mixed → Unknown
	books2 := []*ComicBook{
		{Series: "Y", Volume: 1, SeriesComplete: "Yes"},
		{Series: "Y", Volume: 1, SeriesComplete: "No"},
	}
	stats2 := buildSeriesStats(books2)
	s2 := stats2[seriesKey{Series: "y", Volume: 1}]
	if s2.AllComplete != "Unknown" {
		t.Errorf("AllComplete = %q, want Unknown", s2.AllComplete)
	}
}

func TestBuildSeriesStats_CountField(t *testing.T) {
	books := []*ComicBook{
		{Series: "X", Volume: 1, Count: 12},
		{Series: "X", Volume: 1, Count: 12},
		{Series: "X", Volume: 1, Count: 0}, // zero counts are excluded
	}
	stats := buildSeriesStats(books)
	s := stats[seriesKey{Series: "x", Volume: 1}]
	if s.MinCount != 12 {
		t.Errorf("MinCount = %d, want 12", s.MinCount)
	}
	if s.MaxCount != 12 {
		t.Errorf("MaxCount = %d, want 12", s.MaxCount)
	}
}

func TestParseIssueNumber(t *testing.T) {
	cases := []struct {
		input string
		want  float64
		ok    bool
	}{
		{"1", 1, true},
		{"1.5", 1.5, true},
		{"Annual", 0, false},
		{"", 0, false},
		{"-1", 0, false},
		{"0", 0, false},
	}
	for _, c := range cases {
		got, ok := parseIssueNumber(c.input)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseIssueNumber(%q) = (%v, %v), want (%v, %v)", c.input, got, ok, c.want, c.ok)
		}
	}
}
