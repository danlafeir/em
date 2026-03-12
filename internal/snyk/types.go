// Package snyk provides a client for the Snyk REST API.
package snyk

import "time"

// Credentials holds Snyk authentication details.
type Credentials struct {
	Token string // Snyk API token
	OrgID string // Snyk organization ID
	Site  string // API host (default "api.snyk.io")
}

// BaseURL returns the Snyk API base URL.
func (c *Credentials) BaseURL() string {
	site := c.Site
	if site == "" {
		site = "api.snyk.io"
	}
	return "https://" + site
}

// Org represents a Snyk organization.
type Org struct {
	ID   string
	Name string
}

// Project represents a Snyk project.
type Project struct {
	ID   string
	Name string
}

// Issue represents a Snyk vulnerability issue.
type Issue struct {
	ID        string
	Title     string
	Severity  string // critical, high, medium, low
	IssueType string
	Status    string
	CreatedAt time.Time
}

// projectAttributes holds attributes from the projects API response.
type projectAttributes struct {
	Name string `json:"name"`
	Tags []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"tags"`
}

// projectData represents a single item in the projects API response.
type projectData struct {
	ID         string            `json:"id"`
	Attributes projectAttributes `json:"attributes"`
}

// projectListResponse is the raw API response for listing projects.
type projectListResponse struct {
	Data  []projectData `json:"data"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

// issueAttributes holds attributes from the issues API response.
type issueAttributes struct {
	Title                  string `json:"title"`
	EffectiveSeverityLevel string `json:"effective_severity_level"`
	Type                   string `json:"type"`
	Status                 string `json:"status"`
	CreatedAt              string `json:"created_at"`
}

// issueData represents a single item in the issues API response.
type issueData struct {
	ID         string          `json:"id"`
	Attributes issueAttributes `json:"attributes"`
}

// issueListResponse is the raw API response for listing issues.
type issueListResponse struct {
	Data  []issueData `json:"data"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

// orgAttributes holds attributes from the orgs API response.
type orgAttributes struct {
	Name string `json:"name"`
}

// orgData represents a single item in the orgs API response.
type orgData struct {
	ID         string        `json:"id"`
	Attributes orgAttributes `json:"attributes"`
}

// orgListResponse is the raw API response for listing orgs.
type orgListResponse struct {
	Data  []orgData `json:"data"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

// selfResponse is the raw API response for the self endpoint.
type selfResponse struct {
	Data struct {
		ID string `json:"id"`
	} `json:"data"`
}
