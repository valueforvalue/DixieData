## Problem

The FTS `snippet()` function in SQLite returns non-empty text for any
row that matched the FTS query, even when the match was in a different
column from the one `snippet()` was called on. The current code in
`internal/records/soldier_service.go:560-571` calls `snippet()` on three
columns (biography, notes, scratch) and uses `MAX()` to pick one per
row. Replacing `MAX()` with `CASE WHEN biography_snippet != '' THEN ... END`
breaks the `SearchPage` match-field labelling because biography's
`snippet()` can return non-empty text for a notes-only match.

**Source:** 2026-06-24 full audit; deferred from issue #107 (finding 7.11).

## Goal

Leave the existing MAX-of-three-snippets picker in place. Document the
SQLite-specific behaviour so future readers do not re-attempt the CASE
rewrite and ship a regression.

## Approach

1. Add a code comment above the `MAX()` invocation explaining why
   `CASE` is unsafe (`snippet()` returns non-empty text for any FTS
   match, not only matches in the column being queried).
2. Add a regression test that asserts `SearchPage` returns
   `SearchMatchField='Notes'` when only notes contain the query term,
   to lock in the current behaviour.

## Files likely touched

- `internal/records/soldier_service.go` (comment + test)
- `internal/records/soldier_service_test.go`

## Out of scope

- Switching the FTS index from FTS4 to FTS5 (already shipped per
  CHANGELOG v1.1.16; does not change `snippet()` semantics).
- Removing the unused `scratch_snippet` column (still used by the
  scratchpad search path).