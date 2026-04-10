# GitHub Deployment Frequency

Deployment frequency measures how often your team ships to production. `em` derives this from GitHub Actions workflow run history — no instrumentation or third-party integrations required.

## What counts as a deployment

A deployment is a **successful GitHub Actions workflow run** (`conclusion == "success"`) for a workflow file that you have configured as a deployment workflow. Failed runs, cancelled runs, and skipped runs are not counted.

You configure which workflows represent deployments per team:

```bash
em metrics github config
```

This lets you map one or more repositories to one or more workflow filenames. For example, a team might count successful runs of `deploy-production.yml` in their API repo and `release.yml` in their frontend repo.

## Aggregation

Deployments are counted in **7-day buckets** anchored backward from the report end date (matching the throughput period logic). This means the most recent bucket ends today, and older buckets represent complete calendar weeks.

Output includes:
- Total successful workflow runs in the date range
- Deployments per week (total ÷ number of weeks)
- Per-week breakdown for the trend chart

## DORA context

Deployment frequency is one of the four DORA metrics. The industry benchmarks for elite performing teams is multiple deployments per day; high performers deploy at least once per week. The weekly view in `em` makes it easy to spot regressions (a sudden drop in deploy frequency often signals a process problem or a difficult release period).

## Configuration

| Config key | Description |
|---|---|
| `github.org` | GitHub organization name |
| `teams.<name>.github.slug` | Team slug (used to scope queries) |
| `teams.<name>.github.workflows` | Map of `repo → [workflow filenames]` |

The GitHub API token is stored in the system keychain under the `em` namespace. It requires at least `repo` scope to read Actions workflow run history.
