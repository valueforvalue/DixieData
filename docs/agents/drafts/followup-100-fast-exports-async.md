## Problem

The fast exports in `internal/appshell/exports_handlers.go`
(`handleExportJSON`, `handleExportCSV`, `handleExportICalendar`,
`handleExportBackup`) all block the HTTP goroutine until completion.
They are typically fast (sub-second) but the audit issue #100 spec
leaves the door open to migrate them to the background-jobs pattern
for consistency.

**Source:** 2026-06-24 full audit; deferred from issue #100.

## Goal

Decide whether each fast export stays synchronous or moves to async,
and migrate the chosen ones.

## Approach

1. Profile each fast export on a 1000-record archive to confirm
   whether the synchronous path actually blocks for non-trivial time.
2. If any export is borderline (>= 1s typical), opt it into the
   `?async=1` path with a status page redirect.
3. Update the corresponding share-page button to submit to the async
   route.
4. Add a brief note in `CHANGELOG.md` [Unreleased] explaining which
   exports are async now and which stay sync.

## Files likely touched

- `internal/appshell/exports_handlers.go`
- `internal/templates/share.templ`
- `CHANGELOG.md`

## Out of scope

- Replacing the `SaveFileDialog` model. The async path still needs a
  destination path before it can enqueue.