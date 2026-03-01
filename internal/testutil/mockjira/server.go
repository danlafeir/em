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
	s.mux.HandleFunc("/rest/api/3/project/", s.handleProject)
	s.mux.HandleFunc("/rest/api/3/filter/", s.handleFilter)
	s.mux.HandleFunc("/rest/agile/1.0/board", s.handleBoards)
	s.mux.HandleFunc("/rest/agile/1.0/board/", s.handleBoards)
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

func (s *Server) handleProject(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL.String())
	key := strings.TrimPrefix(r.URL.Path, "/rest/api/3/project/")
	// Return a project with a deterministic ID based on the key
	writeJSON(w, map[string]any{
		"id":   "10000",
		"key":  key,
		"name": "Test Project",
	})
}

func (s *Server) handleFilter(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL.String())
	filterID := strings.TrimPrefix(r.URL.Path, "/rest/api/3/filter/")
	if s.Dataset.Filters != nil {
		if f, ok := s.Dataset.Filters[filterID]; ok {
			writeJSON(w, f)
			return
		}
	}
	http.NotFound(w, r)
}

func (s *Server) handleBoards(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)

	// Check for board configuration sub-path: /rest/agile/1.0/board/{id}/configuration
	path := r.URL.Path
	if strings.Count(path, "/") > 4 {
		// Route like /rest/agile/1.0/board/123/configuration
		parts := strings.Split(strings.TrimPrefix(path, "/rest/agile/1.0/board/"), "/")
		if len(parts) == 2 && parts[1] == "configuration" {
			boardID, err := strconv.Atoi(parts[0])
			if err != nil {
				http.NotFound(w, r)
				return
			}
			// Find the board and return a config with a filter ID matching the board ID
			for _, b := range s.Dataset.Boards {
				if b.ID == boardID {
					writeJSON(w, map[string]any{
						"filter": map[string]any{
							"id": strconv.Itoa(boardID * 10),
						},
					})
					return
				}
			}
			http.NotFound(w, r)
			return
		}
	}

	// Board listing
	startAt := queryInt(r, "startAt", 0)
	maxResults := queryInt(r, "maxResults", s.MaxPageSize)
	if maxResults > s.MaxPageSize {
		maxResults = s.MaxPageSize
	}

	boards := s.Dataset.Boards
	total := len(boards)

	end := startAt + maxResults
	if end > total {
		end = total
	}
	if startAt > total {
		startAt = total
	}

	isLast := end >= total
	writeJSON(w, map[string]any{
		"startAt":    startAt,
		"maxResults": maxResults,
		"total":      total,
		"isLast":     isLast,
		"values":     boards[startAt:end],
	})
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
