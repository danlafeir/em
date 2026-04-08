// Package mockdatadog provides an in-process Datadog API mock for testing.
package mockdatadog

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"time"

	"em/internal/datadog"
)

// Dataset holds all mock Datadog data served by the Server.
type Dataset struct {
	Monitors      []datadog.Monitor
	MonitorEvents []MonitorEventRecord
	SLOs          []datadog.SLOData
	SLOHistory    map[string]SLOHistoryRecord // keyed by SLO ID
	SLOEvents     []SLOEventRecord
}

// MonitorEventRecord is the server-side representation of a monitor alert event.
type MonitorEventRecord struct {
	ID        string
	MonitorID int64
	Title     string
	Status    string
	Timestamp time.Time
	Tags      []string
}

// SLOHistoryRecord stores SLI/budget values for a single SLO.
type SLOHistoryRecord struct {
	SLIValue float64
	Budget   float64
}

// SLOEventRecord is the server-side representation of an SLO violation event.
type SLOEventRecord struct {
	ID        string
	SLOID     string
	Title     string
	Timestamp time.Time
	Tags      []string
}

// NewDataset creates an empty Dataset.
func NewDataset() *Dataset {
	return &Dataset{
		SLOHistory: make(map[string]SLOHistoryRecord),
	}
}

// MonitorBuilder provides a fluent API for constructing test monitors.
type MonitorBuilder struct {
	id           int64
	name         string
	monitorType  string
	overallState string
	tags         []string
}

// NewMonitor starts building a new monitor.
func NewMonitor(id int64, name string) *MonitorBuilder {
	return &MonitorBuilder{
		id:           id,
		name:         name,
		monitorType:  "metric alert",
		overallState: "OK",
		tags:         []string{"team:platform"},
	}
}

func (b *MonitorBuilder) WithType(t string) *MonitorBuilder     { b.monitorType = t; return b }
func (b *MonitorBuilder) WithState(s string) *MonitorBuilder    { b.overallState = s; return b }
func (b *MonitorBuilder) WithTags(tags ...string) *MonitorBuilder { b.tags = tags; return b }
func (b *MonitorBuilder) AsAlerted() *MonitorBuilder            { b.overallState = "Alert"; return b }

// Build returns the constructed datadog.Monitor.
func (b *MonitorBuilder) Build() datadog.Monitor {
	return datadog.Monitor{
		ID:           b.id,
		Name:         b.name,
		Type:         b.monitorType,
		Tags:         b.tags,
		OverallState: b.overallState,
	}
}

// SLOBuilder provides a fluent API for constructing test SLOs.
type SLOBuilder struct {
	id      string
	app     string
	name    string
	sloType string
	tags    []string
	target  float64
	current float64
	budget  float64
}

// NewSLO starts building a new SLO.
func NewSLO(id, app, name string) *SLOBuilder {
	return &SLOBuilder{
		id:      id,
		app:     app,
		name:    name,
		sloType: "metric",
		tags:    []string{"team:platform", "app:" + app},
		target:  99.9,
		current: 99.95,
		budget:  50.0,
	}
}

func (b *SLOBuilder) WithType(t string) *SLOBuilder     { b.sloType = t; return b }
func (b *SLOBuilder) WithTarget(t float64) *SLOBuilder  { b.target = t; return b }
func (b *SLOBuilder) WithCurrent(c float64) *SLOBuilder { b.current = c; return b }
func (b *SLOBuilder) WithBudget(bud float64) *SLOBuilder { b.budget = bud; return b }

// AsViolated marks the SLO as below its target.
func (b *SLOBuilder) AsViolated() *SLOBuilder {
	b.current = b.target - 1.0
	b.budget = -10.0
	return b
}

// Build returns the SLOData and SLOHistoryRecord for the Dataset.
func (b *SLOBuilder) Build() (datadog.SLOData, SLOHistoryRecord) {
	slo := datadog.SLOData{
		ID:   b.id,
		Name: b.name,
		Type: b.sloType,
		Tags: b.tags,
		Thresholds: []datadog.SLOThreshold{
			{Timeframe: "7d", Target: b.target},
		},
	}
	history := SLOHistoryRecord{
		SLIValue: b.current,
		Budget:   b.budget,
	}
	return slo, history
}

// LoadFromSLOCSV loads SLO data from the --save-raw-data CSV format.
// CSV format: slo_id,app,name,type,target,current,budget,violated,event_count
func LoadFromSLOCSV(path string) (*Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening SLO CSV %s: %w", path, err)
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading SLO CSV: %w", err)
	}

	ds := NewDataset()
	base := time.Now().UTC().Truncate(24 * time.Hour)

	for i, row := range rows[1:] {
		if len(row) < 9 {
			continue
		}
		sloID := row[0]
		app := row[1]
		name := row[2]
		sloType := row[3]
		target, _ := strconv.ParseFloat(row[4], 64)
		current, _ := strconv.ParseFloat(row[5], 64)
		budget, _ := strconv.ParseFloat(row[6], 64)
		eventCount, _ := strconv.Atoi(row[8])

		tags := []string{"team:platform"}
		if app != "" {
			tags = append(tags, "app:"+app)
		}

		ds.SLOs = append(ds.SLOs, datadog.SLOData{
			ID:   sloID,
			Name: name,
			Type: sloType,
			Tags: tags,
			Thresholds: []datadog.SLOThreshold{
				{Timeframe: "7d", Target: target},
			},
		})
		ds.SLOHistory[sloID] = SLOHistoryRecord{SLIValue: current, Budget: budget}

		// Synthesize SLO events based on event_count
		for j := range eventCount {
			ts := base.Add(-time.Duration(i*24+j) * time.Hour)
			ds.SLOEvents = append(ds.SLOEvents, SLOEventRecord{
				ID:        fmt.Sprintf("slo-event-%s-%d", sloID, j),
				SLOID:     sloID,
				Title:     fmt.Sprintf("SLO violated: %s", name),
				Timestamp: ts,
				Tags:      append([]string{"slo_id:" + sloID}, tags...),
			})
		}
	}

	return ds, nil
}
