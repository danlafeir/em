# Monte Carlo Forecasting

Monte Carlo forecasting answers "when will this be done?" by running thousands of simulated futures using the team's real historical delivery pace. Instead of a single point estimate, it produces a probability distribution: you might be 50% likely to finish by June 15, and 85% likely by July 3.

## How it works

**Input:**
- Historical weekly throughput (number of items completed per week over the lookback window)
- Number of remaining items in the epic or scope
- Number of parallel work threads (default: 1)

**One trial:**

1. Start from today with all remaining items still to be done.
2. Pick a random week from the historical throughput data (excluding zero-throughput weeks).
3. Multiply the sampled count by the work threads setting.
4. Subtract that week's simulated output from remaining.
5. Advance the date by 7 days.
6. Repeat until remaining ≤ 0. Record the completion date.

**Full simulation:**

Run 10,000 trials (configurable with `--trials`). Sort the completion dates. The percentile values give you the probability distribution:

| Percentile | Interpretation |
|---|---|
| p50 | 50% of simulated futures finish by this date |
| p70 | 70% of simulated futures finish by this date |
| p85 | 85% of simulated futures finish by this date — recommended planning target |
| p95 | 95% of simulated futures finish by this date |

## Parallel work threads

When a team works multiple items simultaneously, throughput effectively scales. The `work_threads` setting (configured per team) multiplies the sampled weekly throughput in each simulated week.

The multiplier is not applied uniformly across percentiles — pessimistic scenarios use fewer threads:

| Percentile | Effective threads |
|---|---|
| p50 | Full `work_threads` |
| p70 | ¾ × `work_threads` (floor) |
| p85 | ½ × `work_threads` (ceiling) |
| p95 | 1 (fully sequential) |

This models the intuition that optimistic outcomes assume full parallel capacity, while pessimistic outcomes assume things will be more sequential than planned.

## Zero-throughput weeks

Weeks with zero completions are **excluded from random sampling** but are still included when calculating average throughput statistics. The rationale: a holiday week or all-hands sprint is an unusual event, not a reliable predictor of future pace. Drawing from only productive weeks keeps the simulation grounded in realistic delivery capacity.

## Epic forecasting

When forecasting multiple epics, two modes are available:

**Independent** — each epic is simulated separately using the same throughput distribution. Good when epics can be worked in any order or in parallel across sub-teams.

**Sequential** — epics are simulated in priority order. The completion date of epic N becomes the start date of epic N+1. This models a single team working through a backlog in sequence and gives more conservative (realistic) dates when work truly flows one epic at a time.

Sequential mode is used automatically when epic priorities are configured via `em metrics jira config`.

## Deadline confidence

If a deadline is configured (`--deadline` flag or `montecarlo.deadline` config key), the output includes a **deadline confidence** percentage: the fraction of simulated futures that complete before the deadline.

## Configuration

| Setting | Default | Description |
|---|---|---|
| `--trials` | 10,000 | Number of simulated futures |
| `--history-days` | 120 | Days of throughput history to sample from |
| `--deadline` | none | Target date for confidence calculation |
| `teams.<name>.jira.work_threads` | 1 | Parallel work threads per team |
| `montecarlo.deadline` | none | Persistent deadline (set via config file) |

## Interpreting results

The p85 date is a good planning commitment — it means 85 out of 100 simulated futures finish by then. Using p50 systematically underestimates because it assumes everything goes exactly as fast as the median historical week.

A wide gap between p50 and p95 indicates high throughput variance. This usually means the forecast is unreliable and the team should look at what's causing inconsistency before committing to dates.
