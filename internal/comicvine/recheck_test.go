package comicvine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/localid"
)

func localBook(id, series, number string) library.ComicBook {
	b := library.ComicBook{ID: id, Series: series, Number: number, FilePath: series + " " + number + ".cbz"}
	localid.Set(&b, localid.Identity{Volume: "lv-0000000001", Issue: "li-" + id})
	return b
}

func TestRecheck_QueuesSuggestionAndNeverWritesTags(t *testing.T) {
	fx := &scrapeFixture{searchResults: map[string][]Volume{
		"Ghost Rider Epic": {{ID: 77, Name: "Ghost Rider Epic", StartYear: "2020", CountOfIssues: 6}},
	}}
	client, _ := newScrapeFixtureClient(t, fx)
	cache := testCache(t)
	books := []library.ComicBook{localBook("b1", "Ghost Rider Epic", "1")}
	backend := &fakeBackend{books: map[string]*library.ComicBook{"b1": &books[0]}}
	s := NewScraper(client, cache, backend, DefaultScraperConfig())

	res, err := s.RecheckLocalIdentity(context.Background(), books)
	if err != nil {
		t.Fatal(err)
	}
	if res.Suggested != 1 || res.Searches != 1 {
		t.Fatalf("res = %+v", res)
	}
	pending, _ := cache.GetPendingReviewBooks("")
	if len(pending) != 1 || pending[0].Error != RecheckNote || pending[0].Candidates[0].VolumeID != 77 {
		t.Fatalf("pending = %+v", pending)
	}
	if len(backend.updated) != 0 {
		t.Error("a re-check must never write the book")
	}
	if _, ok := library.GetCustomValue(books[0].CustomValuesStore, "comicvine_volume"); ok {
		t.Error("no comicvine_* tag may be written")
	}
	if !localid.Has(&books[0]) {
		t.Error("identity must stay until the person accepts")
	}

	// A second pass does not search or queue again while the suggestion waits.
	res, _ = s.RecheckLocalIdentity(context.Background(), books)
	if res.Eligible != 0 || fx.searchCalls != 1 {
		t.Errorf("second pass = %+v, search calls %d", res, fx.searchCalls)
	}
}

func TestRecheck_DismissIsPermanentAndKeepsIdentity(t *testing.T) {
	fx := &scrapeFixture{searchResults: map[string][]Volume{
		"Ghost Rider Epic": {{ID: 77, Name: "Ghost Rider Epic", StartYear: "2020", CountOfIssues: 6}},
	}}
	client, _ := newScrapeFixtureClient(t, fx)
	cache := testCache(t)
	books := []library.ComicBook{localBook("b1", "Ghost Rider Epic", "1")}
	s := NewScraper(client, cache, &fakeBackend{}, DefaultScraperConfig())
	if _, err := s.RecheckLocalIdentity(context.Background(), books); err != nil {
		t.Fatal(err)
	}
	if err := cache.DismissRecheck("b1"); err != nil {
		t.Fatal(err)
	}
	if pending, _ := cache.GetPendingReviewBooks(""); len(pending) != 0 {
		t.Errorf("dismissed suggestion still queued: %+v", pending)
	}
	if d, _ := cache.RecheckDismissed("b1"); !d {
		t.Error("not recorded as dismissed")
	}
	// Even long after, a dismissed book is not searched again.
	if err := cache.setRecheck("b1", recheckDismissed, time.Now().Add(-2*RecheckAgainAfter)); err != nil {
		t.Fatal(err)
	}
	res, _ := s.RecheckLocalIdentity(context.Background(), books)
	if res.Eligible != 0 || fx.searchCalls != 1 {
		t.Errorf("res = %+v, search calls %d", res, fx.searchCalls)
	}
}

func TestRecheck_NoConvincingMatchWaitsThenRechecks(t *testing.T) {
	fx := &scrapeFixture{searchResults: map[string][]Volume{
		// One hit sharing no words with the series: not a match.
		"Obscure Omnibus": {{ID: 5, Name: "Totally Different Title", StartYear: "2001", CountOfIssues: 3}},
	}}
	client, _ := newScrapeFixtureClient(t, fx)
	cache := testCache(t)
	books := []library.ComicBook{localBook("b1", "Obscure Omnibus", "1"), localBook("b2", "Obscure Omnibus", "2")}
	s := NewScraper(client, cache, &fakeBackend{}, DefaultScraperConfig())

	res, err := s.RecheckLocalIdentity(context.Background(), books)
	if err != nil {
		t.Fatal(err)
	}
	if res.Searches != 1 || res.NotFound != 2 || res.Suggested != 0 {
		t.Errorf("res = %+v (siblings share one search)", res)
	}
	if pending, _ := cache.GetPendingReviewBooks(""); len(pending) != 0 {
		t.Errorf("queued %+v", pending)
	}
	res, _ = s.RecheckLocalIdentity(context.Background(), books)
	if res.Eligible != 0 {
		t.Errorf("rechecked too soon: %+v", res)
	}
	_ = cache.setRecheck("b1", recheckChecked, time.Now().Add(-2*RecheckAgainAfter))
	_ = cache.setRecheck("b2", recheckChecked, time.Now().Add(-2*RecheckAgainAfter))
	res, _ = s.RecheckLocalIdentity(context.Background(), books)
	if res.Eligible != 2 || res.Searches != 1 {
		t.Errorf("after the interval: %+v", res)
	}
}

func TestRecheck_SkipsBooksWithoutIdentityOrFile(t *testing.T) {
	fx := &scrapeFixture{}
	client, _ := newScrapeFixtureClient(t, fx)
	s := NewScraper(client, testCache(t), &fakeBackend{}, DefaultScraperConfig())
	plain := library.ComicBook{ID: "p", Series: "X", FilePath: "x.cbz"}
	wanted := localBook("w", "Y", "1")
	wanted.FilePath = ""
	res, err := s.RecheckLocalIdentity(context.Background(), []library.ComicBook{plain, wanted})
	if err != nil || res.Eligible != 0 || fx.searchCalls != 0 {
		t.Errorf("res = %+v, err %v, calls %d", res, err, fx.searchCalls)
	}
}

func TestRecheck_SearchCapPerPass(t *testing.T) {
	fx := &scrapeFixture{searchResults: map[string][]Volume{}}
	client, _ := newScrapeFixtureClient(t, fx)
	s := NewScraper(client, testCache(t), &fakeBackend{}, DefaultScraperConfig())
	var books []library.ComicBook
	for i := 0; i < RecheckMaxSearches+3; i++ {
		books = append(books, localBook(string(rune('a'+i)), "Series "+string(rune('A'+i)), "1"))
	}
	res, err := s.RecheckLocalIdentity(context.Background(), books)
	if err != nil {
		t.Fatal(err)
	}
	if res.Searches != RecheckMaxSearches || fx.searchCalls != RecheckMaxSearches {
		t.Errorf("searches = %d / %d, want %d", res.Searches, fx.searchCalls, RecheckMaxSearches)
	}
}

func TestRecheck_StopsOnOpenCircuitWithoutCallingComicVine(t *testing.T) {
	fx := &scrapeFixture{}
	client, _ := newScrapeFixtureClient(t, fx)
	client.Circuit().RecordFailure()
	cache := testCache(t)
	s := NewScraper(client, cache, &fakeBackend{}, DefaultScraperConfig())
	books := []library.ComicBook{localBook("b1", "Alpha", "1"), localBook("b2", "Beta", "1")}
	res, err := s.RecheckLocalIdentity(context.Background(), books)
	var open ErrCircuitOpen
	if !errors.As(err, &open) {
		t.Fatalf("err = %v, want ErrCircuitOpen", err)
	}
	if fx.searchCalls != 0 || res.Searches != 1 {
		t.Errorf("calls %d, searches %d: the pass must stop at the first failure", fx.searchCalls, res.Searches)
	}
	// Nothing was recorded as checked, so the books are tried again later.
	states, _ := cache.recheckStates()
	if len(states) != 0 {
		t.Errorf("states = %+v", states)
	}
}
