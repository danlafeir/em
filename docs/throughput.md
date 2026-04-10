# Throughput

Throughput counts how many issues your team completes per time period. It answers "how fast are we actually delivering?" without requiring story points or estimates — just the count of items that made it to done.

## What counts as completed

An issue is counted in a period if:

1. Its resolution date falls within that period's date range.
2. It has a changelog entry showing it transitioned into the started status at some point — issues closed without ever being worked on are excluded.

The same status boundaries used for cycle time apply here (`workflow.cycle_time.started` and `workflow.cycle_time.completed`).

## Aggregation periods

Throughput can be aggregated by four frequencies:

| Frequency | Period length | Anchoring |
|---|---|---|
| `daily` | 1 day | Forward from `--from` |
| `weekly` | 7 days | Backward from `--to` |
| `biweekly` | 14 days | Backward from `--to` |
| `monthly` | Calendar month | Forward from `--from` |

**Weekly and biweekly** periods are built backward from the end date and then reversed. This means the last bucket always ends on today (or the `--to` date), which is useful for "last N weeks through today" reporting. Daily and monthly periods start from the `--from` date and move forward.

## Statistical summary

| Metric | Description |
|---|---|
| Total items | Sum of all completions across all periods |
| Avg/period | Total ÷ number of periods |
| Median/period | 50th percentile of per-period counts |
| Min / Max | Lowest and highest single-period counts |

Unlike cycle time, throughput values are **not outlier-filtered**. Every period is included in the statistics, including slow weeks and holiday periods. This gives an honest view of actual delivery pace over time.

## Relationship to forecasting

Weekly throughput is the direct input to Monte Carlo simulation. The historical distribution of "how many items did we complete each week" is sampled randomly to model future delivery pace. See [monte-carlo.md](./monte-carlo.md) for details.

## Tips

- A **90-day window** (`--from` / `--to`) captures roughly 13 weeks of weekly data — enough for stable Monte Carlo sampling without pulling in work that predates a team restructure.
- If your team has had a significant change in size or process, shorten the history window so the forecast reflects the team as it is now.
- Zero-throughput weeks (holidays, all-hands, etc.) are included in period statistics but are excluded when Monte Carlo randomly samples past weeks — the simulation only draws from weeks where at least one item was completed.
