package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
	ws "github.com/duckpuppy/comic-server/internal/websocket"
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
	if len(books) == 0 {
		http.Error(w, "No books to scrape", http.StatusBadRequest)
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

	s.writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
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
	s.writeJSON(w, http.StatusOK, pending)
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

	s.wsHub.Broadcast(ws.EventScrapeProgress, map[string]any{
		"job_id":  jobID,
		"book_id": result.BookID,
		"status":  result.Status,
		"series":  result.Series,
		"issue":   result.IssueID,
	})

	s.writeJSON(w, http.StatusOK, result)
}
