# devctl-em

CLI tools for engineering managers to generate JIRA agile metrics reports.

## Features

- **Cycle Time Analysis** - Scatter plots and statistics for issue completion times
- **Throughput Metrics** - Track team delivery velocity over time
- **Monte Carlo Forecasting** - Probabilistic completion predictions for epics
- **HTML Reports** - Comprehensive dashboards with all metrics

## Installation

```sh
curl -sSL https://raw.githubusercontent.com/danlafeir/devctl-em/main/scripts/install.sh | sh
```

This script will detect your OS and architecture, download the latest pre-built binary, and install it to `~/.local/bin`. Ensure `~/.local/bin` is in your PATH.

## Requirements

- Go 1.21+ (for building from source)
- JIRA Cloud instance with API access

## Building

```bash
make build          # Build for current platform
make build-all      # Cross-compile for all supported OS/ARCH
make install        # Install to ~/.local/bin
make test           # Run all tests
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

### Project Scoping (Optional)

Set a default project to automatically scope all metrics to child issues of active (unresolved) epics:

```bash
devctl-em config set jira.project MYPROJ
```

With this set, you no longer need to pass `--jql` to every command:

```bash
# These just work — scoped to active epics in MYPROJ
devctl-em metrics jira cycle-time
devctl-em metrics jira forecast
devctl-em metrics jira report -o report.html
```

JQL resolution order: `--jql` flag > `jira.default_jql` config > `jira.project` config.

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
