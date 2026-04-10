# Outlier Filtering

Cycle time distributions almost always contain a handful of tickets that took far longer than anything else — issues that were opened months ago, sat untouched, and were eventually closed. Including them would skew percentiles upward and compress the useful part of the chart into a thin band at the bottom.

`em` removes these outliers automatically before computing statistics or rendering charts. The method used is Tukey's fence, applied to the IQR (interquartile range).

## How it works

Given a list of cycle time values:

1. Sort the values.
2. Calculate **Q1** (25th percentile) and **Q3** (75th percentile).
3. **IQR** = Q3 − Q1
4. **Lower fence** = Q1 − 2.0 × IQR
5. **Upper fence** = Q3 + 2.0 × IQR
6. Any value outside [lower fence, upper fence] is removed.

The multiplier `2.0` is more permissive than the classic Tukey value of `1.5`. A tighter fence would remove too many legitimate slow tickets from a team with high variance; the wider fence targets only genuine outliers.

## Minimum sample size

Outlier filtering requires at least **4 data points**. With fewer than 4 issues, no filtering is applied — the sample is too small to compute a reliable IQR.

If IQR is zero (all values are identical), filtering is also skipped.

## What this affects

Outlier filtering is applied to **cycle time** only. Throughput is not filtered — every period's count is included as-is.

The removed issues are not shown in the scatter plot or included in any percentile calculation, but they are not deleted from JIRA. The filter is purely for display and statistics.

## Example

Suppose a team has these cycle times (in business days):

```
1, 2, 2, 3, 3, 4, 4, 5, 5, 47
```

- Q1 = 2, Q3 = 5
- IQR = 3
- Upper fence = 5 + 2.0 × 3 = 11
- The 47-day issue exceeds the upper fence and is removed.

The statistics and chart reflect the remaining 9 issues. The long-running ticket is still in JIRA; it just does not distort the team's typical cycle time picture.
