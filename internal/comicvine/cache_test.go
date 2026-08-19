package comicvine

import (
	"path/filepath"
	"testing"
	"time"
)

func testCache(t *testing.T) *Cache {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cv_cache.db")
	c, err := OpenCache(path)
	if err != nil {
		t.Fatalf("open cache: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestCache_UpsertAndGetVolume(t *testing.T) {
	c := testCache(t)

	v := &CachedVolume{
		CVID:       20784,
		Name:       "Batman",
		Publisher:  "DC Comics",
		StartYear:  "2011",
		IssueCount: 52,
		SiteURL:    "https://comicvine.gamespot.com/batman/4050-20784/",
		SyncStatus: SyncStatusSynced,
		SyncedAt:   time.Now().Truncate(time.Second),
	}
	if err := c.UpsertVolume(v); err != nil {
		t.Fatal(err)
	}

	got, err := c.GetVolume(20784)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected volume, got nil")
	}
	if got.Name != "Batman" || got.IssueCount != 52 || got.Publisher != "DC Comics" {
		t.Errorf("got %+v", got)
	}
}

func TestCache_GetVolume_NotFound(t *testing.T) {
	c := testCache(t)

	got, err := c.GetVolume(99999)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestCache_UpsertVolume_Update(t *testing.T) {
	c := testCache(t)

	v := &CachedVolume{CVID: 100, Name: "Old", SyncStatus: SyncStatusPending}
	c.UpsertVolume(v)

	v.Name = "New"
	v.SyncStatus = SyncStatusSynced
	c.UpsertVolume(v)

	got, _ := c.GetVolume(100)
	if got.Name != "New" || got.SyncStatus != SyncStatusSynced {
		t.Errorf("update failed: got %+v", got)
	}
}

func TestCache_UpsertAndGetIssues(t *testing.T) {
	c := testCache(t)

	c.UpsertVolume(&CachedVolume{CVID: 100, Name: "Test", SyncStatus: SyncStatusSynced})

	issues := []CachedIssue{
		{CVID: 1, VolumeCVID: 100, Number: "1", Name: "Pilot", CoverDate: "2020-01-01"},
		{CVID: 2, VolumeCVID: 100, Number: "2", Name: "Part Two", CoverDate: "2020-02-01"},
		{CVID: 3, VolumeCVID: 100, Number: "3", CoverDate: "2020-03-01"},
	}
	if err := c.UpsertIssues(issues); err != nil {
		t.Fatal(err)
	}

	got, err := c.GetIssuesForVolume(100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 issues, got %d", len(got))
	}
	if got[0].Number != "1" || got[1].Number != "2" || got[2].Number != "3" {
		t.Errorf("unexpected issues: %+v", got)
	}
}

func TestCache_EnsureVolumesExist(t *testing.T) {
	c := testCache(t)

	// Pre-insert one volume as synced
	c.UpsertVolume(&CachedVolume{CVID: 100, Name: "Existing", SyncStatus: SyncStatusSynced, IssuesSynced: true})

	// Ensure both existing and new IDs
	if err := c.EnsureVolumesExist([]int{100, 200, 300}); err != nil {
		t.Fatal(err)
	}

	// Existing volume should not be overwritten
	v, _ := c.GetVolume(100)
	if v.Name != "Existing" || v.SyncStatus != SyncStatusSynced {
		t.Errorf("existing volume was overwritten: %+v", v)
	}

	// New volumes should be pending
	v, _ = c.GetVolume(200)
	if v == nil || v.SyncStatus != SyncStatusPending {
		t.Errorf("new volume 200: %+v", v)
	}
}

func TestCache_PendingVolumeIDs_Priority(t *testing.T) {
	c := testCache(t)

	c.EnsureVolumesExist([]int{100, 200, 300})

	owned := map[int]int{100: 5, 200: 50, 300: 10}
	ids, err := c.PendingVolumeIDs(owned)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 {
		t.Fatalf("want 3 pending, got %d", len(ids))
	}
	// Should be sorted by owned count descending: 200(50), 300(10), 100(5)
	if ids[0] != 200 || ids[1] != 300 || ids[2] != 100 {
		t.Errorf("priority order wrong: %v", ids)
	}
}

func TestCache_SyncStats(t *testing.T) {
	c := testCache(t)

	c.UpsertVolume(&CachedVolume{CVID: 1, Name: "A", SyncStatus: SyncStatusSynced, IssuesSynced: true})
	c.UpsertVolume(&CachedVolume{CVID: 2, Name: "B", SyncStatus: SyncStatusPending})
	c.UpsertVolume(&CachedVolume{CVID: 3, Name: "C", SyncStatus: SyncStatusFailed})

	synced, pending, failed, err := c.SyncStats()
	if err != nil {
		t.Fatal(err)
	}
	if synced != 1 || pending != 1 || failed != 1 {
		t.Errorf("stats: synced=%d pending=%d failed=%d", synced, pending, failed)
	}
}

func TestCache_SyncState(t *testing.T) {
	c := testCache(t)

	c.SetSyncState("backoff_until", "2025-01-01T00:00:00Z")
	got, err := c.GetSyncState("backoff_until")
	if err != nil {
		t.Fatal(err)
	}
	if got != "2025-01-01T00:00:00Z" {
		t.Errorf("got %q", got)
	}

	// Missing key returns empty
	got, err = c.GetSyncState("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("expected empty for missing key, got %q", got)
	}
}

func TestCache_UpsertAndGetIssueDetail(t *testing.T) {
	c := testCache(t)

	detail := &IssueDetail{
		ID:          12345,
		IssueNumber: "1",
		Name:        "Origins",
		CoverDate:   "1940-01-01",
		Description: "The first appearance.",
		Image:       ImageURLs{SuperURL: "https://example.com/super.jpg"},
		PersonCredits: []PersonCredit{
			{ID: 1, Name: "Bill Finger", Role: "writer"},
		},
		CharacterCredits: []NamedCredit{{ID: 100, Name: "Batman"}},
	}
	detail.Volume.ID = 999
	detail.Volume.Name = "Batman"

	if err := c.UpsertIssueDetail(detail); err != nil {
		t.Fatal(err)
	}

	got, err := c.GetIssueDetail(12345)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected issue detail, got nil")
	}
	if got.Name != "Origins" || got.Volume.Name != "Batman" {
		t.Errorf("got %+v", got)
	}
	if len(got.PersonCredits) != 1 || got.PersonCredits[0].Role != "writer" {
		t.Errorf("PersonCredits = %+v", got.PersonCredits)
	}

	// Upsert again with updated data
	detail.Name = "Origins (Updated)"
	if err := c.UpsertIssueDetail(detail); err != nil {
		t.Fatal(err)
	}
	got, err = c.GetIssueDetail(12345)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Origins (Updated)" {
		t.Errorf("got Name = %q, want updated", got.Name)
	}
}

func TestCache_GetIssueDetail_NotFound(t *testing.T) {
	c := testCache(t)

	got, err := c.GetIssueDetail(99999)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestCache_ScrapeJobMeta(t *testing.T) {
	c := testCache(t)

	job := &ScrapeJob{ID: "job-1", Status: JobStatusRunning, Total: 10, Completed: 3, StartedAt: time.Now().Truncate(time.Second), UpdatedAt: time.Now().Truncate(time.Second)}
	if err := c.SaveScrapeJobMeta(job); err != nil {
		t.Fatal(err)
	}

	got, err := c.GetScrapeJob("job-1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Status != JobStatusRunning || got.Total != 10 || got.Completed != 3 {
		t.Fatalf("got %+v", got)
	}

	job.Status = JobStatusComplete
	job.Completed = 10
	if err := c.SaveScrapeJobMeta(job); err != nil {
		t.Fatal(err)
	}
	got, err = c.GetScrapeJob("job-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != JobStatusComplete || got.Completed != 10 {
		t.Errorf("got %+v", got)
	}
}

func TestCache_GetScrapeJob_NotFound(t *testing.T) {
	c := testCache(t)
	got, err := c.GetScrapeJob("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestCache_ScrapeJobBooks(t *testing.T) {
	c := testCache(t)

	r1 := &BookScrapeResult{BookID: "book-1", Filename: "a.cbz", Series: "A", Status: BookStatusScraped, VolumeID: 100, IssueID: 1000}
	r2 := &BookScrapeResult{
		BookID: "book-2", Filename: "b.cbz", Series: "B", Status: BookStatusPendingReview,
		Candidates: []ReviewCandidate{{VolumeID: 1, Name: "B", Score: 10, Confidence: ConfidenceLow}},
	}
	if err := c.UpsertScrapeJobBook("job-1", r1); err != nil {
		t.Fatal(err)
	}
	if err := c.UpsertScrapeJobBook("job-1", r2); err != nil {
		t.Fatal(err)
	}

	all, err := c.GetScrapeJobBooks("job-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d books, want 2", len(all))
	}
	if all["book-1"].Status != BookStatusScraped || all["book-1"].VolumeID != 100 {
		t.Errorf("book-1 = %+v", all["book-1"])
	}
	if len(all["book-2"].Candidates) != 1 || all["book-2"].Candidates[0].Name != "B" {
		t.Errorf("book-2 candidates = %+v", all["book-2"].Candidates)
	}

	// Update book-1 to failed; verify overwrite.
	if err := c.UpsertScrapeJobBook("job-1", &BookScrapeResult{BookID: "book-1", Status: BookStatusFailed, Error: "boom"}); err != nil {
		t.Fatal(err)
	}
	all, err = c.GetScrapeJobBooks("job-1")
	if err != nil {
		t.Fatal(err)
	}
	if all["book-1"].Status != BookStatusFailed || all["book-1"].Error != "boom" {
		t.Errorf("book-1 after update = %+v", all["book-1"])
	}

	pending, err := c.GetPendingReviewBooks("job-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].BookID != "book-2" {
		t.Errorf("pending = %+v", pending)
	}

	pendingAllJobs, err := c.GetPendingReviewBooks("")
	if err != nil {
		t.Fatal(err)
	}
	if len(pendingAllJobs) != 1 {
		t.Errorf("pendingAllJobs = %+v", pendingAllJobs)
	}
}
