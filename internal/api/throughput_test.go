package api

import (
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

func TestBuildThroughput(t *testing.T) {
	now := time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)
	cbz, scrape := workflow.StageConvertToCBZ.String(), workflow.StageScrape.String()
	mv := func(daysAgo int, book, from, to string) configdb.StageMove {
		return configdb.StageMove{At: now.AddDate(0, 0, -daysAgo), BookID: book, Series: "S", StageBefore: from, StageAfter: to}
	}
	moves := []configdb.StageMove{
		mv(0, "a", cbz, scrape),
		mv(1, "a", cbz, scrape),
		mv(1, "b", cbz, scrape),
		mv(2, "c", scrape, cbz),  // rollback: ignored
		mv(8, "d", cbz, scrape),  // last week
		mv(20, "e", cbz, scrape), // outside window
	}
	got := buildThroughput(moves, now)
	if len(got.Days) != 14 || got.Days[13].Date != "2026-10-12" || got.Days[0].Date != "2026-09-29" {
		t.Fatalf("days = %+v", got.Days)
	}
	if got.Days[13].Count != 1 || got.Days[12].Count != 2 || got.Days[11].Count != 0 {
		t.Errorf("counts = %+v", got.Days)
	}
	if got.ThisWeek != 3 || got.LastWeek != 1 {
		t.Errorf("this/last = %d/%d, want 3/1", got.ThisWeek, got.LastWeek)
	}
	// Recent is distinct books, advance rows only (a, b, d, e).
	if len(got.Recent) != 4 || got.Recent[0].BookID != "a" {
		t.Errorf("recent = %+v", got.Recent)
	}
}
