package mockjira

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	"devctl-em/internal/jira"
)

// Server is a mock JIRA API server backed by a Dataset.
type Server struct {
	Dataset     *Dataset
	MaxPageSize int
	mux         *http.ServeMux
}

// New creates a new mock JIRA server with the given dataset.
func New(ds *Dataset) *Server {
	s := &Server{
		Dataset:     ds,
		MaxPageSize: 100,
	}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/rest/api/3/myself", s.handleMyself)
	s.mux.HandleFunc("/rest/api/3/search/jql", s.handleSearch)
	s.mux.HandleFunc("/rest/api/3/issue/", s.handleIssue)
	return s
}

// Start returns an httptest.Server for use in go test.
func (s *Server) Start() *httptest.Server {
	return httptest.NewServer(s.mux)
}

// ListenAndServe starts the server on the given address (e.g., ":8080").
func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s.mux)
}

func (s *Server) handleMyself(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL.String())
	writeJSON(w, map[string]any{
		"accountId":    "mock-user-id",
		"emailAddress": "mock@test.com",
		"displayName":  "Mock User",
		"active":       true,
	})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)

	startAt := queryInt(r, "startAt", 0)
	maxResults := queryInt(r, "maxResults", s.MaxPageSize)
	if maxResults > s.MaxPageSize {
		maxResults = s.MaxPageSize
	}
	expand := r.URL.Query().Get("expand")

	issues := s.Dataset.Issues
	total := len(issues)

	// Paginate
	end := startAt + maxResults
	if end > total {
		end = total
	}
	if startAt > total {
		startAt = total
	}
	page := issues[startAt:end]

	// If expand=changelog, attach inline changelogs (possibly truncated)
	if strings.Contains(expand, "changelog") {
		page = s.attachChangelogs(page)
	}

	result := jira.SearchResult{
		PaginatedResponse: jira.PaginatedResponse{
			StartAt:    startAt,
			MaxResults: maxResults,
			Total:      total,
		},
		Issues: page,
	}
	writeJSON(w, result)
}

func (s *Server) attachChangelogs(issues []jira.Issue) []jira.Issue {
	out := make([]jira.Issue, len(issues))
	copy(out, issues)

	for i, issue := range out {
		entries := s.Dataset.Changelogs[issue.Key]
		total := len(entries)

		// Simulate JIRA's inline changelog truncation
		inline := entries
		if len(inline) > s.MaxPageSize {
			inline = inline[:s.MaxPageSize]
		}

		out[i].Changelog = &jira.Changelog{
			PaginatedResponse: jira.PaginatedResponse{
				StartAt:    0,
				MaxResults: s.MaxPageSize,
				Total:      total,
			},
			Histories: inline,
		}
	}
	return out
}

func (s *Server) handleIssue(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)

	// Parse /rest/api/3/issue/{key}/changelog
	path := strings.TrimPrefix(r.URL.Path, "/rest/api/3/issue/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[1] != "changelog" {
		http.NotFound(w, r)
		return
	}
	issueKey := parts[0]

	entries, ok := s.Dataset.Changelogs[issueKey]
	if !ok {
		http.NotFound(w, r)
		return
	}

	startAt := queryInt(r, "startAt", 0)
	maxResults := queryInt(r, "maxResults", s.MaxPageSize)
	if maxResults > s.MaxPageSize {
		maxResults = s.MaxPageSize
	}

	total := len(entries)
	end := startAt + maxResults
	if end > total {
		end = total
	}
	if startAt > total {
		startAt = total
	}

	result := jira.ChangelogResult{
		PaginatedResponse: jira.PaginatedResponse{
			StartAt:    startAt,
			MaxResults: maxResults,
			Total:      total,
		},
		Values: entries[startAt:end],
	}
	writeJSON(w, result)
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

// Usage prints usage instructions showing how to point devctl-em at this server.
func Usage(addr string) string {
	return fmt.Sprintf(`Mock JIRA server running at http://localhost%s

Usage:
  export JIRA_BASE_URL=http://localhost%s
  devctl-em metrics jira cycle-time --jql "project = PROJ"
  devctl-em metrics jira throughput --jql "project = PROJ"
  devctl-em metrics jira wip --jql "project = PROJ"
  devctl-em metrics jira report --jql "project = PROJ"
`, addr, addr)
}
