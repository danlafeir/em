package export

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/danlafeir/em/pkg/metrics"
)

// helpers

func makeCycleTimeResults() []metrics.CycleTimeResult {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 11, 0, 0, 0, 0, time.UTC)
	return []metrics.CycleTimeResult{
		{
			IssueKey:  "PROJ-1",
			IssueType: "Story",
			Summary:   "First issue",
			CycleTime: 10 * 24 * time.Hour,
			StartDate: start,
			EndDate:   end,
		},
		{
			IssueKey:  "PROJ-2",
			IssueType: "Bug",
			Summary:   "Second issue",
			CycleTime: 5 * 24 * time.Hour,
			StartDate: start,
			EndDate:   time.Date(2024, 1, 6, 0, 0, 0, 0, time.UTC),
		},
	}
}

func makeCycleTimeStats() metrics.CycleTimeStats {
	return metrics.CycleTimeStats{
		Count:        2,
		Mean:         7 * 24 * time.Hour,
		Median:       7 * 24 * time.Hour,
		Percentile50: 7 * 24 * time.Hour,
		Percentile70: 9 * 24 * time.Hour,
		Percentile85: 10 * 24 * time.Hour,
		Percentile95: 10 * 24 * time.Hour,
		Min:          5 * 24 * time.Hour,
		Max:          10 * 24 * time.Hour,
	}
}

func makeThroughputResult() metrics.ThroughputResult {
	return metrics.ThroughputResult{
		Periods: []metrics.ThroughputPeriod{
			{
				PeriodStart: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				PeriodEnd:   time.Date(2024, 1, 7, 0, 0, 0, 0, time.UTC),
				Count:       3,
			},
			{
				PeriodStart: time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC),
				PeriodEnd:   time.Date(2024, 1, 14, 0, 0, 0, 0, time.UTC),
				Count:       5,
			},
		},
		TotalCount: 8,
		AvgCount:   4.0,
	}
}

// -- CycleTimeCSV -------------------------------------------------------------

func TestCycleTimeCSV_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cycle_time.csv")

	results := makeCycleTimeResults()
	if err := CycleTimeCSV(results, path); err != nil {
		t.Fatalf("CycleTimeCSV failed: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}

func TestCycleTimeCSV_HeaderRow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cycle_time.csv")

	if err := CycleTimeCSV(makeCycleTimeResults(), path); err != nil {
		t.Fatalf("CycleTimeCSV failed: %v", err)
	}

	f, _ := os.Open(path)
	defer f.Close()
	records, _ := csv.NewReader(f).ReadAll()

	if len(records) == 0 {
		t.Fatal("CSV is empty")
	}
	header := records[0]
	expected := []string{"Issue Key", "Type", "Summary", "Start Date", "End Date", "Cycle Time (days)"}
	if len(header) != len(expected) {
		t.Fatalf("expected %d header columns, got %d", len(expected), len(header))
	}
	for i, col := range expected {
		if header[i] != col {
			t.Errorf("header[%d]: expected %q, got %q", i, col, header[i])
		}
	}
}

func TestCycleTimeCSV_DataRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cycle_time.csv")

	results := makeCycleTimeResults()
	if err := CycleTimeCSV(results, path); err != nil {
		t.Fatalf("CycleTimeCSV failed: %v", err)
	}

	f, _ := os.Open(path)
	defer f.Close()
	records, _ := csv.NewReader(f).ReadAll()

	// 1 header + 2 data rows
	if len(records) != 3 {
		t.Fatalf("expected 3 rows (header + 2 data), got %d", len(records))
	}

	row1 := records[1]
	if row1[0] != "PROJ-1" {
		t.Errorf("expected issue key PROJ-1, got %q", row1[0])
	}
	if row1[1] != "Story" {
		t.Errorf("expected type Story, got %q", row1[1])
	}
	if row1[2] != "First issue" {
		t.Errorf("expected summary 'First issue', got %q", row1[2])
	}
	if row1[3] != "2024-01-01" {
		t.Errorf("expected start date 2024-01-01, got %q", row1[3])
	}
	if row1[4] != "2024-01-11" {
		t.Errorf("expected end date 2024-01-11, got %q", row1[4])
	}
	// Cycle time: 10 days
	ct, err := strconv.ParseFloat(row1[5], 64)
	if err != nil {
		t.Fatalf("cycle time not a float: %q", row1[5])
	}
	if ct != 10.0 {
		t.Errorf("expected cycle time 10.0, got %f", ct)
	}
}

func TestCycleTimeCSV_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "dir", "cycle_time.csv")

	if err := CycleTimeCSV(makeCycleTimeResults(), path); err != nil {
		t.Fatalf("CycleTimeCSV failed: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist after creating parent dirs: %v", err)
	}
}

func TestCycleTimeCSV_EmptyResults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.csv")

	if err := CycleTimeCSV(nil, path); err != nil {
		t.Fatalf("CycleTimeCSV with empty results failed: %v", err)
	}

	f, _ := os.Open(path)
	defer f.Close()
	records, _ := csv.NewReader(f).ReadAll()

	// Only header row
	if len(records) != 1 {
		t.Errorf("expected 1 row (header only), got %d", len(records))
	}
}

// -- ThroughputCSV ------------------------------------------------------------

func TestThroughputCSV_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "throughput.csv")

	if err := ThroughputCSV(makeThroughputResult(), path); err != nil {
		t.Fatalf("ThroughputCSV failed: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}

func TestThroughputCSV_HeaderRow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "throughput.csv")

	if err := ThroughputCSV(makeThroughputResult(), path); err != nil {
		t.Fatalf("ThroughputCSV failed: %v", err)
	}

	f, _ := os.Open(path)
	defer f.Close()
	records, _ := csv.NewReader(f).ReadAll()

	if len(records) == 0 {
		t.Fatal("CSV is empty")
	}
	header := records[0]
	expected := []string{"Period Start", "Period End", "Items Completed"}
	if len(header) != len(expected) {
		t.Fatalf("expected %d header columns, got %d", len(expected), len(header))
	}
	for i, col := range expected {
		if header[i] != col {
			t.Errorf("header[%d]: expected %q, got %q", i, col, header[i])
		}
	}
}

func TestThroughputCSV_DataRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "throughput.csv")

	result := makeThroughputResult()
	if err := ThroughputCSV(result, path); err != nil {
		t.Fatalf("ThroughputCSV failed: %v", err)
	}

	f, _ := os.Open(path)
	defer f.Close()
	records, _ := csv.NewReader(f).ReadAll()

	// 1 header + 2 periods
	if len(records) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(records))
	}

	row1 := records[1]
	if row1[0] != "2024-01-01" {
		t.Errorf("expected period start 2024-01-01, got %q", row1[0])
	}
	if row1[1] != "2024-01-07" {
		t.Errorf("expected period end 2024-01-07, got %q", row1[1])
	}
	if row1[2] != "3" {
		t.Errorf("expected count 3, got %q", row1[2])
	}

	row2 := records[2]
	if row2[2] != "5" {
		t.Errorf("expected count 5, got %q", row2[2])
	}
}

// -- CycleTimeExcel -----------------------------------------------------------

func TestCycleTimeExcel_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cycle_time.xlsx")

	if err := CycleTimeExcel(makeCycleTimeResults(), makeCycleTimeStats(), path); err != nil {
		t.Fatalf("CycleTimeExcel failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
	if info.Size() == 0 {
		t.Error("expected non-empty file")
	}
}

func TestCycleTimeExcel_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "cycle_time.xlsx")

	if err := CycleTimeExcel(makeCycleTimeResults(), makeCycleTimeStats(), path); err != nil {
		t.Fatalf("CycleTimeExcel failed: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist: %v", err)
	}
}

func TestCycleTimeExcel_EmptyResults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.xlsx")

	if err := CycleTimeExcel(nil, metrics.CycleTimeStats{}, path); err != nil {
		t.Fatalf("CycleTimeExcel with empty results failed: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist: %v", err)
	}
}

// -- ThroughputExcel ----------------------------------------------------------

func TestThroughputExcel_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "throughput.xlsx")

	if err := ThroughputExcel(makeThroughputResult(), path); err != nil {
		t.Fatalf("ThroughputExcel failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
	if info.Size() == 0 {
		t.Error("expected non-empty file")
	}
}

func TestThroughputExcel_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "throughput.xlsx")

	if err := ThroughputExcel(makeThroughputResult(), path); err != nil {
		t.Fatalf("ThroughputExcel failed: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist: %v", err)
	}
}

func TestThroughputExcel_EmptyPeriods(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.xlsx")

	result := metrics.ThroughputResult{}
	if err := ThroughputExcel(result, path); err != nil {
		t.Fatalf("ThroughputExcel with empty periods failed: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist: %v", err)
	}
}
