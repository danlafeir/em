package metrics

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"devctl-em/internal/charts"
	pkgmetrics "devctl-em/internal/metrics"
	"devctl-em/internal/output"
	snykpkg "devctl-em/internal/snyk"
)

// useSavedDataFlag skips upstream API calls and loads from previously saved CSVs.
var useSavedDataFlag bool

// ---- path helpers ----

func savedGithubDataPath(team string) string {
	return output.Path(teamOutputName("github-deployment-data", team) + ".csv")
}

func savedJiraCycleTimePath(team string) string {
	return output.Path(teamOutputName("jira-cycle-time-data", team) + ".csv")
}

func savedJiraThroughputPath(team string) string {
	return output.Path(teamOutputName("jira-throughput-data", team) + ".csv")
}

func savedJiraForecastPath(team string) string {
	return output.Path(teamOutputName("jira-forecast-data", team) + ".csv")
}

func savedSnykIssuesPath() string     { return output.Path("snyk-issues-data.csv") }
func savedSnykResolvedPath() string   { return output.Path("snyk-resolved-data.csv") }
func savedSnykOpenCountsPath() string { return output.Path("snyk-open-counts.csv") }

// ---- generic throughput CSV (shared by GitHub and JIRA throughput) ----

func saveThroughputCSV(result pkgmetrics.ThroughputResult, path string) error {
	f, err := output.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"period_start", "period_end", "count"}); err != nil {
		return err
	}
	for _, p := range result.Periods {
		if err := w.Write([]string{
			p.PeriodStart.Format(time.RFC3339),
			p.PeriodEnd.Format(time.RFC3339),
			strconv.Itoa(p.Count),
		}); err != nil {
			return err
		}
	}
	return w.Error()
}

func loadThroughputCSV(path string) (pkgmetrics.ThroughputResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return pkgmetrics.ThroughputResult{}, fmt.Errorf("no saved data at %s: %w", path, err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return pkgmetrics.ThroughputResult{}, err
	}
	result := pkgmetrics.ThroughputResult{Frequency: pkgmetrics.FrequencyWeekly}
	for _, row := range rows[1:] {
		if len(row) < 3 {
			continue
		}
		start, err1 := time.Parse(time.RFC3339, row[0])
		end, err2 := time.Parse(time.RFC3339, row[1])
		count, err3 := strconv.Atoi(row[2])
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		result.Periods = append(result.Periods, pkgmetrics.ThroughputPeriod{
			PeriodStart: start,
			PeriodEnd:   end,
			Count:       count,
		})
		result.TotalCount += count
	}
	if len(result.Periods) > 0 {
		result.AvgCount = float64(result.TotalCount) / float64(len(result.Periods))
	}
	return result, nil
}

// ---- GitHub deployment data ----

func saveDeploymentData(result pkgmetrics.ThroughputResult, team string) error {
	return saveThroughputCSV(result, savedGithubDataPath(team))
}

func loadDeploymentData(team string) (pkgmetrics.ThroughputResult, error) {
	r, err := loadThroughputCSV(savedGithubDataPath(team))
	if err != nil {
		return pkgmetrics.ThroughputResult{}, fmt.Errorf("no saved GitHub deployment data: %w", err)
	}
	return r, nil
}

// ---- JIRA cycle time data ----
// CSV: key,type,summary,cycle_time_hours,start_date,end_date,is_outlier

func saveJiraCycleTimeData(results []pkgmetrics.CycleTimeResult, outlierKeys map[string]bool, team string) error {
	f, err := output.Create(savedJiraCycleTimePath(team))
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"key", "type", "summary", "cycle_time_hours", "start_date", "end_date", "is_outlier"}); err != nil {
		return err
	}
	for _, r := range results {
		if err := w.Write([]string{
			r.IssueKey,
			r.IssueType,
			r.Summary,
			strconv.FormatFloat(r.CycleTime.Hours(), 'f', 4, 64),
			r.StartDate.Format(time.RFC3339),
			r.EndDate.Format(time.RFC3339),
			strconv.FormatBool(outlierKeys[r.IssueKey]),
		}); err != nil {
			return err
		}
	}
	return w.Error()
}

type loadedCycleTimeData struct {
	all         []pkgmetrics.CycleTimeResult
	kept        []pkgmetrics.CycleTimeResult
	outlierKeys map[string]bool
}

func loadJiraCycleTimeData(team string) (loadedCycleTimeData, error) {
	f, err := os.Open(savedJiraCycleTimePath(team))
	if err != nil {
		return loadedCycleTimeData{}, fmt.Errorf("no saved JIRA cycle time data: %w", err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return loadedCycleTimeData{}, err
	}
	var d loadedCycleTimeData
	d.outlierKeys = make(map[string]bool)
	for _, row := range rows[1:] {
		if len(row) < 7 {
			continue
		}
		hours, err := strconv.ParseFloat(row[3], 64)
		if err != nil {
			continue
		}
		start, err1 := time.Parse(time.RFC3339, row[4])
		end, err2 := time.Parse(time.RFC3339, row[5])
		if err1 != nil || err2 != nil {
			continue
		}
		r := pkgmetrics.CycleTimeResult{
			IssueKey:  row[0],
			IssueType: row[1],
			Summary:   row[2],
			CycleTime: time.Duration(hours * float64(time.Hour)),
			StartDate: start,
			EndDate:   end,
		}
		isOutlier, _ := strconv.ParseBool(row[6])
		d.all = append(d.all, r)
		if isOutlier {
			d.outlierKeys[r.IssueKey] = true
		} else {
			d.kept = append(d.kept, r)
		}
	}
	return d, nil
}

// ---- JIRA throughput data ----

func saveJiraThroughputData(result pkgmetrics.ThroughputResult, team string) error {
	return saveThroughputCSV(result, savedJiraThroughputPath(team))
}

func loadJiraThroughputData(team string) (pkgmetrics.ThroughputResult, error) {
	r, err := loadThroughputCSV(savedJiraThroughputPath(team))
	if err != nil {
		return pkgmetrics.ThroughputResult{}, fmt.Errorf("no saved JIRA throughput data: %w", err)
	}
	return r, nil
}

// ---- JIRA forecast data ----
// CSV: key,summary,completed,total,forecast_50,forecast_85,forecast_95

func saveJiraForecastData(rows []charts.ForecastRow, team string) error {
	f, err := output.Create(savedJiraForecastPath(team))
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"key", "summary", "completed", "total", "forecast_50", "forecast_85", "forecast_95"}); err != nil {
		return err
	}
	for _, r := range rows {
		if err := w.Write([]string{
			r.EpicKey,
			r.Summary,
			strconv.Itoa(r.Completed),
			strconv.Itoa(r.Total),
			r.Forecast50,
			r.Forecast85,
			r.Forecast95,
		}); err != nil {
			return err
		}
	}
	return w.Error()
}

func loadJiraForecastData(team string) ([]charts.ForecastRow, error) {
	f, err := os.Open(savedJiraForecastPath(team))
	if err != nil {
		return nil, nil // forecast is optional — no error if missing
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	var result []charts.ForecastRow
	for _, row := range rows[1:] {
		if len(row) < 7 {
			continue
		}
		completed, _ := strconv.Atoi(row[2])
		total, _ := strconv.Atoi(row[3])
		result = append(result, charts.ForecastRow{
			EpicKey:    row[0],
			Summary:    row[1],
			Completed:  completed,
			Total:      total,
			Remaining:  total - completed,
			Forecast50: row[4],
			Forecast85: row[5],
			Forecast95: row[6],
		})
	}
	return result, nil
}

// ---- Snyk issues CSV (shared schema for issues and resolved) ----
// CSV: id,title,severity,type,status,is_fixable,is_ignored,created_at,resolved_at

func saveSnykIssueList(issues []snykpkg.Issue, path string) error {
	f, err := output.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"id", "title", "severity", "type", "status", "is_fixable", "is_ignored", "created_at", "resolved_at"}); err != nil {
		return err
	}
	for _, i := range issues {
		if err := w.Write([]string{
			i.ID,
			i.Title,
			i.Severity,
			i.IssueType,
			i.Status,
			strconv.FormatBool(i.IsFixable),
			strconv.FormatBool(i.IsIgnored),
			i.CreatedAt.Format(time.RFC3339),
			i.ResolvedAt.Format(time.RFC3339),
		}); err != nil {
			return err
		}
	}
	return w.Error()
}

func loadSnykIssueList(path string) ([]snykpkg.Issue, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("no saved data at %s: %w", path, err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	var issues []snykpkg.Issue
	for _, row := range rows[1:] {
		if len(row) < 9 {
			continue
		}
		isFixable, _ := strconv.ParseBool(row[5])
		isIgnored, _ := strconv.ParseBool(row[6])
		createdAt, _ := time.Parse(time.RFC3339, row[7])
		resolvedAt, _ := time.Parse(time.RFC3339, row[8])
		issues = append(issues, snykpkg.Issue{
			ID:         row[0],
			Title:      row[1],
			Severity:   row[2],
			IssueType:  row[3],
			Status:     row[4],
			IsFixable:  isFixable,
			IsIgnored:  isIgnored,
			CreatedAt:  createdAt,
			ResolvedAt: resolvedAt,
		})
	}
	return issues, nil
}

// ---- Snyk open counts ----
// CSV: total,fixable,unfixable,ignored_fixable,ignored_unfixable,critical,high,medium,low

func saveSnykOpenCounts(counts snykpkg.OpenCounts) error {
	f, err := output.Create(savedSnykOpenCountsPath())
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	_ = w.Write([]string{"total", "fixable", "unfixable", "ignored_fixable", "ignored_unfixable", "critical", "high", "medium", "low"})
	err = w.Write([]string{
		strconv.Itoa(counts.Total),
		strconv.Itoa(counts.Fixable),
		strconv.Itoa(counts.Unfixable),
		strconv.Itoa(counts.IgnoredFixable),
		strconv.Itoa(counts.IgnoredUnfixable),
		strconv.Itoa(counts.Critical),
		strconv.Itoa(counts.High),
		strconv.Itoa(counts.Medium),
		strconv.Itoa(counts.Low),
	})
	if err != nil {
		return err
	}
	return w.Error()
}

func loadSnykOpenCounts() (snykpkg.OpenCounts, error) {
	f, err := os.Open(savedSnykOpenCountsPath())
	if err != nil {
		return snykpkg.OpenCounts{}, fmt.Errorf("no saved Snyk open counts: %w", err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return snykpkg.OpenCounts{}, err
	}
	if len(rows) < 2 || len(rows[1]) < 9 {
		return snykpkg.OpenCounts{}, fmt.Errorf("invalid saved Snyk open counts")
	}
	row := rows[1]
	atoi := func(s string) int { v, _ := strconv.Atoi(s); return v }
	return snykpkg.OpenCounts{
		Total:            atoi(row[0]),
		Fixable:          atoi(row[1]),
		Unfixable:        atoi(row[2]),
		IgnoredFixable:   atoi(row[3]),
		IgnoredUnfixable: atoi(row[4]),
		Critical:         atoi(row[5]),
		High:             atoi(row[6]),
		Medium:           atoi(row[7]),
		Low:              atoi(row[8]),
	}, nil
}

// ---- Snyk combined fetch-or-load helper ----

// fetchOrLoadSnykData fetches from the Snyk API (saving results) or loads from saved CSVs.
// client may be nil when useSavedDataFlag is true.
func fetchOrLoadSnykData(ctx context.Context, client *snykpkg.Client, from, to time.Time) ([]snykpkg.Issue, []snykpkg.Issue, snykpkg.OpenCounts, error) {
	if useSavedDataFlag {
		issues, err := loadSnykIssueList(savedSnykIssuesPath())
		if err != nil {
			return nil, nil, snykpkg.OpenCounts{}, err
		}
		resolved, err := loadSnykIssueList(savedSnykResolvedPath())
		if err != nil {
			return nil, nil, snykpkg.OpenCounts{}, err
		}
		counts, err := loadSnykOpenCounts()
		if err != nil {
			return nil, nil, snykpkg.OpenCounts{}, err
		}
		return issues, resolved, counts, nil
	}

	issues, err := client.ListIssues(ctx, from, to)
	if err != nil {
		return nil, nil, snykpkg.OpenCounts{}, err
	}
	resolved, err := client.ListResolvedIssues(ctx, from, to)
	if err != nil {
		return nil, nil, snykpkg.OpenCounts{}, err
	}
	counts, err := client.CountOpenIssues(ctx)
	if err != nil {
		return nil, nil, snykpkg.OpenCounts{}, err
	}

	// Save best-effort — don't fail the report if saving fails.
	_ = saveSnykIssueList(issues, savedSnykIssuesPath())
	_ = saveSnykIssueList(resolved, savedSnykResolvedPath())
	_ = saveSnykOpenCounts(counts)

	return issues, resolved, counts, nil
}

// ---- buildLongestCTRows reconstructs the longest cycle time table from saved data ----

func buildLongestCTRows(allResults []pkgmetrics.CycleTimeResult, outlierKeys map[string]bool) []charts.LongestCycleTimeRow {
	sorted := make([]pkgmetrics.CycleTimeResult, 0, len(allResults))
	for _, r := range allResults {
		if r.IssueType != "Epic" {
			sorted = append(sorted, r)
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CycleTime > sorted[j].CycleTime
	})
	n := min(len(sorted), 5)
	var rows []charts.LongestCycleTimeRow
	for _, r := range sorted[:n] {
		rows = append(rows, charts.LongestCycleTimeRow{
			Key:       r.IssueKey,
			Summary:   r.Summary,
			Days:      fmt.Sprintf("%.1f", r.CycleTimeDays()),
			Started:   r.StartDate.Format("Jan 02"),
			Completed: r.EndDate.Format("Jan 02"),
			Outlier:   outlierKeys[r.IssueKey],
		})
	}
	return rows
}
