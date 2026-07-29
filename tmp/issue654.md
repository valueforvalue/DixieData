## Summary
Memorial Archive import job reports must identify every skipped entry and explain its skip reason; import confirmation should support explicit approval of eligible skips.

## User story
As an archive researcher importing memorial evidence, I want to know which Person Records were skipped and why, so that I can verify import completeness and deliberately resolve or approve duplicate cases.

## Locked decisions
1. Job report must show more than aggregate skipped count (decided in user report on 2026-07-26).
2. Skip details must include entry identity and reason (decided in user report on 2026-07-26).
3. Whether an "approve skips" action may override an existing-memorial skip remains a UX/data-integrity decision; exact behavior is not locked.

## Current behavior
`internal/records/memorial_import.go` increments `WouldSkip`/`Skipped` then continues for duplicate memorial IDs within input or memorial IDs already present in Local Archive. `MemorialImportSummary.Skipped` stores only an integer, while `summary.Issues` records failures only. `internal/jobs/jobs.go` therefore renders only `Person records: N added, N skipped, N failed`; skip identity and reason are lost, and error log has no skip rows.

## Proposed UX
On Memorial Archive preview and completed job report, show skipped entry name, memorial ID, source Person Record/Display ID when available, and normalized reason (`duplicate in file` or `already in Local Archive`). Provide explicit review/approval controls only for skip classes that can be safely overridden; never silently create duplicate Source Records.

## Apply sites (v1 checklist)
- [ ] Memorial Archive import preview/confirmation job surface
- [ ] Completed Memorial Archive job report
- [ ] Downloadable import log/report
- [ ] CLI Memorial Archive preview/import output

## Glossary changes (if any)
None.

## Schema sketch (if any)
None expected. Final design must confirm whether approved overrides need durable import-decision state.

## Acceptance criteria
- [ ] Each skipped entry appears with name, memorial ID, and skip reason.
- [ ] Duplicate-in-file and already-in-Local-Archive skips are distinguishable.
- [ ] Preview skip details match completed-job skip details.
- [ ] Downloaded report includes skipped entries, not failures only.
- [ ] User can explicitly approve any safely overridable skip class, or UI clearly states why that class cannot be overridden.
- [ ] Existing Source Record uniqueness/data-integrity behavior remains protected.

## Slice plan
Needs `/skill:discover` then `/skill:plan`; approval semantics require product and integrity decisions.

## Test plan
- Unit: extend `internal/records/memorial_import_test.go` coverage for detailed skip outcomes and both reason classes.
- Handler/job: verify preview and completed `JobResult` preserve skip details.
- Smoke: assert job report renders skipped identity/reason and approval behavior.
- CLI: assert equivalent skip details appear in preview/import output.

## Files
- `internal/records/memorial_import.go`
- `internal/records/memorial_import_test.go`
- `internal/jobs/jobs.go`
- `internal/appshell/imports_handlers.go`
- `internal/appshell/cli_import.go`
- relevant job templates and audit probes, to identify during design
- `CHANGELOG.md`

## Regression net
- Import fixture containing one duplicate-in-file entry and one memorial ID already present in Local Archive.
- Assert both entries retain identity and distinct reasons through preview, import summary, job report, downloadable log, and CLI output.
- Assert approved behavior cannot silently violate existing memorial uniqueness rules.

## Estimate
PERT: O=1 day / A=2 days / N=4 days
P = (1 + 4×2 + 4) / 6 = 2.17 days
Confidence: medium

## Related
- `internal/records/memorial_import.go`
- `internal/jobs/jobs.go`
- User report: bare-array import of four unique memorial IDs produced two opaque skips on 2026-07-26.

