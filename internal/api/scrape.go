package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
	ws "github.com/duckpuppy/comic-server/internal/websocket"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

// scrapeRequest is the POST /api/scrape body.
type scrapeRequest struct {
	BookIDs []string             `json:"book_ids"`
	Options scrapeRequestOptions `json:"options"`
}

type scrapeRequestOptions struct {
	AutoOnly    bool `json:"auto_only"`
	Rescrape    bool `json:"rescrape"`
	DryRun      bool `json:"dry_run"`
	BatchSize   int  `json:"batch_size"`
	CoverVerify bool `json:"cover_verify"`
	// RetryHeldBack includes, in a whole-library scrape, books held back
	// for a past outcome (comic-server-fohc). CVDBSKIP books stay out.
	RetryHeldBack bool `json:"retry_held_back"`
}

type scrapeReviewResolveRequest struct {
	VolumeCVID int `json:"volume_cv_id"`
}

// handleScrapeStart starts a background scrape job. POST /api/scrape
func (s *Server) handleScrapeStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.scraper == nil {
		http.Error(w, "ComicVine scraper is not configured", http.StatusServiceUnavailable)
		return
	}

	var req scrapeRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
	}

	if job := s.scraper.CurrentJob(); job != nil && job.Status == comicvine.JobStatusRunning {
		http.Error(w, "A scrape job is already in progress", http.StatusConflict)
		return
	}

	allBooks, err := s.backend.GetAllBooks()
	if err != nil {
		http.Error(w, "Failed to load books", http.StatusInternalServerError)
		return
	}
	books := selectScrapeBooks(allBooks, req.BookIDs, req.Options.Rescrape)
	var heldBack map[string]int
	if len(req.BookIDs) == 0 {
		books, heldBack = s.filterHeldBack(books, req.Options.RetryHeldBack)
	}
	if len(books) == 0 {
		msg := "No books to scrape"
		if n := sumCounts(heldBack); n > 0 {
			msg = fmt.Sprintf("No books to scrape (%d held back: %s)", n, describeCounts(heldBack))
		}
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	jobID := fmt.Sprintf("api-%d", time.Now().UnixNano())
	opts := comicvine.ScrapeOptions{
		FastRescrape: req.Options.Rescrape,
		AutoOnly:     req.Options.AutoOnly,
		DryRun:       req.Options.DryRun,
		BatchSize:    req.Options.BatchSize,
		CoverVerify:  req.Options.CoverVerify,
	}

	s.wsHub.Broadcast(ws.EventScrapeStarted, map[string]any{
		"job_id":      jobID,
		"total_books": len(books),
	})

	go s.runScrapeJob(jobID, books, opts)

	s.writeJSON(w, http.StatusAccepted, map[string]any{"job_id": jobID, "total_books": len(books), "held_back": heldBack})
}

// filterHeldBack drops, from a whole-library scrape, books a retry would
// not help (comic-server-fohc): tagged CVDBSKIP by the user (always), or
// whose last scrape outcome needs the user (comicvine.HeldBackReason;
// skipped when retry is set). Returns the kept books and a count per
// reason. Without a cache only CVDBSKIP applies.
func (s *Server) filterHeldBack(books []*library.ComicBook, retry bool) ([]*library.ComicBook, map[string]int) {
	var latest map[string]*comicvine.BookScrapeResult
	if s.cvCache != nil && !retry {
		var err error
		if latest, err = s.cvCache.LatestScrapeResults(); err != nil {
			log.Warn().Err(err).Msg("Scrape: could not read past outcomes; not holding any back")
		}
	}
	counts := map[string]int{}
	kept := books[:0:0]
	for _, b := range books {
		reason := ""
		if workflow.IsCVDBSkip(b) {
			reason = "tagged CVDBSKIP"
		} else if workflow.IsNoCVMatch(b) {
			reason = "marked not in ComicVine"
		} else {
			reason = comicvine.HeldBackReason(latest[b.ID])
		}
		if reason != "" {
			counts[reason]++
			continue
		}
		kept = append(kept, b)
	}
	return kept, counts
}

func sumCounts(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

// describeCounts renders {"a": 2, "b": 1} as "2 a, 1 b", largest first.
func describeCounts(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%d %s", m[k], k)
	}
	return strings.Join(parts, ", ")
}

// runScrapeJob runs a scrape job to completion, broadcasting WebSocket
// progress events. It's meant to be called in its own goroutine.
func (s *Server) runScrapeJob(jobID string, books []*library.ComicBook, opts comicvine.ScrapeOptions) {
	job, err := s.scraper.Scrape(context.Background(), jobID, books, opts, func(job *comicvine.ScrapeJob, result *comicvine.BookScrapeResult) {
		s.wsHub.Broadcast(ws.EventScrapeProgress, map[string]any{
			"job_id":  job.ID,
			"book_id": result.BookID,
			"status":  result.Status,
			"series":  result.Series,
			"issue":   result.IssueID,
		})
		if result.Status == comicvine.BookStatusPendingReview {
			s.wsHub.Broadcast(ws.EventScrapeReviewNeeded, map[string]any{
				"book_id":    result.BookID,
				"filename":   result.Filename,
				"candidates": result.Candidates,
			})
		}
	})
	if err != nil {
		log.Warn().Err(err).Str("job_id", jobID).Msg("scrape job ended with error")
	}

	stats := map[string]any{}
	if job != nil {
		stats = map[string]any{
			"total": job.Total, "completed": job.Completed, "skipped": job.Skipped,
			"failed": job.Failed, "pending_review": job.PendingReview,
		}
	}
	s.InvalidateWorkflowCache()
	s.wsHub.Broadcast(ws.EventScrapeCompleted, map[string]any{"job_id": jobID, "stats": stats})
}

// selectScrapeBooks resolves the requested book IDs (or, if empty, all
// books) into the set of books to scrape, excluding already-tagged books
// unless rescrape is requested.
func selectScrapeBooks(allBooks []library.ComicBook, bookIDs []string, rescrape bool) []*library.ComicBook {
	if len(bookIDs) > 0 {
		wanted := make(map[string]bool, len(bookIDs))
		for _, id := range bookIDs {
			wanted[id] = true
		}
		var out []*library.ComicBook
		for i := range allBooks {
			if wanted[allBooks[i].ID] {
				out = append(out, &allBooks[i])
			}
		}
		return out
	}

	var out []*library.ComicBook
	for i := range allBooks {
		if comicvine.HasComicVineTag(allBooks[i].CustomValuesStore) && !rescrape {
			continue
		}
		out = append(out, &allBooks[i])
	}
	return out
}

// handleScrapeStatus returns the current/last scrape job's progress. GET /api/scrape/status
func (s *Server) handleScrapeStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.scraper == nil {
		http.Error(w, "ComicVine scraper is not configured", http.StatusServiceUnavailable)
		return
	}

	job := s.scraper.CurrentJob()
	if job == nil {
		s.writeJSON(w, http.StatusOK, map[string]string{"status": "none"})
		return
	}
	s.writeJSON(w, http.StatusOK, job)
}

// handleScrapeReviewList returns books pending manual review. GET /api/scrape/review
func (s *Server) handleScrapeReviewList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cvCache == nil {
		http.Error(w, "ComicVine scraper is not configured", http.StatusServiceUnavailable)
		return
	}

	pending, err := s.cvCache.GetPendingReviewBooks("")
	if err != nil {
		http.Error(w, "Failed to load pending reviews", http.StatusInternalServerError)
		return
	}
	out := make([]scrapeReviewItem, 0, len(pending))
	for _, p := range pending {
		item := scrapeReviewItem{BookScrapeResult: p}
		if b, err := s.backend.GetBook(p.BookID); err == nil && b != nil {
			if workflow.IsNoCVMatch(b) {
				continue // the user already decided; the old pending result is moot
			}
			item.Book = &scrapeReviewBook{
				Series: b.Series, Number: b.Number, Year: b.Year,
				PageCount: b.PageCount, Format: b.Format, Publisher: b.Publisher,
			}
			item.NeedsIssue = comicvine.NeedsIssueNumber(b)
			if item.NeedsIssue {
				item.OnlyIssues = s.onlyIssuePicks(p.Candidates)
			}
		}
		out = append(out, item)
	}
	s.writeJSON(w, http.StatusOK, out)
}

// scrapeReviewItem is a pending result plus the book's own details, so the
// reviewer can compare it with each candidate (cover, pages, year, number).
type scrapeReviewItem struct {
	*comicvine.BookScrapeResult
	Book *scrapeReviewBook `json:"book,omitempty"`
	// NeedsIssue: the book has no issue number, so the user picks the issue
	// from the volume's list (comic-server-5wer phase 3).
	NeedsIssue bool `json:"needs_issue,omitempty"`
	// OnlyIssues maps a candidate volume id to its single issue, for
	// candidates ComicVine lists with exactly one issue (comic-server-5wer
	// phase 5). The review page offers it as the pick; the user still
	// confirms it in the match preview.
	OnlyIssues map[int]reviewIssuePick `json:"only_issues,omitempty"`
}

// reviewIssuePick is a ready-made issue choice on the review page.
type reviewIssuePick struct {
	IssueID int    `json:"issue_id"`
	Number  string `json:"number"`
	Name    string `json:"name,omitempty"`
}

// onlyIssuePicks finds the candidates that are one-issue volumes and whose
// one issue is already in the cache. Cache only: no ComicVine call is made
// while listing the review queue. A candidate whose issue list is not cached
// keeps the plain "Choose issue" path.
func (s *Server) onlyIssuePicks(cands []comicvine.ReviewCandidate) map[int]reviewIssuePick {
	var picks map[int]reviewIssuePick
	for _, c := range cands {
		if c.IssueCount != 1 {
			continue
		}
		issues, err := s.cvCache.GetIssuesForVolume(c.VolumeID)
		if err != nil || len(issues) != 1 {
			continue
		}
		if picks == nil {
			picks = make(map[int]reviewIssuePick)
		}
		picks[c.VolumeID] = reviewIssuePick{IssueID: issues[0].CVID, Number: issues[0].Number, Name: issues[0].Name}
	}
	return picks
}

type scrapeReviewBook struct {
	Series    string `json:"series"`
	Number    string `json:"number"`
	Year      int    `json:"year"`
	PageCount int    `json:"page_count"`
	Format    string `json:"format"`
	Publisher string `json:"publisher"`
}

// handleScrapeReviewResolve resolves a pending review by applying the chosen
// volume. POST /api/scrape/review/:bookId
func (s *Server) handleScrapeReviewResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.scraper == nil {
		http.Error(w, "ComicVine scraper is not configured", http.StatusServiceUnavailable)
		return
	}

	bookID := parsePathParam(r.URL.Path, "/api/scrape/review/")
	if bookID == "" {
		http.Error(w, "Book ID is required", http.StatusBadRequest)
		return
	}

	var req scrapeReviewResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.VolumeCVID == 0 {
		http.Error(w, "volume_cv_id is required", http.StatusBadRequest)
		return
	}

	jobID := ""
	if job := s.scraper.CurrentJob(); job != nil {
		jobID = job.ID
	}

	result, err := s.scraper.AcceptReview(r.Context(), jobID, bookID, req.VolumeCVID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.InvalidateWorkflowCache()

	s.wsHub.Broadcast(ws.EventScrapeProgress, map[string]any{
		"job_id":  jobID,
		"book_id": result.BookID,
		"status":  result.Status,
		"series":  result.Series,
		"issue":   result.IssueID,
	})

	s.writeJSON(w, http.StatusOK, result)
}
