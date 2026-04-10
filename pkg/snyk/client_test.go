package snyk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"em/pkg/httputil"
)

// mockHTTPDoer is a simple mock for httputil.HTTPDoer that serves a sequence of responses.
type mockHTTPDoer struct {
	responses []*http.Response
	index     int
	requests  []*http.Request
}

func (m *mockHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	m.requests = append(m.requests, req)
	if m.index >= len(m.responses) {
		panic("mockHTTPDoer: no more responses")
	}
	resp := m.responses[m.index]
	m.index++
	return resp, nil
}

func newJSONResponse(statusCode int, body any) *http.Response {
	data, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(string(data))),
		Header:     make(http.Header),
	}
}

func newClient(mock *mockHTTPDoer) *Client {
	return &Client{
		httpClient:  mock,
		credentials: Credentials{Token: "test-token", OrgID: "org-123", Site: "api.snyk.io"},
		rateLimiter: httputil.Default(),
	}
}

// ---- isFixable ----

func TestIsFixable_AllFalse(t *testing.T) {
	coords := []coordinate{{}}
	if isFixable(coords) {
		t.Error("expected false for all-false coordinate")
	}
}

func TestIsFixable_Empty(t *testing.T) {
	if isFixable(nil) {
		t.Error("expected false for nil coordinates")
	}
	if isFixable([]coordinate{}) {
		t.Error("expected false for empty coordinates")
	}
}

func TestIsFixable_EachField(t *testing.T) {
	cases := []struct {
		name  string
		coord coordinate
	}{
		{"IsFixableManually", coordinate{IsFixableManually: true}},
		{"IsFixableSnyk", coordinate{IsFixableSnyk: true}},
		{"IsFixableUpstream", coordinate{IsFixableUpstream: true}},
		{"IsPatchable", coordinate{IsPatchable: true}},
		{"IsPinnable", coordinate{IsPinnable: true}},
		{"IsUpgradeable", coordinate{IsUpgradeable: true}},
	}
	for _, c := range cases {
		if !isFixable([]coordinate{c.coord}) {
			t.Errorf("%s: expected isFixable=true", c.name)
		}
	}
}

func TestIsFixable_TrueWhenAnyCoordTrue(t *testing.T) {
	coords := []coordinate{
		{},
		{IsPinnable: true},
		{},
	}
	if !isFixable(coords) {
		t.Error("expected true when any coordinate is fixable")
	}
}

// ---- CountOpenIssues ----

func makeIssueData(id, projectID, title, severity, status string, ignored bool, coords []coordinate) issueData {
	return issueData{
		ID: id,
		Attributes: issueAttributes{
			Title:                  title,
			EffectiveSeverityLevel: severity,
			Status:                 status,
			Ignored:                ignored,
			Coordinates:            coords,
		},
		Relationships: issueRelationships{
			ScanItem: struct {
				Data struct {
					ID   string `json:"id"`
					Type string `json:"type"`
				} `json:"data"`
			}{Data: struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			}{ID: projectID}},
		},
	}
}

func TestCountOpenIssues_BasicCounts(t *testing.T) {
	// No projects pagination, then issues with one of each severity.
	mock := &mockHTTPDoer{
		responses: []*http.Response{
			// GetProjectTargetMap: one page, no next
			newJSONResponse(200, projectListResponse{
				Data: []projectData{{ID: "proj-1", Relationships: struct {
					Target struct {
						Data struct {
							ID string `json:"id"`
						} `json:"data"`
					} `json:"target"`
				}{Target: struct {
					Data struct {
						ID string `json:"id"`
					} `json:"data"`
				}{Data: struct {
					ID string `json:"id"`
				}{ID: "target-1"}}}},
				},
			}),
			// CountOpenIssues: one page
			newJSONResponse(200, issueListResponse{
				Data: []issueData{
					makeIssueData("i1", "proj-1", "vuln-critical", "critical", "open", false, nil),
					makeIssueData("i2", "proj-1", "vuln-high", "high", "open", false, nil),
					makeIssueData("i3", "proj-1", "vuln-medium", "medium", "open", false, nil),
					makeIssueData("i4", "proj-1", "vuln-low", "low", "open", false, nil),
				},
			}),
		},
	}
	client := newClient(mock)

	counts, err := client.CountOpenIssues(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if counts.Critical != 1 {
		t.Errorf("Critical: want 1, got %d", counts.Critical)
	}
	if counts.High != 1 {
		t.Errorf("High: want 1, got %d", counts.High)
	}
	if counts.Medium != 1 {
		t.Errorf("Medium: want 1, got %d", counts.Medium)
	}
	if counts.Low != 1 {
		t.Errorf("Low: want 1, got %d", counts.Low)
	}
	if counts.Total != 4 {
		t.Errorf("Total: want 4, got %d", counts.Total)
	}
}

func TestCountOpenIssues_Deduplication(t *testing.T) {
	// Two issues with the same (targetID, title, severity) — only one should be counted.
	mock := &mockHTTPDoer{
		responses: []*http.Response{
			newJSONResponse(200, projectListResponse{
				Data: []projectData{
					{ID: "proj-1", Relationships: struct {
						Target struct {
							Data struct {
								ID string `json:"id"`
							} `json:"data"`
						} `json:"target"`
					}{Target: struct {
						Data struct {
							ID string `json:"id"`
						} `json:"data"`
					}{Data: struct {
						ID string `json:"id"`
					}{ID: "target-1"}}}},
					{ID: "proj-2", Relationships: struct {
						Target struct {
							Data struct {
								ID string `json:"id"`
							} `json:"data"`
						} `json:"target"`
					}{Target: struct {
						Data struct {
							ID string `json:"id"`
						} `json:"data"`
					}{Data: struct {
						ID string `json:"id"`
					}{ID: "target-1"}}}}, // same target
				},
			}),
			newJSONResponse(200, issueListResponse{
				Data: []issueData{
					makeIssueData("i1", "proj-1", "same-vuln", "high", "open", false, nil),
					makeIssueData("i2", "proj-2", "same-vuln", "high", "open", false, nil), // duplicate
				},
			}),
		},
	}
	client := newClient(mock)

	counts, err := client.CountOpenIssues(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts.Total != 1 {
		t.Errorf("Total: want 1 (deduplicated), got %d", counts.Total)
	}
	if counts.High != 1 {
		t.Errorf("High: want 1, got %d", counts.High)
	}
}

func TestCountOpenIssues_SkipsNonOpen(t *testing.T) {
	mock := &mockHTTPDoer{
		responses: []*http.Response{
			newJSONResponse(200, projectListResponse{}),
			newJSONResponse(200, issueListResponse{
				Data: []issueData{
					makeIssueData("i1", "proj-1", "resolved-vuln", "high", "resolved", false, nil),
					makeIssueData("i2", "proj-1", "open-vuln", "high", "open", false, nil),
				},
			}),
		},
	}
	client := newClient(mock)

	counts, err := client.CountOpenIssues(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts.Total != 1 {
		t.Errorf("Total: want 1 (only open), got %d", counts.Total)
	}
}

func TestCountOpenIssues_IgnoredBreakdown(t *testing.T) {
	fixableCoord := []coordinate{{IsUpgradeable: true}}
	mock := &mockHTTPDoer{
		responses: []*http.Response{
			newJSONResponse(200, projectListResponse{}),
			newJSONResponse(200, issueListResponse{
				Data: []issueData{
					makeIssueData("i1", "proj-1", "ignored-fixable", "high", "open", true, fixableCoord),
					makeIssueData("i2", "proj-1", "ignored-unfixable", "high", "open", true, nil),
				},
			}),
		},
	}
	client := newClient(mock)

	counts, err := client.CountOpenIssues(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts.Total != 0 {
		t.Errorf("Total: want 0 (ignored issues not in total), got %d", counts.Total)
	}
	if counts.Ignored != 2 {
		t.Errorf("Ignored: want 2, got %d", counts.Ignored)
	}
	if counts.IgnoredFixable != 1 {
		t.Errorf("IgnoredFixable: want 1, got %d", counts.IgnoredFixable)
	}
	if counts.IgnoredUnfixable != 1 {
		t.Errorf("IgnoredUnfixable: want 1, got %d", counts.IgnoredUnfixable)
	}
}

func TestCountOpenIssues_ProjectTargetFallback(t *testing.T) {
	// When project has no target mapping, falls back to projectID as targetID.
	// Two issues from different projects but same title/severity → not deduped since different targetIDs.
	mock := &mockHTTPDoer{
		responses: []*http.Response{
			newJSONResponse(200, projectListResponse{}), // empty map
			newJSONResponse(200, issueListResponse{
				Data: []issueData{
					makeIssueData("i1", "proj-A", "same-vuln", "high", "open", false, nil),
					makeIssueData("i2", "proj-B", "same-vuln", "high", "open", false, nil),
				},
			}),
		},
	}
	client := newClient(mock)

	counts, err := client.CountOpenIssues(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// proj-A and proj-B are different targets (fallback), so both count
	if counts.Total != 2 {
		t.Errorf("Total: want 2 (different fallback targets), got %d", counts.Total)
	}
}

func TestCountOpenIssues_FixableUnfixable(t *testing.T) {
	fixableCoord := []coordinate{{IsPinnable: true}}
	mock := &mockHTTPDoer{
		responses: []*http.Response{
			newJSONResponse(200, projectListResponse{}),
			newJSONResponse(200, issueListResponse{
				Data: []issueData{
					makeIssueData("i1", "proj-1", "fixable-vuln", "high", "open", false, fixableCoord),
					makeIssueData("i2", "proj-1", "unfixable-vuln", "high", "open", false, nil),
				},
			}),
		},
	}
	client := newClient(mock)

	counts, err := client.CountOpenIssues(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts.Fixable != 1 {
		t.Errorf("Fixable: want 1, got %d", counts.Fixable)
	}
	if counts.Unfixable != 1 {
		t.Errorf("Unfixable: want 1, got %d", counts.Unfixable)
	}
}

// ---- ListIssues ----

func TestListIssues_AllStatuses(t *testing.T) {
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)

	mock := &mockHTTPDoer{
		responses: []*http.Response{
			newJSONResponse(200, issueListResponse{
				Data: []issueData{
					makeIssueData("i1", "proj-1", "open-vuln", "high", "open", false, nil),
					makeIssueData("i2", "proj-1", "resolved-vuln", "medium", "resolved", false, nil),
					makeIssueData("i3", "proj-1", "ignored-vuln", "low", "open", true, nil),
				},
			}),
		},
	}
	client := newClient(mock)

	issues, err := client.ListIssues(context.Background(), from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// All three statuses included
	if len(issues) != 3 {
		t.Errorf("want 3 issues, got %d", len(issues))
	}
}

func TestListIssues_IsIgnoredMapped(t *testing.T) {
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)

	mock := &mockHTTPDoer{
		responses: []*http.Response{
			newJSONResponse(200, issueListResponse{
				Data: []issueData{
					makeIssueData("i1", "proj-1", "ignored", "high", "open", true, nil),
					makeIssueData("i2", "proj-1", "not-ignored", "high", "open", false, nil),
				},
			}),
		},
	}
	client := newClient(mock)

	issues, err := client.ListIssues(context.Background(), from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("want 2 issues, got %d", len(issues))
	}
	if !issues[0].IsIgnored {
		t.Error("issues[0].IsIgnored: want true")
	}
	if issues[1].IsIgnored {
		t.Error("issues[1].IsIgnored: want false")
	}
}

// ---- ListResolvedIssues ----

func makeResolvedIssueData(id, resolvedAt string) issueData {
	return issueData{
		ID: id,
		Attributes: issueAttributes{
			Title:                  "vuln-" + id,
			EffectiveSeverityLevel: "high",
			Status:                 "resolved",
			ResolvedAt:             resolvedAt,
		},
	}
}

func TestListResolvedIssues_FiltersToRange(t *testing.T) {
	from := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)

	mock := &mockHTTPDoer{
		responses: []*http.Response{
			newJSONResponse(200, issueListResponse{
				Data: []issueData{
					makeResolvedIssueData("in-range", "2024-01-15T12:00:00Z"),
					makeResolvedIssueData("before-range", "2024-01-05T12:00:00Z"),
					makeResolvedIssueData("after-range", "2024-01-25T12:00:00Z"),
				},
			}),
		},
	}
	client := newClient(mock)

	issues, err := client.ListResolvedIssues(context.Background(), from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue in range, got %d", len(issues))
	}
	if issues[0].ID != "in-range" {
		t.Errorf("want id 'in-range', got %q", issues[0].ID)
	}
}

func TestListResolvedIssues_SkipsNonResolved(t *testing.T) {
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)

	mock := &mockHTTPDoer{
		responses: []*http.Response{
			newJSONResponse(200, issueListResponse{
				Data: []issueData{
					makeResolvedIssueData("resolved-one", "2024-01-15T12:00:00Z"),
					makeIssueData("open-one", "proj-1", "open-vuln", "high", "open", false, nil),
				},
			}),
		},
	}
	client := newClient(mock)

	issues, err := client.ListResolvedIssues(context.Background(), from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 (resolved only), got %d", len(issues))
	}
}

func TestListResolvedIssues_SkipsZeroResolvedAt(t *testing.T) {
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)

	mock := &mockHTTPDoer{
		responses: []*http.Response{
			newJSONResponse(200, issueListResponse{
				Data: []issueData{
					// status=resolved but no resolved_at timestamp
					{ID: "no-ts", Attributes: issueAttributes{Status: "resolved", Title: "no-ts"}},
				},
			}),
		},
	}
	client := newClient(mock)

	issues, err := client.ListResolvedIssues(context.Background(), from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("want 0 (zero resolved_at excluded), got %d", len(issues))
	}
}

// ---- Pagination ----

func TestCountOpenIssues_Pagination(t *testing.T) {
	// Projects: two pages. Issues: two pages.
	mock := &mockHTTPDoer{
		responses: []*http.Response{
			// Projects page 1 with next link
			newJSONResponse(200, projectListResponse{
				Data: []projectData{{ID: "proj-1"}},
				Links: struct {
					Next string `json:"next"`
				}{Next: "/rest/orgs/org-123/projects?page=2&version=2025-11-05"},
			}),
			// Projects page 2 (full URL used)
			newJSONResponse(200, projectListResponse{
				Data: []projectData{{ID: "proj-2"}},
			}),
			// Issues page 1
			newJSONResponse(200, issueListResponse{
				Data: []issueData{
					makeIssueData("i1", "proj-1", "vuln-a", "high", "open", false, nil),
				},
				Links: struct {
					Next string `json:"next"`
				}{Next: "/rest/orgs/org-123/issues?page=2&version=2025-11-05"},
			}),
			// Issues page 2
			newJSONResponse(200, issueListResponse{
				Data: []issueData{
					makeIssueData("i2", "proj-2", "vuln-b", "critical", "open", false, nil),
				},
			}),
		},
	}
	client := newClient(mock)

	counts, err := client.CountOpenIssues(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts.Total != 2 {
		t.Errorf("Total: want 2 (both pages), got %d", counts.Total)
	}
}

func TestListIssues_Pagination(t *testing.T) {
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)

	mock := &mockHTTPDoer{
		responses: []*http.Response{
			newJSONResponse(200, issueListResponse{
				Data: []issueData{
					makeIssueData("i1", "proj-1", "vuln-a", "high", "open", false, nil),
				},
				Links: struct {
					Next string `json:"next"`
				}{Next: "/rest/orgs/org-123/issues?page=2&version=2025-11-05"},
			}),
			newJSONResponse(200, issueListResponse{
				Data: []issueData{
					makeIssueData("i2", "proj-1", "vuln-b", "medium", "open", false, nil),
				},
			}),
		},
	}
	client := newClient(mock)

	issues, err := client.ListIssues(context.Background(), from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 2 {
		t.Errorf("want 2 issues from both pages, got %d", len(issues))
	}
}

// ---- doRequest headers ----

func TestDoRequest_SetsAuthHeader(t *testing.T) {
	mock := &mockHTTPDoer{
		responses: []*http.Response{
			newJSONResponse(200, issueListResponse{}),
		},
	}
	client := newClient(mock)

	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	_, _ = client.ListIssues(context.Background(), from, to)

	if len(mock.requests) == 0 {
		t.Fatal("no requests made")
	}
	req := mock.requests[0]
	authHeader := req.Header.Get("Authorization")
	if authHeader != "token test-token" {
		t.Errorf("Authorization: want %q, got %q", "token test-token", authHeader)
	}
	contentType := req.Header.Get("Content-Type")
	if contentType != "application/vnd.api+json" {
		t.Errorf("Content-Type: want %q, got %q", "application/vnd.api+json", contentType)
	}
}

func TestDoRequest_AppendsVersionParam(t *testing.T) {
	mock := &mockHTTPDoer{
		responses: []*http.Response{
			newJSONResponse(200, issueListResponse{}),
		},
	}
	client := newClient(mock)

	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	_, _ = client.ListIssues(context.Background(), from, to)

	if len(mock.requests) == 0 {
		t.Fatal("no requests made")
	}
	url := mock.requests[0].URL
	if url.Query().Get("version") != apiVersion {
		t.Errorf("version param: want %q, got %q", apiVersion, url.Query().Get("version"))
	}
}

func TestDoRequest_UsesFullURLForPagination(t *testing.T) {
	fullURL := "https://api.snyk.io/rest/orgs/org-123/issues?page=2&version=" + apiVersion
	mock := &mockHTTPDoer{
		responses: []*http.Response{
			// page 1 with relative next link
			newJSONResponse(200, issueListResponse{
				Data: []issueData{makeIssueData("i1", "p1", "v1", "high", "open", false, nil)},
				Links: struct {
					Next string `json:"next"`
				}{Next: fullURL},
			}),
			// page 2
			newJSONResponse(200, issueListResponse{}),
		},
	}
	client := newClient(mock)

	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	_, _ = client.ListIssues(context.Background(), from, to)

	if len(mock.requests) < 2 {
		t.Fatalf("expected 2 requests, got %d", len(mock.requests))
	}
	got := mock.requests[1].URL.String()
	if got != fullURL {
		t.Errorf("pagination request URL: want %q, got %q", fullURL, got)
	}
}

// ---- Credentials ----

func TestCredentials_BaseURL_Default(t *testing.T) {
	c := Credentials{}
	want := "https://api.snyk.io"
	if got := c.BaseURL(); got != want {
		t.Errorf("BaseURL() = %q, want %q", got, want)
	}
}

func TestCredentials_BaseURL_Custom(t *testing.T) {
	c := Credentials{Site: "api.eu.snyk.io"}
	want := "https://api.eu.snyk.io"
	if got := c.BaseURL(); got != want {
		t.Errorf("BaseURL() = %q, want %q", got, want)
	}
}
