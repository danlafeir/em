// Package datadog provides a client for the Datadog API.
package datadog

import (
	"encoding/json"
	"strconv"
	"time"
)

// Credentials holds Datadog authentication details.
type Credentials struct {
	APIKey          string // Datadog API key
	AppKey          string // Datadog Application key
	Site            string // Datadog site (default "datadoghq.com")
	BaseURLOverride string // If set, use instead of https://api.<site> (for testing)
}

// BaseURL returns the standard Datadog API base URL.
func (c *Credentials) BaseURL() string {
	if c.BaseURLOverride != "" {
		return c.BaseURLOverride
	}
	site := c.Site
	if site == "" {
		site = "datadoghq.com"
	}
	return "https://api." + site
}

type SLOErrorBudget = sloErrorBudget

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

// sloErrorBudget parses error_budget_remaining, which can be a float or an object.
type sloErrorBudget float64

func (b *sloErrorBudget) UnmarshalJSON(data []byte) error {
	// Plain float (time-based SLOs)
	var f float64
	if err := json.Unmarshal(data, &f); err == nil {
		*b = sloErrorBudget(f)
		return nil
	}
	// Object (monitor-based SLOs): {"remaining": N}
	var obj struct {
		Remaining float64 `json:"remaining"`
	}
	if err := json.Unmarshal(data, &obj); err == nil {
		*b = sloErrorBudget(obj.Remaining)
		return nil
	}
	*b = 0
	return nil
}

// SLOHistorySLIData holds history data for a single SLO.
type SLOHistorySLIData struct {
	SLIValue             float64        `json:"sli_value"`
	ErrorBudgetRemaining sloErrorBudget `json:"error_budget_remaining"`
}

// sloHistoryResponse is the raw API response for SLO history.
type sloHistoryResponse struct {
	Data struct {
		Overall SLOHistorySLIData `json:"overall"`
	} `json:"data"`
}

// Monitor represents a Datadog monitor definition.
type Monitor struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Tags         []string `json:"tags"`
	OverallState string   `json:"overall_state"` // "Alert", "Warn", "No Data", "OK", "Ignored", "Skipped", "Unknown"
}

// MonitorEvent represents a monitor alert event from the Events v2 API.
type MonitorEvent struct {
	ID          string
	MonitorID   int64 // extracted from nested attributes or tags
	MonitorName string
	Status      string // "Alert", "Warn", "No Data", "Recovered", etc.
	Priority    string
	Timestamp   time.Time
	Tags        []string
}

// eventV2Timestamp is a Unix timestamp that may be an integer or string in the API response.
type eventV2Timestamp struct {
	time.Time
}

func (t *eventV2Timestamp) UnmarshalJSON(data []byte) error {
	// Try numeric first
	s := string(data)
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		t.Time = time.Unix(n, 0).UTC()
		return nil
	}
	// Try quoted string
	if len(s) >= 2 && s[0] == '"' {
		s = s[1 : len(s)-1]
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		t.Time = time.Unix(n, 0).UTC()
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}

// monitorEventInnerAttrs holds the nested monitor object inside event attributes.
type monitorEventInnerAttrs struct {
	Monitor struct {
		ID int64 `json:"id"`
	} `json:"monitor"`
}

// monitorEventAttributes holds raw attributes from the Events v2 API.
type monitorEventAttributes struct {
	Title      string                 `json:"title"`
	Status     string                 `json:"status"`
	Priority   string                 `json:"priority"`
	Timestamp  eventV2Timestamp       `json:"timestamp"`
	Tags       []string               `json:"tags"`
	Attributes monitorEventInnerAttrs `json:"attributes"` // nested attributes.monitor.id
}

// monitorEventData is a single item in the Events v2 response.
type monitorEventData struct {
	ID         string                 `json:"id"`
	Attributes monitorEventAttributes `json:"attributes"`
}

// monitorEventListResponse is the raw Events v2 API list response.
type monitorEventListResponse struct {
	Data []monitorEventData `json:"data"`
	Meta struct {
		Page struct {
			After string `json:"after"`
		} `json:"page"`
	} `json:"meta"`
}

// SLOEvent represents an SLO violation event from the Events v2 API.
type SLOEvent struct {
	ID        string
	SLOID     string // SLO ID, extracted from nested attributes or tags
	Title     string
	Timestamp time.Time
	Tags      []string
}

// sloEventInnerAttrs holds the nested SLO object inside event attributes.
type sloEventInnerAttrs struct {
	SLO struct {
		ID string `json:"id"`
	} `json:"slo"`
}

// sloEventAttributes holds raw attributes from the Events v2 API for SLO events.
type sloEventAttributes struct {
	Title      string             `json:"title"`
	Timestamp  eventV2Timestamp   `json:"timestamp"`
	Tags       []string           `json:"tags"`
	Attributes sloEventInnerAttrs `json:"attributes"`
}

// sloEventData is a single item in the Events v2 SLO response.
type sloEventData struct {
	ID         string             `json:"id"`
	Attributes sloEventAttributes `json:"attributes"`
}

// sloEventListResponse is the raw Events v2 API list response for SLO events.
type sloEventListResponse struct {
	Data []sloEventData `json:"data"`
	Meta struct {
		Page struct {
			After string `json:"after"`
		} `json:"page"`
	} `json:"meta"`
}
