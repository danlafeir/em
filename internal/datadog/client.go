package datadog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client is the main Datadog API client.
type Client struct {
	httpClient  *http.Client
	credentials Credentials
	rateLimiter *RateLimiter
}

// RateLimiter implements exponential backoff with jitter.
type RateLimiter struct {
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	MaxRetries int
}

// NewClient creates a new Datadog API client.
func NewClient(creds Credentials) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		credentials: creds,
		rateLimiter: &RateLimiter{
			BaseDelay:  2 * time.Second,
			MaxDelay:   30 * time.Second,
			MaxRetries: 5,
		},
	}
}

// doRequest executes a request with authentication and rate limit handling.
func (c *Client) doRequest(ctx context.Context, method, baseURL, path string, query url.Values) ([]byte, error) {
	fullURL := baseURL + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	var lastErr error
	for attempt := 0; attempt <= c.rateLimiter.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		req.Header.Set("DD-API-KEY", c.credentials.APIKey)
		req.Header.Set("DD-APPLICATION-KEY", c.credentials.AppKey)
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("executing request: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading response: %w", err)
		}

		if resp.StatusCode == 429 {
			lastErr = fmt.Errorf("rate limited (HTTP 429)")
			delay := c.calculateBackoff(attempt, resp.Header.Get("Retry-After"))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
				continue
			}
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("Datadog API error %d: %s", resp.StatusCode, string(body))
		}

		return body, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
	}
	return nil, fmt.Errorf("max retries exceeded")
}

// calculateBackoff computes delay with exponential backoff and jitter.
func (c *Client) calculateBackoff(attempt int, retryAfter string) time.Duration {
	if retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}

	delay := float64(c.rateLimiter.BaseDelay) * math.Pow(2, float64(attempt))
	jitter := 0.7 + rand.Float64()*0.6
	delay *= jitter

	if delay > float64(c.rateLimiter.MaxDelay) {
		delay = float64(c.rateLimiter.MaxDelay)
	}

	return time.Duration(delay)
}

// TestConnection verifies the Datadog credentials work.
func (c *Client) TestConnection(ctx context.Context) error {
	_, err := c.doRequest(ctx, "GET", c.credentials.BaseURL(), "/api/v1/validate", nil)
	return err
}

// ListMonitors lists monitors filtered by a team tag, e.g. "team:my-team".
// Pass an empty string to list all monitors.
func (c *Client) ListMonitors(ctx context.Context, teamTag string) ([]Monitor, error) {
	query := url.Values{}
	if teamTag != "" {
		query.Set("tags", teamTag)
	}
	query.Set("page", "0")
	query.Set("page_size", "1000")

	body, err := c.doRequest(ctx, "GET", c.credentials.BaseURL(), "/api/v1/monitor", query)
	if err != nil {
		return nil, fmt.Errorf("listing monitors: %w", err)
	}

	var monitors []Monitor
	if err := json.Unmarshal(body, &monitors); err != nil {
		return nil, fmt.Errorf("parsing monitors: %w", err)
	}

	return monitors, nil
}

// ListMonitorEvents fetches monitor alert events from the Events v2 API.
// It returns events where a monitor transitioned to Alert (or Warn/No Data).
// Pass a non-empty tagsQuery to filter by team tag, e.g. "team:my-team".
func (c *Client) ListMonitorEvents(ctx context.Context, tagsQuery string, from, to time.Time) ([]MonitorEvent, error) {
	var all []MonitorEvent
	var cursor string

	filterQuery := "sources:monitor alert_transition:(alert OR \"no data\")"
	if tagsQuery != "" {
		filterQuery += " " + tagsQuery
	}

	for {
		query := url.Values{}
		query.Set("filter[query]", filterQuery)
		query.Set("filter[from]", from.UTC().Format(time.RFC3339))
		query.Set("filter[to]", to.UTC().Format(time.RFC3339))
		query.Set("page[limit]", "1000")
		if cursor != "" {
			query.Set("page[cursor]", cursor)
		}

		body, err := c.doRequest(ctx, "GET", c.credentials.BaseURL(), "/api/v2/events", query)
		if err != nil {
			return nil, fmt.Errorf("listing monitor events: %w", err)
		}

		var resp monitorEventListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parsing monitor events: %w", err)
		}

		for _, d := range resp.Data {
			all = append(all, MonitorEvent{
				ID:          d.ID,
				MonitorName: d.Attributes.Title,
				Status:      d.Attributes.Status,
				Priority:    d.Attributes.Priority,
				Timestamp:   d.Attributes.Timestamp.Time,
				Tags:        d.Attributes.Tags,
			})
		}

		cursor = resp.Meta.Page.After
		if cursor == "" || len(resp.Data) == 0 {
			break
		}
	}

	return all, nil
}

// ListSLOs lists SLOs filtered by a tags query string.
func (c *Client) ListSLOs(ctx context.Context, tagsQuery string) ([]SLOData, error) {
	query := url.Values{}
	if tagsQuery != "" {
		query.Set("tags_query", tagsQuery)
	}

	body, err := c.doRequest(ctx, "GET", c.credentials.BaseURL(), "/api/v1/slo", query)
	if err != nil {
		return nil, fmt.Errorf("listing SLOs: %w", err)
	}

	var resp sloListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing SLOs: %w", err)
	}

	return resp.Data, nil
}

// GetSLOHistory retrieves historical SLI data for a specific SLO.
func (c *Client) GetSLOHistory(ctx context.Context, sloID string, from, to time.Time) (*SLOHistorySLIData, error) {
	path := fmt.Sprintf("/api/v1/slo/%s/history", url.PathEscape(sloID))
	query := url.Values{}
	query.Set("from_ts", strconv.FormatInt(from.Unix(), 10))
	query.Set("to_ts", strconv.FormatInt(to.Unix(), 10))

	body, err := c.doRequest(ctx, "GET", c.credentials.BaseURL(), path, query)
	if err != nil {
		return nil, fmt.Errorf("getting SLO history: %w", err)
	}

	var resp sloHistoryResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing SLO history: %w", err)
	}

	return &resp.Data.Overall, nil
}
