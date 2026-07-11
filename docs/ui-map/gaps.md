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

## Intentionally-orphan routes

Routes that exist in the registered handler set but have no UI
invoker. The `audit/discover_orphan_handlers.mjs` probe flags these
as informational (exit 0, output-only); this section documents
each one with its decision context so the probe output reads as
a known allow-list rather than fresh drift.

### `POST /events/{id}/sources/attach`

- **Handler**: `handleEventSourceAttachForDispatch` dispatched via
  `eventPanelRoute{method: POST, subPath: "attach"}` for the
  `sources` panel (`internal/appshell/event_panel.go:231`).
  Reachable via `POST /events/{id}/sources` catch-all
  (`internal/appshell/routes.go:207-214`).
- **Decision context**: issue **#360** explicitly removed the legacy
  standalone attach form from `/events/{id}`'s Source Records panel
  (the duplicate of the inline #357 attach surface on `/events/new`
  + `/events/{id}/edit`). The route was kept "in case anyone
  bookmarked it" — no UI invokes it after #360. The
  `// expected-orphan-route` marker comment option that #360
  offered for `routes.go` was **not** applied (the slice shipped
  form-removal without the marker), so the orphan probe currently
  flags the handler as drift.
- **`.ddshare` replay claim** (from the issue #380 framing): the
  source code does **NOT** use this route for `.ddshare` import
  replay. Grep `ddshare` + `memorial_json` + `EventSourceAttach`
  returns zero call sites. The framing in #380 inherits the
  "orphan-by-design" wording from the route-survived rationale
  without verifying the replay path; this entry corrects the
  record. If the route is kept, the rationale is "backward
  compatibility for bookmarks" only.
- **Recommendation** (carried from #360's "decision deferred to
  slice 2" + #380 Phase 3):
  - **Delete** the handler + dispatcher entry + chi route
    registration. Save: ~30 lines of dispatcher glue
    (`handleEventSourceAttachForDispatch` body + the entry in
    the sources panel route list + the `// POST /sources/attach`
    comment at `routes.go:207`). Regression: anyone with a
    bookmarked URL breaks; the orphan probe goes clean.
  - **OR keep + mark** with a `// expected-orphan-route: kept for
    backward-compat with bookmarks; see gaps.md` comment near
    the handler so future probes (and the #369 regex-heuristic
    fix) can add an allow-list entry. Cheaper, preserves
    bookmarks.
- **Issue tracking**: #380 Phase 3 will pick one path. Either way,
  the chosen option must update this entry.

(Add additional intentionally-orphan routes here as the audit
surfaces them. Same shape: route + handler location + decision
context + recommendation.)

## Surfaces not in uiids Registry

(Empty so far — `uiids.go` Registry is well-curated. Add findings here
when a DOM ID shows up in templates but not in the Registry.)

## Atomic components not surfaced

`internal/templates/components/` has 6 components (button, card,
empty_state, field, pill, toast). None are registered in `uiids`
because they're primitives, not regions. This is correct — flagged
here only for the lookup table.

## Missing summary-card affordances

### Memorial import log download button (WIRED — closes #159)

`JobResult.LogPath` is populated by
`handleConfirmMemorialJSONImport` (`internal/appshell/imports_handlers.go`)
and `jobs.go` documents the intent in a comment that points
at `jobs.templ::jobSummaryCard` for the secondary download action.
Originally `jobSummaryCard` rendered only the artifact (Open/Save)
based on `Summary().ResultPath`, which stays empty for memorial
imports.

**Fix landed on `fix/jobs-wire-memorial-log-download`**: `jobSummaryCard`
now branches on `job.Result.LogPath != ""` and renders a
"Download log" anchor linking to `/jobs/{id}/log` via the new
`routebuilder.JobLog(jobID)` helper. Backend handler
`streamJobLog` (in `internal/appshell/jobs_handlers.go`) resolves
the path, verifies containment inside `os.TempDir()` via
`filepath.Rel` + prefix check, then streams the file with
`Content-Disposition: attachment`.

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

## Dialog guard audit (complete — 2026-06-29)

Per `docs/agents/dialog-guard.md`, every export/import handler that
opens a native `SaveFileDialog` / `OpenFileDialog` MUST guard with
`a.inFlight.LoadOrStore` (or the inline `enterInFlight` /
`defer leaveInFlight` pattern). Audit re-checked the 5 candidates
originally listed here against the current handler names:

| Original name | Current handler | Dialog? | Guarded? |
| --- | --- | --- | --- |
| `ExportBackup` | `handleExportBackup` (`exports_handlers.go:703`) | `guardedSaveFileDialog` | ✅ |
| `ExportDatabasePDFAsync` | `handleExportDatabasePDF` (`exports_handlers.go:381`) | `guardedSaveFileDialog` | ✅ |
| `SettingsImagesOrphansCleanup` | `handleCleanupImageOrphans` (`settings_handlers.go:96`) | none (starts job) | n/a |
| `SettingsQualityApply` | `handleApplyDataQuality` (`settings_handlers.go:68`) | none (DB op) | n/a |
| `SettingsUpdateApply` | `handleApplyLatestUpdate` (`app_update.go:58`) | none (PowerShell exec) | n/a |

**Real bug found in adjacent scan**: `handleImportBackup`
(`imports_handlers.go:43`) was missing from the original candidate
list. It called `a.OpenFileDialog` directly with no guard. Wrapped
in the inline `enterInFlight` + `defer leaveInFlight` pattern,
keyed by `guardedOpenFileDialogKey("backup_import", opts)`.
Regression test `TestHandleImportBackupDialogGuard` added to
`internal/appshell/open_dialog_guard_test.go`. Closes #158.

## Screen inventory: candidate deletions

- `LayoutV2` already removed (per `layout.templ` doc comment).
- Event Records (issue #320 + #342): 5 new wireframes added
  (24-events-list, 25-event-detail, 26-event-new, 27-event-edit,
  28-event-pdf). The Event surface IDs (`page.event.list`,
  `page.event.detail`, `page.event.new`, `page.event.edit`,
  `panel.event.detail.{sources,tags,linked-persons,images}`,
  `panel.event.form.{sources,linked-persons,tags}`) are registered
  in `internal/uiids/uiids.go`. The `/events/*` route family is
  documented in [routes.md](routes.md).
- Floating-dock (issue #283 / #289 / #313): the dock is a global
  panel rendered once in `layout.templ`. Surface IDs
  (`panel.floating.dock`, `panel.floating.nav-panel`,
  `panel.floating.scratchpad-status`) are registered. The dock is
  not a routable screen, so it lives in the Global section of
  [INDEX.md](INDEX.md) rather than as a numbered wireframe row.
- Top-nav Share foldout (issue #264): the foldout is a global
  navigation primitive (trigger + panel pair), not a routable
  screen. Surface IDs (`layout.share.menu`, `layout.share.menu.trigger`)
  are registered. The 4 destinations (Export / Import / Share Queue
  / Sync) have their own pages — see 08a / 08b / 08c wireframes +
  `/share/queue` documented in [routes.md](routes.md).
- Top-nav Research & Review foldout (issue #378 slice 2): same
  primitive, same documentation pattern. The picker page is
  documented inline in [routes.md](routes.md) (no separate
  wireframe — it is a single-purpose page with no per-screen
  regions worth wireframing beyond what the picker fragment shows).
- Top-nav Tags link (issue #256): a literal `/tags` link. The
  destination page has a wireframe-equivalent in the Tags section
  of [routes.md](routes.md); a numbered wireframe row is deferred
  until the tags page grows beyond its current single-table shape.
- Share Queue pill (issue #182): persistent bottom-center pill
  (`panel.share-queue.pill`). Documented in the Global section of
  [INDEX.md](INDEX.md) and registered in `internal/uiids/uiids.go`.
- Any `.templ` files in `internal/templates/` not yet covered by a
  wireframe will be enumerated after pilot approval.

## Audit forward links

The active UI/UX audit round is in `audit/`. Findings from
`audit/reports-rN/` will be linked from individual wireframes as the
audit lands — see [README.md](README.md) "link, don't absorb"
policy.