package mockgithub

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"

	"em/internal/github"
)

// Server is a mock GitHub API server backed by a Dataset.
type Server struct {
	Dataset     *Dataset
	MaxPageSize int
	mux         *http.ServeMux
}

// New creates a new mock GitHub server with the given dataset.
func New(ds *Dataset) *Server {
	s := &Server{
		Dataset:     ds,
		MaxPageSize: 100,
	}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/user/teams", s.handleUserTeams)
	s.mux.HandleFunc("/user", s.handleUser)
	s.mux.HandleFunc("/orgs/", s.handleOrgs)
	s.mux.HandleFunc("/repos/", s.handleRepos)
	return s
}

// Start returns an httptest.Server for use in go tests.
func (s *Server) Start() *httptest.Server {
	return httptest.NewTLSServer(s.mux)
}

// ListenAndServe starts the server on the given address (e.g., ":8081").
func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s.mux)
}

// NewClient creates a github.Client that talks to the given TLS test server.
func NewClient(ts *httptest.Server) (*github.Client, error) {
	return github.NewClientWithTransport(
		github.Credentials{Token: "mock-token"},
		&rewriteTransport{BaseURL: ts.URL, Inner: ts.Client().Transport},
	)
}

// rewriteTransport redirects all requests to a custom base URL.
// Used to point go-gh's REST client at an httptest server.
type rewriteTransport struct {
	BaseURL string
	Inner   http.RoundTripper
}

func (t *rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	base, err := url.Parse(t.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing base URL: %w", err)
	}
	r = r.Clone(r.Context())
	r.URL.Scheme = base.Scheme
	r.URL.Host = base.Host
	inner := t.Inner
	if inner == nil {
		inner = http.DefaultTransport
	}
	return inner.RoundTrip(r)
}

func (s *Server) handleUser(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL.String())
	writeJSON(w, map[string]any{
		"id":    1,
		"login": "mock-user",
		"name":  "Mock User",
	})
}

func (s *Server) handleUserTeams(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL.String())
	page, link := paginate(s.Dataset.Teams, r, s.MaxPageSize, func(t github.Team) any { return t })
	if link != "" {
		w.Header().Set("Link", link)
	}
	writeJSON(w, page)
}

func (s *Server) handleOrgs(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)

	// /orgs/{org}/teams/{team}/repos
	path := strings.TrimPrefix(r.URL.Path, "/orgs/")
	parts := strings.SplitN(path, "/", 4)
	if len(parts) == 4 && parts[1] == "teams" && parts[3] == "repos" {
		s.handleTeamRepos(w, r)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleTeamRepos(w http.ResponseWriter, r *http.Request) {
	page, link := paginate(s.Dataset.Repos, r, s.MaxPageSize, func(repo github.Repository) any { return repo })
	if link != "" {
		w.Header().Set("Link", link)
	}
	writeJSON(w, page)
}

func (s *Server) handleRepos(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)

	// /repos/{owner}/{repo}/actions/workflows[/{id}/runs]
	path := strings.TrimPrefix(r.URL.Path, "/repos/")
	parts := strings.SplitN(path, "/", 5)
	if len(parts) < 4 || parts[2] != "actions" {
		http.NotFound(w, r)
		return
	}

	owner, repo := parts[0], parts[1]
	repoKey := owner + "/" + repo

	switch {
	case len(parts) == 4 && parts[3] == "workflows":
		s.handleWorkflows(w, r, repoKey)
	case len(parts) == 5 && parts[3] == "workflows" && strings.HasSuffix(parts[4], "/runs"):
		workflowIDStr := strings.TrimSuffix(parts[4], "/runs")
		workflowID, err := strconv.ParseInt(workflowIDStr, 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		s.handleWorkflowRuns(w, r, owner, repo, workflowID)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleWorkflows(w http.ResponseWriter, r *http.Request, repoKey string) {
	workflows := s.Dataset.Workflows[repoKey]
	writeJSON(w, github.WorkflowListResponse{
		TotalCount: len(workflows),
		Workflows:  workflows,
	})
}

func (s *Server) handleWorkflowRuns(w http.ResponseWriter, r *http.Request, owner, repo string, workflowID int64) {
	key := runsKey(owner, repo, workflowID)
	allRuns := s.Dataset.Runs[key]

	// Filter by date range if created= query param is present
	createdFilter := r.URL.Query().Get("created")
	if createdFilter != "" {
		allRuns = filterRunsByDateRange(allRuns, createdFilter)
	}

	// Filter by branch
	if branch := r.URL.Query().Get("branch"); branch != "" {
		var filtered []github.WorkflowRun
		for _, run := range allRuns {
			if run.HeadBranch == branch {
				filtered = append(filtered, run)
			}
		}
		allRuns = filtered
	}

	page, link := paginate(allRuns, r, s.MaxPageSize, func(run github.WorkflowRun) any { return run })
	if link != "" {
		w.Header().Set("Link", link)
	}
	writeJSON(w, map[string]any{
		"total_count":   len(allRuns),
		"workflow_runs": page,
	})
}

// filterRunsByDateRange filters runs by the GitHub created= query param format (e.g. "2024-01-01..2024-03-31").
func filterRunsByDateRange(runs []github.WorkflowRun, filter string) []github.WorkflowRun {
	var from, to string
	if strings.Contains(filter, "..") {
		parts := strings.SplitN(filter, "..", 2)
		from, to = parts[0], parts[1]
	}

	var result []github.WorkflowRun
	for _, run := range runs {
		date := run.CreatedAt.Format("2006-01-02")
		if from != "" && date < from {
			continue
		}
		if to != "" && date > to {
			continue
		}
		result = append(result, run)
	}
	return result
}

// paginate returns a page of items and optionally sets a Link: rel="next" header value.
func paginate[T any](items []T, r *http.Request, maxPageSize int, toAny func(T) any) ([]any, string) {
	page := queryInt(r, "page", 1)
	perPage := queryInt(r, "per_page", maxPageSize)
	if perPage > maxPageSize {
		perPage = maxPageSize
	}

	start := (page - 1) * perPage
	if start >= len(items) {
		return []any{}, ""
	}
	end := start + perPage
	if end > len(items) {
		end = len(items)
	}

	out := make([]any, end-start)
	for i, item := range items[start:end] {
		out[i] = toAny(item)
	}

	var link string
	if end < len(items) {
		q := r.URL.Query()
		q.Set("page", strconv.Itoa(page+1))
		q.Set("per_page", strconv.Itoa(perPage))
		link = fmt.Sprintf("<%s?%s>; rel=\"next\"", r.URL.Path, q.Encode())
	}

	return out, link
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
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
	return fmt.Sprintf(`Mock GitHub API server running at http://localhost%s

Note: The GitHub client uses HTTPS. For manual testing with 'em', use
--use-saved-data with CSVs generated by --save-raw-data instead.

This server is primarily useful for unit tests via httptest.NewTLSServer.

To generate sample deployment data:
  em metrics github report --save-raw-data
  # Then use the saved CSV to test different scenarios.
`, addr)
}
