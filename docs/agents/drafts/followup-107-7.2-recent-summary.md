## Problem

`internal/records/soldier_service.go:1011` `RecentByIDs` returns full
`models.Soldier` rows (all 42 columns + spouse-display-id subquery +
record-count + image-count subqueries). The recent-search view
(`handleRecentSearch` -> `SearchResults` -> `SoldierCard`) renders only
ID, DisplayID, FirstName, LastName, MaidenName, SpouseName, EntryType,
DeathDate, BuriedIn, Unit, NeedsReview, ReviewReason, SearchMatchField,
SearchMatchSnippet, plus the highlighted-pill row fields. The unused
columns are parsed but never displayed, and the subqueries add
round-trip latency on the records/images counts tables.

**Source:** 2026-06-24 full audit; deferred from issue #107 (finding 7.2).

## Goal

Reduce `RecentByIDs` payload by at least 70% by projecting only the
fields the recent view actually renders.

## Approach

1. Add a `models.RecentSummary` struct with ID, DisplayID, EntryType,
   FirstName, LastName, MaidenName, DeathDate, BuriedIn, Unit,
   NeedsReview, ReviewReason.
2. Add a `(s *SoldierService) RecentSummariesByIDs(ids []int64, limit int) ([]models.RecentSummary, error)`
   method that selects only the projected columns.
3. Update viewmodel mapping to expose RecentSummary as PersonRecord
   enough for the recent-search view (or extend PersonRecord with a
   Recent-only projection if needed).
4. Update `handleRecentSearch` to call the new projection method.
5. Add benchmarks that compare RecentByIDs and RecentSummariesByIDs
   payload sizes on a 1000-record fixture.

## Files likely touched

- `internal/models/models.go` (RecentSummary type)
- `internal/records/soldier_service.go` (RecentSummariesByIDs +
  facade update)
- `internal/appshell/app_facades.go` (interface update)
- `internal/appshell/soldiers_handlers.go` (handleRecentSearch)
- `internal/records/soldier_service_test.go` (regression)

## Out of scope

- Refactoring every Soldier return path. Only RecentByIDs is in
  scope for this issue.