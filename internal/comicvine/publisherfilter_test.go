package comicvine

import (
	"context"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestPublisherFilter_Permits(t *testing.T) {
	cases := []struct {
		name   string
		filter PublisherFilter
		pub    string
		want   bool
	}{
		{"no filter", PublisherFilter{}, "Panini Comics", true},
		{"block substring", PublisherFilter{Block: []string{"Panini"}}, "Editorial Panini", false},
		{"block case-insensitive", PublisherFilter{Block: []string{"panini"}}, "PANINI Verlag", false},
		{"block miss", PublisherFilter{Block: []string{"Panini"}}, "DC Comics", true},
		{"allow hit", PublisherFilter{Allow: []string{"DC", "Marvel"}}, "DC Comics", true},
		{"allow miss", PublisherFilter{Allow: []string{"DC", "Marvel"}}, "Image Comics", false},
		{"block carves out of allow", PublisherFilter{Allow: []string{"Marvel"}, Block: []string{"Marvel Italia"}}, "Marvel Italia", false},
		{"empty publisher passes allow", PublisherFilter{Allow: []string{"DC"}}, "", true},
		{"empty publisher passes block", PublisherFilter{Block: []string{"Panini"}}, "  ", true},
		{"blank terms ignored", PublisherFilter{Allow: []string{" ", ""}, Block: []string{""}}, "Anyone", true},
	}
	for _, c := range cases {
		if got := c.filter.Permits(c.pub); got != c.want {
			t.Errorf("%s: Permits(%q) = %v, want %v", c.name, c.pub, got, c.want)
		}
	}
}

func TestPublisherFilter_FilterVolumesKeepsOrder(t *testing.T) {
	vols := []Volume{
		{ID: 1, Publisher: Publisher{Name: "Panini Comics"}},
		{ID: 2, Publisher: Publisher{Name: "DC Comics"}},
		{ID: 3},
	}
	got := PublisherFilter{Block: []string{"Panini"}}.FilterVolumes(vols)
	if len(got) != 2 || got[0].ID != 2 || got[1].ID != 3 {
		t.Errorf("got %+v", got)
	}
}

func TestScrape_PublisherFilterDropsBlockedCandidate(t *testing.T) {
	fx := &scrapeFixture{
		searchResults: map[string][]Volume{
			"Batman": {
				{ID: 200, Name: "Batman", StartYear: "2016", Publisher: Publisher{Name: "Panini Comics"}, CountOfIssues: 85},
				{ID: 100, Name: "Batman", StartYear: "2016", Publisher: Publisher{Name: "DC Comics"}, CountOfIssues: 85},
			},
		},
		issuesByVolume: map[int][]Issue{100: {{ID: 1001, IssueNumber: "1"}}, 200: {{ID: 2001, IssueNumber: "1"}}},
		issueDetails: map[int]*IssueDetail{
			1001: {ID: 1001, IssueNumber: "1", Name: "DC edition"},
			2001: {ID: 2001, IssueNumber: "1", Name: "Panini edition"},
		},
	}
	client, _ := newScrapeFixtureClient(t, fx)
	cfg := DefaultScraperConfig()
	cfg.PublisherFilter = PublisherFilter{Block: []string{"Panini"}}
	backend := &fakeBackend{}
	scraper := NewScraper(client, testCache(t), backend, cfg)

	books := []*library.ComicBook{{ID: "b1", FilePath: "Batman 001.cbz"}}
	job, err := scraper.Scrape(context.Background(), "job-1", books, ScrapeOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if job.Completed != 1 || len(backend.updated) != 1 || backend.updated[0].Title != "DC edition" {
		t.Errorf("job=%+v updated=%+v", job, backend.updated)
	}
}

func TestScrape_PublisherFilterAllFilteredFails(t *testing.T) {
	fx := &scrapeFixture{
		searchResults: map[string][]Volume{
			"Batman": {{ID: 200, Name: "Batman", StartYear: "2016", Publisher: Publisher{Name: "Panini Comics"}, CountOfIssues: 85}},
		},
	}
	client, _ := newScrapeFixtureClient(t, fx)
	cfg := DefaultScraperConfig()
	cfg.PublisherFilter = PublisherFilter{Block: []string{"Panini"}}
	scraper := NewScraper(client, testCache(t), &fakeBackend{}, cfg)

	book := &library.ComicBook{ID: "b1", FilePath: "Batman 001.cbz"}
	res := scraper.scrapeOneBook(context.Background(), book, fx.searchResults["Batman"], nil, map[int][]Issue{}, ScrapeOptions{})
	if res.Status != BookStatusFailed || res.Error != errAllFiltered {
		t.Fatalf("result = %+v", res)
	}
	if HeldBackReason(res) == "" {
		t.Error("expected a held-back reason so a library scrape does not retry it")
	}
}
