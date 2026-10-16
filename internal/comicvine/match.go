package comicvine

import (
	"context"
	"fmt"

	"github.com/duckpuppy/comic-server/internal/library"
)

// IssueMatchPreview is what the user checks before a pasted ComicVine issue
// is applied to a book (comic-server-5wer): the issue, its volume and cover.
type IssueMatchPreview struct {
	IssueID    int    `json:"issue_id"`
	Number     string `json:"number"`
	Name       string `json:"name"`
	CoverDate  string `json:"cover_date"`
	IssueURL   string `json:"issue_url"`
	CoverURL   string `json:"cover_url"`
	VolumeID   int    `json:"volume_id"`
	VolumeName string `json:"volume_name"`
	StartYear  string `json:"start_year"`
	Publisher  string `json:"publisher"`
	IssueCount int    `json:"issue_count"`
	VolumeURL  string `json:"volume_url"`
}

// VolumeMatchPreview lists a pasted volume's issues so the user picks one.
type VolumeMatchPreview struct {
	VolumeID   int          `json:"volume_id"`
	Name       string       `json:"name"`
	StartYear  string       `json:"start_year"`
	Publisher  string       `json:"publisher"`
	IssueCount int          `json:"issue_count"`
	VolumeURL  string       `json:"volume_url"`
	Issues     []MatchIssue `json:"issues"`
}

// MatchIssue is one row of a volume's issue list.
type MatchIssue struct {
	ID        int    `json:"id"`
	Number    string `json:"number"`
	Name      string `json:"name"`
	CoverDate string `json:"cover_date"`
}

// PreviewIssue fetches an issue and its volume for display. It writes nothing.
// The issue detail is cached; a volume the sync already knows costs no call.
func (s *Scraper) PreviewIssue(ctx context.Context, issueID int) (*IssueMatchPreview, error) {
	detail, err := s.issueDetail(ctx, issueID)
	if err != nil {
		return nil, fmt.Errorf("fetch issue %d: %w", issueID, err)
	}
	p := &IssueMatchPreview{
		IssueID: detail.ID, Number: detail.IssueNumber, Name: detail.Name,
		CoverDate: detail.CoverDate, IssueURL: detail.SiteDetailURL,
		CoverURL: firstNonEmpty(detail.Image.SmallURL, detail.Image.MediumURL, detail.Image.IconURL),
		VolumeID: detail.Volume.ID, VolumeName: detail.Volume.Name,
	}
	if detail.Volume.ID > 0 {
		if vol, err := s.cachedVolume(ctx, detail.Volume.ID); err == nil && vol != nil {
			p.VolumeName, p.StartYear, p.Publisher = vol.Name, vol.StartYear, vol.Publisher.Name
			p.IssueCount, p.VolumeURL = vol.CountOfIssues, vol.SiteDetailURL
		}
	}
	return p, nil
}

// PreviewVolume fetches a volume and its issue list for the user to pick from.
func (s *Scraper) PreviewVolume(ctx context.Context, volumeID int) (*VolumeMatchPreview, error) {
	vol, err := s.cachedVolume(ctx, volumeID)
	if err != nil {
		return nil, fmt.Errorf("fetch volume %d: %w", volumeID, err)
	}
	issues, err := s.volumeIssues(ctx, volumeID, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch issues for volume %d: %w", volumeID, err)
	}
	p := &VolumeMatchPreview{
		VolumeID: volumeID, Name: vol.Name, StartYear: vol.StartYear,
		Publisher: vol.Publisher.Name, IssueCount: vol.CountOfIssues, VolumeURL: vol.SiteDetailURL,
		Issues: make([]MatchIssue, 0, len(issues)),
	}
	for _, is := range issues {
		p.Issues = append(p.Issues, MatchIssue{ID: is.ID, Number: is.IssueNumber, Name: is.Name, CoverDate: is.CoverDate})
	}
	return p, nil
}

// ApplyIssue writes a chosen ComicVine issue onto a book through backend (the
// caller picks the audited wrapper), then resolves any pending review of the
// book. The user chose the issue, so no filename or number check is made.
func (s *Scraper) ApplyIssue(ctx context.Context, backend library.Backend, bookID string, issueID int) (*BookScrapeResult, error) {
	book, err := backend.GetBook(bookID)
	if err != nil {
		return nil, fmt.Errorf("get book: %w", err)
	}
	if book == nil {
		return nil, fmt.Errorf("book %s not found", bookID)
	}
	detail, err := s.issueDetail(ctx, issueID)
	if err != nil {
		return nil, fmt.Errorf("fetch issue %d: %w", issueID, err)
	}
	if detail.Volume.ID == 0 {
		return nil, fmt.Errorf("issue %d has no volume on ComicVine", issueID)
	}
	volume, err := s.cachedVolume(ctx, detail.Volume.ID)
	if err != nil {
		return nil, fmt.Errorf("fetch volume %d: %w", detail.Volume.ID, err)
	}
	if _, err := WriteMetadata(backend, book, *volume, detail, s.cfg); err != nil {
		return nil, err
	}
	result := &BookScrapeResult{BookID: bookID, Filename: book.FilePath, Series: volume.Name,
		Status: BookStatusScraped, VolumeID: volume.ID, IssueID: issueID}
	if err := s.cache.ResolvePendingReview(bookID, volume.ID, issueID); err != nil {
		return result, fmt.Errorf("clear pending review: %w", err)
	}
	return result, nil
}
