## In-place update safety

> **Required.** Every PR to `dev` or `stable` carries one of:
> - [ ] `safe-for-in-place` — reviewed against the 4 rules in
>       [`docs/agents/build-protocol.md`](docs/agents/build-protocol.md) §5;
>       safe for in-place update on `stable`.
> - [ ] `unsafe-for-in-place` — intentional destructive change;
>       ships via full re-install + restore-point only. **Explain
>       WHY the change is destructive** in the body below.
>
> The 4 rules:
> 1. No destructive schema migrations (`DROP TABLE`, `DROP COLUMN`,
>    `RENAME` without preserve-old, `DELETE FROM` in a migration block,
>    removal of an `IF EXISTS` guard).
> 2. No handler signature changes that break existing routes
>    (rename without backward-compat shim, HTTP method change).
> 3. No auto-destructive flows on startup (first-launch triggers
>    that delete records, drop tables, or replace the data dir).
> 4. No removal of safety gates (`updateEligibility`,
>    `inFlight` dialog guard, the dialog-guard law in `CONTEXT.md`).

> CI does **not block** the merge on this label, but a stern
> warning comment is posted if the label is missing. See
> `docs/agents/build-protocol.md` §5 for the rationale.

## Summary

<!-- One-paragraph description of what this PR changes. -->

## Test plan

<!-- What you ran to verify the change works. -->

## Regression net

<!-- New tests added + smoke probes + manual steps. -->

## Related

<!-- Issue numbers, ADR numbers, prior commits. -->