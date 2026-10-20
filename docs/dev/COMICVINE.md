# ComicVine

ComicVine enrichment (sync, cache, rate limits), scraping, the Not-in-ComicVine flow and Missing Issues.

Moved from `CLAUDE.md`. See the index there for the other developer reference files.

## ComicVine Enrichment

Background sync pipeline that fetches series/issue data from the ComicVine API to enable series completeness checking and missing issue detection.

### How It Works

1. **Seeding**: On server startup, extracts `comicvine_volume` IDs from library books' `CustomValuesStore` fields (~97.5% of books are tagged)
2. **Background sync**: Fetches volume metadata and issue lists from ComicVine API, prioritized by how many issues you own per volume
3. **Completeness computation**: Compares owned issues against ComicVine's issue list to determine completeness percentage and missing issues
4. **Matcher evaluation**: CV enrichment matchers use the pre-computed data during smart list evaluation

### Configuration

```yaml
server:
  comicvine_api_key: "your-api-key-here"
```

The API key can also be set via `COMIC_SERVER_COMICVINE_API_KEY` environment variable.

### Rate Limiting

ComicVine's API is burst-hostile — sustained requests at any rate will eventually trigger timeouts. The client uses:

- **Burst-and-pause pacing**: 20 requests, then 5-minute pause. After 200 requests, 30-minute cooldown.
- **Circuit breaker**: On ANY failure (timeout, 429, 5xx), immediately stops all requests. Minimum 1-hour backoff with exponential increase up to 24-hour cap.
- **Resumable sync**: Progress tracked in SQLite. Survives server restarts, backoff pauses, and interruptions.
- **State survives restarts** (tracker #26): the pacer's counters and last-hour request times, and the circuit's open state, are saved in `cv_sync_state` (`pacer_state`, `circuit_state`). The sync, the scraper and the CLI share one pacer and one circuit (`WithSharedPersistentLimits`). A pause counts from the last request, so time served before a restart counts. The sync start line and a 420/429 line log the requests of the last hour.
- **Headroom for people** (tracker #26 follow-up): the background sync (`Syncer.Run` marks its context with `WithBackground`) stops 5 requests short of the burst limit, 20 short of the window limit, and uses at most 120 requests an hour (`defaultBackgroundHourlyCap`; ComicVine allows about 200). A re-scrape or match started from the UI is interactive and may use the full limits, so it finds free slots instead of queueing behind the sync's burst pause. Before this the sync alone used about 200 requests an hour and a re-scrape waited up to 5 minutes. The cost is a slower initial sync (about 40% fewer requests per hour). Constants, not config; change them in `internal/comicvine/pacer.go`.
- **Web requests do not wait out long pauses**: `cv-match` (preview and apply) and Scrape Review's Accept use `comicvine.WithMaxWait` (`cvPaceWaitLimit`, 20 s). A pacing pause longer than that comes back at once as `ErrPaceWait` and the API answers **429** with `Retry-After` and a sentence naming the limit (burst, window or hourly) and the time. Before this the request hung until the reverse proxy cut it off with a bare 504 (prod 2026-10-19: log only `fetch issue N: context canceled`). Scrape jobs and the sync are not limited; they wait and say so in the job log. The window limit is the likely culprit after a restart: its count is carried over, so a restart close to 200 requests means up to 30 minutes from the last request.
- **Background users of the headroom** (tracker #47): the sync and the local-identity re-check (`Scraper.RecheckLocalIdentity`, off by default, see LIBRARY_TOOLS.md "Collected Editions") both use `WithBackground`. The re-check adds no new rate path: it goes through the same shared client, pacer and circuit breaker, makes at most 5 searches a day and stops at the first error.
- **Pacing status and timers**: `GET /api/comicvine/pacing` (`internal/api/cvpacing.go`, `Pacer.Status`, `comicvine.SharedPacingStatus`) reports requests in the last hour, burst and window use, seconds until an interactive request may go (`next_request_seconds`, the longer of the pacer wait and an open circuit) and the same for the background sync. `js/comicvineStatus.js` draws it as a pill in the bottom-left corner of every page (polled every 15 s, counted down locally; hidden without an API key; the tooltip has the numbers). The match box (`js/cvMatch.js`) turns a 429's `Retry-After` into a countdown and repeats the call by itself; nothing is queued on the server, so leaving the page just stops the timer. A real queue with a Pending tasks page is tracker #29 (not built). Smoke-tested in a headless browser with the two endpoints mocked; not against a live ComicVine key.

Initial sync of ~12K volumes takes approximately 2 weeks at conservative pacing.

### Cache Storage

Data cached in SQLite at `$XDG_DATA_HOME/comic-server/comicvine_cache.db` (default: `~/.local/share/comic-server/comicvine_cache.db`).

Tables:

- `cv_volumes` — Volume metadata (name, publisher, issue count, sync status)
- `cv_issues` — Issue list per volume (number, cover date, store date)
- `cv_sync_state` — Key-value store for sync state tracking

### CLI Commands

```bash
# Show sync status
comic-server comicvine status
```

### Architecture

```
internal/comicvine/
├── client.go        # API client with burst-and-pause rate limiting
├── circuit.go       # Circuit breaker (closed→open→half-open state machine)
├── pacer.go         # Burst/window rate limiting with context cancellation
├── cache.go         # SQLite cache (cv_volumes, cv_issues, cv_sync_state)
├── sync.go          # Background sync orchestrator (seed → fetch → cache)
├── completeness.go  # Builds book ID → CVCompleteness map from cache
└── types.go         # API response types and cached data types
```

## Scrape Hold-back

comic-server-fohc. A whole-library scrape (`POST /api/scrape` with no `book_ids`) leaves out books a retry would not help: tagged `CVDBSKIP` (always), and books whose latest outcome across all jobs (`Cache.LatestScrapeResults`) needs the user - `comicvine.HeldBackReason`: pending review, "no candidate volumes found", "no matching issue number found in selected volume". API errors/timeouts are retried. `options.retry_held_back` re-includes the outcome-based ones (never CVDBSKIP). Scraping by `book_ids` (book page Re-scrape, Duplicates) ignores all of this. The 202 response carries `total_books` and `held_back` (reason -> count); the workflow Scrape card shows them.

**Publisher filter** (comic-server-2w44, UI/DB in comic-server-pl94): case-insensitive substrings of the CV volume's publisher. `comicvine.PublisherFilter` drops candidates in `scrapeOneBook` before scoring (allow narrows, block carves out, empty publisher passes). Not applied to `AcceptReview` (manual pick) or `FastRescrape`. Every candidate excluded gives `errAllFiltered`, which `HeldBackReason` treats as held back. Source of truth: the config.db `scrape_publisher_filter` table (lazy, no schema bump) once saved from Settings > Scrape publisher filter (`GET|PUT /api/settings/scrape-publisher-filter`, response carries `source` = saved|config); until then `server.scrape_publisher_allow|block` / env `COMIC_SERVER_SCRAPE_PUBLISHER_ALLOW|BLOCK` apply (`Server.effectiveScrapePublisherFilter`). Saving two empty lists overrides config.yaml with no filter. The scraper reads the filter once at the start of each job (`SetPublisherFilterSource`), so a save needs no restart and a batch uses one consistent filter. Idea from ComicManager (ideas only). The older built-in -6 Panini/mirror-publisher penalty in `publisherScore` is unchanged. Saves are not audit-logged (the audit log is per book; trash settings are not either).

## Not in ComicVine

comic-server-5wer (phase 1). Collected volumes, web comics and other books ComicVine does not list get an explicit decision instead of sitting at the Scrape stage forever. `comic_server_cv_nomatch` = `<reason>|<YYYY-MM-DD>` (reasons `collected`, `not_listed`, `web_comic`, `other`; `workflow.SetNoCVMatch` / `ClearNoCVMatch` / `NoCVMatch`). `workflow.InferStage` no longer returns `StageScrape` for an untagged book that is `ExemptFromScrape` (the marker **or** the older `CVDBSKIP` tag; prod 2026-10-16: 82 of 83 CVDBSKIP books were stuck at Scrape). Setting the marker advances a stored Scrape stage; clearing it puts a book with no CV tag and no CVDBSKIP back at Scrape. Whole-library scrapes skip marked books (reason "marked not in ComicVine", `retry_held_back` does not re-include them); a scrape by `book_ids` still works, and any successful `WriteMetadata` removes the marker. `POST /api/library/books/:id/cv-nomatch {set, reason}` (audited as `cv_nomatch`; 409 for a fileless book). UI: "None of these - not in ComicVine" on each Scrape Review item (a marked book drops out of the review list), "Mark as not in ComicVine" / Undo and a badge on the book page, a count card on the workflow page (`cv_nomatch` in `GET /api/library/workflow`). **CVDBSKIP migration**: books that already had a *stored* Scrape stage stayed there (InferStage only covers unstaged books), so `Server.MigrateCVDBSkip` (`internal/api/cvnomatch_migrate.go`) copies the tag into the marker (reason `legacy_cvdbskip`, not offered in the UI) and advances the stage. The tag is left on the book (ComicRack still reads it). Only files without a CV tag or marker are touched; idempotent; runs in the background at every start and after every library import (ComicRack's XML still carries the tag). Audited as `cv_nomatch`. **Phase 2 - match by pasted link/ID**: `comicvine.ParseCVRef` reads an issue/volume URL (comicvine.gamespot.com / comicvine.com only), a typed id (`4000-N` issue, `4050-N` volume; other types such as characters are refused with a clear message) or a bare number (an issue id). `POST /api/library/books/:id/cv-match {ref, issue_id, apply}`: `apply` false previews and writes nothing (an issue: cover, number, date, volume, publisher, issue count next to the book's own pages/year; a volume: its issue list to pick from, then `issue_id`); `apply` true runs `Scraper.ApplyIssue` (same `WriteMetadata` as a scrape, no number or file name check since the user chose, audited as `cv_match`, marks any pending review of the book resolved via `Cache.ResolvePendingReview`, removes the not-in-ComicVine marker). Uses the paced, circuit-broken client; an open circuit is a 503 with the retry time, other ComicVine failures 502; a preview costs one issue call (cached forever) and no volume call for a synced volume. 409 for a fileless book on apply. UI: `js/cvMatch.js` (`CVMatchBox`) in a "Match by ComicVine link or ID" section on the book page and on each Scrape Review item. **Phase 3 - unnumbered books**: the issue number to look for is the file name's, else the library `Number` (`issueNumberFor`); a scrape never guesses #1. A book with neither, whose volume matched with high confidence, becomes `pending_review` (candidates = top 3 volumes) instead of the old dead-end "no matching issue number" failure; with `auto_only` it is skipped. So it is held back from whole-library scrapes and shows in Scrape Review, where `needs_issue` (computed at list time, `comicvine.NeedsIssueNumber`) swaps Accept for "Choose issue…", which opens the match box on that volume's issue list (Phase 2 preview, then confirm). `AcceptReview` also uses the library Number. Not built: auto-picking the only issue of a one-issue volume (the user still confirms). **Phase 4 - review page polish**: every candidate row has an "Issues…" button (next to Accept) that opens the match box on that volume's issue list (the Phase 2 volume preview: cached, one paced call when the volume is unsynced), and each item has a "Re-scrape" button (`POST /api/scrape {book_ids:[id]}`). `Cache.UpsertScrapeJobBook` deletes a book's pending_review rows from OTHER jobs when it stores a new pending_review row, so a re-scrape replaces stale candidates instead of listing the book twice; a scraped (matched) result removes them too, so a Re-scrape that now matches drops the book from review; other outcomes (skipped, failed) leave a pending review alone. `AcceptReview` records the outcome in the job that holds the book's pending row (`Cache.PendingReviewJobID`), not only the job passed in, since the web UI passes the latest job and any later scrape replaces it. Not browser-smoke-tested. **Live scrape log**: Re-scrape on the book page and on Scrape Review opens a log dialog (`js/scrapeLog.js`, `ScrapeLog.open(jobId)`). The scraper writes plain-sentence steps through a ctx-carried sink (`comicvine/joblog.go`, `jobLog(ctx, ...)`, so the client and pacer can report ComicVine calls, rate-limit waits and an open circuit without knowing about jobs); `Scraper.SetJobLog` gets them (also logged at debug with `job_id`). The API keeps the last 2000 lines of the last 10 jobs in memory (`internal/api/scrapelog.go`), pushes each as a `scrape_log` WebSocket event, and serves the backlog at `GET /api/scrape/log?job_id=` (`running` false once the job ended). `websocket.js` sets `window.wsClient`: pages that hooked `window.wsClient` (Scrape Review live refresh, Devices) silently never subscribed before. **Phase 5 - close-out**: `GET /api/library/workflow/cv-nomatch` pages the marked books (newest mark first, with reason and date; from the workflow cache's `cvNoMatch` list), shown from a View button on the workflow page's "Not in ComicVine" card with an Undo per row. `POST` on the same path `{book_ids, set, reason}` marks or clears many books in one save (audited per book as `cv_nomatch`; fileless or unknown ids come back in `skipped`; at most 5000 ids); Scrape Review has checkboxes, Select all and "Mark selected as not in ComicVine". For an unnumbered book, a candidate volume ComicVine lists with exactly one issue, whose issue is already cached, gets `only_issues[volume_id]` in the review list (cache only, no API call) and a "Use #N…" button that opens the match preview on that issue; the user still confirms. The Phase 5 UI was smoke-tested in a headless browser (review queue mocked).

## Missing Issues

comic-server-buhs. `/missing-issues` page: per ComicVine volume, which issues the library owns and which it does not. `GET /api/library/series-completeness` (ranked volume rollup, `q`, `only_missing`, paged; memoized 2 min on the Server) and `GET /api/library/series-completeness/:volumeId` (every issue, numerically ordered, with owned / wanted / missing state and the matching book id), both from `comicvine.BuildVolumeSummaries` / `BuildSeriesCompleteness` over the same cache `BuildCompletenessMap` uses. `POST .../:volumeId/wanted {issue_ids}` creates Wanted placeholders (no `FilePath`) carrying `comicvine_volume`/`comicvine_issue`, skipping owned or already-wanted issues; audited as `wanted_from_missing`.

**A Wanted placeholder (no `FilePath`, not a paper copy) is never "owned"** in completeness, including in `BuildCompletenessMap` and so the CV matchers; a paper copy is owned (see Paper Copies in [LIBRARY_TOOLS.md](LIBRARY_TOOLS.md#paper-copies)). Only volumes whose issue list is synced appear (unknown completeness is not zero). Owned books of a volume with no issue id are reported as `unmatched_owned`, not counted as owned. The book page shows a completeness bar for the book's own volume (comic-server-n2yb; lazy fetch of `GET /api/library/series-completeness/:volumeId`, silent when ComicVine is off or the volume is unsynced) and `/missing-issues?volume=<id>` opens that volume. Not done: variants/annuals edge cases from ComicManager's page, a completion bar on list pages.
