package mocksnyk

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	"em/internal/snyk"
)

// Server is a mock Snyk REST API server backed by a Dataset.
type Server struct {
	Dataset     *Dataset
	MaxPageSize int
	mux         *http.ServeMux
}

// New creates a new mock Snyk server with the given dataset.
func New(ds *Dataset) *Server {
	s := &Server{
		Dataset:     ds,
		MaxPageSize: 100,
	}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/rest/self", s.handleSelf)
	s.mux.HandleFunc("/rest/orgs", s.handleOrgs)
	s.mux.HandleFunc("/rest/orgs/", s.handleOrgRoutes)
	return s
}

// Start returns an httptest.Server for use in go tests.
func (s *Server) Start() *httptest.Server {
	return httptest.NewTLSServer(s.mux)
}

// ListenAndServe starts the server on the given address (e.g., ":8082").
func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s.mux)
}

// NewClient creates a snyk.Client that talks to the given TLS test server.
func NewClient(ts *httptest.Server) *snyk.Client {
	// ts.URL is "https://127.0.0.1:PORT" — strip the scheme for Credentials.Site
	site := strings.TrimPrefix(ts.URL, "https://")
	creds := snyk.Credentials{
		Token: "mock-token",
		OrgID: "mock-org",
		Site:  site,
	}
	return snyk.NewClientWithHTTPDoer(ts.Client(), creds)
}

func (s *Server) handleSelf(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL.String())
	writeJSON(w, map[string]any{
		"data": map[string]any{
			"id": "mock-user-id",
		},
	})
}

func (s *Server) handleOrgs(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
	orgs := []map[string]any{
		{
			"id": s.Dataset.OrgID,
			"attributes": map[string]any{
				"name": s.Dataset.OrgName,
			},
		},
	}
	writeJSON(w, map[string]any{
		"data":  orgs,
		"links": map[string]any{"next": ""},
	})
}

func (s *Server) handleOrgRoutes(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)

	// /rest/orgs/{orgID}/issues  or  /rest/orgs/{orgID}/projects
	path := strings.TrimPrefix(r.URL.Path, "/rest/orgs/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}

	switch parts[1] {
	case "issues":
		s.handleIssues(w, r)
	case "projects":
		s.handleProjects(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	projects := make([]map[string]any, len(s.Dataset.Projects))
	for i, p := range s.Dataset.Projects {
		projects[i] = map[string]any{
			"id": p.ID,
			"relationships": map[string]any{
				"target": map[string]any{
					"data": map[string]any{"id": p.TargetID},
				},
			},
		}
	}
	writeJSON(w, map[string]any{
		"data":  projects,
		"links": map[string]any{"next": ""},
	})
}

func (s *Server) handleIssues(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := queryInt(r, "limit", s.MaxPageSize)
	if limit > s.MaxPageSize {
		limit = s.MaxPageSize
	}
	offsetStr := q.Get("starting_after")
	offset := 0
	if offsetStr != "" {
		offset, _ = strconv.Atoi(offsetStr)
	}

	all := s.Dataset.issues()

	// Filter by updated_after / updated_before for resolved issues
	if after := q.Get("updated_after"); after != "" {
		all = filterByUpdated(all, after, q.Get("updated_before"))
	}

	// Filter by created_after / created_before for all issues
	if after := q.Get("created_after"); after != "" {
		all = filterByCreated(all, after, q.Get("created_before"))
	}

	total := len(all)
	end := offset + limit
	if end > total {
		end = total
	}
	if offset > total {
		offset = total
	}
	page := all[offset:end]

	data := make([]map[string]any, len(page))
	for i, iwp := range page {
		data[i] = issueToAPIData(iwp)
	}

	var nextLink string
	if end < total {
		nextLink = fmt.Sprintf("/rest/orgs/%s/issues?starting_after=%d&limit=%d&version=%s",
			s.Dataset.OrgID, end, limit, q.Get("version"))
	}

	writeJSON(w, map[string]any{
		"data":  data,
		"links": map[string]any{"next": nextLink},
	})
}

// issueToAPIData converts a snyk.Issue to the JSON:API response format.
func issueToAPIData(iwp issueWithProject) map[string]any {
	issue := iwp.Issue
	resolvedAt := ""
	if !issue.ResolvedAt.IsZero() {
		resolvedAt = issue.ResolvedAt.Format("2006-01-02T15:04:05Z")
	}

	coordinates := []map[string]any{}
	if issue.IsFixable {
		coordinates = append(coordinates, map[string]any{
			"is_fixable_snyk":     true,
			"is_fixable_manually": false,
			"is_fixable_upstream": false,
			"is_patchable":        false,
			"is_pinnable":         false,
			"is_upgradeable":      false,
		})
	} else {
		coordinates = append(coordinates, map[string]any{
			"is_fixable_snyk":     false,
			"is_fixable_manually": false,
			"is_fixable_upstream": false,
			"is_patchable":        false,
			"is_pinnable":         false,
			"is_upgradeable":      false,
		})
	}

	return map[string]any{
		"id": issue.ID,
		"attributes": map[string]any{
			"title":                    issue.Title,
			"effective_severity_level": issue.Severity,
			"type":                     issue.IssueType,
			"status":                   issue.Status,
			"ignored":                  issue.IsIgnored,
			"created_at":               issue.CreatedAt.Format("2006-01-02T15:04:05Z"),
			"resolved_at":              resolvedAt,
			"coordinates":              coordinates,
		},
		"relationships": map[string]any{
			"scan_item": map[string]any{
				"data": map[string]any{
					"id":   iwp.ProjectID,
					"type": "project",
				},
			},
		},
	}
}

func filterByUpdated(issues []issueWithProject, after, before string) []issueWithProject {
	var result []issueWithProject
	for _, iwp := range issues {
		issue := iwp.Issue
		// For resolved issues: use resolved_at as the updated proxy
		t := issue.ResolvedAt
		if t.IsZero() {
			t = issue.CreatedAt
		}
		ts := t.Format("2006-01-02T15:04:05Z")
		if after != "" && ts < after {
			continue
		}
		if before != "" && ts > before {
			continue
		}
		result = append(result, iwp)
	}
	return result
}

func filterByCreated(issues []issueWithProject, after, before string) []issueWithProject {
	var result []issueWithProject
	for _, iwp := range issues {
		ts := iwp.Issue.CreatedAt.Format("2006-01-02T15:04:05Z")
		if after != "" && ts < after {
			continue
		}
		if before != "" && ts > before {
			continue
		}
		result = append(result, iwp)
	}
	return result
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/vnd.api+json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func queryInt(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return n
}

// Usage prints instructions for using the mock server.
func Usage(addr string) string {
	return fmt.Sprintf(`Mock Snyk API server running at http://localhost%s

Note: The Snyk client uses HTTPS. For manual testing with 'em', use
--use-saved-data with CSVs generated by --save-raw-data instead.

This server is primarily useful for unit tests via httptest.NewTLSServer.

To generate sample Snyk data:
  em metrics snyk report --save-raw-data
  # Then edit snyk-issues-data.csv to simulate different scenarios.
`, addr)
}
