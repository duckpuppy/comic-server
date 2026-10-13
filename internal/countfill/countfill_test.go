package countfill

import (
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestPlan(t *testing.T) {
	counts := map[int]int{100: 12, 200: 40}
	tests := []struct {
		name  string
		book  library.ComicBook
		want  bool
		count int
	}{
		{"fills zero", library.ComicBook{CustomValuesStore: "comicvine_volume=100"}, true, 12},
		{"untagged", library.ComicBook{}, false, 0},
		{"volume not synced", library.ComicBook{CustomValuesStore: "comicvine_volume=999"}, false, 0},
		{"hand-set count kept", library.ComicBook{Count: 5, CustomValuesStore: "comicvine_volume=100"}, false, 0},
		{"own count updated", library.ComicBook{Count: 30, CustomValuesStore: "comicvine_volume=200,comic_server_count_auto=30"}, true, 40},
		{"own count unchanged", library.ComicBook{Count: 40, CustomValuesStore: "comicvine_volume=200,comic_server_count_auto=40"}, false, 0},
		{"marker stale, count edited by hand", library.ComicBook{Count: 7, CustomValuesStore: "comicvine_volume=200,comic_server_count_auto=30"}, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, ok := Plan(&tt.book, counts)
			if ok != tt.want || (ok && p.New != tt.count) {
				t.Fatalf("Plan = %+v, %v; want ok=%v count=%d", p, ok, tt.want, tt.count)
			}
		})
	}
}

func TestApplyThenPlanIsNoOp(t *testing.T) {
	counts := map[int]int{100: 12}
	b := library.ComicBook{CustomValuesStore: "comicvine_volume=100"}
	p, ok := Plan(&b, counts)
	if !ok {
		t.Fatal("expected a proposal")
	}
	Apply(&b, p)
	if b.Count != 12 {
		t.Fatalf("Count = %d", b.Count)
	}
	if _, ok := Plan(&b, counts); ok {
		t.Fatal("second plan should be a no-op")
	}
	counts[100] = 15 // ongoing series grew
	if p, ok := Plan(&b, counts); !ok || p.New != 15 {
		t.Fatalf("grown volume: %+v %v", p, ok)
	}
}
