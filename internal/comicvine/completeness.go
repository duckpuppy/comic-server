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
	// Group books by CV volume ID
	type volumeBooks struct {
		cvVolumeID int
		bookIDs    []string
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
		if cvIssID > 0 {
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
		ownedIssues := 0
		for _, iss := range issues {
			if vb.ownedIssueIDs[iss.CVID] {
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
