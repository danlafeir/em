package snyk

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

const apiVersion = "2024-10-15"

// Client is the main Snyk API client.
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

// NewAuthClient creates a Snyk client with only token and site — no OrgID required.
// Use for operations that don't need an org (e.g. ListOrgs during config).
func NewAuthClient(token, site string) *Client {
	return NewClient(Credentials{Token: token, Site: site})
}

// NewClient creates a new Snyk API client.
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
// If fullURL is provided (non-empty), it is used as-is (for pagination next links).
// Otherwise, the request is built from path and query against the base URL.
func (c *Client) doRequest(ctx context.Context, method, path string, query url.Values, fullURL string) ([]byte, error) {
	reqURL := fullURL
	if reqURL == "" {
		reqURL = c.credentials.BaseURL() + path
		if query == nil {
			query = url.Values{}
		}
		query.Set("version", apiVersion)
		reqURL += "?" + query.Encode()
	}

	var lastErr error
	for attempt := 0; attempt <= c.rateLimiter.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		req.Header.Set("Authorization", "token "+c.credentials.Token)
		req.Header.Set("Content-Type", "application/vnd.api+json")

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
			return nil, fmt.Errorf("Snyk API error %d: %s", resp.StatusCode, string(body))
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

// TestConnection verifies the Snyk credentials work.
func (c *Client) TestConnection(ctx context.Context) error {
	body, err := c.doRequest(ctx, "GET", "/rest/self", nil, "")
	if err != nil {
		return err
	}

	var resp selfResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parsing self response: %w", err)
	}
	return nil
}

// ListOrgs lists all Snyk organizations accessible to the authenticated user.
func (c *Client) ListOrgs(ctx context.Context) ([]Org, error) {
	var all []Org

	query := url.Values{}
	query.Set("limit", "100")

	nextURL := ""
	for {
		var body []byte
		var err error
		if nextURL != "" {
			body, err = c.doRequest(ctx, "GET", "", nil, nextURL)
		} else {
			body, err = c.doRequest(ctx, "GET", "/rest/orgs", query, "")
		}
		if err != nil {
			return nil, fmt.Errorf("listing orgs: %w", err)
		}

		var resp orgListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parsing orgs: %w", err)
		}

		for _, d := range resp.Data {
			all = append(all, Org{ID: d.ID, Name: d.Attributes.Name})
		}

		if resp.Links.Next == "" {
			break
		}
		nextURL = resp.Links.Next
		if nextURL != "" && nextURL[0] == '/' {
			nextURL = c.credentials.BaseURL() + nextURL
		}
	}

	return all, nil
}

// ListProjects lists Snyk projects filtered by a team tag.
func (c *Client) ListProjects(ctx context.Context, teamTag string) ([]Project, error) {
	var allProjects []Project
	path := fmt.Sprintf("/rest/orgs/%s/projects", url.PathEscape(c.credentials.OrgID))

	query := url.Values{}
	query.Set("limit", "100")
	if teamTag != "" {
		query.Set("tags", "team:"+teamTag)
	}

	nextURL := ""
	for {
		var body []byte
		var err error
		if nextURL != "" {
			body, err = c.doRequest(ctx, "GET", "", nil, nextURL)
		} else {
			body, err = c.doRequest(ctx, "GET", path, query, "")
		}
		if err != nil {
			return nil, fmt.Errorf("listing projects: %w", err)
		}

		var resp projectListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parsing projects: %w", err)
		}

		for _, d := range resp.Data {
			allProjects = append(allProjects, Project{
				ID:   d.ID,
				Name: d.Attributes.Name,
			})
		}

		if resp.Links.Next == "" {
			break
		}
		nextURL = resp.Links.Next
		// Ensure next URL is absolute
		if nextURL != "" && nextURL[0] == '/' {
			nextURL = c.credentials.BaseURL() + nextURL
		}
	}

	return allProjects, nil
}

// ListIssues fetches all issues for the org within the given date range.
func (c *Client) ListIssues(ctx context.Context, from, to time.Time) ([]Issue, error) {
	var all []Issue
	path := fmt.Sprintf("/rest/orgs/%s/issues", url.PathEscape(c.credentials.OrgID))

	query := url.Values{}
	query.Set("limit", "100")
	query.Set("created_after", from.Format(time.RFC3339))
	query.Set("created_before", to.Format(time.RFC3339))

	nextURL := ""
	for {
		var body []byte
		var err error
		if nextURL != "" {
			body, err = c.doRequest(ctx, "GET", "", nil, nextURL)
		} else {
			body, err = c.doRequest(ctx, "GET", path, query, "")
		}
		if err != nil {
			return nil, fmt.Errorf("listing issues: %w", err)
		}

		var resp issueListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parsing issues: %w", err)
		}

		for _, d := range resp.Data {
			issue := Issue{
				ID:        d.ID,
				Title:     d.Attributes.Title,
				Severity:  d.Attributes.EffectiveSeverityLevel,
				IssueType: d.Attributes.Type,
				Status:    d.Attributes.Status,
			}
			if t, err := time.Parse(time.RFC3339, d.Attributes.CreatedAt); err == nil {
				issue.CreatedAt = t
			}
			all = append(all, issue)
		}

		if resp.Links.Next == "" {
			break
		}
		nextURL = resp.Links.Next
		if nextURL != "" && nextURL[0] == '/' {
			nextURL = c.credentials.BaseURL() + nextURL
		}
	}

	return all, nil
}
