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

At least one data source must be configured:

- **JIRA Cloud** — API token with read access to your projects
- **GitHub** — personal access token with `repo` scope; GitHub Actions workflows used for deployment frequency
- **Snyk** — API token and org ID from your Snyk account

The binary runs on macOS and Linux (amd64/arm64). No runtime dependencies.

## Build and Test

```bash
make build          # Build for current platform
make build-all      # Cross-compile for all supported OS/ARCH
make install        # Install to ~/.local/bin
make test           # Run all tests
```

## Configuration

Each data source has an interactive setup command that walks you through the required values and stores credentials securely in the system keychain.

### JIRA

```bash
em metrics jira config
```

- **Domain** — your Atlassian subdomain (e.g. `mycompany` for `mycompany.atlassian.net`)
- **Email** — the email address associated with your Atlassian account
- **API token** — a personal API token from [id.atlassian.com/manage-profile/security/api-tokens](https://id.atlassian.com/manage-profile/security/api-tokens); stored in the system keychain
- **Project / JQL filter** — the JIRA project key or a JQL query that scopes all metrics to your team's issues; saved per team. JQL resolution order: `--jql` flag > `jira.jql_filter_for_metrics` config > `jira.project` config
- **Work threads** — number of parallel API requests when fetching issue histories (default: 4)
- **Workflow stages** — maps your JIRA status names to workflow stages (Backlog, In Progress, Review, Done, etc.) for cycle time calculation; uses sensible defaults if not set
- **Cycle time boundaries** — which stage name marks the start and end of cycle time measurement

### GitHub

```bash
em metrics github config
```

- **Organization** — your GitHub organization name
- **API token** — a personal access token with `repo` scope; stored in the system keychain
- **Team slug** — the team name used to look up deployment workflows
- **Workflows** — one or more GitHub Actions workflow names whose successful runs count as deployments for frequency tracking

### Snyk

```bash
em metrics snyk config
```

- **Site** — Snyk API hostname (default: `api.snyk.io`; change for EU/AU tenants)
- **API token** — a Snyk personal or service account token; stored in the system keychain
- **Organization** — the Snyk org ID and display name to pull vulnerability data from

