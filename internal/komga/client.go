// Package komga implements a client for the Komga REST API, used to push
// comic-server smart list results into Komga collections and read lists
// since Komga has no native smart-list concept of its own.
package komga

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	defaultTimeout = 30 * time.Second
	userAgent      = "comic-server/1.0"

	// pageSize is used when paginating series/books listings. Komga's
	// default page size is much smaller; a larger page reduces the number
	// of round trips for libraries in the tens of thousands of books.
	pageSize = 500
)

// Client is a Komga REST API client, authenticated via API key.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithHTTPClient overrides the underlying *http.Client (used in tests).
func WithHTTPClient(c *http.Client) ClientOption {
	return func(cl *Client) { cl.http = c }
}

// NewClient creates a Komga API client. baseURL is the Komga instance's
// root URL (e.g. "https://comics.example.com"), without a trailing slash.
func NewClient(baseURL, apiKey string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: defaultTimeout},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Series is the subset of Komga's SeriesDto fields comic-server needs.
type Series struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// URL is the absolute file path as Komga sees it, e.g.
	// "/data/Publisher/Series Name". Used for path-mapping identity
	// matching against comic-server's own library paths.
	URL string `json:"url"`
}

// Book is the subset of Komga's BookDto fields comic-server needs.
type Book struct {
	ID       string `json:"id"`
	SeriesID string `json:"seriesId"`
	Name     string `json:"name"`
	// URL is the absolute file path as Komga sees it, e.g.
	// "/data/Publisher/Series Name/Series Name #01.cbz".
	URL string `json:"url"`
}

type pageResponse[T any] struct {
	Content       []T  `json:"content"`
	TotalElements int  `json:"totalElements"`
	Number        int  `json:"number"` // current page, 0-indexed
	Size          int  `json:"size"`
	Last          bool `json:"last"`
}

// ListAllSeries returns every series in the Komga instance, paginating as
// needed.
func (c *Client) ListAllSeries(ctx context.Context) ([]Series, error) {
	var all []Series
	page := 0
	for {
		var resp pageResponse[Series]
		params := url.Values{
			"page": {strconv.Itoa(page)},
			"size": {strconv.Itoa(pageSize)},
		}
		if err := c.get(ctx, "/api/v1/series", params, &resp); err != nil {
			return nil, fmt.Errorf("list series (page %d): %w", page, err)
		}
		all = append(all, resp.Content...)
		if resp.Last {
			break
		}
		page++
	}
	return all, nil
}

// ListAllBooks returns every book in the Komga instance, paginating as
// needed.
func (c *Client) ListAllBooks(ctx context.Context) ([]Book, error) {
	var all []Book
	page := 0
	for {
		var resp pageResponse[Book]
		params := url.Values{
			"page": {strconv.Itoa(page)},
			"size": {strconv.Itoa(pageSize)},
		}
		if err := c.get(ctx, "/api/v1/books", params, &resp); err != nil {
			return nil, fmt.Errorf("list books (page %d): %w", page, err)
		}
		all = append(all, resp.Content...)
		if resp.Last {
			break
		}
		page++
	}
	return all, nil
}

type collectionDto struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Ordered   bool     `json:"ordered"`
	SeriesIDs []string `json:"seriesIds"`
}

type readListDto struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Ordered bool     `json:"ordered"`
	Summary string   `json:"summary"`
	BookIDs []string `json:"bookIds"`
}

// UpsertCollection creates a Komga collection with the given name and
// series IDs if none exists, or updates the existing one (matched by exact
// name) to contain exactly those series IDs otherwise. Komga's collection
// search is a substring match, so results are filtered client-side for an
// exact name match before deciding create vs. update.
func (c *Client) UpsertCollection(ctx context.Context, name string, seriesIDs []string) error {
	existing, err := c.findCollectionByExactName(ctx, name)
	if err != nil {
		return err
	}

	if existing == nil {
		body := map[string]any{
			"name":      name,
			"ordered":   false,
			"seriesIds": seriesIDs,
		}
		return c.post(ctx, "/api/v1/collections", body, &collectionDto{})
	}

	body := map[string]any{
		"seriesIds": seriesIDs,
	}
	return c.patch(ctx, "/api/v1/collections/"+existing.ID, body)
}

// UpsertReadList creates a Komga read list with the given name and book IDs
// if none exists, or updates the existing one (matched by exact name) to
// contain exactly those book IDs otherwise.
func (c *Client) UpsertReadList(ctx context.Context, name string, bookIDs []string) error {
	existing, err := c.findReadListByExactName(ctx, name)
	if err != nil {
		return err
	}

	if existing == nil {
		body := map[string]any{
			"name":    name,
			"ordered": false,
			"summary": "",
			"bookIds": bookIDs,
		}
		return c.post(ctx, "/api/v1/readlists", body, &readListDto{})
	}

	body := map[string]any{
		"bookIds": bookIDs,
	}
	return c.patch(ctx, "/api/v1/readlists/"+existing.ID, body)
}

// findCollectionByExactName lists every collection and matches the name
// client-side, rather than using Komga's "search" query param. Komga's
// collection search has been observed to silently return zero results for
// collections it only just created via this same API (even for a plain
// single-word substring of the name), well after the request that created
// them returns — a stale-search-index quirk on Komga's side, not something
// comic-server can fix. Relying on "search" for the exact-match lookup that
// decides create-vs-update means a freshly-created collection looks
// "not found" on the next sync, so the code re-POSTs a create and Komga
// (correctly) rejects it as a duplicate name. Listing unfiltered sidesteps
// that entirely.
// SetBookReadProgress marks a Komga book as read (completed) or unread to
// match comic-server's own known read state - one-way, comic-server is
// authoritative (see comic-server-bkh). Komga has no partial/page-level
// concept needed here: PATCH .../read-progress with completed:true marks
// read, DELETE .../read-progress marks unread.
func (c *Client) SetBookReadProgress(ctx context.Context, komgaBookID string, read bool) error {
	if read {
		return c.patch(ctx, "/api/v1/books/"+komgaBookID+"/read-progress", map[string]any{"completed": true})
	}
	return c.delete(ctx, "/api/v1/books/"+komgaBookID+"/read-progress")
}

func (c *Client) findCollectionByExactName(ctx context.Context, name string) (*collectionDto, error) {
	page := 0
	for {
		var resp pageResponse[collectionDto]
		params := url.Values{"page": {strconv.Itoa(page)}, "size": {strconv.Itoa(pageSize)}}
		if err := c.get(ctx, "/api/v1/collections", params, &resp); err != nil {
			return nil, fmt.Errorf("list collections (page %d): %w", page, err)
		}
		for i := range resp.Content {
			if resp.Content[i].Name == name {
				return &resp.Content[i], nil
			}
		}
		if resp.Last {
			return nil, nil
		}
		page++
	}
}

// findReadListByExactName lists every read list and matches the name
// client-side. See findCollectionByExactName for why: Komga's "search"
// query param cannot be trusted to find recently-created entries.
func (c *Client) findReadListByExactName(ctx context.Context, name string) (*readListDto, error) {
	page := 0
	for {
		var resp pageResponse[readListDto]
		params := url.Values{"page": {strconv.Itoa(page)}, "size": {strconv.Itoa(pageSize)}}
		if err := c.get(ctx, "/api/v1/readlists", params, &resp); err != nil {
			return nil, fmt.Errorf("list readlists (page %d): %w", page, err)
		}
		for i := range resp.Content {
			if resp.Content[i].Name == name {
				return &resp.Content[i], nil
			}
		}
		if resp.Last {
			return nil, nil
		}
		page++
	}
}

func (c *Client) get(ctx context.Context, path string, params url.Values, dest any) error {
	reqURL := fmt.Sprintf("%s%s?%s", c.baseURL, path, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	return c.do(req, dest)
}

func (c *Client) post(ctx context.Context, path string, body any, dest any) error {
	return c.send(ctx, http.MethodPost, path, body, dest)
}

func (c *Client) patch(ctx context.Context, path string, body any) error {
	return c.send(ctx, http.MethodPatch, path, body, nil)
}

func (c *Client) delete(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	return c.do(req, nil)
}

func (c *Client) send(ctx context.Context, method, path string, body any, dest any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, dest)
}

func (c *Client) do(req *http.Request, dest any) error {
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("komga returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	if dest == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
