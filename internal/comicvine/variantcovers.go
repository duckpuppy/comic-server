package comicvine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Variant cover support (comic-server-0u48).
//
// ComicVine's issue endpoint exposes an undocumented `associated_images`
// list next to `image` (the main cover). Each entry carries only
// original_url, id, caption (usually null) and image_tags (observed:
// "All Images" - ComicVine's default tag, NOT a variant marker). Nothing in
// the data says an entry is a variant cover, so callers must show the
// candidates to a human before adding any of them to an archive.

// AssociatedImage is one entry of an issue's associated_images.
type AssociatedImage struct {
	ID          int     `json:"id"`
	OriginalURL string  `json:"original_url"`
	Caption     *string `json:"caption"`
	ImageTags   string  `json:"image_tags"`
}

// IssueImages is the image-only view of an issue.
type IssueImages struct {
	ID               int               `json:"id"`
	Name             string            `json:"name"`
	Image            ImageURLs         `json:"image"`
	AssociatedImages []AssociatedImage `json:"associated_images"`
}

// FetchIssueImages fetches an issue's main cover and associated images.
// It goes through the same paced, circuit-broken GET as every other API
// call.
func (c *Client) FetchIssueImages(ctx context.Context, cvIssueID int) (*IssueImages, error) {
	params := url.Values{"field_list": {"id,name,image,associated_images"}}
	var resp apiResponse[IssueImages]
	if err := c.get(ctx, fmt.Sprintf("/issue/4000-%d", cvIssueID), params, &resp); err != nil {
		return nil, err
	}
	if resp.StatusCode != 1 {
		return nil, fmt.Errorf("comicvine API error %d: %s", resp.StatusCode, resp.Error)
	}
	return &resp.Results, nil
}

// ExtractCVIssueID returns the ComicVine issue id stored in a book's custom
// values, or 0 when it has none.
func ExtractCVIssueID(customValuesStore string) int { return extractCVIssueID(customValuesStore) }

// maxCoverImageBytes caps one downloaded image. ComicVine originals are a
// few MB; anything past this is not a cover.
const maxCoverImageBytes = 25 << 20

// AllowedImageHost reports whether rawURL points at ComicVine's image CDN.
// Downloads only ever follow URLs ComicVine itself returned, but this keeps
// a malformed or hostile API response from turning the server into a fetcher
// for arbitrary hosts.
func AllowedImageHost(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "gamespot.com" || strings.HasSuffix(h, ".gamespot.com")
}

// DownloadImage fetches one image from ComicVine's CDN (not the rate-limited
// API, so it bypasses the pacer) and returns its bytes and file extension
// (".jpg", ".png", ...). allowAnyHost exists for tests against a local
// server; production callers pass false.
func (c *Client) DownloadImage(ctx context.Context, rawURL string, allowAnyHost bool) ([]byte, string, error) {
	if rawURL == "" {
		return nil, "", fmt.Errorf("empty image url")
	}
	if !allowAnyHost && !AllowedImageHost(rawURL) {
		return nil, "", fmt.Errorf("refusing to download from %q", rawURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download image: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCoverImageBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read image: %w", err)
	}
	if len(data) > maxCoverImageBytes {
		return nil, "", fmt.Errorf("image larger than %d bytes", maxCoverImageBytes)
	}

	// Trust the bytes, not the URL: only real raster images are accepted.
	switch ct := http.DetectContentType(data); ct {
	case "image/jpeg":
		return data, ".jpg", nil
	case "image/png":
		return data, ".png", nil
	case "image/gif":
		return data, ".gif", nil
	case "image/webp":
		return data, ".webp", nil
	default:
		return nil, "", fmt.Errorf("not an image (detected %s)", ct)
	}
}

// ImageIDString renders an associated image id for custom-value storage.
func ImageIDString(id int) string { return strconv.Itoa(id) }
