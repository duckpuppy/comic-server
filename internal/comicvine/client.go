package comicvine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const (
	baseURL        = "https://comicvine.gamespot.com/api"
	defaultTimeout = 30 * time.Second
	userAgent      = "comic-server/1.0"
)

// Client is a ComicVine API client with built-in rate limiting and circuit breaking.
type Client struct {
	apiKey  string
	baseURL string
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

var (
	sharedMu      sync.Mutex
	sharedPacer   *Pacer
	sharedCircuit *CircuitBreaker
)

// WithSharedPersistentLimits gives the client the process-wide pacer and
// circuit breaker, which keep their state in store and so survive a restart.
// The sync, the scraper and the match endpoint all talk to ComicVine with one
// key, so they must count against one pacer, not one each. The first call
// creates them; later calls reuse them whatever store they pass.
func WithSharedPersistentLimits(store StateStore) ClientOption {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if sharedPacer == nil {
		sharedPacer = NewPacer(WithPacerStore(store))
		sharedCircuit = NewCircuitBreaker(WithCircuitStore(store))
	}
	p, cb := sharedPacer, sharedCircuit
	return func(cl *Client) { cl.pacer, cl.circuit = p, cb }
}

// SharedPacingStatus reports the process-wide pacer and circuit breaker that
// WithSharedPersistentLimits created; ok is false until a client has used them.
func SharedPacingStatus() (st PacingStatus, circuit CircuitState, circuitUntil time.Time, ok bool) {
	sharedMu.Lock()
	p, cb := sharedPacer, sharedCircuit
	sharedMu.Unlock()
	if p == nil || cb == nil {
		return PacingStatus{}, CircuitClosed, time.Time{}, false
	}
	return p.Status(), cb.State(), cb.BackoffUntil(), true
}

// Circuit returns the client's circuit breaker.
func (c *Client) Circuit() *CircuitBreaker { return c.circuit }

// RequestsLastHour returns how many ComicVine requests went out in the last hour, restarts included.
func (c *Client) RequestsLastHour() int { return c.pacer.RequestsLastHour() }

// WithBaseURL overrides the ComicVine API base URL (used in tests).
func WithBaseURL(u string) ClientOption {
	return func(cl *Client) { cl.baseURL = u }
}

// NewClient creates a ComicVine API client.
func NewClient(apiKey string, opts ...ClientOption) *Client {
	c := &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
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
	// Results stay raw until the status is read: for a volume ComicVine does
	// not have, "results" is an empty list, not an object, and decoding it
	// straight into a Volume failed before the status was ever looked at. That
	// hid the real answer and skipped the pacer's count of the call.
	var resp apiResponse[json.RawMessage]
	jobLog(ctx, JobLogInfo, "Fetching volume %d from ComicVine", cvID)
	if err := c.get(ctx, fmt.Sprintf("/volume/4050-%d", cvID), params, &resp); err != nil {
		return nil, err
	}
	if resp.StatusCode == comicVineObjectNotFound {
		return nil, ErrVolumeNotFound{CVID: cvID}
	}
	if resp.StatusCode != 1 {
		return nil, fmt.Errorf("comicvine API error %d: %s", resp.StatusCode, resp.Error)
	}
	results := bytes.TrimSpace(resp.Results)
	if len(results) == 0 || results[0] == '[' || string(results) == "null" {
		return nil, ErrVolumeNotFound{CVID: cvID}
	}
	var vol Volume
	if err := json.Unmarshal(results, &vol); err != nil {
		return nil, fmt.Errorf("decode volume %d: %w", cvID, err)
	}
	return &vol, nil
}

// comicVineObjectNotFound is the API's status_code for an id it does not have.
const comicVineObjectNotFound = 101

// ErrVolumeNotFound means ComicVine has no volume with this id any more.
type ErrVolumeNotFound struct{ CVID int }

func (e ErrVolumeNotFound) Error() string {
	return fmt.Sprintf("volume %d is not on ComicVine (deleted or merged)", e.CVID)
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
		jobLog(ctx, JobLogInfo, "Fetching the issue list of volume %d from ComicVine", volumeCVID)
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

// SearchVolumes searches ComicVine for volumes matching a query string.
func (c *Client) SearchVolumes(ctx context.Context, query string) ([]Volume, error) {
	params := url.Values{
		"query":      {query},
		"resources":  {"volume"},
		"field_list": {"id,name,start_year,publisher,count_of_issues,image,description,deck,site_detail_url"},
	}
	var resp apiResponse[[]Volume]
	jobLog(ctx, JobLogInfo, "Searching ComicVine for %q", query)
	if err := c.get(ctx, "/search", params, &resp); err != nil {
		return nil, err
	}
	if resp.StatusCode != 1 {
		return nil, fmt.Errorf("comicvine API error %d: %s", resp.StatusCode, resp.Error)
	}
	return resp.Results, nil
}

// FetchIssueDetail fetches full metadata for a single issue, including credits and cover images.
func (c *Client) FetchIssueDetail(ctx context.Context, cvIssueID int) (*IssueDetail, error) {
	params := url.Values{
		"field_list": {"id,issue_number,name,volume,cover_date,store_date,description,site_detail_url," +
			"person_credits,character_credits,team_credits,location_credits,story_arc_credits,image"},
	}
	var resp apiResponse[IssueDetail]
	jobLog(ctx, JobLogInfo, "Fetching issue %d from ComicVine", cvIssueID)
	if err := c.get(ctx, fmt.Sprintf("/issue/4000-%d", cvIssueID), params, &resp); err != nil {
		return nil, err
	}
	if resp.StatusCode != 1 {
		return nil, fmt.Errorf("comicvine API error %d: %s", resp.StatusCode, resp.Error)
	}
	return &resp.Results, nil
}

// get performs a rate-limited, circuit-broken GET request.
func (c *Client) get(ctx context.Context, path string, params url.Values, dest any) error {
	if !c.circuit.Allow() {
		until := c.circuit.BackoffUntil()
		jobLog(ctx, JobLogWarn, "ComicVine requests are paused after an error for another %s", untilText(until))
		return ErrCircuitOpen{Until: until}
	}

	if err := c.pacer.Wait(ctx); err != nil {
		return err
	}

	params.Set("api_key", c.apiKey)
	params.Set("format", "json")

	reqURL := fmt.Sprintf("%s%s/?%s", c.baseURL, path, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	// Every request that went out counts toward the pacer's burst and window,
	// whatever came back. It used to be counted only after a good answer, so a
	// run of rate-limit replies was never paced.
	c.pacer.RecordRequest()
	if err != nil {
		c.circuit.RecordFailure()
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// ComicVine says "slow down" with HTTP 420 (status_code 107, "Slow down
	// cowboy") as well as the usual 429. 420 used to fall through as an
	// unexpected error, which left the circuit closed: the sync then failed
	// every pending volume one after another, 200 requests in seconds.
	if resp.StatusCode == comicVineRateLimitHTTP || resp.StatusCode == http.StatusTooManyRequests {
		return c.rateLimited(ctx, fmt.Sprintf("HTTP %d", resp.StatusCode))
	}

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode >= 500 {
		c.circuit.RecordFailure()
		return fmt.Errorf("comicvine returned HTTP %d", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	// The same refusal can arrive inside a 200 envelope.
	var probe struct {
		StatusCode int `json:"status_code"`
	}
	if json.Unmarshal(body, &probe) == nil && probe.StatusCode == comicVineRateLimitStatus {
		return c.rateLimited(ctx, "status_code 107")
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	c.circuit.RecordSuccess()
	return nil
}

const (
	// comicVineRateLimitHTTP and comicVineRateLimitStatus are how ComicVine
	// refuses a request for going too fast: HTTP 420 with status_code 107.
	comicVineRateLimitHTTP   = 420
	comicVineRateLimitStatus = 107
)

// rateLimited opens the circuit and reports it the same way an already-open
// circuit is reported, so every caller (the sync, the scraper, the match
// endpoint) waits instead of treating the refusal as a failure of one volume.
func (c *Client) rateLimited(ctx context.Context, what string) error {
	c.circuit.RecordFailure()
	until := c.circuit.BackoffUntil()
	jobLog(ctx, JobLogWarn, "ComicVine is rate limiting this key (%s) after %d requests in the last hour; requests are paused for %s", what, c.pacer.RequestsLastHour(), untilText(until))
	return ErrCircuitOpen{Until: until}
}
