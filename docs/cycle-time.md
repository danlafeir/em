# Cycle Time

Cycle time measures how long it takes for a piece of work to move from started to done. `em` pulls this from JIRA changelog entries — the history of every status transition an issue went through — so no estimates or manual tracking is required.

## What gets measured

Cycle time starts when an issue first transitions into the configured **started status** (default: `In Progress`) and ends when it first transitions into the configured **done status** (default: `Closed`).

- If the done-status transition is never recorded in the changelog but the issue has a resolution date, the resolution date is used as the end time.
- If the started-status transition is never recorded, the issue is excluded entirely. This keeps the metric honest: an issue that was manually closed without ever being worked on should not be in your cycle time distribution.
- Only business days (Monday–Friday) are counted. Weekend time does not inflate the number.

## Business day calculation

Given a start timestamp and an end timestamp:

1. The start day is **not** counted.
2. Each subsequent calendar day through the end day is counted if it falls Monday–Friday.
3. An issue started on Friday and closed the following Monday = **1 business day**.
4. An issue started and closed on the same day = **0 business days**.

## Statistical summary

After calculating cycle time for every qualifying issue, `em` computes:

| Metric | Description |
|---|---|
| Count | Number of issues included |
| Mean | Average cycle time |
| Median (p50) | Half of issues completed faster than this |
| p70 | 70% of issues completed faster than this |
| p85 | 85% of issues completed faster than this — a common SLE target |
| p95 | 95% of issues completed faster than this |
| Min / Max | Extremes (after outlier removal) |
| Std Dev | Sample standard deviation |

## Outlier filtering

Raw cycle time distributions often contain a small number of very old tickets — issues that sat untouched for months before being closed — that would skew the percentiles and make the chart unreadable. `em` removes these using Tukey's IQR method before computing statistics or plotting.

See [outliers.md](./outliers.md) for how this works.

## Workflow configuration

The statuses used as cycle time boundaries are configurable per team:

```bash
em metrics jira config
```

This prompts for:
- **JIRA status when work starts** — the status transition that starts the clock (default: `In Progress`)
- **JIRA status when work is done** — the status transition that stops the clock (default: `Closed`)

The configured status becomes the primary match for cycle time boundary detection, but all other common done-statuses (`Done`, `Resolved`, `Complete`, `Released`) still count as completed for throughput and forecasting purposes. This prevents work items with slightly different terminal statuses from being dropped.

Config keys: `workflow.cycle_time.started`, `workflow.cycle_time.completed`

## What is excluded

- Epics — only leaf-level work items are measured
- Issues that never entered the started status
- Issues without a resolution date and no done-status transition
- Outliers (see [outliers.md](./outliers.md))
