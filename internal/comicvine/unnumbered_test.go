package comicvine

import (
	"context"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

// Real unnumbered prod names (2026-10-16): the parser must give no issue number.
func TestParseFilename_UnnumberedOneShots(t *testing.T) {
	for _, name := range []string{
		"/comics/IDW/Godzilla - Here There Be Aliens ()/Godzilla - Here There Be Aliens (April, 2026).cbz",
		"/comics/X/Captain Hammer -Be Like Me ()/Captain Hammer -Be Like Me (2011).cbz",
		"/comics/Y/Grimm Fairy Tales Covers (2009).cbz",
	} {
		if got := ParseFilename(name); got.IssueNumber != "" {
			t.Errorf("%q parsed issue %q, want none", name, got.IssueNumber)
		}
	}
}

func unnumberedFixture() *scrapeFixture {
	return &scrapeFixture{
		searchResults: map[string][]Volume{
			"Godzilla Here There Be Aliens": {{ID: 300, Name: "Godzilla Here There Be Aliens", StartYear: "2025", Publisher: Publisher{Name: "IDW Publishing"}, CountOfIssues: 5}},
		},
		issuesByVolume: map[int][]Issue{300: {{ID: 3001, IssueNumber: "1"}, {ID: 3007, IssueNumber: "7"}}},
		issueDetails: map[int]*IssueDetail{
			3001: {ID: 3001, IssueNumber: "1", Name: "One"},
			3007: {ID: 3007, IssueNumber: "7", Name: "Seven"},
		},
	}
}

func TestScrape_UnnumberedBookGoesToReviewNotIssueOne(t *testing.T) {
	client, _ := newScrapeFixtureClient(t, unnumberedFixture())
	backend := &fakeBackend{}
	scraper := NewScraper(client, testCache(t), backend, DefaultScraperConfig())

	books := []*library.ComicBook{{ID: "u1", FilePath: "Godzilla Here There Be Aliens (2026).cbz"}}
	job, err := scraper.Scrape(context.Background(), "job-u", books, ScrapeOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if job.PendingReview != 1 || job.Completed != 0 {
		t.Fatalf("job = %+v, want one pending review and no auto match", job)
	}
	pending, _ := scraper.cache.GetPendingReviewBooks("job-u")
	if len(pending) != 1 || len(pending[0].Candidates) == 0 {
		t.Fatalf("pending = %+v", pending)
	}
	if extractCVIssueID(books[0].CustomValuesStore) != 0 {
		t.Error("an unnumbered book was tagged with an issue")
	}
}

func TestScrape_UnnumberedFileUsesLibraryNumber(t *testing.T) {
	client, _ := newScrapeFixtureClient(t, unnumberedFixture())
	scraper := NewScraper(client, testCache(t), &fakeBackend{}, DefaultScraperConfig())

	books := []*library.ComicBook{{ID: "u2", FilePath: "Godzilla Here There Be Aliens (2026).cbz", Number: "7"}}
	job, err := scraper.Scrape(context.Background(), "job-n", books, ScrapeOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if job.Completed != 1 || extractCVIssueID(books[0].CustomValuesStore) != 3007 {
		t.Fatalf("job = %+v, issue = %d, want issue 3007 from the library Number", job, extractCVIssueID(books[0].CustomValuesStore))
	}
}
