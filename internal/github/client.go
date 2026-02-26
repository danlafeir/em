package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Client is the main GitHub API client.
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

// NewClient creates a new GitHub API client.
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
// Returns the response body and headers (needed for Link pagination).
func (c *Client) doRequest(ctx context.Context, method, path string, query url.Values) ([]byte, http.Header, error) {
	fullURL := c.credentials.BaseURL() + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	var lastErr error
	for attempt := 0; attempt <= c.rateLimiter.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("creating request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+c.credentials.Token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, nil, fmt.Errorf("executing request: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("reading response: %w", err)
		}

		// Handle rate limiting (429 or 403 with Retry-After)
		if resp.StatusCode == 429 || (resp.StatusCode == 403 && resp.Header.Get("Retry-After") != "") {
			lastErr = fmt.Errorf("rate limited (HTTP %d)", resp.StatusCode)
			delay := c.calculateBackoff(attempt, resp.Header.Get("Retry-After"))
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(delay):
				continue
			}
		}

		// Handle errors
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(body))
		}

		return body, resp.Header, nil
	}

	if lastErr != nil {
		return nil, nil, fmt.Errorf("max retries exceeded: %w", lastErr)
	}
	return nil, nil, fmt.Errorf("max retries exceeded")
}

// calculateBackoff computes delay with exponential backoff and jitter.
func (c *Client) calculateBackoff(attempt int, retryAfter string) time.Duration {
	// Use Retry-After header if provided
	if retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}

	// Exponential backoff: base * 2^attempt
	delay := float64(c.rateLimiter.BaseDelay) * math.Pow(2, float64(attempt))

	// Add jitter (0.7 to 1.3 multiplier)
	jitter := 0.7 + rand.Float64()*0.6
	delay *= jitter

	// Cap at maximum
	if delay > float64(c.rateLimiter.MaxDelay) {
		delay = float64(c.rateLimiter.MaxDelay)
	}

	return time.Duration(delay)
}

// linkNextRe matches rel="next" in Link headers.
var linkNextRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// parseLinkHeader extracts the "next" URL from a GitHub Link header.
func parseLinkHeader(header string) string {
	if header == "" {
		return ""
	}
	matches := linkNextRe.FindStringSubmatch(header)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

// doRequestURL executes a request against an absolute URL (for pagination).
func (c *Client) doRequestURL(ctx context.Context, rawURL string) ([]byte, http.Header, error) {
	var lastErr error
	for attempt := 0; attempt <= c.rateLimiter.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("creating request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+c.credentials.Token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, nil, fmt.Errorf("executing request: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("reading response: %w", err)
		}

		if resp.StatusCode == 429 || (resp.StatusCode == 403 && resp.Header.Get("Retry-After") != "") {
			lastErr = fmt.Errorf("rate limited (HTTP %d)", resp.StatusCode)
			delay := c.calculateBackoff(attempt, resp.Header.Get("Retry-After"))
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(delay):
				continue
			}
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(body))
		}

		return body, resp.Header, nil
	}

	if lastErr != nil {
		return nil, nil, fmt.Errorf("max retries exceeded: %w", lastErr)
	}
	return nil, nil, fmt.Errorf("max retries exceeded")
}

// ListTeamRepos lists repositories for a team, handling Link-header pagination.
func (c *Client) ListTeamRepos(ctx context.Context, org, teamSlug string) ([]Repository, error) {
	path := fmt.Sprintf("/orgs/%s/teams/%s/repos", url.PathEscape(org), url.PathEscape(teamSlug))
	query := url.Values{}
	query.Set("per_page", "100")

	body, headers, err := c.doRequest(ctx, "GET", path, query)
	if err != nil {
		return nil, fmt.Errorf("listing team repos: %w", err)
	}

	var repos []Repository
	if err := json.Unmarshal(body, &repos); err != nil {
		return nil, fmt.Errorf("parsing team repos: %w", err)
	}

	// Follow Link pagination
	nextURL := parseLinkHeader(headers.Get("Link"))
	for nextURL != "" {
		body, headers, err = c.doRequestURL(ctx, nextURL)
		if err != nil {
			return nil, fmt.Errorf("listing team repos (pagination): %w", err)
		}

		var page []Repository
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("parsing team repos page: %w", err)
		}
		repos = append(repos, page...)
		nextURL = parseLinkHeader(headers.Get("Link"))
	}

	return repos, nil
}

// ListWorkflows lists GitHub Actions workflows for a repository.
func (c *Client) ListWorkflows(ctx context.Context, owner, repo string) ([]Workflow, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/workflows", url.PathEscape(owner), url.PathEscape(repo))

	body, _, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("listing workflows: %w", err)
	}

	var resp WorkflowListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing workflows: %w", err)
	}

	return resp.Workflows, nil
}

// ListWorkflowRuns lists runs for a specific workflow, filtered by date range.
func (c *Client) ListWorkflowRuns(ctx context.Context, owner, repo string, workflowID int64, branch string, from, to time.Time) ([]WorkflowRun, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/workflows/%d/runs",
		url.PathEscape(owner), url.PathEscape(repo), workflowID)

	query := url.Values{}
	query.Set("status", "completed")
	query.Set("per_page", "100")
	if branch != "" {
		query.Set("branch", branch)
	}
	// GitHub API uses ISO 8601: created=YYYY-MM-DD..YYYY-MM-DD
	if !from.IsZero() || !to.IsZero() {
		var parts []string
		if !from.IsZero() {
			parts = append(parts, from.Format("2006-01-02"))
		}
		parts = append(parts, "..")
		if !to.IsZero() {
			parts = append(parts, to.Format("2006-01-02"))
		}
		query.Set("created", strings.Join(parts, ""))
	}

	var allRuns []WorkflowRun

	body, headers, err := c.doRequest(ctx, "GET", path, query)
	if err != nil {
		return nil, fmt.Errorf("listing workflow runs: %w", err)
	}

	var resp WorkflowRunsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing workflow runs: %w", err)
	}
	allRuns = append(allRuns, resp.WorkflowRuns...)

	// Follow Link pagination
	nextURL := parseLinkHeader(headers.Get("Link"))
	for nextURL != "" {
		body, headers, err = c.doRequestURL(ctx, nextURL)
		if err != nil {
			return nil, fmt.Errorf("listing workflow runs (pagination): %w", err)
		}

		var page WorkflowRunsResponse
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("parsing workflow runs page: %w", err)
		}
		allRuns = append(allRuns, page.WorkflowRuns...)
		nextURL = parseLinkHeader(headers.Get("Link"))
	}

	return allRuns, nil
}

// TestConnection verifies the GitHub credentials work.
func (c *Client) TestConnection(ctx context.Context) error {
	_, _, err := c.doRequest(ctx, "GET", "/user", nil)
	return err
}
