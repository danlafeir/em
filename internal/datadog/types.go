// Package datadog provides a client for the Datadog API.
package datadog

import "time"

// Credentials holds Datadog authentication details.
type Credentials struct {
	APIKey string // Datadog API key
	AppKey string // Datadog Application key
	Site   string // Datadog site (default "datadoghq.com")
}

// BaseURL returns the standard Datadog API base URL.
func (c *Credentials) BaseURL() string {
	site := c.Site
	if site == "" {
		site = "datadoghq.com"
	}
	return "https://api." + site
}

// OnCallBaseURL returns the on-call API base URL.
func (c *Credentials) OnCallBaseURL() string {
	site := c.Site
	if site == "" {
		site = "datadoghq.com"
	}
	return "https://navy.oncall." + site
}

// Page represents a single on-call page/incident.
type Page struct {
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	Urgency        string    `json:"urgency"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	AcknowledgedAt time.Time `json:"acknowledged_at"`
	ResolvedAt     time.Time `json:"resolved_at"`
	Responder      string    `json:"responder"`
}

// pageAttributes holds the attributes from the on-call pages API response.
type pageAttributes struct {
	Title          string  `json:"title"`
	Urgency        string  `json:"urgency"`
	Status         string  `json:"status"`
	CreatedAt      string  `json:"created_at"`
	AcknowledgedAt string  `json:"acknowledged_at"`
	ResolvedAt     string  `json:"resolved_at"`
	Responder      *string `json:"responder"`
}

// pageData represents a single item in the pages API response.
type pageData struct {
	ID         string         `json:"id"`
	Attributes pageAttributes `json:"attributes"`
}

// pageListResponse is the raw API response for listing pages.
type pageListResponse struct {
	Data []pageData `json:"data"`
	Meta struct {
		Pagination struct {
			NextOffset int `json:"next_offset"`
		} `json:"pagination"`
	} `json:"meta"`
}

// SLOData represents a Datadog SLO definition.
type SLOData struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Tags       []string       `json:"tags"`
	Thresholds []SLOThreshold `json:"thresholds"`
}

// SLOThreshold represents a target threshold for an SLO.
type SLOThreshold struct {
	Timeframe string  `json:"timeframe"`
	Target    float64 `json:"target"`
	Warning   float64 `json:"warning"`
}

// sloListResponse is the raw API response for listing SLOs.
type sloListResponse struct {
	Data []SLOData `json:"data"`
}

// SLOHistorySLIData holds history data for a single SLO.
type SLOHistorySLIData struct {
	SLIValue             float64 `json:"sli_value"`
	ErrorBudgetRemaining float64 `json:"error_budget_remaining"`
}

// sloHistoryResponse is the raw API response for SLO history.
type sloHistoryResponse struct {
	Data struct {
		Overall SLOHistorySLIData `json:"overall"`
	} `json:"data"`
}
