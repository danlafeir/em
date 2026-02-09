# devctl-em

CLI tools for engineering managers to generate JIRA agile metrics reports.

## Features

- **Cycle Time Analysis** - Scatter plots and statistics for issue completion times
- **Throughput Metrics** - Track team delivery velocity over time
- **Cumulative Flow Diagram (CFD)** - Visualize work distribution across stages
- **WIP Aging** - Identify stale items and bottlenecks
- **Monte Carlo Forecasting** - Probabilistic completion predictions for epics
- **Burn-up Charts** - Track progress with forecast confidence bands
- **HTML Reports** - Comprehensive dashboards with all metrics

## Requirements

- Go 1.21+
- JIRA Cloud instance with API access

## Building

```bash
# Clone the repository
git clone https://github.com/yourusername/devctl-em.git
cd devctl-em

# Download dependencies
go mod download

# Build the binary
go build -o devctl-em .

# Or install to $GOPATH/bin
go install .
```

## Testing

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests with coverage
go test -cover ./...
```

## Configuration

### JIRA Connection

Set up your JIRA Cloud credentials:

```bash
# Set your Atlassian domain (e.g., "mycompany" for mycompany.atlassian.net)
devctl-em config set jira.domain mycompany

# Set your email
devctl-em config set jira.email user@company.com

# Set API token via environment variable
export JIRA_API_TOKEN=your_api_token_here
```

To generate an API token:
1. Go to https://id.atlassian.com/manage-profile/security/api-tokens
2. Click "Create API token"
3. Copy the token and set it as `JIRA_API_TOKEN`

### Workflow Mapping (Optional)

Create `~/.devctl-em/config.yaml` to customize workflow stage mapping:

```yaml
jira:
  domain: "mycompany"
  email: "user@company.com"
  default_jql: "project = MYPROJ"
  story_points_field: "customfield_10026"

workflow:
  stages:
    - name: "Backlog"
      statuses: ["Open", "To Do", "Backlog"]
    - name: "In Progress"
      statuses: ["In Development", "In Progress"]
    - name: "Review"
      statuses: ["In Review", "Code Review"]
    - name: "Done"
      statuses: ["Done", "Closed", "Resolved"]
  cycle_time:
    started: "In Progress"
    completed: "Done"
```

## Usage

All JIRA metrics commands are under `devctl-em metrics jira`.

### Common Flags

| Flag | Description |
|------|-------------|
| `--jql` | JQL query to filter issues |
| `--from` | Start date (YYYY-MM-DD), default: 90 days ago |
| `--to` | End date (YYYY-MM-DD), default: today |
| `-o, --output` | Output file path |
| `-f, --format` | Output format: png, csv, xlsx, html |

### Cycle Time

Analyze how long issues take from start to completion:

```bash
# Generate cycle time scatter plot
devctl-em metrics jira cycle-time --jql "project = MYPROJ"

# Specify date range and output
devctl-em metrics jira cycle-time \
  --jql "project = MYPROJ AND type = Story" \
  --from 2024-01-01 \
  --to 2024-06-30 \
  -o cycle-time.png
```

### Throughput

Track team delivery velocity:

```bash
# Weekly throughput chart
devctl-em metrics jira throughput --jql "project = MYPROJ"

# Daily frequency with CSV export
devctl-em metrics jira throughput \
  --jql "project = MYPROJ" \
  --frequency daily \
  -f csv \
  -o throughput.csv
```

Frequency options: `daily`, `weekly`, `biweekly`, `monthly`

### Cumulative Flow Diagram

Visualize work distribution across workflow stages:

```bash
devctl-em metrics jira cfd --jql "project = MYPROJ"
```

### WIP Aging

Identify stale work items:

```bash
# Show current WIP with aging analysis
devctl-em metrics jira wip --jql "project = MYPROJ"

# Custom thresholds (days)
devctl-em metrics jira wip \
  --jql "project = MYPROJ" \
  --warning 7 \
  --critical 14
```

### Monte Carlo Forecast

Predict epic completion dates using Monte Carlo simulation:

```bash
# Forecast all open epics in your default project
devctl-em metrics jira forecast

# Forecast a specific epic
devctl-em metrics jira forecast --epic MYPROJ-123

# Forecast with a deadline
devctl-em metrics jira forecast --epic MYPROJ-123 --deadline 2024-12-31

# Forecast arbitrary remaining items
devctl-em metrics jira forecast --remaining 25
```

Output includes probability distribution:
- **50th percentile** - 50% chance of completion by this date
- **85th percentile** - 85% chance (common planning target)
- **95th percentile** - 95% chance (conservative estimate)

### Burn-up Chart

Track progress with forecast confidence bands:

```bash
devctl-em metrics jira burnup --epic MYPROJ-123
```

### Comprehensive Report

Generate an HTML report with all metrics:

```bash
devctl-em metrics jira report --jql "project = MYPROJ" -o report.html
```

This creates:
- `report.html` - Interactive dashboard
- `cycle-time-scatter.png` - Cycle time chart
- `throughput-trend.png` - Throughput chart
- `cycle-time-data.csv` - Raw cycle time data
- `throughput-data.csv` - Raw throughput data

## Examples

### Sprint Retrospective Report

```bash
devctl-em metrics jira report \
  --jql "project = MYPROJ AND sprint = 'Sprint 42'" \
  --title "Sprint 42 Metrics" \
  -o sprint-42-report.html
```

### Epic Progress Tracking

```bash
# Get forecast for all epics
devctl-em metrics jira forecast --jql "project = MYPROJ"

# Export to CSV for stakeholder reporting
devctl-em metrics jira forecast \
  --jql "project = MYPROJ" \
  -o epic-forecasts.csv
```

### Team Health Check

```bash
# Check for aging work items
devctl-em metrics jira wip \
  --jql "project = MYPROJ AND assignee in membersOf('my-team')"

# Analyze cycle time trends
devctl-em metrics jira cycle-time \
  --jql "project = MYPROJ" \
  --from 2024-01-01 \
  -f csv \
  -o cycle-times.csv
```

## Output Formats

| Format | Extension | Description |
|--------|-----------|-------------|
| PNG | `.png` | Chart images |
| CSV | `.csv` | Raw data for spreadsheets |
| Excel | `.xlsx` | Formatted workbooks with multiple sheets |
| HTML | `.html` | Self-contained reports with styling |

## License

MIT
