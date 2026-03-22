# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`devctl-em` is a Go CLI tool for engineering managers that generates JIRA agile metrics (cycle time, throughput, Monte Carlo forecasting) and combined PNG reports. Built with Cobra.

## Build & Test Commands

```bash
make build          # Build for current platform → bin/devctl-em-<os>-<arch>-<hash>
make build-all      # Cross-compile (linux/darwin × amd64/arm64)
make test           # Run all tests
make clean          # Remove build artifacts
go test ./pkg/...   # Test specific packages
go test -run TestName ./pkg/metrics/  # Run a single test
```

## Architecture

**Data flow**: JIRA API → status transitions from changelog → workflow stage mapping → metric calculations → visualization/export

### Key packages

- **`pkg/jira`** — JIRA Cloud REST API client with basic auth, rate limiting (exponential backoff), and pagination. Issues are fetched with full changelog to extract status transitions.
- **`pkg/workflow`** — Maps JIRA status names to workflow stages (Backlog, Analysis, In Progress, Review, Testing, Done). Stage definitions are user-configurable.
- **`pkg/metrics`** — Pure calculation logic: cycle time statistics (percentile-based), throughput aggregation, Monte Carlo simulation. No I/O.
- **`pkg/charts`** — Generates PNG/SVG visualizations using gonum/plot. Includes `CombinedReport` for single-PNG multi-panel output.
- **`pkg/export`** — CSV and Excel (.xlsx) export.
- **`cmd/metrics`** — CLI command handlers that wire together the above packages. `jira.go` contains shared config helpers (`getJiraClient`, `getConfigString`).

### Configuration

Config lives under the `em` namespace in `~/.devctl/config.yaml` (managed by the external `github.com/danlafeir/devctl` package). Sensitive values like `api_token` are stored in the system keychain via devctl's secrets package, with fallback to `JIRA_API_TOKEN` env var.

### Command structure

```
devctl-em
├── config (get/set/delete/list)
├── metrics jira (cycle-time|throughput|forecast|report)
└── update
```

Common flags across metrics commands: `--jql`, `--from`/`--to` (YYYY-MM-DD), `-o`/`--output`, `-f`/`--format`.

## Go Module

Module path: `devctl-em`, Go 1.24.3. Key dependencies: `github.com/danlafeir/devctl` (config/secrets/update), `spf13/cobra`, `gonum.org/v1/plot`, `xuri/excelize/v2`.

## Testing Guidelines

Use tests to indicate when we are breaking existing functionality. If that happens, prompt the user to ensure we are doing the right thing by changing the behavior intentionally.
