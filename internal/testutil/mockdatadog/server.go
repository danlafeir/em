package mockdatadog

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"em/internal/datadog"
)

// Server is a mock Datadog API server backed by a Dataset.
type Server struct {
	Dataset *Dataset
	mux     *http.ServeMux
}

// New creates a new mock Datadog server with the given dataset.
func New(ds *Dataset) *Server {
	s := &Server{Dataset: ds}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/api/v1/validate", s.handleValidate)
	s.mux.HandleFunc("/api/v1/monitor", s.handleMonitors)
	s.mux.HandleFunc("/api/v1/slo", s.handleSLORoutes)
	s.mux.HandleFunc("/api/v1/slo/", s.handleSLORoutes)
	s.mux.HandleFunc("/api/v2/events", s.handleEvents)
	return s
}

// Start returns an httptest.Server for use in Go tests.
func (s *Server) Start() *httptest.Server {
	return httptest.NewTLSServer(s.mux)
}

// ListenAndServe starts the server on the given address (e.g., ":8083").
func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s.mux)
}

// NewClient creates a datadog.Client that talks to the given TLS test server.
func NewClient(ts *httptest.Server) *datadog.Client {
	creds := datadog.Credentials{
		APIKey:          "mock-api-key",
		AppKey:          "mock-app-key",
		BaseURLOverride: ts.URL,
	}
	return datadog.NewClientWithHTTPDoer(ts.Client(), creds)
}

func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL.String())
	writeJSON(w, map[string]any{"valid": true})
}

func (s *Server) handleMonitors(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)

	teamTag := r.URL.Query().Get("monitor_tags")
	var result []datadog.Monitor
	for _, m := range s.Dataset.Monitors {
		if teamTag == "" || monitorHasTag(m, teamTag) {
			result = append(result, m)
		}
	}
	if result == nil {
		result = []datadog.Monitor{}
	}
	writeJSON(w, result)
}

func (s *Server) handleSLORoutes(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)

	// /api/v1/slo            → list SLOs
	// /api/v1/slo/{id}/history → SLO history
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/slo")
	if path == "" || path == "/" {
		s.handleListSLOs(w, r)
		return
	}
	// /{id}/history
	parts := strings.SplitN(strings.Trim(path, "/"), "/", 2)
	if len(parts) == 2 && parts[1] == "history" {
		s.handleSLOHistory(w, r, parts[0])
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleListSLOs(w http.ResponseWriter, r *http.Request) {
	tagsQuery := r.URL.Query().Get("tags_query")
	var result []datadog.SLOData
	for _, slo := range s.Dataset.SLOs {
		if tagsQuery == "" || sloMatchesTagsQuery(slo, tagsQuery) {
			result = append(result, slo)
		}
	}
	if result == nil {
		result = []datadog.SLOData{}
	}
	writeJSON(w, map[string]any{"data": result})
}

func (s *Server) handleSLOHistory(w http.ResponseWriter, r *http.Request, sloID string) {
	history, ok := s.Dataset.SLOHistory[sloID]
	if !ok {
		// Return zero history rather than 404 so the client can still process it.
		history = SLOHistoryRecord{SLIValue: 100.0, Budget: 100.0}
	}
	writeJSON(w, map[string]any{
		"data": map[string]any{
			"overall": map[string]any{
				"sli_value":              history.SLIValue,
				"error_budget_remaining": history.Budget,
			},
		},
	})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)

	q := r.URL.Query()
	filterQuery := q.Get("filter[query]")
	from := parseEventTime(q.Get("filter[from]"))
	to := parseEventTime(q.Get("filter[to]"))

	if strings.Contains(filterQuery, "sources:monitor") {
		s.handleMonitorEvents(w, from, to)
	} else if strings.Contains(filterQuery, "sources:slo") {
		s.handleSLOEvents(w, from, to)
	} else {
		writeJSON(w, map[string]any{"data": []any{}, "meta": map[string]any{"page": map[string]any{"after": ""}}})
	}
}

func (s *Server) handleMonitorEvents(w http.ResponseWriter, from, to time.Time) {
	var data []map[string]any
	for _, e := range s.Dataset.MonitorEvents {
		if !from.IsZero() && e.Timestamp.Before(from) {
			continue
		}
		if !to.IsZero() && e.Timestamp.After(to) {
			continue
		}
		data = append(data, map[string]any{
			"id": e.ID,
			"attributes": map[string]any{
				"title":     e.Title,
				"status":    e.Status,
				"priority":  "3",
				"timestamp": e.Timestamp.Unix(),
				"tags":      append([]string{fmt.Sprintf("monitor_id:%d", e.MonitorID)}, e.Tags...),
				"attributes": map[string]any{
					"monitor": map[string]any{"id": e.MonitorID},
				},
			},
		})
	}
	if data == nil {
		data = []map[string]any{}
	}
	writeJSON(w, map[string]any{
		"data": data,
		"meta": map[string]any{"page": map[string]any{"after": ""}},
	})
}

func (s *Server) handleSLOEvents(w http.ResponseWriter, from, to time.Time) {
	var data []map[string]any
	for _, e := range s.Dataset.SLOEvents {
		if !from.IsZero() && e.Timestamp.Before(from) {
			continue
		}
		if !to.IsZero() && e.Timestamp.After(to) {
			continue
		}
		data = append(data, map[string]any{
			"id": e.ID,
			"attributes": map[string]any{
				"title":     e.Title,
				"timestamp": e.Timestamp.Unix(),
				"tags":      e.Tags,
				"attributes": map[string]any{
					"slo": map[string]any{"id": e.SLOID},
				},
			},
		})
	}
	if data == nil {
		data = []map[string]any{}
	}
	writeJSON(w, map[string]any{
		"data": data,
		"meta": map[string]any{"page": map[string]any{"after": ""}},
	})
}

func monitorHasTag(m datadog.Monitor, tag string) bool {
	for _, t := range m.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

func sloMatchesTagsQuery(slo datadog.SLOData, query string) bool {
	for _, tag := range slo.Tags {
		if strings.Contains(query, tag) {
			return true
		}
	}
	return false
}

func parseEventTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Usage prints instructions for using the mock server.
func Usage(addr string) string {
	return fmt.Sprintf(`Mock Datadog API server running at http://localhost%s

Note: The Datadog client uses HTTPS. For manual testing with 'em', use
--use-saved-data with CSVs generated by --save-raw-data instead.

This server is primarily useful for unit tests via httptest.NewTLSServer.

To generate sample Datadog SLO data:
  em metrics datadog slos --save-raw-data
  # Then edit the CSV to simulate different scenarios.
`, addr)
}
