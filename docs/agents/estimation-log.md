# Estimation log

Per-issue estimate + actual for the DixieData project's
calibration loop. Populated on issue close (the agent that
ships the final slice appends the row with actuals from
`git log <issue-#> --oneline` slice commits, counting working
days from first to last commit that references the issue
number). See `docs/agents/issue-tracker.md` §"Estimate" for
the PERT formula + confidence convention.

## Schema

| Column | Definition |
|---|---|
| Issue | GitHub issue number (`#N`) |
| Title | Issue title at close time |
| Estimate (P) | PERT weighted mean `(O + 4A + N) / 6` in days |
| Confidence | low / medium / high (per the issue header) |
| Actual | Working days from first slice commit to last slice commit (rounded to 0.5d) |
| Δ | Actual − Estimate (positive = slipped; negative = ahead) |
| %Δ | (Actual − Estimate) / Estimate × 100 |
| Notes | Slice count, multi-PR vs single-PR, anything that explains the Δ |

## Calibration goal

After 10 closed issues with both estimate and actual, compute
mean absolute percentage error (MAPE). Target: **MAPE < 30%
at 90 days** (per the locked decision in the 2026-07
pragmatic-programmer audit, Estimation row).

## Log

| Issue | Title | Estimate (P) | Confidence | Actual | Δ | %Δ | Notes |
|---|---|---|---|---|---|---|---|
| #612 | Articles image support | 8d | medium | TBD | TBD | TBD | shipped via 10-slice chain |
| #613 | Repository seam | 2.2d | high | TBD | TBD | TBD | 7 slices shipped; full service refactor deferred |
| #617 | ADR Author + 0003 dedup | 1.0d | high | 1d | 0d | 0% | single PR; bake bug fix piggybacked |
| #620 | Glossary sync test | 2h | high | 1h | -1h | -50% | clean RED→GREEN, first-run caught real drift |

## How to append a row

1. On issue close, run `gh issue view <N> --json body -q '.body'`
   and extract the `## Estimate` block.
2. Compute Actual: `git log <first-slice-sha>..<last-slice-sha> --oneline | wc -l` and divide by 2 (assumption: 2 commits per working day is a reasonable proxy until the team calibrates the actual commit/day ratio).
3. Add a row to the table above. Keep it terse — the Notes column is for the slice count + anything that explains the Δ.
4. Update the MAPE rolling average at the bottom of the file once 10+ rows exist.

## Audit

`audit/probe_issue_estimates.mjs` (added by the slice that
ships this convention) lists every open issue missing the
`## Estimate` header. Run nightly in CI; flag the offending
issues with `needs-info`.

## Author

Jeremy Morris (@jeremymorris) — 2026-07-17
