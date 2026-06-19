package comicvine

import "time"

// Volume represents a ComicVine volume (series).
type Volume struct {
	ID            int       `json:"id"`
	Name          string    `json:"name"`
	StartYear     string    `json:"start_year"`
	Publisher     Publisher `json:"publisher"`
	CountOfIssues int       `json:"count_of_issues"`
	Description   string    `json:"description"`
	Deck          string    `json:"deck"`
	SiteDetailURL string    `json:"site_detail_url"`
	DateAdded     string    `json:"date_added"`
	DateUpdated   string    `json:"date_last_updated"`
}

// Publisher represents a ComicVine publisher.
type Publisher struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Issue represents a ComicVine issue.
type Issue struct {
	ID          int    `json:"id"`
	IssueNumber string `json:"issue_number"`
	Name        string `json:"name"`
	Volume      struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"volume"`
	CoverDate     string `json:"cover_date"`
	StoreDate     string `json:"store_date"`
	SiteDetailURL string `json:"site_detail_url"`
	DateAdded     string `json:"date_added"`
}

// apiResponse is the envelope for all ComicVine API responses.
type apiResponse[T any] struct {
	Error                string `json:"error"`
	Limit                int    `json:"limit"`
	Offset               int    `json:"offset"`
	NumberOfPageResults   int    `json:"number_of_page_results"`
	NumberOfTotalResults  int    `json:"number_of_total_results"`
	StatusCode           int    `json:"status_code"`
	Results              T      `json:"results"`
}

// SyncStatus represents the sync state of a volume in the local cache.
type SyncStatus string

const (
	SyncStatusPending SyncStatus = "pending"
	SyncStatusSynced  SyncStatus = "synced"
	SyncStatusFailed  SyncStatus = "failed"
)

// CachedVolume is a locally cached ComicVine volume with sync metadata.
type CachedVolume struct {
	CVID          int
	Name          string
	Publisher     string
	StartYear     string
	IssueCount    int
	SiteURL       string
	SyncStatus    SyncStatus
	SyncedAt      time.Time
	IssuesSynced  bool
}

// CachedIssue is a locally cached ComicVine issue.
type CachedIssue struct {
	CVID       int
	VolumeCVID int
	Number     string
	Name       string
	CoverDate  string
	StoreDate  string
}
