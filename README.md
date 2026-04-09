# em

CLI tools for engineering managers to codify the mechanics of managing a team and prioritizing working on the hard stuff.

## Features

**JIRA Metrics**
- Cycle time analysis — scatter plot with percentile lines (50th/85th/95th), business-day calculation, IQR-based outlier filtering
- Throughput tracking — weekly/daily/biweekly/monthly delivery frequency with trend line
- Monte Carlo forecasting — probability-based completion dates for epics using historical throughput
- Longest cycle time table — highlights recently completed issues that took the most time
- Combined JIRA report — cycle time, throughput, forecasting, and longest CT in a single HTML file

**GitHub**
- Deployment frequency — track release cadence per team from GitHub Actions workflow runs

**Snyk**
- Open vulnerability tracking — counts by severity (critical/high/medium/low) and fix category (fixable/unfixable/ignored)
- Exploitability highlighting — flags issues with Proof of Concept maturity or higher
- Weekly trend chart — visualize how the vulnerability backlog changes over time
- Standalone Snyk security report

**Combined Engineering Report**
- Executive Healthcheck — at-a-glance summary of cycle time, throughput, active epics, deploy frequency, and Snyk vulnerability counts
- Aggregates JIRA, GitHub, and Snyk data into a single HTML report per team

**Multi-team support** — configure multiple teams; commands run per-team with separate output files

## Installation

```sh
curl -sSL https://raw.githubusercontent.com/danlafeir/em/main/scripts/install.sh | sh
```

This script will detect your OS and architecture, download the latest pre-built binary, and install it to `~/.local/bin`. Ensure `~/.local/bin` is in your PATH.

## Requirements

- Go 1.26+ (for building from source)
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
em config set jira.domain mycompany

# Set your email
em config set jira.email user@company.com

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
em config set jira.project MYPROJ
```

With this set, you no longer need to pass `--jql` to every command:

```bash
# These just work — scoped to active epics in MYPROJ
em metrics jira cycle-time
em metrics jira forecast
em metrics jira report
```

JQL resolution order: `--jql` flag > `jira.jql_filter_for_metrics` config > `jira.project` config.

### Workflow Mapping (Optional)

Create `~/.em/config.yaml` to customize workflow stage mapping:

```yaml
jira:
  domain: "mycompany"
  email: "user@company.com"
  jql_filter_for_metrics: "project = MYPROJ"

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

All JIRA metrics commands are under `em metrics jira`.

### Common Flags

| Flag | Description |
|------|-------------|
| `--jql` | JQL query to filter issues |
| `--from` | Start date (YYYY-MM-DD), default: 6 weeks ago |
| `--to` | End date (YYYY-MM-DD), default: today |
| `-o, --output` | Output file path |
| `-f, --format` | Output format: png, csv, xlsx, html |

### Cycle Time

Analyze how long issues take from start to completion:

```bash
# Generate cycle time scatter plot
em metrics jira cycle-time --jql "project = MYPROJ"

# Specify date range and output
em metrics jira cycle-time \
  --jql "project = MYPROJ AND type = Story" \
  --from 2024-01-01 \
  --to 2024-06-30 \
  -o cycle-time.png
```

### Throughput

Track team delivery velocity:

```bash
# Weekly throughput chart
em metrics jira throughput --jql "project = MYPROJ"

# Daily frequency with CSV export
em metrics jira throughput \
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
em metrics jira forecast

# Forecast a specific epic
em metrics jira forecast --epic MYPROJ-123

# Forecast with a deadline
em metrics jira forecast --epic MYPROJ-123 --deadline 2024-12-31

# Forecast arbitrary remaining items
em metrics jira forecast --remaining 25
```

Output includes probability distribution:
- **50th percentile** - 50% chance of completion by this date
- **85th percentile** - 85% chance (common planning target)
- **95th percentile** - 95% chance (conservative estimate)

### Combined Report

Generate a single HTML report combining cycle time, throughput, longest CT table, and epic forecast:

```bash
em metrics jira report
em metrics jira report --from 2024-01-01
```

This creates a `jira-report.html` file with all panels in one page.

## Examples

### Epic Progress Tracking

```bash
# Get forecast for all epics
em metrics jira forecast --jql "project = MYPROJ"

# Export to CSV for stakeholder reporting
em metrics jira forecast \
  --jql "project = MYPROJ" \
  -o epic-forecasts.csv
```

### Team Health Check

```bash
# Analyze cycle time trends
em metrics jira cycle-time \
  --jql "project = MYPROJ" \
  --from 2024-01-01 \
  -f csv \
  -o cycle-times.csv
```

## Output Formats

| Format | Extension | Description |
|--------|-----------|-------------|
| HTML | `.html` | Interactive charts and reports (default) |
| CSV | `.csv` | Raw data for spreadsheets |
| Excel | `.xlsx` | Formatted workbooks with multiple sheets |

## License

MIT
