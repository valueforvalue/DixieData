# Changelog

All notable changes to DixieData are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/) and the project adheres to
[Semantic Versioning](https://semver.org/) — DixieData uses `v1.2.N` where
N is `CurrentSchemaVersion` from `internal/versioninfo/versioninfo.go`.

Release dates are the commit date of the tagged release. Internal refactors
that do not change user-visible behavior live under `### Maintenance` so
the Added / Changed / Fixed / Removed lists stay scannable.

## [Unreleased]

### Fixed

- CI: `.github/workflows/test.yml` "Restore Typst binary for render
  tests" step called `Restore-DixieDataTypstBinary` without the
  mandatory `-Root` parameter, causing the Windows runner to fail
  before `go test` could run on `internal/archive` and `pkg/render`.
  Resolve `$root` via `Get-DixieDataRoot` (already exported from
  `scripts/build-common.ps1`) and pass it through.

### Added

- `make ui-diff` target for v1-vs-v2 visual regression (issue #74
  Phase 0 PR4). `scripts/ui-diff.mjs` boots Playwright against the
  running `dixiedata-web` server, walks four routes (`/`,
  `/soldiers`, `/browse`, `/settings`) at desktop (1280×800) and
  mobile (390×844) viewports, captures both `?ui=v2`-off (v1) and
  `?ui=v2`-on (v2) screenshots per surface, and writes a JSON
  summary to `audit/reports/ui-diff/summary.json`. Reuses
  `audit/harness.mjs` helpers (`detectVisualIssues`) so v1 vs v2
  visual heuristic diff lands in the same shape as the existing
  audit reports. Connection-refused failures exit with code 2 and
  a friendly pointer to `audit/README.md` instead of a stack
  trace. Eight PNGs (~3.7 MB) captured on first end-to-end run.
- `?ui=v2` query-string feature flag: `internal/uiver/uiver.go` exposes
  `Middleware` (reads `?ui=v2` and stores a boolean on the request
  context) and `IsV2(ctx)`. `internal/appshell/routes.go` wraps the
  mux with `recoverMiddleware(uiver.Middleware(mux))`. The Wails
  desktop build never sends `?ui=v2`, so production behavior is
  unchanged; future component-primitive refactors (#74 Phase 1) can
  branch on `IsV2(ctx)` and ship behind the flag without forcing a
  binary rollback. The `Layout()` template wrapper dispatches to a
  new `LayoutV2()` stub (currently a minimal passthrough shell) so
  end-to-end verification is possible in web-mode audits. New
  `internal/uiver/uiver_test.go` exercises five cases: default
  context, explicit v2 context, no query param, `?ui=v2`, and
  rejection of any other value (`v1`, `V2`, `v2x`, `true`, `1`).
- Design tokens wired into `tailwind.config.js` `theme.extend`:
  `gold`, `sepia-500`, `sepia-300`, `parchment`, `parchment-soft`,
  `ink`, `ink-muted`, `ink-faint`, `bg-amber-50`, `bg-slate-200`,
  `review-red`, `review-red-tint`, `success-green`, `error-red`,
  `radius.surface`, `radius.dialog`, `shadow.card`, `shadow.modal`,
  `motion.fast`, `motion.med`. Tailwind generates the utility
  classes; no existing CSS or template literal is migrated yet —
  that follows in per-component-class PRs (PR2a/PR2b/...) so each
  pixel shift is reviewed in isolation. Hex literal migration
  follows the locked names from ADR-0003.
- ADR-0003 design system tokens: `docs/adr/0003-design-system-tokens.md`
  locks the color, radius, shadow, motion, and typography vocabulary
  for the #74 Phase 1 component primitives. The companion
  `docs/adr/0003-design-system-tokens-reference.md` lists every token
  name + canonical value + intended use. Subsequent component
  extractions reference these names instead of inventing new ones.
- Implementation plan for the remaining open work of issue #74 (UI/UX
  revamp): `.rpiv/artifacts/plans/2026-06-25_74-ui-revamp.md`. Six
  phases, ~22 PRs sequenced behind `?ui=v2`; Phase 0 (htmx load in
  web-mode `index.html`, ADR-0003 design tokens, `?ui=v2` flag, and
  `make ui-diff` harness) detailed for immediate execution.
- Test, build, and audit GitHub Actions workflows (`.github/workflows/test.yml`,
  `build.yml`, `audit.yml`). Test runs `go test -short` on every push; build
  verifies the Wails binary builds and embeds no absolute source paths (the
  `-trimpath` flag); audit runs the UI/UX harness weekly and on PRs touching
  templates or frontend.
- `scripts/bump-version.ps1 -VerifyOnly` — non-mutating validation pass that
  fails the build if `versioninfo.go`, user-manual, implementation-and-features,
  ai-handoff, or CHANGELOG disagree on the current version.
- Reproducible Typst + PDFium bootstrap (`scripts/build-common.ps1`): downloads
  pinned releases, verifies SHA256, refuses to install on mismatch. A fresh
  clone can build without manually vendoring binaries.
- `bin/MANIFEST.md` — authoritative list of every native binary the build
  pipeline expects, with version, source URL, pinned SHA256, and an upgrade
  procedure.
- `scripts/token-clean.ps1` sweep extensions — removes untracked `*.exe` from
  repo root and release zips older than the last two tags.

### Changed

- Implementation stack reference (`docs/implementation-and-features.md`) now
  lists the Typst CLI as the PDF renderer (the `go-pdf/fpdf` path was retired
  in slice 7). Section 6.7 carries a migration note.
- User-manual, implementation-and-features, and ai-handoff now agree on the
  current release line (`v1.2.55`); the version source of truth is
  `internal/versioninfo/versioninfo.go`.
- `Makefile` `render-svg` target guards on the local `render-svg.sh` script
  and exits 0 with a skip message on machines where the script is absent
  (was a hard failure before).
- 7 previously undocumented `Makefile` targets (`tune`, `tune-smoke`,
  `tune-snapshots`, `render-round`, `render-round-ONE`, `update-snapshots-ONE`,
  `render-svg`) now print descriptions in `make help`.
- Stress test files (`internal/appshell/app_stress_test.go`,
  `tests/stress/*.go`) honour `testing.Short()` — `make test` skips them,
  `make stress` still runs them.
- `wails build` in `scripts/build-common.ps1` passes `-trimpath` so
  distributed binaries do not embed absolute source paths.

### Fixed

- `.gitignore` no longer ignores `google-oauth-defaults.example.json` (the
  example is intentionally tracked; the entry made contributors think their
  edits to the example were being saved).
- `tests/goldmaster/playwright/test-results/.last-run.json` is no longer
  tracked (was a runtime artifact slipping through the gitignore filter).
- 6 release zips older than the last two tags removed from `release/`
  (cleaned by the extended `token-clean.ps1`).
- Performance fixes from the 2026-06-24 sweep (issue #107): the quick-search
  form now carries `hx-sync="this:replace"` so each new keystroke aborts the
  in-flight XHR (no more out-of-order responses); `BrowsePage` runs the
  count + paginated select in a single CTE so every browse filter change
  costs one round-trip instead of two. `FormSuggestions` caching,
  `BrowseResults` hx-boost partial, and the FTS snippet column were already
  shipped before this batch landed; `RecentSummary` projection (7.2) and
  feedback retention setting (7.13) are deferred to a future pass.
- Background-job pattern for long exports (issue #100): adds an
  in-process job registry (`internal/jobs`) and a new `/jobs/{id}`
  status page. The share page's Static Archive and Printable PDF
  exports accept `?async=1` and now run as background jobs that
  the user can poll and cancel from a dedicated progress page
  instead of blocking the HTTP goroutine for minutes. Issue #125
  closes out the visible part of the flow: completed exports now
  expose a `/jobs/{id}/artifact` endpoint that streams the saved
  file back with a `Content-Disposition: attachment` header, and
  the status page renders an `Open {kind}` pill-link instead of
  the previous text-only `Saved to …` line. Issue #122 caps
  concurrent workers with a semaphore (default 2, override via the
  `DIXIEDATA_JOBS_CONCURRENCY` env var); saturated submissions
  stay in `queued` until a slot frees. Issue #123 wires the
  registry to a JSONL log in `dataDir/jobs.jsonl` so completed
  exports survive a webview reload or app restart; jobs that were
  `running` when the previous process exited are flagged
  `interrupted` so the status page is honest about lost work.
  Issue #120 documents the FTS snippet picker (it uses MAX-of-three
  snippets, not a CASE rewrite, because SQLite's `snippet()` returns
  non-empty text for any FTS match in a row regardless of which
  column actually matched) so the next reader does not refactor it
  into a regression. Issue #118 adds the same alt-text sanitisation
  the SoldierCard thumbnail already has (issue #99) to the image
  preview modal so pasted HTML in captions never lands in an alt
  attribute. Issue #117 converts the three modal dialogs
  (feedback / print-config / google-preferences) to native
  `<dialog>` elements so focus trapping, ESC-to-close, and
  inert-background come from the browser instead of a custom
  div overlay. Issue #124 adds `/jobs/{id}/stream` so the
  registry can push Server-Sent Events to clients in real time;
  the existing `/jobs/{id}/status` htmx polling endpoint stays
  as the primary visible path, and a future change can swap the
  page over to `EventSource` when the audit harness asks for it.
  Issue #126 makes the call on whether the fast exports
  (`/export/json`, `/export/csv`, `/export/ical`,
  `/export/backup`) should migrate to the background-jobs
  pattern: they stay synchronous because each runs in well under
  a second on a 1000-record archive; only the two exports flagged
  as blockers in the audit (`/export/database-pdf` and
  `/export/static-archive`) accept `?async=1`. Issue #121 adds a
  startup prune of the feedback log (default 365-day retention)
  so the JSONL file stops growing unbounded on long-running
  desktop sessions; the prune is best-effort, leaves corrupt
  lines in place, and ships without a settings UI toggle (the
  retention window is hard-coded for now). Issue #119 slims
  `RecentByIDs` from 45 to 38 columns by dropping the correlated
  record/image count subqueries and the long-form fields the
  recent-search view never renders; a smoke benchmark tracks the
  new path.
- Search results no longer render the highlighted `SoldierCard` pill row
  (entry-type / death-date / burial-place). The same data now appears as
  a small plain `<dl>` inside the card. The `Needs Review` pill row stays
  as it was.
- Accessibility audit findings from the 2026-06-24 sweep (issue #99):
  quick-search input gets a meaningful `aria-label` (no longer `q`);
  search results pagination lives inside an `aria-label`-ed `<nav>`
  landmark with `aria-current="page"`; image thumbnails fall back to
  `Image for Person Record {DisplayID}` alt text when the caption is
  blank and strip HTML from non-blank captions; the browse results
  table declares `scope="col"` on every header; the disabled `Compare
  Selected` button is `aria-describedby` the manual-comparison help text;
  the feedback message `<textarea>` declares `aria-required="true"`
  alongside `required`; the feedback / print-config / google-preferences
  modals declare `role="dialog"` + `aria-modal="true"` and are
  `aria-labelledby` their `<h3>` heading; `lang="en"` on the root
  `<html>` carries a comment marker for the future i18n pass.

### Removed

- `audit/package.json` (deps merged into root `package.json`; the `audit`
  npm script now lives there too).
- Sub-768px hamburger drawer from the top nav (`data-top-nav-toggle`,
  `#top-nav-drawer`, `initializeTopNav` handler, and the
  `@media (max-width: 780px)` block in `frontend/tailwind.css`). DixieData
  is a Wails desktop app; the drawer was dead UI. The split-screen
  breakpoints (`max-width: 1040px`, `1100px`, `900px`) and content-template
  `md:hidden` / `md:flex` toggles stay (16" monitor split-screen layout).

### Maintenance

- `audit/reports-r3/audit-v3.md` narrative summary written, matching the
  structure of round 1 / round 2 reports.
- `AGENTS.md` expanded with a glossary index pointing at `CONTEXT.md` and an
  11-row file map of the codebase entry points.
- `bin/README.md` documents the current typst platform gap (Windows shipped,
  macOS / Linux land with the bootstrap follow-up).
- Cumulative PR1+PR2+PR3 of issue #42 (God-class reduction) completed:
  `internal/appshell/app.go` shrank from 4,334 to 2,116 LOC across the
  PRs below; 11 new domain files created under `internal/appshell/`.
  All 72 registered routes preserved; all 17 test packages pass.
  - PR1: extracted `internal/archive/pdf_layout.go` and
    `internal/archive/static_archive.go` from `export_service.go`
    (4,510 → 1,610 LOC).
  - PR2: split `internal/appshell/app.go` into 10 new files
    (`routes.go`, `lifecycle.go`, `google_handlers.go`, `calendar_handlers.go`,
    `imports_handlers.go`, `exports_handlers.go`, `settings_handlers.go`,
    `insights_handlers.go`, `research_handlers.go`, `soldiers_handlers.go`,
    `reviews_handlers.go`). Each PR step was a pure file move with no public
    API or behavior change.
- `Makefile` added as the preferred entry point for build / test / asset
  generation / release tasks; every target routes through PowerShell with
  verbose output captured to `build/log/<target>.log` and `pipefail` so
  failures propagate.
- `scripts/bump-version.ps1` (`make bump`) — strict schema-version increment
  with paired-migration-note enforcement.
- `scripts/release-github.ps1` (`make release-github`) — tag + push + draft
  GitHub release with five safety gates before any mutation.
- `docs/RELEASING.md` — release-process documentation.
- Generated `*_templ.go` and `frontend/wailsjs/*` untracked from the index
  (regenerated by `make tpl` and `wails build`).
- `.gitignore`, `.agentignore`, `.aiderignore`, `.cursorignore` hardened with
  canonical GOTH/Wails patterns plus `build/log/` for captured build output.

## v1.2.55 - 2026-06-25

### Added

- `internal/models/constants.go` with `EntryTypeSoldier`, `EntryTypeWife`,
  `EntryTypeWidow`, `EntryTypeLinkedPerson`, and the `EvidenceType*` family
  (`LocalArchive`, `SharedArchive`, `BackupArchive`, `StaticArchive`,
  `RestorePoint`, `MemorialJSON`, `FindAGrave`, `PensionRecord`,
  `ApplicationRecord`, `Other`). Templates and viewmodels now reference
  these constants instead of bare string literals.

### Changed

- `soldiers.entry_type` carries an application-level discipline enforced at
  the migration boundary (`internal/db/schema.go` `migrateEntryTypeDiscipline`).
  Any future INSERT or UPDATE with a value outside the canonical set is
  rejected. SQLite CHECK constraints cannot be added in-place; the function
  records a one-time migration log so the rule is enforced on every
  subsequent schema open.
- `research_log.evidence_type = 'archive'` was rewritten in place to
  `'local_archive'` to match the glossary. A forward-only helper
  (`isNoSuchTableError`) lets the migration succeed on archives where the
  `research_log` table does not yet exist (planned for v56+).

## v1.2.54 - 2026-06-08

### Fixed

- Hardened calendar sync UX and popout layout.

## v1.2.53 - 2026-06-08

### Added

- Managed calendar event preferences and a dry-run sync mode.

## v1.2.52 - 2026-06-08

### Changed

- Enforced Chicago timezone for calendar sync and iCal export.
- Synced calendar events stay at the user's local morning hour.

## v1.2.51 - 2026-06-08

### Fixed

- Google Calendar reminder payload format.

## v1.2.50 - 2026-06-08

### Added

- Google calendar timezone fallback coverage.

### Fixed

- Google Calendar sync timezone requirement.

## v1.2.49 - 2026-06-08

### Fixed

- Bumped release line forward; broadened server-side post-update trust clear
  and hardened launch-state clearing.
- Fixed UI freeze on the intro screen caused by a `setBusyGroupState`
  ReferenceError.
- Hardened startup bootstrap and bundled OAuth defaults in release zips.

### Added

- Pre-update backup and managed Google calendars.
- Settings data-quality scan workflow.
- Previewed memorial JSON import workflow.

## v1.2.45 - 2026-06-07

### Fixed

- Stabilized search hydration.
- Added landscape biography pages and safer draft delete.
- Shipped export layout help.
- Made edit drafts version-aware.
- Clarified stale draft review copy.
- Tightened compressed quick-action buttons.

## v1.2.37 - 2026-06-01

### Fixed

- Fixed calendar alignment.

## v1.2.36 - 2026-05-31

### Fixed

- Fixed release build import.
- Fixed browse filters.

## v1.2.35 - 2026-05-31

### Fixed

- Fixed printable export modal viewport.

## v1.2.34 - 2026-05-31

### Fixed

- Fixed normalized pension-state filtering.

## v1.2.33 - 2026-05-31

### Fixed

- Fixed split-screen layouts.

## v1.2.32 - 2026-05-31

### Changed

- Polished calendar and browse workflows.

## v1.2.31 - 2026-05-31

### Added

- Calendar items and display fixes.

## v1.2.29 - 2026-05-30

### Maintenance

- Bumped release line forward.

## v1.2.28 - 2026-05-30

### Added

- Restore points for in-place updates.
- Single-record JPG export polish.
- Made scratchpads database-backed.
- Browse and startup improvements.
- Linked-person records renamed to person records.
- Shared import memory and software updates.

## v1.2.22 - 2025 (date not captured at tag) - Minor Release

- Added the generic linked-person workflow across entry creation, presentation, import/export paths, and legacy backup restore.
- Added clickable internal `[[DISPLAY-ID]]` links, global feedback capture/export, and maiden-name italics across live and exported views.
- Fixed printable PDF record cards so audit metadata is omitted and oversized entries continue onto additional pages instead of shrinking to unreadable text.
- Moved the production release line forward to `v1.2.22` so runtime metadata, build packaging, and docs stay aligned with the current feature set.

## v1.1.21 - Patch Release

- Replaced the UI's remote Tailwind CDN dependency with a checked-in local CSS bundle so desktop installs render correctly on offline machines.
- Automated CSS regeneration in the shared PowerShell build path so debug and release builds refresh the bundled stylesheet before Wails packaging.
- Carried the release line forward to `v1.1.21` so the schema version, runtime metadata, Wails title, and packaged release artifacts stay aligned.

## v1.1.20 - Patch Release

- Fixed the entry-form draft recovery banner so the **Discard local draft** button remains visible after the page initializes and after draft-status updates run.
- Carried the release line forward to `v1.1.20` so the schema version, runtime metadata, Wails title, and packaged release artifacts stay aligned.

## v1.1.19 - Patch Release

- Fixed the new-record localStorage draft flow so successful creates clear the cached entry instead of repopulating the next record form.
- Added an in-app **Discard local draft** recovery action on new/edit record forms so stuck entry drafts can be cleared without DevTools or a debug build.
- Enabled Confederate Home fields for wife and widow records in the entry form.
- Carried the release line forward to `v1.1.19` so the schema version, runtime metadata, Wails title, and packaged release artifacts stay aligned.

## v1.1.18 - Full Release

- Hardened `.ddbak`, `.ddshare`, diagnostics, and static archive ZIP creation to write through a temp file, verify ZIP finalization, and only then replace the destination file.
- This avoids success-shaped partial archives caused by unverified final ZIP close/flush behavior at the final save path.

## v1.1.17 - Patch Release

- Fixed the static web archive detail view so exported `index.html` and `viewer.html` can open a selected person without leaving the expanded data area blank.
- Carried the release line forward to `v1.1.17` so the runtime metadata, Wails title, exported artifacts, and docs stay aligned.

## v1.1.16 - Gold Master
