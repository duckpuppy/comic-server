package comicvine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// Per-book scrape outcomes.
const (
	BookStatusScraped       = "scraped"
	BookStatusSkipped       = "skipped"
	BookStatusFailed        = "failed"
	BookStatusPendingReview = "pending_review"
)

// Job-level statuses.
const (
	JobStatusRunning  = "running"
	JobStatusComplete = "complete"
	JobStatusFailed   = "failed"
)

// ErrScrapeInProgress is returned by Scrape when another job is already running.
var ErrScrapeInProgress = errors.New("a scrape job is already in progress")

// reviewCandidatesLimit caps how many candidates are stored for manual review.
const reviewCandidatesLimit = 3

// ScrapeOptions controls how a scrape job selects and processes books.
type ScrapeOptions struct {
	// FastRescrape reuses a book's existing comicvine_volume/issue custom
	// values instead of re-searching and re-matching.
	FastRescrape bool

	// AutoOnly skips (rather than queues for review) books whose best match
	// isn't high confidence.
	AutoOnly bool

	// DryRun computes matches and metadata changes without persisting them.
	DryRun bool

	// BatchSize stops the job after this many books have been processed.
	// 0 means no limit.
	BatchSize int

	// CoverVerify compares the local CBZ cover against the top candidates'
	// cover art (perceptual hash) to sharpen confidence and catch
	// single-issue-vs-TPB confusion. Requires reading the comic file from
	// disk and downloading cover images, so it's opt-in.
	CoverVerify bool
}

// coverVerifyTopN caps how many top candidates get their covers downloaded
// and compared per book.
const coverVerifyTopN = 3

// ReviewCandidate is a lightweight, JSON-serializable summary of a scored
// volume candidate, used for manual review display.
type ReviewCandidate struct {
	VolumeID   int     `json:"volume_id"`
	Name       string  `json:"name"`
	StartYear  string  `json:"start_year"`
	Publisher  string  `json:"publisher"`
	Score      float64 `json:"score"`
	Confidence string  `json:"confidence"`
}

// BookScrapeResult records the outcome of scraping a single book.
type BookScrapeResult struct {
	BookID     string            `json:"book_id"`
	Filename   string            `json:"filename"`
	Series     string            `json:"series"`
	Status     string            `json:"status"`
	VolumeID   int               `json:"volume_id,omitempty"`
	IssueID    int               `json:"issue_id,omitempty"`
	Error      string            `json:"error,omitempty"`
	Candidates []ReviewCandidate `json:"candidates,omitempty"` // populated when Status == pending_review
}

// ScrapeJob tracks progress of a batch scrape run.
type ScrapeJob struct {
	ID            string    `json:"job_id"`
	Status        string    `json:"status"`
	Total         int       `json:"total"`
	Completed     int       `json:"completed"`
	Skipped       int       `json:"skipped"`
	Failed        int       `json:"failed"`
	PendingReview int       `json:"pending_review"`
	StartedAt     time.Time `json:"started_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ProgressFunc is invoked after each book is processed, letting callers
// (CLI, REST/WebSocket layer) surface progress without the scraper needing
// to know about them.
type ProgressFunc func(job *ScrapeJob, result *BookScrapeResult)

// Scraper orchestrates filename parsing, ComicVine search/matching, and
// metadata writing for a set of library books. Only one job runs at a time.
type Scraper struct {
	client  *Client
	cache   *Cache
	backend library.Backend
	cfg     ScraperConfig

	mu         sync.Mutex
	currentJob *ScrapeJob
}

// NewScraper creates a Scraper.
func NewScraper(client *Client, cache *Cache, backend library.Backend, cfg ScraperConfig) *Scraper {
	return &Scraper{client: client, cache: cache, backend: backend, cfg: cfg}
}

// CurrentJob returns the most recently started job (running or finished), or nil.
func (s *Scraper) CurrentJob() *ScrapeJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentJob
}

// Scrape runs a scrape job over books, grouping them by parsed series name
// so each series is searched on ComicVine once no matter how many issues of
// it are being scraped. If jobID matches a previously persisted job, books
// already recorded with a terminal status are skipped, allowing the job to
// resume after an interruption.
func (s *Scraper) Scrape(ctx context.Context, jobID string, books []*library.ComicBook, opts ScrapeOptions, onProgress ProgressFunc) (*ScrapeJob, error) {
	s.mu.Lock()
	if s.currentJob != nil && s.currentJob.Status == JobStatusRunning {
		s.mu.Unlock()
		return nil, ErrScrapeInProgress
	}

	job := &ScrapeJob{ID: jobID, Status: JobStatusRunning, StartedAt: time.Now(), UpdatedAt: time.Now()}
	s.currentJob = job
	s.mu.Unlock()

	prior, err := s.cache.GetScrapeJobBooks(jobID)
	if err != nil {
		return nil, fmt.Errorf("load prior job state: %w", err)
	}
	for _, r := range prior {
		switch r.Status {
		case BookStatusScraped:
			job.Completed++
		case BookStatusSkipped:
			job.Skipped++
		case BookStatusFailed:
			job.Failed++
		case BookStatusPendingReview:
			job.PendingReview++
		}
	}

	pending := make([]*library.ComicBook, 0, len(books))
	for _, b := range books {
		if r, ok := prior[b.ID]; ok && r.Status != "" {
			continue // already processed in a previous run of this job
		}
		pending = append(pending, b)
	}
	job.Total = len(prior) + len(pending)
	s.saveJobMeta(job)

	issueCache := make(map[int][]Issue) // volume CV ID -> issues, reused within this run

	processed := 0
	for _, group := range groupBooksBySeries(pending) {
		if ctx.Err() != nil {
			break
		}

		var candidates []Volume
		var searchErr error
		if groupNeedsSearch(group.books, opts) {
			candidates, searchErr = s.client.SearchVolumes(ctx, group.seriesName)
		}

		for _, book := range group.books {
			if ctx.Err() != nil {
				break
			}

			result := s.scrapeOneBook(ctx, book, candidates, searchErr, issueCache, opts)
			s.recordResult(job, result, onProgress)

			processed++
			if opts.BatchSize > 0 && processed >= opts.BatchSize {
				break
			}
		}
		if opts.BatchSize > 0 && processed >= opts.BatchSize {
			break
		}
	}

	job.Status = JobStatusComplete
	if ctx.Err() != nil {
		job.Status = JobStatusFailed
	}
	job.UpdatedAt = time.Now()
	s.saveJobMeta(job)

	return job, ctx.Err()
}

// AcceptReview resolves a book pending manual review by applying the chosen
// volume, selecting the best issue within it, and writing metadata.
func (s *Scraper) AcceptReview(ctx context.Context, jobID, bookID string, volumeCVID int) (*BookScrapeResult, error) {
	book, err := s.backend.GetBook(bookID)
	if err != nil {
		return nil, fmt.Errorf("get book: %w", err)
	}
	if book == nil {
		return nil, fmt.Errorf("book %s not found", bookID)
	}

	prior, err := s.cache.GetScrapeJobBooks(jobID)
	if err != nil {
		return nil, fmt.Errorf("load job state: %w", err)
	}
	pending, ok := prior[bookID]
	if !ok || pending.Status != BookStatusPendingReview {
		return nil, fmt.Errorf("book %s is not pending review in job %s", bookID, jobID)
	}

	volume, err := s.client.FetchVolume(ctx, volumeCVID)
	if err != nil {
		return nil, fmt.Errorf("fetch volume %d: %w", volumeCVID, err)
	}

	parsed := ParseFilename(book.FilePath)
	issues, err := s.volumeIssues(ctx, volumeCVID, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch issues for volume %d: %w", volumeCVID, err)
	}
	issue := SelectIssue(parsed.IssueNumber, issues)
	if issue == nil {
		result := &BookScrapeResult{BookID: bookID, Filename: book.FilePath, Series: parsed.Series,
			Status: BookStatusFailed, VolumeID: volumeCVID, Error: "no matching issue number found in selected volume"}
		s.cache.UpsertScrapeJobBook(jobID, result)
		return result, nil
	}

	detail, err := s.issueDetail(ctx, issue.ID)
	if err != nil {
		result := &BookScrapeResult{BookID: bookID, Filename: book.FilePath, Series: parsed.Series,
			Status: BookStatusFailed, VolumeID: volumeCVID, IssueID: issue.ID, Error: err.Error()}
		s.cache.UpsertScrapeJobBook(jobID, result)
		return result, nil
	}

	if _, err := WriteMetadata(s.backend, book, *volume, detail, s.cfg); err != nil {
		result := &BookScrapeResult{BookID: bookID, Filename: book.FilePath, Series: parsed.Series,
			Status: BookStatusFailed, VolumeID: volumeCVID, IssueID: issue.ID, Error: err.Error()}
		s.cache.UpsertScrapeJobBook(jobID, result)
		return result, nil
	}

	result := &BookScrapeResult{BookID: bookID, Filename: book.FilePath, Series: parsed.Series,
		Status: BookStatusScraped, VolumeID: volumeCVID, IssueID: issue.ID}
	if err := s.cache.UpsertScrapeJobBook(jobID, result); err != nil {
		return result, fmt.Errorf("save result: %w", err)
	}
	return result, nil
}

func (s *Scraper) scrapeOneBook(ctx context.Context, book *library.ComicBook, candidates []Volume, searchErr error, issueCache map[int][]Issue, opts ScrapeOptions) *BookScrapeResult {
	parsed := ParseFilename(book.FilePath)
	result := &BookScrapeResult{BookID: book.ID, Filename: book.FilePath, Series: parsed.Series}

	existingVolID := extractCVVolumeID(book.CustomValuesStore)
	existingIssID := extractCVIssueID(book.CustomValuesStore)

	if opts.FastRescrape && existingVolID > 0 && existingIssID > 0 {
		detail, err := s.issueDetail(ctx, existingIssID)
		if err != nil {
			result.Status = BookStatusFailed
			result.Error = err.Error()
			return result
		}
		volume, err := s.cachedVolume(ctx, existingVolID)
		if err != nil {
			result.Status = BookStatusFailed
			result.Error = err.Error()
			return result
		}
		return s.applyOrDryRun(book, *volume, detail, existingVolID, existingIssID, opts)
	}

	if searchErr != nil {
		result.Status = BookStatusFailed
		result.Error = fmt.Sprintf("search volumes: %v", searchErr)
		return result
	}
	if len(candidates) == 0 {
		result.Status = BookStatusFailed
		result.Error = "no candidate volumes found"
		return result
	}

	scored := ScoreVolumes(parsed, candidates, existingVolID)

	ambiguousCover := false
	if opts.CoverVerify {
		scored, ambiguousCover = s.verifyCovers(ctx, book, scored)
	}
	best := scored[0]

	if ambiguousCover || best.Confidence != ConfidenceHigh {
		if opts.AutoOnly {
			result.Status = BookStatusSkipped
			reason := fmt.Sprintf("best match confidence %q below auto-only threshold", best.Confidence)
			if ambiguousCover {
				reason = "ambiguous cover match between top candidates"
			}
			result.Error = reason
			return result
		}
		result.Status = BookStatusPendingReview
		result.Candidates = toReviewCandidates(scored)
		return result
	}

	issues, err := s.volumeIssues(ctx, best.Volume.ID, issueCache)
	if err != nil {
		result.Status = BookStatusFailed
		result.Error = fmt.Sprintf("fetch issues: %v", err)
		return result
	}
	issue := SelectIssue(parsed.IssueNumber, issues)
	if issue == nil {
		result.Status = BookStatusFailed
		result.VolumeID = best.Volume.ID
		result.Error = "no matching issue number found in selected volume"
		return result
	}

	detail, err := s.issueDetail(ctx, issue.ID)
	if err != nil {
		result.Status = BookStatusFailed
		result.VolumeID = best.Volume.ID
		result.IssueID = issue.ID
		result.Error = fmt.Sprintf("fetch issue detail: %v", err)
		return result
	}

	return s.applyOrDryRun(book, best.Volume, detail, best.Volume.ID, issue.ID, opts)
}

func (s *Scraper) applyOrDryRun(book *library.ComicBook, volume Volume, detail *IssueDetail, volumeID, issueID int, opts ScrapeOptions) *BookScrapeResult {
	result := &BookScrapeResult{BookID: book.ID, Filename: book.FilePath, Series: volume.Name,
		Status: BookStatusScraped, VolumeID: volumeID, IssueID: issueID}

	if opts.DryRun {
		dryBook := *book
		ApplyMetadata(&dryBook, volume, detail, s.cfg)
		return result
	}

	if _, err := WriteMetadata(s.backend, book, volume, detail, s.cfg); err != nil {
		result.Status = BookStatusFailed
		result.Error = err.Error()
	}
	return result
}

// volumeIssues fetches issues for a volume, preferring the cache, then the
// per-run issueCache (if provided), then the live API (which is then cached
// both locally and in the shared SQLite cache for future scrapes/enrichment).
func (s *Scraper) volumeIssues(ctx context.Context, volumeCVID int, issueCache map[int][]Issue) ([]Issue, error) {
	if issueCache != nil {
		if issues, ok := issueCache[volumeCVID]; ok {
			return issues, nil
		}
	}

	if cached, err := s.cache.GetIssuesForVolume(volumeCVID); err == nil && len(cached) > 0 {
		issues := issuesFromCached(cached)
		if issueCache != nil {
			issueCache[volumeCVID] = issues
		}
		return issues, nil
	}

	issues, err := s.client.FetchVolumeIssues(ctx, volumeCVID)
	if err != nil {
		return nil, err
	}
	if issueCache != nil {
		issueCache[volumeCVID] = issues
	}

	cached := make([]CachedIssue, len(issues))
	for i, iss := range issues {
		cached[i] = CachedIssue{CVID: iss.ID, VolumeCVID: volumeCVID, Number: iss.IssueNumber,
			Name: iss.Name, CoverDate: iss.CoverDate, StoreDate: iss.StoreDate}
	}
	if err := s.cache.UpsertIssues(cached); err != nil {
		log.Warn().Err(err).Int("volume_cv_id", volumeCVID).Msg("scrape: failed to cache issues")
	}

	return issues, nil
}

func (s *Scraper) issueDetail(ctx context.Context, issueCVID int) (*IssueDetail, error) {
	if cached, err := s.cache.GetIssueDetail(issueCVID); err == nil && cached != nil {
		return cached, nil
	}
	detail, err := s.client.FetchIssueDetail(ctx, issueCVID)
	if err != nil {
		return nil, err
	}
	if err := s.cache.UpsertIssueDetail(detail); err != nil {
		log.Warn().Err(err).Int("issue_cv_id", issueCVID).Msg("scrape: failed to cache issue detail")
	}
	return detail, nil
}

func (s *Scraper) cachedVolume(ctx context.Context, volumeCVID int) (*Volume, error) {
	if cached, err := s.cache.GetVolume(volumeCVID); err == nil && cached != nil && cached.Name != "" {
		return &Volume{ID: cached.CVID, Name: cached.Name, StartYear: cached.StartYear,
			Publisher: Publisher{Name: cached.Publisher}, CountOfIssues: cached.IssueCount,
			SiteDetailURL: cached.SiteURL}, nil
	}
	return s.client.FetchVolume(ctx, volumeCVID)
}

// verifyCovers compares the local comic's cover against the top-N scored
// candidates' cover art, adjusting scores and confidence accordingly. If the
// local cover can't be extracted or hashed (missing file, CBR archive,
// unsupported format), it returns scored unchanged — cover verification is
// a best-effort sharpening step, never a hard requirement.
func (s *Scraper) verifyCovers(ctx context.Context, book *library.ComicBook, scored []MatchResult) ([]MatchResult, bool) {
	if len(scored) == 0 {
		return scored, false
	}

	localData, err := ExtractCover(book.FilePath)
	if err != nil {
		log.Debug().Err(err).Str("book_id", book.ID).Msg("scrape: cover extraction failed, skipping cover verification")
		return scored, false
	}
	localHash, err := ComputeDHash(localData)
	if err != nil {
		log.Debug().Err(err).Str("book_id", book.ID).Msg("scrape: cover hash failed, skipping cover verification")
		return scored, false
	}

	n := min(len(scored), coverVerifyTopN)
	hashes := make(map[int]CoverHash, n)
	for i := range n {
		volID := scored[i].Volume.ID
		if hash, ok, err := s.cache.GetVolumeCoverHash(volID); err == nil && ok {
			hashes[volID] = hash
			continue
		}
		hash, err := s.client.DownloadCoverHash(ctx, scored[i].Volume.Image.SmallURL)
		if err != nil {
			log.Debug().Err(err).Int("volume_cv_id", volID).Msg("scrape: cover download failed")
			continue
		}
		if err := s.cache.SaveVolumeCoverHash(volID, hash); err != nil {
			log.Warn().Err(err).Int("volume_cv_id", volID).Msg("scrape: failed to cache cover hash")
		}
		hashes[volID] = hash
	}
	if len(hashes) == 0 {
		return scored, false
	}

	ambiguous := AmbiguousByCover(scored, hashes)
	scored = ApplyCoverVerification(scored, localHash, hashes)
	return scored, ambiguous
}

func (s *Scraper) recordResult(job *ScrapeJob, result *BookScrapeResult, onProgress ProgressFunc) {
	switch result.Status {
	case BookStatusScraped:
		job.Completed++
	case BookStatusSkipped:
		job.Skipped++
	case BookStatusFailed:
		job.Failed++
	case BookStatusPendingReview:
		job.PendingReview++
	}
	job.UpdatedAt = time.Now()

	if err := s.cache.UpsertScrapeJobBook(job.ID, result); err != nil {
		log.Warn().Err(err).Str("book_id", result.BookID).Msg("scrape: failed to persist book result")
	}
	s.saveJobMeta(job)

	if onProgress != nil {
		onProgress(job, result)
	}
}

func (s *Scraper) saveJobMeta(job *ScrapeJob) {
	if err := s.cache.SaveScrapeJobMeta(job); err != nil {
		log.Warn().Err(err).Str("job_id", job.ID).Msg("scrape: failed to persist job state")
	}
}

func toReviewCandidates(scored []MatchResult) []ReviewCandidate {
	n := min(len(scored), reviewCandidatesLimit)
	out := make([]ReviewCandidate, n)
	for i := range n {
		out[i] = ReviewCandidate{
			VolumeID:   scored[i].Volume.ID,
			Name:       scored[i].Volume.Name,
			StartYear:  scored[i].Volume.StartYear,
			Publisher:  scored[i].Volume.Publisher.Name,
			Score:      scored[i].Score,
			Confidence: scored[i].Confidence,
		}
	}
	return out
}

func issuesFromCached(cached []CachedIssue) []Issue {
	issues := make([]Issue, len(cached))
	for i, c := range cached {
		issues[i] = Issue{ID: c.CVID, IssueNumber: c.Number, Name: c.Name, CoverDate: c.CoverDate, StoreDate: c.StoreDate}
	}
	return issues
}

// groupNeedsSearch reports whether any book in the group requires a
// ComicVine search (i.e. isn't eligible for the fast-rescrape shortcut).
func groupNeedsSearch(books []*library.ComicBook, opts ScrapeOptions) bool {
	if !opts.FastRescrape {
		return true
	}
	for _, b := range books {
		if extractCVVolumeID(b.CustomValuesStore) == 0 || extractCVIssueID(b.CustomValuesStore) == 0 {
			return true
		}
	}
	return false
}

type seriesGroup struct {
	seriesName string
	books      []*library.ComicBook
}

// groupBooksBySeries groups books by their parsed series name (case-insensitive),
// preserving first-seen order, so each series is searched on ComicVine once.
func groupBooksBySeries(books []*library.ComicBook) []seriesGroup {
	var order []string
	byKey := make(map[string]*seriesGroup)

	for _, b := range books {
		parsed := ParseFilename(b.FilePath)
		key := strings.ToLower(strings.TrimSpace(parsed.Series))
		g, ok := byKey[key]
		if !ok {
			g = &seriesGroup{seriesName: parsed.Series}
			byKey[key] = g
			order = append(order, key)
		}
		g.books = append(g.books, b)
	}

	groups := make([]seriesGroup, len(order))
	for i, k := range order {
		groups[i] = *byKey[k]
	}
	return groups
}
