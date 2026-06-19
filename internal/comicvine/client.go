package comicvine

import (
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
	baseURL           = "https://comicvine.gamespot.com/api"
	defaultTimeout    = 30 * time.Second
	userAgent         = "comic-server/1.0"
)

// Client is a ComicVine API client with built-in rate limiting and circuit breaking.
type Client struct {
	apiKey  string
	http    *http.Client
	circuit *CircuitBreaker
	pacer   *Pacer
}

// ClientOption configures a Client.
type ClientOption func(*Client)

func WithHTTPClient(c *http.Client) ClientOption {
	return func(cl *Client) { cl.http = c }
}

func WithCircuitBreaker(cb *CircuitBreaker) ClientOption {
	return func(cl *Client) { cl.circuit = cb }
}

func WithPacer(p *Pacer) ClientOption {
	return func(cl *Client) { cl.pacer = p }
}

// NewClient creates a ComicVine API client.
func NewClient(apiKey string, opts ...ClientOption) *Client {
	c := &Client{
		apiKey:  apiKey,
		http:    &http.Client{Timeout: defaultTimeout},
		circuit: NewCircuitBreaker(),
		pacer:   NewPacer(),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// ErrCircuitOpen is returned when the circuit breaker is open.
type ErrCircuitOpen struct {
	Until time.Time
}

func (e ErrCircuitOpen) Error() string {
	return fmt.Sprintf("circuit breaker open until %s", e.Until.Format(time.RFC3339))
}

// FetchVolume fetches a single volume by ComicVine ID.
func (c *Client) FetchVolume(ctx context.Context, cvID int) (*Volume, error) {
	params := url.Values{
		"field_list": {"id,name,start_year,publisher,count_of_issues,description,deck,site_detail_url,date_added,date_last_updated"},
	}
	var resp apiResponse[Volume]
	if err := c.get(ctx, fmt.Sprintf("/volume/4050-%d", cvID), params, &resp); err != nil {
		return nil, err
	}
	if resp.StatusCode != 1 {
		return nil, fmt.Errorf("comicvine API error %d: %s", resp.StatusCode, resp.Error)
	}
	return &resp.Results, nil
}

// FetchVolumeIssues fetches all issues for a volume, handling pagination.
func (c *Client) FetchVolumeIssues(ctx context.Context, volumeCVID int) ([]Issue, error) {
	var all []Issue
	offset := 0
	limit := 100

	for {
		params := url.Values{
			"field_list": {"id,issue_number,name,volume,cover_date,store_date,site_detail_url,date_added"},
			"filter":     {fmt.Sprintf("volume:%d", volumeCVID)},
			"limit":      {strconv.Itoa(limit)},
			"offset":     {strconv.Itoa(offset)},
			"sort":       {"cover_date:asc"},
		}
		var resp apiResponse[[]Issue]
		if err := c.get(ctx, "/issues", params, &resp); err != nil {
			return all, err
		}
		if resp.StatusCode != 1 {
			return all, fmt.Errorf("comicvine API error %d: %s", resp.StatusCode, resp.Error)
		}

		all = append(all, resp.Results...)

		if len(all) >= resp.NumberOfTotalResults || resp.NumberOfPageResults < limit {
			break
		}
		offset += limit
	}
	return all, nil
}

// get performs a rate-limited, circuit-broken GET request.
func (c *Client) get(ctx context.Context, path string, params url.Values, dest any) error {
	if !c.circuit.Allow() {
		return ErrCircuitOpen{Until: c.circuit.BackoffUntil()}
	}

	if err := c.pacer.Wait(ctx); err != nil {
		return err
	}

	params.Set("api_key", c.apiKey)
	params.Set("format", "json")

	reqURL := fmt.Sprintf("%s%s/?%s", baseURL, path, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		c.circuit.RecordFailure()
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests ||
		resp.StatusCode == http.StatusForbidden ||
		resp.StatusCode >= 500 {
		c.circuit.RecordFailure()
		return fmt.Errorf("comicvine returned HTTP %d", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected HTTP %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	c.circuit.RecordSuccess()
	c.pacer.RecordRequest()
	return nil
}
