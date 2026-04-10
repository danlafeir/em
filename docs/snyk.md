# Snyk Vulnerability Metrics

`em` pulls open vulnerability data from Snyk and presents it in a way that's useful for an engineering manager: total counts by severity, which issues are actually fixable, how many are being ignored, and whether the backlog is growing or shrinking week over week.

## What is counted

Every open Snyk issue is categorized along three dimensions:

**Severity**
- Critical
- High
- Medium
- Low

**Fixability** — whether there is a known fix available
- Fixable: a remediation exists (upgrade, patch, or Snyk-suggested fix)
- Unfixable: no known remediation

**Status**
- Open: actively tracked
- Ignored: suppressed in Snyk (still counted separately so they do not disappear)

**Exploitability** — issues with maturity level `Proof of Concept`, `Functional`, or `High` are flagged as exploitable. These are the issues most likely to be targeted in the wild.

## Deduplication

Snyk can report the same vulnerability across multiple targets (e.g., the same library vulnerability found in several repos). `em` deduplicates open issues by `(target_id, title, severity)` before counting, so a single vulnerability affecting five repos counts as one issue rather than five. This gives a more accurate picture of distinct security problems rather than raw noise volume.

## Weekly trend

The trend chart shows how the total open vulnerability count has changed each week. This is reconstructed from historical data:

1. Start from the **current open count** (a known exact number from the Snyk API).
2. Walk backward week by week through the date range.
3. For each week: `count_at_end_of_previous_week = count_at_end_of_this_week - issues_created_this_week + issues_resolved_this_week`

This backward reconstruction means the most recent data point is always accurate, and older points are derived from the net change history.

## Configuration

| Config key | Description |
|---|---|
| `snyk.org_id` | Snyk organization ID (required) |
| `snyk.org_name` | Display name for the organization |
| `snyk.site` | API hostname — default `api.snyk.io`; use `api.eu.snyk.io` or `api.au.snyk.io` for regional tenants |
| `snyk.team` | Team label used in report output |

The Snyk API token is stored in the system keychain. It requires read access to issues in the configured organization.

## Reading the output

A healthy security posture typically looks like:
- **Fixable count trending down** — the team is remediating known-fix issues over time
- **Exploitable count near zero** — high-maturity exploits are being prioritized
- **Ignored count stable or declining** — suppressions are being reviewed, not accumulated

A growing fixable count that the team is not addressing is a risk signal worth raising in your next review cycle.
