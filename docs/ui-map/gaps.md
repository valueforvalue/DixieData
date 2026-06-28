# Gaps

Architectural debt surfaced while building the UI map. Each finding
links to the relevant code path. Updated as the audit progresses.

## Redundancy

### Empty-state duplication

`internal/templates/partials/empty_state.templ` defines
`EmptyStateCard(archiveKeyword, counts)` — the older API. The newer
`components/empty_state.templ` provides `EmptyState(archiveKeyword,
counts)` with the same signature.

- `partials/empty_state.templ` — older partial.
- `components/empty_state.templ` — newer component.
- Both callable with archive keywords (calendar, soldiers, browse,
  review_queue, insights, research, jobs, export, settings).

**Action**: pick one API and migrate the other. Track in
`docs/TASKS.csv` if not already.

## Routes without routebuilders

These are referenced from templates/handlers as bare string paths and
are candidates for `routebuilder.*` coverage:

| Route | Used by | Risk |
| --- | --- | --- |
| `/calendar` | `Layout` top nav, all month links | Medium — duplicated |
| `/soldiers` | `Layout` top nav, dashboard links | Medium |
| `/browse` | `Layout` top nav | Medium |
| `/review-queue` | `Layout` top nav | Medium |
| `/insights` | `Layout` top nav | Medium |
| `/share` | `Layout` top nav | Medium |
| `/settings` | `Layout` top nav, footer | Medium |
| `/soldiers/new` | `Layout` top nav primary CTA | Medium |
| `/setup` | First-launch redirect | Low |
| `/recovery` | Recovery flow | Low |

The top-nav links in `Layout` use bare hrefs. If a nav link is renamed
without updating the route registration, navigation silently breaks.
**Recommendation**: add `routebuilder.NavHome()`, `routebuilder.NavSearch()`,
etc. for nav links.

## Surfaces not in uiids Registry

(Empty so far — `uiids.go` Registry is well-curated. Add findings here
when a DOM ID shows up in templates but not in the Registry.)

## Atomic components not surfaced

`internal/templates/components/` has 6 components (button, card,
empty_state, field, pill, toast). None are registered in `uiids`
because they're primitives, not regions. This is correct — flagged
here only for the lookup table.

## Missing summary-card affordances

### Memorial import log download button (added in commit `70878ac`, not wired)

`JobResult.LogPath` is populated by
`handleConfirmMemorialJSONImport` (`internal/appshell/imports_handlers.go`)
and `jobs.go` even documents the intent in a comment that points
at `jobs.templ::jobSummaryCard` for the secondary download action.
But `jobSummaryCard` still renders only the artifact (Open/Save)
based on `Summary().ResultPath`, which stays empty for memorial
imports.

**Effect**: the summary card itself loads fine — this is a missing
**affordance**, not a load failure. When a memorial import completes
with `Failed > 0`, the user sees the failed count on the summary
card but has no in-app way to retrieve the error log written to
disk. They have to find the file via Settings → Debug or
hand-traverse the data directory.

**Fix**: render a second download button in `jobSummaryCard` when
`job.Result.LogPath != ""`, mirroring the artifact button pattern.
The backend already has the data (`LogPath`) — only the templ is
missing the branch.

### Shared-import conflicts link

`Summary().DetailLines` includes
`Conflicts staged for review: N — see Merge Review below.` for
`shared_import` jobs when `Conflicts > 0`, but the line is text-only.
The user has to navigate back to Share to find the Merge Review
section. Add a deep-link pill on the summary card.

## Dialog guard audit (deferred)

Per `docs/agents/dialog-guard.md`, every export/import handler that
opens a native `SaveFileDialog` / `OpenFileDialog` MUST guard with
`a.inFlight.LoadOrStore`. Candidates:

- `ExportBackup`
- `ExportDatabasePDFAsync`
- `SettingsImagesOrphansCleanup`
- `SettingsQualityApply`
- `SettingsUpdateApply`

**Action**: full audit deferred to dedicated pass. Run a dedicated
bug-hunter session with the dialog-guard checklist.

## Screen inventory: candidate deletions

- `LayoutV2` already removed (per `layout.templ` doc comment).
- Any `.templ` files in `internal/templates/` not yet covered by a
  wireframe will be enumerated after pilot approval.

## Audit forward links

The active UI/UX audit round is in `audit/`. Findings from
`audit/reports-rN/` will be linked from individual wireframes as the
audit lands — see [README.md](README.md) "link, don't absorb"
policy.