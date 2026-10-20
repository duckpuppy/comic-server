package comicvine

import (
	"strconv"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// BuildCompletenessMap builds a map of book ID → CVCompleteness by comparing
// the library's books against the ComicVine cache. Only books with a
// comicvine_volume custom value and a corresponding synced volume in the cache
// are included.
func BuildCompletenessMap(books []library.ComicBook, cache *Cache) (map[string]*library.CVCompleteness, error) {
	return BuildCompletenessMapOpts(books, cache, CompletenessOptions{})
}

// BuildCompletenessMapOpts is BuildCompletenessMap with options. With
// CountCoveredAsOwned, an issue a collected edition contains counts as owned
// for the volume's books (tracker #46); without it collections are ignored,
// which is the meaning the CV smart-list matchers always had.
func BuildCompletenessMapOpts(books []library.ComicBook, cache *Cache, opts CompletenessOptions) (map[string]*library.CVCompleteness, error) {
	var runs map[int]*volumeHolding
	if opts.CountCoveredAsOwned {
		runs = map[int]*volumeHolding{}
		for i := range books {
			addCollectionRuns(runs, &books[i])
		}
	}
	// Group books by CV volume ID
	type volumeBooks struct {
		cvVolumeID    int
		bookIDs       []string
		ownedIssueIDs map[int]bool // CV issue IDs owned
	}
	byVolume := make(map[int]*volumeBooks)

	for i := range books {
		book := &books[i]
		cvVolID := extractCVVolumeID(book.CustomValuesStore)
		if cvVolID == 0 {
			continue
		}
		cvIssID := extractCVIssueID(book.CustomValuesStore)

		vb, ok := byVolume[cvVolID]
		if !ok {
			vb = &volumeBooks{cvVolumeID: cvVolID, ownedIssueIDs: make(map[int]bool)}
			byVolume[cvVolID] = vb
		}
		vb.bookIDs = append(vb.bookIDs, book.ID)
		// A placeholder (no file yet, e.g. a Wanted book created from the
		// missing-issues view) is not an owned issue even though it carries
		// the issue's id. A paper copy is owned (comic-server-vffq).
		if cvIssID > 0 && !library.IsWantedPlaceholder(book) {
			vb.ownedIssueIDs[cvIssID] = true
		}
	}

	result := make(map[string]*library.CVCompleteness)

	for _, vb := range byVolume {
		vol, err := cache.GetVolume(vb.cvVolumeID)
		if err != nil || vol == nil || vol.SyncStatus != SyncStatusSynced || !vol.IssuesSynced {
			continue
		}

		issues, err := cache.GetIssuesForVolume(vb.cvVolumeID)
		if err != nil {
			continue
		}

		totalIssues := len(issues)
		var covered map[int]string
		if h := runs[vb.cvVolumeID]; h != nil {
			covered = h.coveredIssues(issues)
		}
		ownedIssues := 0
		for _, iss := range issues {
			if _, ok := covered[iss.CVID]; vb.ownedIssueIDs[iss.CVID] || ok {
				ownedIssues++
			}
		}

		missingCount := totalIssues - ownedIssues
		percentOwned := 0
		if totalIssues > 0 {
			percentOwned = ownedIssues * 100 / totalIssues
		}

		isComplete := "No"
		if missingCount == 0 && totalIssues > 0 {
			isComplete = "Yes"
		}

		cv := &library.CVCompleteness{
			TotalIssues:  totalIssues,
			OwnedIssues:  ownedIssues,
			MissingCount: missingCount,
			PercentOwned: percentOwned,
			IsComplete:   isComplete,
		}

		for _, bookID := range vb.bookIDs {
			result[bookID] = cv
		}
	}

	return result, nil
}

func extractCVIssueID(store string) int {
	for _, pair := range strings.Split(store, ",") {
		pair = strings.TrimSpace(pair)
		if !strings.HasPrefix(pair, "comicvine_issue=") {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		id, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			continue
		}
		return id
	}
	return 0
}
