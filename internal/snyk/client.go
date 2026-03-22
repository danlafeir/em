package snyk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"devctl-em/internal/httputil"
)

const apiVersion = "2025-11-05"

// Client is the main Snyk API client.
type Client struct {
	httpClient  httputil.HTTPDoer
	credentials Credentials
	rateLimiter *httputil.RateLimiter
}

// NewAuthClient creates a Snyk client with only token and site — no OrgID required.
// Use for operations that don't need an org (e.g. ListOrgs during config).
func NewAuthClient(token, site string) *Client {
	return NewClient(Credentials{Token: token, Site: site})
}

// NewClient creates a new Snyk API client.
func NewClient(creds Credentials) *Client {
	return &Client{
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		credentials: creds,
		rateLimiter: httputil.Default(),
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
			delay := c.rateLimiter.Backoff(attempt, resp.Header.Get("Retry-After"))
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


// isFixable returns true if any coordinate has a fix available.
func isFixable(coords []coordinate) bool {
	for _, c := range coords {
		if c.IsFixableManually || c.IsFixableSnyk || c.IsFixableUpstream || c.IsPatchable || c.IsPinnable || c.IsUpgradeable {
			return true
		}
	}
	return false
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

// GetProjectTargetMap returns a map of project ID → target ID for all projects in the org.
func (c *Client) GetProjectTargetMap(ctx context.Context) (map[string]string, error) {
	path := fmt.Sprintf("/rest/orgs/%s/projects", url.PathEscape(c.credentials.OrgID))

	query := url.Values{}
	query.Set("limit", "100")

	result := make(map[string]string)
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
			return nil, fmt.Errorf("fetching projects: %w", err)
		}

		var resp projectListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parsing projects: %w", err)
		}

		for _, p := range resp.Data {
			result[p.ID] = p.Relationships.Target.Data.ID
		}

		if resp.Links.Next == "" {
			break
		}
		nextURL = resp.Links.Next
		if nextURL[0] == '/' {
			nextURL = c.credentials.BaseURL() + nextURL
		}
	}
	return result, nil
}

// CountOpenIssues returns the current open issue counts broken down by severity,
// deduplicated by (target, title, severity) to match the Snyk UI's per-target grouping.
func (c *Client) CountOpenIssues(ctx context.Context) (OpenCounts, error) {
	projectTargets, err := c.GetProjectTargetMap(ctx)
	if err != nil {
		return OpenCounts{}, err
	}

	path := fmt.Sprintf("/rest/orgs/%s/issues", url.PathEscape(c.credentials.OrgID))

	query := url.Values{}
	query.Set("limit", "100")

	type issueKey struct {
		targetID string
		title    string
		severity string
	}
	seenOpen := make(map[issueKey]bool)
	seenIgnored := make(map[issueKey]bool)

	var counts OpenCounts
	nextURL := ""
	for {
		var body []byte
		if nextURL != "" {
			body, err = c.doRequest(ctx, "GET", "", nil, nextURL)
		} else {
			body, err = c.doRequest(ctx, "GET", path, query, "")
		}
		if err != nil {
			return OpenCounts{}, fmt.Errorf("counting open issues: %w", err)
		}

		var resp issueListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return OpenCounts{}, fmt.Errorf("counting open issues: %w", err)
		}

		for _, d := range resp.Data {
			if d.Attributes.Status != "open" {
				continue
			}
			projectID := d.Relationships.ScanItem.Data.ID
			targetID := projectTargets[projectID]
			if targetID == "" {
				targetID = projectID
			}
			key := issueKey{
				targetID: targetID,
				title:    d.Attributes.Title,
				severity: strings.ToLower(d.Attributes.EffectiveSeverityLevel),
			}
			if d.Attributes.Ignored {
				if seenIgnored[key] {
					continue
				}
				seenIgnored[key] = true
				counts.Ignored++
				if isFixable(d.Attributes.Coordinates) {
					counts.IgnoredFixable++
				} else {
					counts.IgnoredUnfixable++
				}
				continue
			}
			if seenOpen[key] {
				continue
			}
			seenOpen[key] = true
			counts.Total++
			isFixable := isFixable(d.Attributes.Coordinates)
			if isFixable {
				counts.Fixable++
			} else {
				counts.Unfixable++
			}
			switch key.severity {
			case "critical":
				counts.Critical++
			case "high":
				counts.High++
			case "medium":
				counts.Medium++
			case "low":
				counts.Low++
			}
		}

		if resp.Links.Next == "" {
			break
		}
		nextURL = resp.Links.Next
		if nextURL[0] == '/' {
			nextURL = c.credentials.BaseURL() + nextURL
		}
	}
	return counts, nil
}

// ListResolvedIssues fetches issues resolved within the given date range.
// It filters by update time to catch recently resolved issues, then returns
// only those with status "resolved" and a resolved_at within the range.
func (c *Client) ListResolvedIssues(ctx context.Context, from, to time.Time) ([]Issue, error) {
	var all []Issue
	path := fmt.Sprintf("/rest/orgs/%s/issues", url.PathEscape(c.credentials.OrgID))

	query := url.Values{}
	query.Set("limit", "100")
	query.Set("updated_after", from.Format(time.RFC3339))
	query.Set("updated_before", to.Format(time.RFC3339))

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
			return nil, fmt.Errorf("listing resolved issues: %w", err)
		}

		var resp issueListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parsing resolved issues: %w", err)
		}

		for _, d := range resp.Data {
			if d.Attributes.Status != "resolved" {
				continue
			}
			issue := Issue{
				ID:        d.ID,
				Title:     d.Attributes.Title,
				Severity:  d.Attributes.EffectiveSeverityLevel,
				IssueType: d.Attributes.Type,
				Status:    d.Attributes.Status,
				IsFixable: isFixable(d.Attributes.Coordinates),
			}
			if t, err := time.Parse(time.RFC3339, d.Attributes.CreatedAt); err == nil {
				issue.CreatedAt = t
			}
			if t, err := time.Parse(time.RFC3339, d.Attributes.ResolvedAt); err == nil {
				issue.ResolvedAt = t
			}
			// Only include if resolved_at falls within the requested range
			if issue.ResolvedAt.IsZero() || issue.ResolvedAt.Before(from) || issue.ResolvedAt.After(to) {
				continue
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
				IsFixable: isFixable(d.Attributes.Coordinates),
				IsIgnored: d.Attributes.Ignored,
			}
			if t, err := time.Parse(time.RFC3339, d.Attributes.CreatedAt); err == nil {
				issue.CreatedAt = t
			}
			if t, err := time.Parse(time.RFC3339, d.Attributes.ResolvedAt); err == nil {
				issue.ResolvedAt = t
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
