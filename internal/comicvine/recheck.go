package comicvine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/localid"
)

// Background re-check of local-identity books (tracker #47).
//
// A book with a local identity is held back from scrapes. This asks
// ComicVine, now and then and at background priority, whether it has since
// listed one of them, and queues what it finds in Scrape Review as a
// SUGGESTION. It never writes a tag: the person accepts (the normal Scrape
// Review accept, which retires the identity) or dismisses.

// RecheckNote is the Error text on the pending-review row of a suggestion.
// It is how Scrape Review tells a suggestion from an ordinary pending book.
const RecheckNote = "local identity re-check"

// RecheckJobID is the scrape job that holds the suggestion rows. It has no
// scrape_jobs row, so it never shows up as the latest scrape.
const RecheckJobID = "localid-recheck"

const (
	recheckChecked   = "checked"   // searched, nothing convincing
	recheckSuggested = "suggested" // a suggestion is (or was) queued
	recheckDismissed = "dismissed" // the person said no; never searched again

	// RecheckAgainAfter is how long a book with no result waits before it
	// is searched again.
	RecheckAgainAfter = 90 * 24 * time.Hour

	// RecheckMaxSearches caps ComicVine searches (one per series name) in a
	// single pass.
	RecheckMaxSearches = 5
)

func (c *Cache) ensureRecheckTable() error {
	_, err := c.db.Exec(`CREATE TABLE IF NOT EXISTS localid_recheck (
		book_id    TEXT PRIMARY KEY,
		outcome    TEXT NOT NULL,
		checked_at TEXT NOT NULL
	)`)
	return err
}

type recheckState struct {
	outcome string
	at      time.Time
}

func (c *Cache) recheckStates() (map[string]recheckState, error) {
	rows, err := c.db.Query(`SELECT book_id, outcome, checked_at FROM localid_recheck`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]recheckState{}
	for rows.Next() {
		var id, outcome, at string
		if err := rows.Scan(&id, &outcome, &at); err != nil {
			return nil, err
		}
		t, _ := time.Parse(time.RFC3339, at)
		out[id] = recheckState{outcome: outcome, at: t}
	}
	return out, rows.Err()
}

func (c *Cache) setRecheck(bookID, outcome string, at time.Time) error {
	_, err := c.db.Exec(`INSERT INTO localid_recheck (book_id, outcome, checked_at) VALUES (?, ?, ?)
		ON CONFLICT(book_id) DO UPDATE SET outcome = excluded.outcome, checked_at = excluded.checked_at`,
		bookID, outcome, at.UTC().Format(time.RFC3339))
	return err
}

// DismissRecheck records that the person does not want this suggestion:
// the book is never searched again and its queued suggestion is removed.
// The local identity stays.
func (c *Cache) DismissRecheck(bookID string) error {
	if err := c.setRecheck(bookID, recheckDismissed, time.Now()); err != nil {
		return err
	}
	_, err := c.db.Exec(`DELETE FROM scrape_job_books WHERE book_id = ? AND status = ? AND error = ?`,
		bookID, BookStatusPendingReview, RecheckNote)
	return err
}

// RecheckDismissed reports whether a suggestion for the book was dismissed.
func (c *Cache) RecheckDismissed(bookID string) (bool, error) {
	var outcome string
	err := c.db.QueryRow(`SELECT outcome FROM localid_recheck WHERE book_id = ?`, bookID).Scan(&outcome)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return outcome == recheckDismissed, err
}

// RecheckResult summarises one pass.
type RecheckResult struct {
	Eligible  int // local-identity books due for a check
	Searches  int // ComicVine searches made
	Suggested int // books newly queued in Scrape Review
	NotFound  int // books checked with no convincing volume
}

// RecheckLocalIdentity runs one background pass over books (work from library
// metadata only: nothing here opens a file). A book is due when it has a
// local identity and a file, was never checked or was last checked more than
// RecheckAgainAfter ago, has no queued suggestion and was not dismissed.
// Books of one series share a search; at most RecheckMaxSearches searches are
// made. The first failure (circuit open, pacing, a timeout, an API error)
// stops the pass and is returned: ComicVine is burst-hostile, so nothing is
// retried here.
func (s *Scraper) RecheckLocalIdentity(ctx context.Context, books []library.ComicBook) (RecheckResult, error) {
	var res RecheckResult
	ctx = WithBackground(ctx)

	states, err := s.cache.recheckStates()
	if err != nil {
		return res, fmt.Errorf("load re-check state: %w", err)
	}
	now := time.Now()

	groups := map[string][]*library.ComicBook{}
	var order []string
	for i := range books {
		b := &books[i]
		if b.FilePath == "" || !localid.Has(b) {
			continue
		}
		key := localid.SeriesKey(b.Series)
		if key == "" {
			continue
		}
		if st, ok := states[b.ID]; ok {
			if st.outcome == recheckDismissed || st.outcome == recheckSuggested {
				continue
			}
			if now.Sub(st.at) < RecheckAgainAfter {
				continue
			}
		}
		res.Eligible++
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], b)
	}
	sort.Strings(order) // a stable order, so a restart does not reshuffle the queue

	for _, key := range order {
		if res.Searches >= RecheckMaxSearches {
			break
		}
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		group := groups[key]
		name := strings.Join(strings.Fields(group[0].Series), " ")

		candidates, err := s.client.SearchVolumes(ctx, name)
		res.Searches++
		if err != nil {
			return res, fmt.Errorf("search %q: %w", name, err)
		}
		if f := s.publisherFilter(); f.Active() {
			candidates = f.FilterVolumes(candidates)
		}

		for _, b := range group {
			cands := recheckCandidates(b, candidates)
			if len(cands) == 0 {
				res.NotFound++
				if err := s.cache.setRecheck(b.ID, recheckChecked, now); err != nil {
					return res, err
				}
				continue
			}
			row := &BookScrapeResult{BookID: b.ID, Filename: b.FilePath, Series: b.Series,
				Status: BookStatusPendingReview, Error: RecheckNote, Candidates: cands}
			if err := s.cache.UpsertScrapeJobBook(RecheckJobID, row); err != nil {
				return res, err
			}
			if err := s.cache.setRecheck(b.ID, recheckSuggested, now); err != nil {
				return res, err
			}
			res.Suggested++
		}
	}
	return res, nil
}

// recheckCandidates scores the volumes for a book and returns the review
// candidates, or none when the best is not convincing: low confidence, or a
// name that shares no words with the book's series (a single odd search hit
// would otherwise score "high"). Unknown is not a match.
func recheckCandidates(b *library.ComicBook, volumes []Volume) []ReviewCandidate {
	if len(volumes) == 0 {
		return nil
	}
	parsed := ParsedFilename{Series: b.Series, IssueNumber: b.Number, Year: b.Year}
	scored := ScoreVolumes(parsed, volumes, 0)
	best := scored[0]
	if best.Confidence == ConfidenceLow || nameScore(b.Series, best.Volume.Name) <= 0 {
		return nil
	}
	return toReviewCandidates(scored)
}
