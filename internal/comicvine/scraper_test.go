package comicvine

import (
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

// scrapeFixture is an in-memory ComicVine API fixture for scraper tests.
type scrapeFixture struct {
	mu             sync.Mutex
	searchCalls    int
	searchResults  map[string][]Volume // keyed by query string
	issuesByVolume map[int][]Issue
	issueDetails   map[int]*IssueDetail
	covers         map[string][]byte // keyed by URL path, for cover-hash downloads
}

func newScrapeFixtureClient(t *testing.T, fx *scrapeFixture) (*Client, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.Path, "/")
		switch {
		case strings.HasPrefix(path, "/search"):
			fx.mu.Lock()
			fx.searchCalls++
			results := fx.searchResults[r.URL.Query().Get("query")]
			fx.mu.Unlock()
			json.NewEncoder(w).Encode(apiResponse[[]Volume]{StatusCode: 1, Results: results})

		case strings.HasPrefix(path, "/issues"):
			var volID int
			fmt.Sscanf(r.URL.Query().Get("filter"), "volume:%d", &volID)
			fx.mu.Lock()
			issues := fx.issuesByVolume[volID]
			fx.mu.Unlock()
			json.NewEncoder(w).Encode(apiResponse[[]Issue]{
				StatusCode: 1, Results: issues,
				NumberOfTotalResults: len(issues), NumberOfPageResults: len(issues),
			})

		case strings.HasPrefix(path, "/issue/4000-"):
			id, _ := strconv.Atoi(strings.TrimPrefix(path, "/issue/4000-"))
			fx.mu.Lock()
			detail := fx.issueDetails[id]
			fx.mu.Unlock()
			if detail == nil {
				json.NewEncoder(w).Encode(apiResponse[IssueDetail]{StatusCode: 101, Error: "Object Not Found"})
				return
			}
			json.NewEncoder(w).Encode(apiResponse[IssueDetail]{StatusCode: 1, Results: *detail})

		case strings.HasPrefix(path, "/volume/4050-"):
			id, _ := strconv.Atoi(strings.TrimPrefix(path, "/volume/4050-"))
			fx.mu.Lock()
			var found *Volume
			for _, vols := range fx.searchResults {
				for i := range vols {
					if vols[i].ID == id {
						found = &vols[i]
					}
				}
			}
			fx.mu.Unlock()
			if found == nil {
				json.NewEncoder(w).Encode(apiResponse[Volume]{StatusCode: 101, Error: "Object Not Found"})
				return
			}
			json.NewEncoder(w).Encode(apiResponse[Volume]{StatusCode: 1, Results: *found})

		default:
			fx.mu.Lock()
			data, ok := fx.covers[r.URL.Path]
			fx.mu.Unlock()
			if !ok {
				t.Errorf("unexpected request path: %s", r.URL.Path)
				return
			}
			w.Write(data)
		}
	}))
	t.Cleanup(srv.Close)
	return NewClient("test-key", WithBaseURL(srv.URL)), srv.URL
}

func TestGroupBooksBySeries(t *testing.T) {
	books := []*library.ComicBook{
		{ID: "1", FilePath: "Batman 001.cbz"},
		{ID: "2", FilePath: "batman 002.cbz"},
		{ID: "3", FilePath: "X-Men 001.cbz"},
	}
	groups := groupBooksBySeries(books)
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
	if len(groups[0].books) != 2 || groups[0].books[0].ID != "1" || groups[0].books[1].ID != "2" {
		t.Errorf("group 0 = %+v", groups[0])
	}
	if len(groups[1].books) != 1 || groups[1].books[0].ID != "3" {
		t.Errorf("group 1 = %+v", groups[1])
	}
}

func TestScrape_HighConfidenceAutoScrapes(t *testing.T) {
	fx := &scrapeFixture{
		searchResults: map[string][]Volume{
			"Batman": {{ID: 100, Name: "Batman", StartYear: "2016", Publisher: Publisher{Name: "DC Comics"}, CountOfIssues: 85}},
		},
		issuesByVolume: map[int][]Issue{
			100: {{ID: 1001, IssueNumber: "1"}},
		},
		issueDetails: map[int]*IssueDetail{
			1001: {ID: 1001, IssueNumber: "1", Name: "I Am Gotham"},
		},
	}
	client, _ := newScrapeFixtureClient(t, fx)
	cache := testCache(t)
	backend := &fakeBackend{}
	scraper := NewScraper(client, cache, backend, DefaultScraperConfig())

	books := []*library.ComicBook{{ID: "book-1", FilePath: "Batman 001.cbz"}}
	job, err := scraper.Scrape(context.Background(), "job-1", books, ScrapeOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if job.Completed != 1 || job.Failed != 0 || job.PendingReview != 0 {
		t.Errorf("job = %+v", job)
	}
	if len(backend.updated) != 1 {
		t.Fatalf("expected 1 book updated, got %d", len(backend.updated))
	}
	if backend.updated[0].Series != "Batman" || backend.updated[0].Title != "I Am Gotham" {
		t.Errorf("updated book = %+v", backend.updated[0])
	}
}

func TestScrape_AmbiguousQueuesForReview(t *testing.T) {
	ambiguous := []Volume{
		{ID: 1, Name: "Ambiguous Comic", StartYear: "2020", CountOfIssues: 150},
		{ID: 2, Name: "Ambiguous Comic", StartYear: "2020", CountOfIssues: 150},
	}
	fx := &scrapeFixture{searchResults: map[string][]Volume{"Ambiguous Comic": ambiguous}}
	client, _ := newScrapeFixtureClient(t, fx)
	cache := testCache(t)
	backend := &fakeBackend{}
	scraper := NewScraper(client, cache, backend, DefaultScraperConfig())

	books := []*library.ComicBook{{ID: "book-1", FilePath: "Ambiguous Comic 001.cbz"}}
	job, err := scraper.Scrape(context.Background(), "job-1", books, ScrapeOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if job.PendingReview != 1 {
		t.Errorf("job = %+v, want PendingReview=1", job)
	}
	if len(backend.updated) != 0 {
		t.Errorf("expected no writes for ambiguous match, got %d", len(backend.updated))
	}

	pending, err := cache.GetPendingReviewBooks("job-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || len(pending[0].Candidates) != 2 {
		t.Errorf("pending = %+v", pending)
	}
}

func TestScrape_AutoOnlySkipsAmbiguous(t *testing.T) {
	ambiguous := []Volume{
		{ID: 1, Name: "Ambiguous Comic", StartYear: "2020", CountOfIssues: 150},
		{ID: 2, Name: "Ambiguous Comic", StartYear: "2020", CountOfIssues: 150},
	}
	fx := &scrapeFixture{searchResults: map[string][]Volume{"Ambiguous Comic": ambiguous}}
	client, _ := newScrapeFixtureClient(t, fx)
	cache := testCache(t)
	backend := &fakeBackend{}
	scraper := NewScraper(client, cache, backend, DefaultScraperConfig())

	books := []*library.ComicBook{{ID: "book-1", FilePath: "Ambiguous Comic 001.cbz"}}
	job, err := scraper.Scrape(context.Background(), "job-1", books, ScrapeOptions{AutoOnly: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if job.Skipped != 1 || job.PendingReview != 0 {
		t.Errorf("job = %+v, want Skipped=1, PendingReview=0", job)
	}
}

func TestScrape_SeriesGroupingSearchesOnce(t *testing.T) {
	fx := &scrapeFixture{
		searchResults: map[string][]Volume{
			"Batman": {{ID: 100, Name: "Batman", StartYear: "2016", CountOfIssues: 85}},
		},
		issuesByVolume: map[int][]Issue{
			100: {{ID: 1001, IssueNumber: "1"}, {ID: 1002, IssueNumber: "2"}},
		},
		issueDetails: map[int]*IssueDetail{
			1001: {ID: 1001, IssueNumber: "1"},
			1002: {ID: 1002, IssueNumber: "2"},
		},
	}
	client, _ := newScrapeFixtureClient(t, fx)
	cache := testCache(t)
	backend := &fakeBackend{}
	scraper := NewScraper(client, cache, backend, DefaultScraperConfig())

	books := []*library.ComicBook{
		{ID: "book-1", FilePath: "Batman 001.cbz"},
		{ID: "book-2", FilePath: "Batman 002.cbz"},
	}
	job, err := scraper.Scrape(context.Background(), "job-1", books, ScrapeOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if job.Completed != 2 {
		t.Errorf("job.Completed = %d, want 2", job.Completed)
	}
	fx.mu.Lock()
	calls := fx.searchCalls
	fx.mu.Unlock()
	if calls != 1 {
		t.Errorf("searchCalls = %d, want 1 (one search per series group)", calls)
	}
}

func TestScrape_FastRescrapeSkipsSearch(t *testing.T) {
	fx := &scrapeFixture{
		searchResults: map[string][]Volume{
			"Batman": {{ID: 100, Name: "Batman", StartYear: "2016", CountOfIssues: 85}},
		},
		issueDetails: map[int]*IssueDetail{
			1001: {ID: 1001, IssueNumber: "1", Name: "Refreshed Title"},
		},
	}
	client, _ := newScrapeFixtureClient(t, fx)
	cache := testCache(t)
	backend := &fakeBackend{}
	scraper := NewScraper(client, cache, backend, DefaultScraperConfig())

	books := []*library.ComicBook{
		{ID: "book-1", FilePath: "Batman 001.cbz", CustomValuesStore: ",comicvine_volume=100,comicvine_issue=1001"},
	}
	job, err := scraper.Scrape(context.Background(), "job-1", books, ScrapeOptions{FastRescrape: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if job.Completed != 1 {
		t.Errorf("job = %+v", job)
	}
	fx.mu.Lock()
	calls := fx.searchCalls
	fx.mu.Unlock()
	if calls != 0 {
		t.Errorf("searchCalls = %d, want 0 (fast rescrape skips search)", calls)
	}
	if len(backend.updated) != 1 || backend.updated[0].Title != "Refreshed Title" {
		t.Errorf("updated = %+v", backend.updated)
	}
}

func TestScrape_DryRunDoesNotPersist(t *testing.T) {
	fx := &scrapeFixture{
		searchResults: map[string][]Volume{
			"Batman": {{ID: 100, Name: "Batman", StartYear: "2016", CountOfIssues: 85}},
		},
		issuesByVolume: map[int][]Issue{100: {{ID: 1001, IssueNumber: "1"}}},
		issueDetails:   map[int]*IssueDetail{1001: {ID: 1001, IssueNumber: "1"}},
	}
	client, _ := newScrapeFixtureClient(t, fx)
	cache := testCache(t)
	backend := &fakeBackend{}
	scraper := NewScraper(client, cache, backend, DefaultScraperConfig())

	books := []*library.ComicBook{{ID: "book-1", FilePath: "Batman 001.cbz"}}
	job, err := scraper.Scrape(context.Background(), "job-1", books, ScrapeOptions{DryRun: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if job.Completed != 1 {
		t.Errorf("job = %+v", job)
	}
	if len(backend.updated) != 0 {
		t.Errorf("expected no persisted writes in dry-run, got %d", len(backend.updated))
	}
}

func TestScrape_ResumeSkipsAlreadyProcessedBooks(t *testing.T) {
	fx := &scrapeFixture{
		searchResults: map[string][]Volume{
			"Batman": {{ID: 100, Name: "Batman", StartYear: "2016", CountOfIssues: 85}},
		},
		issuesByVolume: map[int][]Issue{100: {{ID: 1002, IssueNumber: "2"}}},
		issueDetails:   map[int]*IssueDetail{1002: {ID: 1002, IssueNumber: "2"}},
	}
	client, _ := newScrapeFixtureClient(t, fx)
	cache := testCache(t)
	backend := &fakeBackend{}
	scraper := NewScraper(client, cache, backend, DefaultScraperConfig())

	// Pre-seed job state as if book-1 was already scraped in a prior run.
	if err := cache.UpsertScrapeJobBook("job-1", &BookScrapeResult{
		BookID: "book-1", Status: BookStatusScraped, VolumeID: 100, IssueID: 1001,
	}); err != nil {
		t.Fatal(err)
	}

	books := []*library.ComicBook{
		{ID: "book-1", FilePath: "Batman 001.cbz"},
		{ID: "book-2", FilePath: "Batman 002.cbz"},
	}
	job, err := scraper.Scrape(context.Background(), "job-1", books, ScrapeOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if job.Total != 2 || job.Completed != 2 {
		t.Errorf("job = %+v, want Total=2 Completed=2 (1 resumed + 1 new)", job)
	}
	if len(backend.updated) != 1 || backend.updated[0].ID != "book-2" {
		t.Errorf("expected only book-2 to be (re)written, got %+v", backend.updated)
	}
}

func TestScraper_AcceptReview(t *testing.T) {
	fx := &scrapeFixture{
		searchResults: map[string][]Volume{
			"Ambiguous Comic": {
				{ID: 1, Name: "Ambiguous Comic", StartYear: "2020", CountOfIssues: 150},
				{ID: 2, Name: "Ambiguous Comic", StartYear: "2020", CountOfIssues: 150},
			},
		},
		issuesByVolume: map[int][]Issue{2: {{ID: 2001, IssueNumber: "1"}}},
		issueDetails:   map[int]*IssueDetail{2001: {ID: 2001, IssueNumber: "1", Name: "Chosen"}},
	}
	client, _ := newScrapeFixtureClient(t, fx)
	cache := testCache(t)
	book := &library.ComicBook{ID: "book-1", FilePath: "Ambiguous Comic 001.cbz"}
	backend := &fakeBackend{books: map[string]*library.ComicBook{"book-1": book}}
	scraper := NewScraper(client, cache, backend, DefaultScraperConfig())

	// First run: queues for review.
	job, err := scraper.Scrape(context.Background(), "job-1", []*library.ComicBook{book}, ScrapeOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if job.PendingReview != 1 {
		t.Fatalf("expected pending review, got job = %+v", job)
	}

	result, err := scraper.AcceptReview(context.Background(), "job-1", "book-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != BookStatusScraped || result.VolumeID != 2 || result.IssueID != 2001 {
		t.Errorf("result = %+v", result)
	}
	if len(backend.updated) != 1 || backend.updated[0].Title != "Chosen" {
		t.Errorf("updated = %+v", backend.updated)
	}

	pending, err := cache.GetPendingReviewBooks("job-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("expected no more pending reviews, got %+v", pending)
	}
}

func TestScrape_NoCandidatesFails(t *testing.T) {
	fx := &scrapeFixture{searchResults: map[string][]Volume{}}
	client, _ := newScrapeFixtureClient(t, fx)
	cache := testCache(t)
	backend := &fakeBackend{}
	scraper := NewScraper(client, cache, backend, DefaultScraperConfig())

	books := []*library.ComicBook{{ID: "book-1", FilePath: "Nonexistent Series 001.cbz"}}
	job, err := scraper.Scrape(context.Background(), "job-1", books, ScrapeOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if job.Failed != 1 {
		t.Errorf("job = %+v, want Failed=1", job)
	}
}

func TestScrape_ConcurrentJobRejected(t *testing.T) {
	fx := &scrapeFixture{searchResults: map[string][]Volume{}}
	client, _ := newScrapeFixtureClient(t, fx)
	cache := testCache(t)
	backend := &fakeBackend{}
	scraper := NewScraper(client, cache, backend, DefaultScraperConfig())

	scraper.mu.Lock()
	scraper.currentJob = &ScrapeJob{ID: "already-running", Status: JobStatusRunning}
	scraper.mu.Unlock()

	_, err := scraper.Scrape(context.Background(), "job-2", nil, ScrapeOptions{}, nil)
	if err != ErrScrapeInProgress {
		t.Errorf("err = %v, want ErrScrapeInProgress", err)
	}
}

func TestScrape_CoverVerifyResolvesAmbiguousTextMatch(t *testing.T) {
	localCover := gradientImage(t, 32, 32)
	matchingCover := gradientImage(t, 32, 32)                    // identical to local -> boosted
	mismatchedCover := solidImage(t, color.RGBA{A: 255}, 32, 32) // very different -> penalized

	fx := &scrapeFixture{
		searchResults: map[string][]Volume{
			"Ambiguous Comic": {
				{ID: 1, Name: "Ambiguous Comic", StartYear: "2020", CountOfIssues: 150},
				{ID: 2, Name: "Ambiguous Comic", StartYear: "2020", CountOfIssues: 150},
			},
		},
		issuesByVolume: map[int][]Issue{2: {{ID: 2001, IssueNumber: "1"}}},
		issueDetails:   map[int]*IssueDetail{2001: {ID: 2001, IssueNumber: "1", Name: "Right Match"}},
		covers: map[string][]byte{
			"/covers/vol1.jpg": mismatchedCover,
			"/covers/vol2.jpg": matchingCover,
		},
	}
	client, baseURL := newScrapeFixtureClient(t, fx)
	fx.searchResults["Ambiguous Comic"][0].Image.SmallURL = baseURL + "/covers/vol1.jpg"
	fx.searchResults["Ambiguous Comic"][1].Image.SmallURL = baseURL + "/covers/vol2.jpg"

	cache := testCache(t)
	backend := &fakeBackend{}
	scraper := NewScraper(client, cache, backend, DefaultScraperConfig())

	path := writeCBZ(t, t.TempDir(), "Ambiguous Comic 001.cbz", map[string][]byte{"page001.png": localCover})
	books := []*library.ComicBook{{ID: "book-1", FilePath: path}}

	job, err := scraper.Scrape(context.Background(), "job-1", books, ScrapeOptions{CoverVerify: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if job.Completed != 1 || job.PendingReview != 0 {
		t.Fatalf("job = %+v, want Completed=1 PendingReview=0", job)
	}
	if len(backend.updated) != 1 || backend.updated[0].Title != "Right Match" {
		t.Errorf("updated = %+v, want the cover-matching volume's metadata applied", backend.updated)
	}
}

func TestScrape_CoverVerifyIgnoredWhenLocalCoverMissing(t *testing.T) {
	fx := &scrapeFixture{
		searchResults: map[string][]Volume{
			"Batman": {{ID: 100, Name: "Batman", StartYear: "2016", CountOfIssues: 85}},
		},
		issuesByVolume: map[int][]Issue{100: {{ID: 1001, IssueNumber: "1"}}},
		issueDetails:   map[int]*IssueDetail{1001: {ID: 1001, IssueNumber: "1"}},
	}
	client, _ := newScrapeFixtureClient(t, fx)
	cache := testCache(t)
	backend := &fakeBackend{}
	scraper := NewScraper(client, cache, backend, DefaultScraperConfig())

	// FilePath points at a file that doesn't exist; cover verification should
	// silently no-op rather than failing the whole scrape.
	books := []*library.ComicBook{{ID: "book-1", FilePath: "/nonexistent/Batman 001.cbz"}}
	job, err := scraper.Scrape(context.Background(), "job-1", books, ScrapeOptions{CoverVerify: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if job.Completed != 1 {
		t.Errorf("job = %+v, want Completed=1 despite missing local cover file", job)
	}
}
