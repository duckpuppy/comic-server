package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/spf13/cobra"
)

var (
	scrapeLibraryPath string
	scrapeAll         bool
	scrapeRescrape    bool
	scrapeDryRun      bool
	scrapeAutoOnly    bool
	scrapeBatchSize   int
	scrapeSeries      string
)

const defaultScrapeJobID = "cli"

var scrapeCmd = &cobra.Command{
	Use:   "scrape [book-ids...]",
	Short: "Scrape ComicVine metadata into the library",
	Long: `Scrape ComicVine metadata for library books: parses filenames, searches and
matches ComicVine volumes/issues, and writes metadata back to the library.

With no book IDs, scrapes all books that don't yet have ComicVine identity
tags. Use --rescrape to also refresh books that are already tagged.`,
	RunE: runScrape,
}

var scrapeStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the most recent scrape job status",
	RunE:  runScrapeStatus,
}

var scrapeReviewCmd = &cobra.Command{
	Use:   "review",
	Short: "List books pending manual review from the last scrape job",
	RunE:  runScrapeReview,
}

var scrapeAcceptCmd = &cobra.Command{
	Use:   "accept BOOK_ID VOLUME_CV_ID",
	Short: "Accept a candidate volume for a book pending review",
	Args:  cobra.ExactArgs(2),
	RunE:  runScrapeAccept,
}

func init() {
	rootCmd.AddCommand(scrapeCmd)
	scrapeCmd.AddCommand(scrapeStatusCmd)
	scrapeCmd.AddCommand(scrapeReviewCmd)
	scrapeCmd.AddCommand(scrapeAcceptCmd)

	scrapeCmd.Flags().StringVar(&scrapeLibraryPath, "library", "", "path to ComicDb.xml (required if not in config)")
	scrapeCmd.Flags().BoolVar(&scrapeAll, "all", false, "scrape all books without ComicVine tags (default behavior)")
	scrapeCmd.Flags().BoolVar(&scrapeRescrape, "rescrape", false, "re-scrape books that already have ComicVine tags, reusing the existing match")
	scrapeCmd.Flags().BoolVar(&scrapeDryRun, "dry-run", false, "show what would be scraped without making changes")
	scrapeCmd.Flags().BoolVar(&scrapeAutoOnly, "auto-only", false, "only scrape high-confidence matches; skip ambiguous ones instead of queuing for review")
	scrapeCmd.Flags().IntVar(&scrapeBatchSize, "batch-size", 0, "process at most N books then stop (0 = no limit)")
	scrapeCmd.Flags().StringVar(&scrapeSeries, "series", "", "only scrape books whose parsed series name contains this string")
}

func runScrape(cmd *cobra.Command, args []string) error {
	libPath, backend, cache, client, err := openScraperDeps(scrapeLibraryPath)
	if err != nil {
		return err
	}
	defer cache.Close()
	defer backend.Close()

	allBooks, err := backend.GetAllBooks()
	if err != nil {
		return fmt.Errorf("load books: %w", err)
	}

	books, err := selectBooksToScrape(allBooks, args)
	if err != nil {
		return err
	}
	if len(books) == 0 {
		fmt.Println("No books match the given selection.")
		return nil
	}

	scraper := comicvine.NewScraper(client, cache, backend, comicvine.DefaultScraperConfig())
	opts := comicvine.ScrapeOptions{
		FastRescrape: scrapeRescrape,
		AutoOnly:     scrapeAutoOnly,
		DryRun:       scrapeDryRun,
		BatchSize:    scrapeBatchSize,
	}

	fmt.Printf("Scraping %d book(s) from %s%s\n", len(books), libPath, dryRunSuffix())

	job, runErr := scraper.Scrape(context.Background(), defaultScrapeJobID, books, opts, func(job *comicvine.ScrapeJob, r *comicvine.BookScrapeResult) {
		fmt.Printf("  [%d/%d] %s: %s\n", job.Completed+job.Skipped+job.Failed+job.PendingReview, job.Total, r.Filename, scrapeResultLine(r))
	})
	if runErr != nil {
		fmt.Printf("\nScrape stopped early: %v\n", runErr)
	}

	fmt.Println()
	printJobSummary(job)
	return nil
}

func dryRunSuffix() string {
	if scrapeDryRun {
		return " (dry run)"
	}
	return ""
}

func scrapeResultLine(r *comicvine.BookScrapeResult) string {
	switch r.Status {
	case comicvine.BookStatusScraped:
		return fmt.Sprintf("scraped (volume=%d issue=%d)", r.VolumeID, r.IssueID)
	case comicvine.BookStatusPendingReview:
		return fmt.Sprintf("pending review (%d candidates)", len(r.Candidates))
	case comicvine.BookStatusSkipped:
		return "skipped: " + r.Error
	case comicvine.BookStatusFailed:
		return "failed: " + r.Error
	default:
		return r.Status
	}
}

func printJobSummary(job *comicvine.ScrapeJob) {
	fmt.Printf("Job %s: %s\n", job.ID, job.Status)
	fmt.Printf("  Total:          %d\n", job.Total)
	fmt.Printf("  Scraped:        %d\n", job.Completed)
	fmt.Printf("  Skipped:        %d\n", job.Skipped)
	fmt.Printf("  Failed:         %d\n", job.Failed)
	fmt.Printf("  Pending review: %d\n", job.PendingReview)
}

// selectBooksToScrape resolves the CLI args/flags into the concrete set of
// books to scrape: explicit book IDs, a --series filter, or (by default)
// books without existing ComicVine identity tags.
func selectBooksToScrape(allBooks []library.ComicBook, args []string) ([]*library.ComicBook, error) {
	if len(args) > 0 {
		wanted := make(map[string]bool, len(args))
		for _, id := range args {
			wanted[id] = true
		}
		var out []*library.ComicBook
		for i := range allBooks {
			if wanted[allBooks[i].ID] {
				out = append(out, &allBooks[i])
			}
		}
		return out, nil
	}

	var out []*library.ComicBook
	for i := range allBooks {
		book := &allBooks[i]

		if scrapeSeries != "" {
			parsed := comicvine.ParseFilename(book.FilePath)
			if !strings.Contains(strings.ToLower(parsed.Series), strings.ToLower(scrapeSeries)) {
				continue
			}
		}

		hasTag := hasComicVineTag(book.CustomValuesStore)
		if hasTag && !scrapeRescrape {
			continue
		}
		out = append(out, book)
	}
	return out, nil
}

func hasComicVineTag(store string) bool {
	for pair := range strings.SplitSeq(store, ",") {
		if strings.HasPrefix(strings.TrimSpace(pair), "comicvine_volume=") {
			return true
		}
	}
	return false
}

func runScrapeStatus(cmd *cobra.Command, args []string) error {
	_, _, cache, _, err := openScraperDeps(scrapeLibraryPath)
	if err != nil {
		return err
	}
	defer cache.Close()

	job, err := cache.GetScrapeJob(defaultScrapeJobID)
	if err != nil {
		return fmt.Errorf("load job status: %w", err)
	}
	if job == nil {
		fmt.Println("No scrape job has been run yet.")
		return nil
	}
	printJobSummary(job)
	fmt.Printf("  Started:        %s\n", job.StartedAt.Format(time.RFC3339))
	fmt.Printf("  Updated:        %s\n", job.UpdatedAt.Format(time.RFC3339))
	return nil
}

func runScrapeReview(cmd *cobra.Command, args []string) error {
	_, _, cache, _, err := openScraperDeps(scrapeLibraryPath)
	if err != nil {
		return err
	}
	defer cache.Close()

	pending, err := cache.GetPendingReviewBooks(defaultScrapeJobID)
	if err != nil {
		return fmt.Errorf("load pending review books: %w", err)
	}
	if len(pending) == 0 {
		fmt.Println("No books are pending review.")
		return nil
	}

	for _, r := range pending {
		fmt.Printf("%s (%s)\n", r.Filename, r.BookID)
		for _, c := range r.Candidates {
			fmt.Printf("    volume=%-8d %-40s (%s) score=%.1f confidence=%s\n", c.VolumeID, c.Name, c.StartYear, c.Score, c.Confidence)
		}
		fmt.Printf("  Accept with: comic-server scrape accept %s VOLUME_CV_ID\n\n", r.BookID)
	}
	return nil
}

func runScrapeAccept(cmd *cobra.Command, args []string) error {
	bookID := args[0]
	volumeCVID, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid volume ID %q: %w", args[1], err)
	}

	_, backend, cache, client, err := openScraperDeps(scrapeLibraryPath)
	if err != nil {
		return err
	}
	defer cache.Close()
	defer backend.Close()

	scraper := comicvine.NewScraper(client, cache, backend, comicvine.DefaultScraperConfig())
	result, err := scraper.AcceptReview(context.Background(), defaultScrapeJobID, bookID, volumeCVID)
	if err != nil {
		return fmt.Errorf("accept review: %w", err)
	}
	fmt.Printf("%s: %s\n", result.Filename, scrapeResultLine(result))
	return nil
}

// openScraperDeps loads config, the library backend, the ComicVine cache,
// and an API client shared by all scrape subcommands.
func openScraperDeps(libraryPathOverride string) (libPath string, backend library.Backend, cache *comicvine.Cache, client *comicvine.Client, err error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("get config path: %w", err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("load config: %w", err)
	}

	libPath = libraryPathOverride
	if libPath == "" {
		libPath = cfg.Server.LibraryPath
	}
	if libPath == "" {
		return "", nil, nil, nil, fmt.Errorf("no library path: pass --library or set server.library_path in config")
	}

	xmlBackend, err := library.NewXMLBackend(libPath, 0)
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("load library: %w", err)
	}

	if cfg.Server.ComicVineAPIKey == "" {
		xmlBackend.Close()
		return "", nil, nil, nil, fmt.Errorf("no ComicVine API key configured (server.comicvine_api_key)")
	}

	dataDir, err := config.GetDataDir()
	if err != nil {
		xmlBackend.Close()
		return "", nil, nil, nil, fmt.Errorf("get data directory: %w", err)
	}
	cachePath := filepath.Join(dataDir, "comicvine_cache.db")
	cvCache, err := comicvine.OpenCache(cachePath)
	if err != nil {
		xmlBackend.Close()
		return "", nil, nil, nil, fmt.Errorf("open ComicVine cache: %w", err)
	}

	cvClient := comicvine.NewClient(cfg.Server.ComicVineAPIKey)
	return libPath, xmlBackend, cvCache, cvClient, nil
}
