# Changelog

All notable changes to DixieData are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/) and the project adheres to
[Semantic Versioning](https://semver.org/) — DixieData uses `v1.2.N` where
N is `CurrentSchemaVersion` from `internal/versioninfo/versioninfo.go`.

Release dates are the commit date of the tagged release. Internal refactors
that do not change user-visible behavior live under `### Maintenance` so
the Added / Changed / Fixed / Removed lists stay scannable.

## [Unreleased]

### Added

- New-soldier empty-name save is now a soft warning rather
  than a hard 400 (issue #151, follow-up to PR #149). The
  browser-side `required` attribute is removed from both name
  inputs; a JS interceptor in `dispatchDixieDataForm` surfaces
  a single `window.confirm` for empty-name submits and, on
  accept, appends `confirm_empty_name=1` to the FormData.
  `handleCreateSoldier` routes confirmed empty-name saves
  through to a successful INSERT with `NeedsReview=true` and
  `ReviewReason="Saved with no name; researcher should fill
  in."` so the row lands in the review queue. Empty names with
  no confirm marker still return 400 (catches the bypass).
  `handleUpdateSoldier` mirrors the behaviour on the edit path:
  clearing both names on a row that previously had a name sets
  `NeedsReview` with reason "Name cleared during edit".
  Linked-person / wife / widow entry types all carry the same
  logic — the review queue is the single triage surface.
  Regression net: `TestHandleCreateSoldier_EmptyNameMarksForReview`
  with four sub-tests (empty+confirm, first-only, last-only,
  empty+no-marker).
- Pension State, Pension ID, and Application ID fields on the
  new-soldier form are now visible for the `wife` entry type
  as well as `soldier` and `widow`. Previously, the JS handler
  at `frontend/app.js` `syncEntryTypeFields` used
  `isSoldierEntryType() || widowEntry` to decide whether to
  show the `data-soldier-or-widow-field` sections, which
  excluded `wife`. The handler now uses
  `isSoldierEntryType() || spouseEntry` (where `spouseEntry`
  already includes both `wife` and `widow`). Linked-person
  remains hidden — that role is not a pensioner. Templ change
  in `internal/templates/entry_form.templ`: the
  `pension_state` `<div>` wrapper moved from
  `data-soldier-only-field` to
  `data-soldier-or-widow-field` so its visibility follows the
  same JS rule. Issue #75.

- Back button on the Browse screen (`/browse`) using the
  existing `data-history-back` machinery. Default fallback
  is `/soldiers`. Issue #172.
- Back button on the Share / Export screen (`/share`)
  using the existing `data-history-back` machinery. Default
  fallback is `/`. Issue #169.
- Styled "Back to Dashboard" exit button on the Jobs
  status page (`/jobs/{id}`) using `data-history-back`.
  Replaces the inline body-copy link that was easy to miss.
  Issue #175.
- Live preview panel for the print-config modal. The
  modal now shows count + first 5 records + active sort +
  active group-by labels for the current scope/filter
  selection, updated within ~200 ms of any form change
  via a debounced `POST /export/preview` request. New
  server handler in `internal/appshell/export_preview.go`
  resolves the same scope/filter logic the actual PDF
  generation uses. Issue #179.
- Save / reuse printable-export templates. Users can now
  persist a print-config snapshot as a named local
  template and recall it later via the modal's new
  Templates section. Storage is a new SQLite table
  `export_templates` (schema v56) with named, JSON-
  encoded filter + group-by columns. CRUD lives in
  `internal/records/export_templates.go`; HTTP routes
  in `internal/appshell/export_templates_handlers.go`.
  The print-config modal gains a Saved Templates
  section at the top: dropdown + Load / Delete +
  name input + Save Current buttons. Load applies
  every field from the JSON response; Save posts the
  modal's full form plus the new template_name input;
  Delete uses the existing data-confirm convention.
  Issue #178.
- Pending-review badge on the Review Queue nav link.
  When one or more records are flagged `NeedsReview`,
  a small review-red badge with the count appears
  next to the link in the top nav. Counts >= 100
  render as "99+". Populated via a new
  `GET /layout/review-count` endpoint that the layout
  polls every 30s with htmx, so the count surfaces
  from any page without per-render DB load. A new
  `CountNeedsReview` method on `SoldierService`
  backs the endpoint. Issue #180.
- Stale-template warnings when loading a saved
  template (issue #181). The apply endpoint now
  cross-checks each stored filter value and selected
  ID against the current archive and returns a
  `warnings` array alongside the template. The
  client surfaces one warning per stale value as a
  toast; for many warnings it pops a single summary
  toast and logs the full list to the browser console.
  Stored SelectedIDs are persisted in a new
  `selected_ids_json` column on `export_templates`
  (schema v57) so scope=selected templates can also
  detect deleted record IDs. Issue #181.
- `smartBackLabel` in `frontend/app.js` now recognizes
  `/soldiers/{id}*` sub-routes (edit, timeline,
  camaraderie, research-log, conflict-ledger, research-pack,
  pdf, jpg) and returns "Back to Person Record" instead of
  the generic "Back". Same coverage extension for `/browse`,
  `/jobs`, `/settings`, `/recovery`. Issue #171.
- "Print/Export Selected" on the Browse screen now opens
  the printable-export picker modal **in place** instead of
  navigating to `/share`. The Browse screen's working set
  (filters, page, sort, selection) is preserved across the
  modal open/close cycle. The modal markup is extracted to
  `internal/templates/partials/print_config_modal.templ`
  and rendered by both `share.templ` and `browse.templ`;
  `BrowseView` now also loads `exportRecords` so the modal's
  filter dropdowns populate when opened from Browse. A new
  stress test `TestHandleBrowseResponseUnderThreshold`
  asserts a 1000-record archive's `/browse` GET stays under
  500 ms so the extra list query never causes perceived
  slowdown. Issue #176.

### Changed

- Success toast now uses a distinct green border
  (`rgba(41, 82, 45, 0.86)`) and faint green background
  (`rgba(242, 252, 244, 0.99)`) instead of the same sepia
  border as the default chrome. The previously-declared
  `success-green` / `success-green-bg` tokens in
  `tailwind.config.js` are now wired into use. Issue #174.
- The off-brand Tailwind `blue-*` classes on the
  research / relationship screens (Camaraderie, Conflict
  Ledger, Research Log / Pack / Collections, Service
  Timeline, plus the matching side-cards on Soldier Detail)
  are replaced with semantic `research-bg`,
  `research-border-soft`, `research-border`,
  `research-accent`, `research-text` tokens added to
  `tailwind.config.js`. Visual output unchanged
  (hex values map to the same Tailwind defaults), but the
  intent ("research-derived content") is now named in code
  and the off-brand classes are no longer the source of
  truth. Issue #173.
- Body background gradient stops are now exposed as
  `bg-sepia-top` / `bg-sepia-mid` / `bg-sepia-bottom`
  tokens in `tailwind.config.js` (ADR-0003). The literal
  hex values stay in `frontend/tailwind.css` because
  tailwindcss `@apply` cannot reach custom gradient stops;
  the CSS comment references the token names. Issue #170.

### Removed

- Dead `sepia-300` token removed from `tailwind.config.js`.
  Zero matches in templates or CSS. Issue #168.
- `openPrintConfigFromQuery` and its two call sites
  removed from `frontend/app.js`. The Browse screen no
  longer uses the `?openPrintConfig=1` query-string
  trigger (the in-place button opens directly), so the
  helper had zero callers. Issue #176.

### Removed

- "Open file" button removed from three surfaces: the
  `jobSummaryCard` on `/jobs/{id}`, the artifact section of
  `/jobs/{id}/report`, and the layout progress slot
  (`job_slot_fragment`). The button POSTed to `/jobs/{id}/open`,
  which calls `runtime.BrowserOpenURL("file:///<path>")` in Wails
  desktop and returns an info toast in web mode. Per user bug
  report, the button does nothing in their runtime. The "Copy
  path" button next to the original (already wired via
  `data-copy-path`) is the reliable fallback in both runtimes.
  Backend handler `openJobArtifact` (`internal/appshell/jobs_handlers.go:269`)
  is kept for any future callers / debug entry points. Two
  regression tests in `internal/templates/jobs_artifact_link_test.go`
  inverted: they now assert the POST form + button are NOT
  rendered. Closes #166.

### Fixed

- Floating dock (Scratch Pad / Feedback / Menu) no longer overlaps
  page content on `/compare`, `/calendar`, `/browse`, or the deep
  soldier routes. `applyResponsiveLayout` now measures the dock's
  rendered height via `getBoundingClientRect()` and writes the
  result to both the `--floating-dock-height` CSS variable on
  `<html>` AND `.app-shell` `padding-bottom` directly. The CSS
  variable is exposed in `frontend/index.html`'s inline `<style>`
  (the Tailwind minifier strips unused `:root` variables, so the
  declaration lives outside the scanned CSS bundle). The direct
  `padding-bottom` write is the binding effect that prevents overlap
  today; once the build pipeline gains CSS-variable awareness the
  direct write becomes redundant. Historical baseline padding values
  (7.5rem / 9rem / 9.5rem) preserved as the relaxed-mode default;
  the JS measurement only kicks in when the dock grows (split-screen
  wrap). Per `docs/COMMON_BUGS.md §4.14` this is the 6th attempt at
  fixing dock-vs-content spacing; the JS-measured value is the
  prescribed single source of truth. Closes #160 (audit r1 top-2).

### Fixed

- `internal/confederatehomestatus.Normalize` used to silently rewrite any unknown status value to "N/A" (the default branch fell through to the N/A case). Real bug, surfaced while reviewing issue #23 (schema-level normalization cleanup). Effect: (a) a user filtering browse by a non-canonical value like "Resident" got 0 results because the filter got normalized to "N/A"; (b) any non-canonical stored value (legacy data, imported backups, direct SQL) was silently re-bucketed as "N/A" on the next browse. Mirrored the pattern in `internal/pensionstate/pensionstate.Normalize` which was already correct: unknown values now pass through (trimmed); only the documented legacy "not applicable" variants ("", "none", "na", "n/a", "not recorded") collapse to the canonical N/A bucket. Three new tests in `internal/confederatehomestatus/confederatehomestatus_test.go` pin the contract for canonical, legacy, and unknown values. `go test ./... -short` passes; the existing browse filter test (which inserts a "Resident" row and expects 3 N/A matches out of 4) still passes because the SQL CASE was already correctly preserving stored values \u2014 only the Go function on the filter-input path was wrong. Issue #23 (partial).

- Three pre-existing audit-workflow gaps closed together with the
  pkg/render build-tag fix (`8503f3a`):
    1. **Missing templ-generate step.** `internal/templates/*_templ.go`
       is gitignored (generated from .templ source), so a fresh CI
       checkout has only the .templ files plus the plain .go files.
       The plain .go files (e.g. `linked_text_support.go`) call
       helpers like `isSoldierEntry` that are defined in the
       generated `*_templ.go` files. Without regenerating templ, the
       audit workflow's `go build` failed with
       `undefined: isSoldierEntry`. The test workflow already had
       this step (test.yml:39-44); audit was missing it. Added a
       `Regenerate templ files` step that runs
       `go run github.com/a-h/templ/cmd/templ@v0.3.1001 generate`
       before the build step.
    2. **Path filter too narrow.** The pull_request trigger was
       limited to `internal/templates/**`, `frontend/**`,
       `audit/**`. Changes to `pkg/`, `cmd/`, or
       `.github/workflows/audit.yml` itself did NOT trigger the
       audit. The pkg/render fix (PR #153) demonstrated this: the
       audit never ran on the branch, so the fix landed without
       CI confirmation. Expanded to `internal/**`, `pkg/**`,
       `cmd/**`, `frontend/**`, `audit/**`,
       `.github/workflows/audit.yml`. The audit workflow is now
       self-triggering on its own edits.
    3. **Server port mismatch.** The `Start dev server` step
       polled `http://localhost:8080/` for readiness, but the
       server command was
       `./build/bin/dixiedata-web -scratch-dir .scratch/webmode`
       — the default port is 8765 (see cmd/dixiedata-web/main.go),
       so the wait-for-readiness check timed out after 30s and the
       workflow failed even though the server was running. Added
       `-addr 127.0.0.1:8080` to the boot line. The poll loop was
       already correct; it was just polling the wrong port.

  All three changes match the existing test.yml pattern. Verified
  end-to-end: `gh workflow run audit.yml --ref
  fix/audit-workflow-templ-generate` returns `success` after the
  three fixes. Risk: low. `templ generate` is idempotent. Broader
  path filter increases audit CI usage — a few extra runs per
  PR, but each is Ubuntu + cached deps + takes ~3-4 minutes.

- CI audit workflow on ubuntu-latest was failing because
  `pkg/render/renderers.go` referenced `syscall.SysProcAttr{
  HideWindow: true, CreationFlags: 0x08000000}` directly. Those
  fields only exist on Windows; the Linux build failed with
  `unknown field HideWindow in struct literal of type
  "syscall".SysProcAttr`. The runtime.GOOS check inside the
  function body did NOT help \u2014 Go still type-checks the literal
  on every platform, so the Linux compile unit failed even though
  the function was never called on Linux.

  Split `hideWindow` into two files with build tags, matching
  the established convention in `internal/archive/pdfium_{windows,
  nonwindows}.go`:
    - `pkg/render/renderers_windows.go` (`//go:build windows`):
      real Windows impl that sets `SysProcAttr{HideWindow: true,
      CreationFlags: CREATE_NO_WINDOW}`.
    - `pkg/render/renderers_nonwindows.go` (`//go:build !windows`):
      no-op stub with the same signature, for Linux + macOS.

  The cross-platform test `TestHideWindowExistsAllPlatforms` in
  `pkg/render/renderers_build_tags_test.go` pins the contract
  from the caller's perspective. A second Linux-only test
  `TestRenderersBuildTagsLinux` in `renderers_build_tags_linux_test.go`
  (`//go:build linux`) exists so a future contributor who
  removes the `//go:build !windows` tag from the non-Windows
  stub will see the file fail to compile on the audit workflow's
  Linux runner. Verified: `GOOS=linux GOARCH=amd64 go build ./...`
  succeeds; `go test ./... -short` on Windows passes.

  Root cause pattern: a single commit (`6f096e9`) added
  Windows-only code to a non-Windows-tagged file. The audit
  workflow has been failing on every PR since then, but the
  failure was masked because `test` and `build` workflows run
  on Windows and pass. The fix is the `//go:build` split, but
  the broader design principle is: any time you reach for
  Windows-specific `syscall` fields, the call goes in a
  `{name}_windows.go` file. `docs/agents/dialog-guard.md` and
  the new `pkg/render/renderers_{windows,nonwindows}.go` files
  make this explicit. Pattern: `internal/archive/pdfium_{windows,
  nonwindows}.go` (added 2026-05-30 in `2839768`).

### Fixed


- CI `test` workflow started failing on
  `TestHandleJobArtifactAttachmentForDownloadTypes` (`.csv` case
  returned 409 instead of 200) and
  `TestJobReportHandlerReturnsSummaryForFinishedJob` (report body
  missing "Backup archive complete"). Two related races that
  earlier ran reliably on fast runners but tripped on the GitHub
  Actions runner once `ec451f4`'s reloadServices change altered
  the registry re-allocation timing:
    1. `seedArtifactJob`'s worker closure captured `id` by
       reference and raced the `id = app.jobs.Start(...)` assignment
       below. On a fast worker pool the goroutine fired while `id`
       was still `""`, `SetResultPath("")` was a no-op, and the
       subsequent GET returned 409 because the snapshot had no
       ResultPath. Fixed by binding the ID into the closure via an
       `atomic.Value` indirection so the worker reads the value
       assigned *after* `Start` returns.
    2. `TestJobReportHandlerReturnsSummaryForFinishedJob` fired
       the report endpoint synchronously after `Start`, racing
       the worker goroutine's transition to `StatusDone`. Fixed
       by polling for `StatusDone` (with a 2s ceiling) before the
       request.
  Both fixes are test-only; the production code path was always
  correct.
- `openJobsRegistry(dataDir)` ran before `db.Open(dataDir)`
  created the parent directory, so on a fresh install the jobs
  JSONL log silently failed to open and every job state change
  was dropped until the next app restart (which then saw no log
  and started empty). Added an `os.MkdirAll(dataDir, 0o755)`
  before opening the log so the persistence layer is actually
  wired on first launch.

- Several in-progress toast messages and progress-label
  attributes shipped the seven-char ASCII literal `\u2026`
  instead of the actual U+2026 HORIZONTAL ELLIPSIS rune
  (issue #135). Go does **not** interpret `\uXXXX` inside
  ordinary double-quoted strings — it ships the raw bytes
  `\`, `u`, `2`, `0`, `2`, `6` verbatim. The browser then
  surfaces mojibake like `Shared archive import startedâ¦`
  on the toast. Fixed 10 occurrences across `imports_handlers.go`,
  `google_handlers.go`, `insights_handlers.go`,
  `reviews_handlers.go`, `settings_handlers.go`,
  `entry_form.templ`, `recovery.templ`, and `soldier_card.templ`
  by replacing the broken escape with the actual `…` character
  in the source. Added a source-level regression net
  (`TestInProgressToastStringsContainActualEllipsis`) that walks
  every production `.go` file under `internal/appshell/` and
  fails the test if any non-comment, non-backtick-raw-string
  line contains the seven-char literal `\u2026`. Backtick raw
  strings are exempt because the JS engine resolves the escape
  at runtime — the broken form only affects Go double-quoted
  string literals.

- `reloadServices()` was unconditionally replacing `a.jobs` with a
  fresh empty `jobs.Registry`, which silently dropped every job in
  two contexts:
    1. **App startup**: `lifecycle.go` had already wired the
       persistent `jobs.jsonl` rehydrated Registry into `a.jobs` on
       line 141; `reloadServices()` then ran on line 210 and
       discarded it, so the `jobs.jsonl` persistence layer was
       effectively dead code — no job that survived a previous
       session ever appeared in the new session's registry.
    2. **`.ddbak` restore**: `handleImportBackup`'s worker calls
       `a.reopenDatabase()` after replacing the data dir.
       `reopenDatabase()` runs `reloadServices()`, which used to
       replace `a.jobs` while the `backup_import` job was still
       running. The user landed on `/jobs/{id}` (rendered fine from
       the pre-reload registry), but the 2s `hx-get` poll against
       `/jobs/{id}/status` started returning 404 the moment the
       reload happened — the page logged a flood of
       `htmx:responseError` events and never showed the final
       summary card, even though the import itself succeeded.
  Now `reloadServices()` preserves `a.jobs` when one is already
  wired and only allocates a fresh Registry on the very first call
  (the test-bypass-startup path where `NewApp()` leaves it nil).
  Two regression nets pin the contract: pointer-identity check
  across multiple reloads and an in-flight `Start`-then-reload
  round-trip that asserts `Get(jobID)` still returns ok.

- Shared import re-copied every image and inflated the
  `ImagesUpdated` counter on a full-duplicate archive (issue
  #136). The job report surfaced `Images inserted: 1140` even
  though every Person Record was filtered as a duplicate and no
  net change happened. Two fixes:
    1. `copySharedImageFile` short-circuits when the target file
       already exists with the same byte count. Sharded image
       filenames are derived from content hashes, so size-equal
       means same content for any well-formed export. Avoids
       touching the file on disk and keeps mtime stable.
    2. `upsertSharedImage` now compares the pre-update
       `file_name`, `file_path`, `caption`, and `is_primary`
       columns against the incoming row and only flags the row
       as `changed` when at least one of those columns differs.
       The merge loop only increments `summary.ImagesUpdated`
       for changed rows. Memorial import call site updated for
       the new 3-return-value signature.
  Regression net: new
  `TestBackupService_ImportSharedBackupImageDedup` builds a
  source archive with one image, imports it twice into the same
  target, and asserts the second import reports zero inserts /
  zero updates AND the on-disk file's mtime is unchanged
  (`os.Chtimes` to a stable time, then `time.Time.Equal` after
  the second import).

- Share → "Export Feedback Log" button appeared to do nothing on
  click (issue #137). The handler returned a 200 response directly
  while `dispatchDixieDataForm` only writes the response body into
  a target div when the form opted into `data-results-target` (added
  by the issue #134 fix). For every other click target the dispatcher
  stashed the toast in `sessionStorage` but never re-rendered it
  because the success path never invoked `initializeDynamicContent`.
  The `/export/feedback-log` surface now mirrors the Bug Report
  Bundle pattern (`handleExportBugReport`): the file copy runs
  inside `enqueueExport` and the user lands on `/jobs/{id}` for a
  progress card + final summary, matching every other export on
  `/share`. The no-feedback-yet branch returns a 200 +
  `X-DixieData-Toast` header so the dispatcher renders the
  empty-state toast on the share page. Regression net: new
  `TestHandleExportFeedbackLogEmptyState` pins the empty-state
  response shape (toast header, info kind, no redirect), and the
  existing smoke assertions on `/export/feedback-log` continue to
  verify the success path through `enqueueExport`. The carve-out
  comment in `audit/smoke.mjs` for the feedback-log path was
  updated to reflect the new behaviour (the carve-out still
  applies to the empty-log case, which legitimately stays on
  `/share`).

- Settings → "Scan for Orphaned Images" and "Run Data Quality Scan"
  buttons appeared to do nothing on click (issue #134). The forms
  used `data-dixie-submit` so the click hit `dispatchDixieDataForm`,
  which read the response headers but discarded the response body.
  The handlers were returning rendered HTML fragments
  (`SettingsOrphanedImages`, `SettingsQualityScanResults`) that
  never landed in `#settings-orphan-results` /
  `#settings-quality-results`. Added a `data-results-target`
  convention: when a form opts in via `data-results-target="#id"`,
  the dispatcher writes the response body into the matched element
  and re-runs `initializeDynamicContent` on the subtree (mirrors the
  browse-view refresh pattern). Wired the convention onto both
  scan forms in `entry_form.templ`. Regression net: new
  `TestSettingsOrphanScanEndpointRendersResults` asserts the orphan
  handler still returns 200 + the empty-state marker (no
  `X-DixieData-Redirect` / `Location` header), and `audit/smoke.mjs`
  now submits both scan forms against the live `dixiedata-web` server
  and asserts the result divs are non-empty.

- Toast text still rendered as mojibake after issue #135 shipped
  the real U+2026 / U+2014 runes into the source. The source was
  correct (`curl -i` shows valid UTF-8 bytes on the wire), but
  Chromium / WebView2 decode HTTP/1.x response headers as
  Windows-1252, not UTF-8, per the WHATWG Fetch spec — every byte
  above `0x7F` gets reinterpreted as a separate codepoint,
  producing `Shared archive import startedâ¦` on the toast.
  Introduced `sanitiseToastForHeader` next to `setToastHeader` /
  `setToastHeaderWithType` in `exports_handlers.go`. Source code
  keeps the polished Unicode characters; the helper rewrites a
  short table of common punctuation to ASCII twins (`…` → `...`,
  `—` → `--`, `–` → `-`, curly quotes → straight, NBSP → space,
  `→` → `->`, `✓` → `OK`, `·` → `*`, `§` → ``) at the boundary
  where the toast text enters the response header. User-data
  characters (accented Latin, CJK) pass through unchanged so
  future toasts that quote user input are not silently mangled.
  Every existing `setToastHeader*` call site benefits without
  changes — the substitution is centralised at the contract
  boundary. Captured the decision in
  `docs/adr/0005-toast-header-ascii-safe.md`. Regression net:
  `TestSanitiseToastForHeaderReplacements` pins every table entry
  including ASCII / user-data passthrough and empty input;
  `TestSetToastHeaderAppliesSanitisation` asserts no byte above
  `0x7F` reaches the wire; `TestToastHeaderSourceStillContainsUnicode`
  pins the contract that source keeps the polished characters so
  future contributors update the table instead of stripping
  Unicode at the source. The existing
  `TestInProgressToastStringsContainActualEllipsis` source sweep
  is updated to allow legitimate single-quoted rune literals
  (`'\u2026'`) which were previously false-positives after the
  helper table landed.

- `pkg/render.SoldierLister` interface removed (issue #143). The
  interface was declared but never referenced outside its own
  declaration site — `grep -rn "SoldierLister" pkg/` matches
  only `pkg/render/render.go` itself. The accompanying doc
  comment claimed the interface existed "so the render package
  does not import internal/records transitively," but the file
  already imports `internal/records` for `AnalyticsSnapshot` /
  `AnalyticsCount` re-aliases, so the rationale was stale. No
  call sites to update (interface was dead); `pkg/exportbridge`
  uses `*archive.SoldierService` directly via its own
  `BulkRenderer` type. `pkg/render` still imports `records` for
  the analytics re-aliases; that import is documented and
  load-bearing.

- Architectural boundary test tightened (issue #141). Two new
  layers of enforcement in `internal/architecture/architecture_test.go`:
    1. `forbiddenByPackage` now covers the grey-box layer too:
       `internal/viewmodel` is forbidden from `appshell`,
       `a-h/templ`, `wails`, and `templates` (the delivery
       surface); `internal/presentation` is forbidden from
       `appshell` and `wails` (templ is allowed because
       presentation IS the templ-rendering adapter). Both
       packages are still allowed to import deeper modules
       (`records`, `archive`, `models`, `jobs`, `update`, `debug`)
       because that is their documented grey-box role.
    2. New `TestPkgImportsAreAllowlisted` + the
       `allowedInternalImportsPerPackage` table enforce that each
       `pkg/*` package only imports the `internal/...` types it
       genuinely needs. Allowlists mirror the current imports:
       `pkg/render` → `{models, records}`, `pkg/exportbridge` →
       `{archive, db, models}`, `pkg/encode` → `{buildinfo,
       models}`, `pkg/templatespec` → `{}`. Any new `internal/`
       import requires updating the allowlist in the same commit.
  Also: `TestArchitectureMapsToContract` now requires
  `internal/viewmodel` and `internal/presentation` to be in the
  forbidden table. No production code changed.

- `internal/services/` shim deleted (issue #142). The 89-line
  shim was 55 type/func re-exports of `records`, `archive`,
  `integrations`, and `db` symbols with zero behavioral
  purpose. The three consumer files (`cmd/gold-master/main.go`,
  `cmd/gold-master/portability.go`, `internal/seed/seed.go`) now
  import the deep modules directly. `services.NewSoldierService`
  → `records.NewSoldierService`,
  `services.NewExportService` → `archive.NewExportService`,
  `services.NewBackupService` → `archive.NewBackupService`,
  `services.NewAnalyticsService` → `records.NewAnalyticsService`,
  `services.PrintSettings` / `BackupManifest` /
  `SharedImportSummary` / `SoldierService` → `archive.*` /
  `records.*`. The boundary test from issue #141 now guarantees
  no future re-introduction of `internal/services/` — if a new
  file accidentally re-imports it, CI fails.

- Feedback modal no longer silently swallows confirmation. Saving
  feedback through the floating-dock modal used to close the
  window and queue a toast for the next page nav — but no nav
  fires on the close-feedback path, so the toast never displayed
  and the user saw a closed modal with no acknowledgment. Two
  coordinated changes:
    1. `internal/templates/layout.templ`: the feedback form now
       carries `data-dixie-submit` + native `action=`
       + `method="post"` instead of relying on the htmx-only
       `hx-post` / `hx-swap="none"` wiring. The htmx-attrs were
       never read by the `app.js` dispatcher; without
       `data-dixie-submit` the form was htmx-only, htmx fired
       the POST, and the `X-DixieData-Close-Feedback` /
       `X-DixieData-Toast` headers were dropped on the floor.
       `action=` + `method="post"` + `data-dixie-submit` routes
       through the existing dispatcher (matches the
       calendar PDF export form pattern, the only
       previously-working form of this shape).
    2. `frontend/app.js`: when the dispatcher reads
       `X-DixieData-Close-Feedback`, it (a) hides the modal
       (existing), (b) **clears the form** via `form.reset()`
       so the next open starts blank and the save is visible,
       and (c) **renders the toast immediately** via
       `showToast(...)` instead of queueing via
       `savePendingToast(...)`. The trailing `savePendingToast`
       is suppressed for the close-feedback path so the same
       toast isn't queued for a nav that will never happen.
  `audit/smoke.mjs` grows a `[7d]` block with six
  end-to-end assertions: `feedback-modal-openable`,
  `feedback-save-sends-close-header`,
  `feedback-save-sends-toast-header`,
  `feedback-save-hides-modal`, `feedback-save-clears-form`,
  `feedback-save-shows-toast`.

- `docs/COMMON_BUGS.md` grown with five new sections from the
  60-day UI fix survey: �1.10 `redirect-contract-drift` (7
  instances), �1.11 `htmx-attr-strip-by-boot-js` /
  `data-dixie-submit` opt-in (3 instances), �3.5
  `stale-status-panel-after-submit` (4 instances), �4.11
  `duplicate-job-handling` (3 instances), �4.12
  `toast-encoding-mojibake` (2 instances), �4.13
  `route-misregistered-or-wrong-verb` (2-3 instances), �4.14
  `floating-dock-layout-overlap` (4 instances). �1.9 status
  updated from �Eliminated� to �REGRESSION-PRONE� with a
  pointer to the new �1.10. The �Bug class → first place to
  look� table at �11 grows rows for each new pattern. New file
  `docs/agents/bug-pattern-grep.md` is the copy-paste grep
  cookbook for all 8 patterns: one section per pattern with
  the grep, the false-positive filter, and a link back to the
  canonical recipe in `COMMON_BUGS.md`. No code change.

- Manual UI audit playbook (this slice). Three new artifacts:
    1. `docs/agents/manual-audit-playbook.md` — guided protocol
       for walking every UI surface by hand, capturing findings,
       and filing them as GitHub issues. Includes a “what to look
       for” checklist (visual / behaviour / a11y / performance /
       data integrity), per-finding templates for [BUG] /
       [FEATURE] / [CORRECTION], and a growth path.
    2. `audit/run-interactive.mjs` — Playwright walker that
       automates the deterministic parts (page loads, form
       submits, network round trips, screenshot capture) and
       flags `? (manual)` for the human-only checks. Writes
       `audit/audit-interactive-report.json` (machine-readable
       summary) + `audit/screenshots-interactive/<surface>-{before,after}.png`.
       Surfaces covered: calendar, soldier-new, browse, share,
       settings, feedback-modal, floating-dock-layout, jobs-page.
       18 auto checks pass on the current `dev`.
    3. `docs/agents/audit-notes-TEMPLATE.md` — drop-in template
       for capturing findings during a manual audit round. One
       block per finding. Links to COMMON_BUGS.md pattern
       reference and to the playbook for the full protocol.
  Phase 1 deliverable complete. Next: Phase 2 (smoke.mjs
  expansion + Wails-free test path via OpenDirectoryDialog +
  BrowserOpenURL override hooks).

- `internal/appshell/runtime.go` grows two override hooks
  matching the existing `SetOpenFileDialogOverride` /
  `SetSaveFileDialogOverride` / `SetOpenMultipleFilesDialogOverride`
  pattern. The web-mode binary now installs them via the
  `DIXIE_OPEN_DIRECTORY_DIALOG_PATH` and
  `DIXIE_BROWSER_OPEN_URL_LOG` env vars, closing two of the
  four Wails-only gaps that the smoke harness could not reach:
    1. `SetOpenDirectoryDialogOverride` lets the
       "Download images to folder" and "Choose where to copy
       record images" flows run end-to-end in the audit
       harness. Without it, the web-mode binary returns
       `errWailsFrontendUnavailable` and the user sees an
       uninformative toast.
    2. `SetBrowserOpenURLOverride` records the requested
       `file://` URL into a log file so the audit harness can
       assert the "Open result" + "Open log folder" flows
       land the right path. Without it, the user sees an
       info-toast "Open in OS file manager" fallback and the
       harness has no way to assert correctness.
  Both overrides follow the same precedence as the existing
  three: hook first, frontend guard second, real Wails call
  last. Four new unit tests in `runtime_test.go` cover the
  override-takes-precedence + without-override-still-sentinel
  pattern for each. The `runtime.go` and `runtime_test.go`
  changes are the only Go changes in this slice; the next
  slice wires the new hooks into the smoke harness.

- `audit/smoke.mjs` grows a `[9]` block that exercises the two
  new Wails-free hooks end-to-end against the live server.
  `[9a] jobs-open-button-uses-browser-open-override`: re-seeds
  a job via `/export/json` from `/share`, navigates to
  `/jobs/{id}`, polls the job until terminal, POSTs directly
  to `/jobs/{id}/open` (bypassing the polling overlay via
  `page.request.post`), then reads the
  `DIXIE_BROWSER_OPEN_URL_LOG` file and asserts a `file://`
  URL was recorded. `[9b] open-directory-dialog-override-is-wired`:
  indirect assertion that confirms the runtime_test.go suite
  covers the override precedence. The summary line at the
  end of the smoke now reports a “skipped” count alongside
  pass/fail so a missing env var is visible in the output
  (not a failure). 58 pass / 1 fail / 0 skipped after the
  change; the `memorial-import-flow` carve-out remains the
  only failure, unrelated.

- `audit/run-interactive.mjs` grows a Phase 2 surface
  `jobs-open-artifact` that exercises the new
  BrowserOpenURL override hook end-to-end. Re-seeds a job via
  `/export/json` from `/share`, navigates to `/jobs/{id}`,
  polls until terminal, POSTs directly to `/jobs/{id}/open`
  via `page.request.post()` to bypass the polling overlay,
  then reads `DIXIE_BROWSER_OPEN_URL_LOG` and asserts a
  `file://` URL was recorded. The summary line now reports
  a 'skipped' count alongside pass/fail/manual. 19/0/4/0 after
  the change. Phase 2 surface coverage closes the BrowserOpenURL
  gap from the Wails-free test feasibility audit.

### Maintenance

- Stopped `dixiedata-web.exe` from leaking across probe runs.
  Three audit probes (`audit/probe-backup-status.mjs`,
  `probe-full-restore.mjs`, `probe-share-status-scroll.mjs`)
  used `go run ./cmd/dixiedata-web` + a `finally` cleanup that
  could not reach the grandchild process tree on Windows, so a
  Ctrl-C or thrown error left the server running and the next
  `make debug` failed with `unlinkat ... dixiedata-web.exe: The
  process cannot access the file`. Switched the probes to spawn
  the prebuilt `build/bin/dixiedata-web.exe` directly and use a
  new `audit/_lib/cleanup.mjs` helper that installs SIGINT /
  SIGTERM / uncaughtException handlers and taskkills the named
  exe as a safety net. Added `make probe-clean` to nuke any
  straggler `dixiedata-web.exe` / `DixieData.exe` /
  `seed-data.exe` / `gold-master.exe` processes; `make debug`
  and `make build` now run it automatically before rebuilding
  the sibling binaries.

### Changed

- The recurring "export options status pages not landing" bug
  is fixed at the architecture level. Every post-then-navigate
  flow (export buttons, import buttons, merge-review actions,
  delete confirmations, settings toggles, soldier create/update)
  now navigates reliably because the contract is single-sourced.
  The browser always lands on the destination page or back on
  the originating page with a clear toast on dedup — never
  silently in the background. (Verified end-to-end via the
  dev-server smoke harness; Wails desktop smoke is manual — see
  `docs/adr/0004-option-c-dispatcher.md` for the rationale.)

### Fixed

- Web-mode (`cmd/dixiedata-web.exe`) save-dialog exports
  (`/export/json`, `/export/csv`, `/export/ical`,
  `/export/backup`, `/export/shared-archive`,
  `/export/database-pdf`, `/export/bug-report`) silently
  bounced users back to `/share` because the binary never
  installed `SetSaveFileDialogOverride`. Wired the override
  (commit `30ab8e7`) so the web-mode binary auto-routes every
  export to `<DIXIE_SAVE_FILE_DIR>` (defaulting to
  `<dataDir>/exports/`). The Wails desktop binary is
  unaffected — it has a real native `SaveFileDialog`.
- Split `guardedSaveFileDialog`'s outcome into three states:
  `SaveOutcomeOK`, `SaveOutcomeDuplicated`,
  `SaveOutcomeDialogAborted` (commit `14a2aa8`). The old
  bool-shape collapsed "duplicate in flight" and "user
  cancelled" into one branch, which was the proximate cause
  of the misleading "Export already in progress" toast
  surfaced on every cancel. Handlers updated for all 9
  save-dialog-backed exports plus `handleExportFeedbackLog`.
  The `(*App).inFlight` dedup map stays — the Wails v2.12.0
  UI-thread crash from two simultaneous native dialogs is
  still real even though the dual-JS-handler race is gone.
- Audit smoke harness tightened to require `/jobs/{id}`
  specifically for non-carve-out exports (commit `c9e5da3`),
  with two documented carve-outs: `/export/static-archive`
  (plain `<form method="post">` carve-out, follows 303
  natively) and `/export/feedback-log` (no-data early
  return). The previous `/share`-as-success acceptance
  masked the missing save-dialog override.

### Maintenance

- Replaced `frontend/app.js`'s custom htmx-clone dispatcher
  (`request()`, plus all helper functions) with a 32-line
  `dispatchDixieDataForm`. Net -411 lines from `app.js`.
- Migrated 13 Go handlers from `303 + Location + HX-Redirect`
  to `200 + X-DixieData-Redirect` via the new `writeExportRedirect`
  helper. `handleExportStaticArchive` opts into
  `enqueueExportOpt{NativeRedirect: true}` to keep the 303 path
  for its plain-`<form method="post">` carve-out.
- Retagged 9 templ files (`calendar`, `calendar_day`, `entry_form`,
  `insights`, `research_collections`, `research_log`,
  `review_queue`, `share`, `soldier_card`) from
  `hx-post`/`hx-put`/`hx-delete`/`hx-confirm` to
  `action`/`data-action` + `data-dixie-submit` + `data-confirm`.
  ~75 attribute changes. htmx stays loaded for GET-only polling
  on `/jobs/active` and `/jobs/{id}`.
- Registered `htmx.on("htmx:load", ...)` to re-init swapped
  subtrees. Polling fragments swap fresh DOM every 2–3s; without
  re-init, JS handlers on those subtrees never re-bind.
- Restored the 200ms debounce on the browse-filter change
  handler. The legacy `queueRequest` had it; it was dropped in
  the initial dispatcher rewrite because the harness test waited
  50ms. Restoring it prevents fetch storms on rapid filter
  changes (e.g. typing in a select).
- Trimmed the dead `hx-post` / `hx-delete` / `data-hx-*` selectors
  from the dispatcher interceptors. After the templ retag, no
  elements match those selectors; the translator window is gone.
- Rewrote `internal/templates/components/conventions.md` §"Buttons
  that POST and expect navigation" to describe the Option C
  contract instead of the dead `HX-Redirect` recipe. Without
  this rewrite, the next author would write the same broken
  contract the bug class was built on.
- Replaced `docs/COMMON_BUGS.md` §1.9 (the original
  "export-options-status-pages-not-landing" bug postmortem)
  with a short pointer to the new contract and the regression
  nets that prevent reintroduction. The postmortem's "fix"
  (adding `HX-Redirect`) is documented as dead code so the
  next reader understands why the section was removed.
- Wrote `docs/adr/0004-option-c-dispatcher.md` capturing the
  architectural decision (why the bug class recurred, what the
  new contract is, which regression nets guard it).

### Added

- Three source-scan regression nets that fail the build if the
  Option C bug class is reintroduced:
  `TestPostThenNavigateUsesDixieRedirect` (appshell) — fail on
  303 writers without `X-DixieData-Redirect`.
  `TestNoPostThenNavigateHXXAttrs` (templates) — fail on any
  `hx-post` / `hx-put` / `hx-delete` / `hx-confirm` in templ.
  `TestNoDeadHXRedirectWrites` (appshell) — fail on any handler
  writing `HX-Redirect`. Together they form a tripwire: any author
  who tries to write the old contract hits a build failure with a
  file:line citation.
- `audit/discover_export_buttons.mjs` learned the `data-action`
  literal pattern so the auto-discovery for smoke tests still
  finds every share-page button after the templ retag.
- `audit/smoke.mjs` `share-${btn.path}-navigates-to-jobs` asserts
  the user-visible contract (page lands on `/jobs/{id}` or back at
  `/share` on dedup) instead of asserting a specific response
  shape, so the contract switch can't silently regress navigation.

- `/jobs/{id}` summary cards now show per-kind stats so the
  user can see what an export or import actually contained
  without re-opening the artifact. Six Wails share-page
  exports and three import flows were upgraded:

  **Exports** (kinds that surface `Person records:`,
  `Images:`, and/or `Source records:`):
  - JSON export → `Person records: N` (records count)
  - Excel export → `Person records: N`
  - iCalendar export → `Person records: N` (soldiers enumerated)
  - Printable archive PDF → `Person records: N` + `Images: N`
  - Backup (.ddbak) → `Person records: N` + `Images: N` +
    `Source records: N`
  - Shared archive (.ddshare) → same as backup

  **Imports** (kinds that surface the merge-review headline or
  the replace + schema migration line):
  - Shared archive import → `N added, N merged, N skipped`,
    plus `Conflicts staged for review: N` when >= 1 (so the
    user is reminded to open Merge Review), plus
    `Images imported: N`.
  - Memorial JSON import → `N added, N skipped, N failed`,
    plus `Images imported: N` when applicable.
  - Backup restore → `Replaced: N records, N images`, plus a
    schema line that reads `Schema migrated: backup vX → current vY`
    when the migration ran or `Schema: backup vX = current vY (no migration)`
    when schema parity held.

  Lines render conditionally on the populated count (zero
  counts stay absent), so legacy kinds that don't fill the
  struct are unaffected.

- Plumbed end-to-end:
  - `internal/jobs/jobs.go`: new `JobResult` struct + `Job.Result`
    field + `Registry.SetResult` setter. Promotes `Path` to
    `ResultPath` so `/jobs/{id}/artifact` still streams when
    callers forget to call `SetResultPath` explicitly.
  - `internal/jobs/jobs.go`: `Summary()` now surfaces the new
    counts via four helpers — `appendExportStats`,
    `appendSharedImportStats`, `appendMemorialImportStats`,
    `appendBackupRestoreStats`. Each kind's existing copy is
    preserved; stats lines append only when populated.
  - `internal/archive/export_service.go`: new with-stats
    variants — `ExportJSONWithStats`,
    `ExportExcelWithStats`,
    `ExportICalendarWithStats`,
    `ExportFullDatabasePDFWithStats`,
    `ExportStaticArchiveWithStats`. Existing `ExportXxx`
    methods are unchanged; the CLI in
    `internal/appshell/cli_export.go` still calls the
    count-less variants because shell output does not surface
    per-record stats. When the CLI gains structured output it
    should switch.
  - `internal/appshell/app_facades.go`: facade lists the new
    with-stats methods so `a.export.ExportXxxWithStats` type-checks.
  - `internal/appshell/exports_handlers.go`: new
    `enqueueExportWithResult` helper alongside the existing
    `enqueueExport`. The six handlers that produce structured
    artifacts (`json_export`, `excel_export`, `icalendar_export`,
    `database_pdf`, `backup_archive`, `shared_archive`) now use
    it. The remaining kinds (`soldier_pdf`, `soldier_jpg`,
    `monthly_pdf`, `insights_pdf`, `image_import`, `bug_report`,
    `static_archive`) continue to use the original helper
    unchanged.
  - `internal/appshell/imports_handlers.go`: the three import
    workers (`backup_import`, `shared_import`, `memorial_import`)
    now call `SetResult` with the appropriate counts before
    returning nil. Memorial import also records `LogPath` so a
    future UI iteration can wire the error log download.

### Maintenance

- The global layout progress popup is now named consistently
  with the rest of the UI surface vocabulary:
  - `uiids.OverlayJobsProgress` is the canonical surface ID
    (kind: overlay). Added to `internal/uiids/uiids.go`
    alongside the other overlays (FloatingMenu, FeedbackModal,
    ImageViewer, etc.).
  - CSS class `progress-region` renamed to
    `jobs-progress-overlay` in `frontend/tailwind.css`.
  - Data attribute `data-progress-region` renamed to
    `data-jobs-progress-region` (follows the three-attribute
    namespace rule: `data-<feature>-...` for runtime hooks).
  - `hx-target` selector in
    `internal/templates/job_slot_fragment.templ` updated
    accordingly.
  - All 25 grep matches across 9 files updated: 5 test files
    (job_slot_swap_test.go, page_snapshot_test.go,
    jobs_handlers_test.go, audit/smoke.mjs,
    audit/probe-setup-stacking.mjs), 3 doc files (CHANGELOG,
    COMMON_BUGS, RESEARCH), and the live audit smoke
    assertion (renamed `progress-region-survives-polls` to
    `jobs-progress-overlay-survives-polls`).

### Fixed

- `internal/appshell`: duplicate export requests (issue #130) no
  longer strand the user on an error page. Each in-flight dedup
  key now stores the background `JobID` once the worker has been
  started, so a duplicate click that races against the save
  dialog roundtrip is redirected 303 to `/jobs/{id}` instead of
  replacing the modal/document with the "Export already in
  progress" body. When no `JobID` is known yet (the dialog is
  still open), the duplicate still receives an `HX-Redirect` +
  toast so the originating page stays put. Covers the five
  SaveFileDialog sites in `app.go` (soldier PDF / soldier PDF
  no-images / soldier JPG / calendar PDF / image screenshot),
  the printable-PDF flow in `exports_handlers.go`, and every
  `guardedSaveFileDialog` caller (`json`, `insights_pdf`,
  `excel`, `icalendar`, `static_archive`, `backup_archive`,
  `shared_archive`, `bug_report`, `feedback_log`).
- `scripts/build-common.ps1` + `scripts/build-debug.ps1`:
  `make debug` now actually builds a debug binary. Previously
  the recipe passed `wails build -clean -trimpath` (a
  production build with stripped source paths) and only
  generated a thin launcher wrapper. The wrapper was a no-op
  that just re-exec'd the production binary. With this fix:

    - `Invoke-DixieDataBuild -DebugBuild` swaps the default
      Wails args to drop `-trimpath` and add `-debug`, which
      makes Wails:
      * Preserve source paths in DWARF (so dlv can set
        breakpoints by file:line; the existing
        `scripts/debug-crash.dlv` workflow now works as
        written).
      * Add `-gcflags=all=-N -l` automatically (Go's
        optimiser no longer elides frames or inlines past
        breakpoints).
      * Enable the WebView2 DevTools + default context menu
        in the running Wails app. `F12` / `Ctrl+Shift+I`
        now opens the inspector without rebuilding.

    - The `Run-DixieData-Debug.ps1` launcher regenerated with
      debug-friendly env defaults:
      * `GOTRACEBACK=all` — full stack on panic.
      * `DIXIEDATA_DEVTOOLS=1` — forces the Wails
        `EnableDefaultContextMenu` env-gate (new in
        `main.go`) to enable DevTools in any build, including
        a release binary launched via the debug launcher.
      * `DIXIEDATA_WAIT_FOR_DEBUGGER` — opt-in pause at
        process start so `dlv attach $PID` from another shell
        can attach before Startup runs.

  Regression net in `internal/appshell/build_flags_test.go`
  pins down: DWARF source paths present, 10k+ symbols, the
  launcher writes the new env vars. Skips cleanly when
  `build/bin/DixieData.exe` is absent so release-only CI
  doesn't fail.
- `internal/templates/jobs.templ`: the `/jobs/{id}` landing
  page (`JobStatusView`) was a static snapshot — it rendered
  the body of the page but did NOT include the `hx-get` /
  `hx-trigger="every 2s"` that drives the 2s poll. The page
  froze at the value captured in the 303 redirect even while
  the job ran to completion in the background. Fast exports
  (`static_archive` in particular) finished during the
  redirect window, so the user always landed on a page that
  read "running" / "queued" forever even though the artifact
  sat ready in `/jobs/{id}/artifact`.

  Fix: extract the body of the status page into a single
  `jobStatusBody` sub-template that both `JobStatusView` (the
  full page) and `JobStatusFragment` (the polling fragment
  served from `/jobs/{id}/status`) call. Now both render the
  same `id="job-status-body"` wrapper with the same `hx-get`
  / `hx-trigger` so the landing page polls automatically. The
  extraction also prevents the view and the fragment from
  drifting apart in future edits.

  Regression net:
  - `internal/templates/jobs_artifact_link_test.go`:
    * `TestJobStatusViewPollsForUpdates/running_job_wires_the_poll`
      asserts the page renders `hx-get="/jobs/{id}/status"`.
    * `TestJobStatusViewPollsForUpdates/done_job_stops_polling`
      asserts the page renders `hx-trigger="none"` when the
      job is done (so polling stops once the summary card
      is visible).
    * `TestJobStatusViewPollsForUpdates/view_and_fragment_share_the_poll_url`
      asserts the view and the fragment agree on the poll
      URL — the extraction cannot drift.
  - `internal/templates/page_snapshot_test.go`:
    `TestPageSnapshotJobsStatus` now also asserts the running
    page renders `hx-get="/jobs/job-abc/status"`.
  - `internal/appshell/jobs_handlers_test.go`:
    `TestHandleJobStatusFullPageWiresThePoll` is the
    end-to-end net: GET `/jobs/{id}` returns a body that
    wires the poll (holds the worker on a channel so the
    job stays running through the render).
- `internal/appshell/exports_handlers.go` +
  `internal/appshell/imports_handlers.go` +
  `internal/appshell/app.go`:
  Fixed the share-page export-lands-on-blank-page bug that
  hid the new per-kind stats summary card. htmx 2.x with
  `hx-swap="none"` silently swallows 303 responses unless the
  server also writes `HX-Redirect`; the export + import + dedup
  helpers only wrote `Location`, so the user clicked the
  button, the export ran to completion in the background, and
  the page silently stayed on `/share`. Now `enqueueExport`,
  `enqueueExportWithResult`, `respondDuplicateInFlight`, and
  the backup restore's in-flight redirect write both
  `Location` (for plain `<form method="post">` submits like
  static archive) and `HX-Redirect` (for htmx). Static archive
  was unaffected because it already uses a plain HTML form,
  not htmx.
  Regression net:
  - `TestEnqueueExportRecordsJobIDOnEntry` now also asserts
    `HX-Redirect`.
  - `TestImportBackupInFlightGuardRedirectsToExistingJob`
    same.
  - `TestEnqueueExportWithResultSetsHXRedirect` (new) pins
    both headers on the with-stats helper.
- `audit/smoke.mjs`: every share-page export button now also
  asserts `share-{path}-navigates-to-jobs` — after the click,
  `page.url()` must include `/jobs/`. The previous
  `share-{path}-redirects-303` assertion only checked the
  response headers; it did NOT prove the browser actually
  followed the redirect, which is how the htmx `hx-swap="none"`
  + 303 silent-swallow bug slipped through. Now the live
  harness catches both: response shape AND navigation.
- "Upload Backup to Google Drive" and "Export CSV to Google
  Sheets" share-page buttons now land the user on `/jobs/{id}`
  after the worker starts. Previously the two Google handlers
  wrote a `Location` header but no `HX-Redirect`, so with the
  buttons' `hx-swap="none"` htmx 2.x swallowed the redirect and
  the user stayed on `/share`. Pinned by
  `appshell.TestGoogleHandlersRedirectToJobs` (two assertions:
  `/integrations/google/backup` and
  `/integrations/google/sheets/export`) and the new
  `share-/integrations/google/backup-navigates-to-jobs` /
  `share-/integrations/google/sheets/export-navigates-to-jobs`
  smoke assertions.
- The Printable PDF export modal (Share → "Printable PDF…")
  now lands on `/jobs/{id}` instead of dumping markup into the
  `#share-status` panel. Dropped the Wails-bridge JS interceptor
  in `app.js::submitPrintConfig` and the brittle
  `hx-on::after-request` 303 shim on the modal form, and made
  the form a plain htmx form that relies on
  `handleExportDatabasePDF`'s existing `HX-Redirect` header
  (same pattern as every other share-page export). Pinned by
  the new `[5b]` smoke block.
- `internal/appshell` 303-redirect handlers now ship HX-Redirect
  alongside Location so `hx-swap="none"` buttons land the user
  on the destination page instead of silently swallowing the
  redirect. Five additional handlers were missed by the original
  3612dab sweep and were repaired in the same commit that added
  the global guard:
  - `handleImportSoldierImages` (`app.go`)
  - `handleRunDuplicateAudit` (`insights_handlers.go`)
  - `handleReviewQueueBulk` (`reviews_handlers.go`)
  - `handleCleanupImageOrphans` (`settings_handlers.go`)
  - `handleCreateSoldier` / `handleSoldierByID` (DELETE branch) /
    `handleUpdateSoldier` (`soldiers_handlers.go`)
  The new `appshell.TestAll303sWriteHXRedirect` walks every
  function in the package, finds every `StatusSeeOther` write,
  and asserts a sibling `HX-Redirect` is set on the same
  handler (with an explicit allow-list for server-initiated
  middleware redirects). Verified to fail when the header is
  removed and pass when restored; the allow-list requires a
  one-line reason per exempt function so the next reader knows
  why no htmx button reaches it.
- `audit/smoke.mjs` now auto-discovers share-page export buttons
  by scanning `internal/templates/*.templ` instead of
  hand-maintaining the `shareButtons` array. New export routes
  added to `share.templ` are covered by `share-{path}-navigates-
  to-jobs` assertions without manual harness edits. The new
  `audit/discover_export_buttons.mjs` walks every form and bare
  button, resolves label inference for both `components.Button`
  and `components.ButtonContent` patterns, and gates inclusion
  on an explicit override table (`builderPrefixOverrides` for
  routebuilder-driven buttons, `literalPathOverrides` for
  literal-string hx-post paths, `actionPathOverrides` for plain
  `<form method="post">` actions). The companion
  `discover_export_buttons.test.mjs` pins the manifest shape
  (10 canonical share-page buttons, Google Calendar / connect
  / disconnect excluded, printable PDF modal excluded because
  its dedicated `[5b]` smoke block covers it). The hand-written
  `shareButtons` array now derives from the discovery result.

### Maintenance

- **Doc consolidation for click-driven surfaces.** Five
  edits land in one commit so the htmx `hx-swap="none"` + 303
  trap and the surrounding patterns have a single source of
  truth:
  - `internal/templates/components/conventions.md`: new
    section "Buttons that POST and expect navigation" —
    recipe for the canonical `Location` + `HX-Redirect`
    pair, checklist for new POST-then-navigate handlers.
  - `docs/COMMON_BUGS.md`: new §1.9 — bug catalog entry with
    grep commands, root cause, fix recipe, and the regression
    net (audit/smoke.mjs `-navigates-to-jobs` assertion).
  - `AGENTS.md`: new "Commits and branches" section —
    one-commit-one-logical-change rule, message shape,
    branch naming, pre-push checks, CHANGELOG rule, and the
    cross-link to the new conventions rule for any new
    click-driven button.
  - `docs/ai-handoff.md`: new "Adding a feature: canonical
    workflow" section — 8-step skeleton (surface → routebuilder
    → service → handler → templ → regression net → verify →
    CHANGELOG) with cross-links to per-layer checklist docs
    and explicit warnings about the htmx + 303 trap.
  - `audit/smoke.mjs`: comment block above the share-page
    export assertions tightened to clarify that the success
    path (enqueueExport) writes BOTH Location AND HX-Redirect,
    not just the dedup-fallback path.

  `CONTEXT.md` Laws stays slim — the trap is documented in
  `conventions.md` (recipe) + `COMMON_BUGS.md` (postmortem),
  cross-linked from AGENTS.md.

### Maintenance

- `Makefile`: `make debug` now builds every sibling binary
  the debug workflow expects to be present:
  `build/bin/DixieData.exe`, `build/bin/dixiedata-web.exe`,
  `build/bin/seed-data.exe`, `build/bin/gold-master.exe`,
  `tools/tune/bin/dixiedata-tune.exe`. New standalone targets:
  `make web`, `make seed`, `make gold`, `make tune-bin` (the
  existing `make tune` target runs the render harness, so the
  build step is split off under a new name to avoid a
  collision). `migrate-logs` is intentionally NOT included —
  no script in this repo calls it; add it when a workflow needs
  it.
- `internal/jobs/jobs.go`: new `SilentKinds` set + `IsSilentKind`
  helper, and `Registry.MostRecentActive` filters out kinds in
  the set. The global layout progress popup is now opt-out
  per kind: jobs whose `/jobs/{id}` status page is the
  intended landing (and whose artifact does not preview well
  in a new tab) get filtered out so the floating popup card
  never appears. Kinds register by adding to the map; the
  call site (the export handler) is unchanged.

- `static_archive` is the first silent kind: clicking "Export
  Static Web Archive" used to render a popup card whose
  "Open result" link opened a blank tab (the artifact is a
  .zip, which falls through to `Content-Disposition:
  attachment` and the browser consumes the response in its
  download manager without rendering anything). With this
  fix the popup stays empty and the user lands on
  `/jobs/{id}` via the standard 303.

- `internal/jobs/jobs_test.go` +
  `internal/appshell/jobs_handlers_test.go`: 3 new tests pin
  down the contract (silent kinds are filtered, non-silent
  kinds still surface, `/jobs/{id}` still renders for the
  silent job so the user isn't stranded).

- `internal/templates/job_slot_fragment.templ`: comment now
  documents the SilentKinds filter so future authors know
  why some jobs don't show up in the popup.

- `audit/smoke.mjs`: closed the three live regression gaps
  that commit b185f0e deferred. New assertions cover:

    - `share-{path}-redirects-303` on every share-page export
      button (proves the issue #130 redirect path fires
      end-to-end; accepts either the Wails `Location: /jobs/{id}`
      header OR the `HX-Redirect: /share` fallback that web-mode
      uses because it has no native dialog).
    - `debug-console-panel-appends-beforeend` (proves the
      b185f0e beforeend swap fix is in place; without it the
      debug-mode toggle would wipe the document).
    - `jobs-progress-overlay-survives-polls` (proves the
      `JobStatusSlotFragment` `outerHTML`->`innerHTML` fix is
      in place; without it the progress bar would freeze after
      the first poll).

  Live regression net jumped from 26 to 32 assertions.
- `internal/appshell`: native OpenFileDialog, OpenDirectoryDialog,
  and OpenMultipleFilesDialog callsites now route through
  dedicated guarded helpers (`guardedOpenFileDialog`,
  `guardedOpenDirectoryDialog`, `guardedOpenMultipleFilesDialog`
  in `internal/appshell/exports_handlers.go`) so the
  WebView2 `Chrome_WidgetWin_0. Error = 1412` re-entry race
  is closed for the import flows the original save-dialog
  law deferred. Closes the "open question" item in
  `docs/agents/dialog-guard.md`. Covers
  `handleImportSharedArchive`, `handlePreviewMemorialJSONImport`
  (file pickers), `handleImportSoldierImages` (multi-file
  picker), and `handleDownloadSoldierImages` (directory
  picker). The 3-value return shape (`path, admitted, ok`)
  lets each handler distinguish dup-hit (redirect to
  `/jobs/{id}`) from cancel (validation error) without
  re-reading the in-flight map. Regression net:
  `internal/appshell/open_dialog_guard_test.go`.
- `internal/appshell`: new `/jobs/{id}/report` route renders
  the job's terminal-state payload on a printable layout
  (status, summary, timeline, artifact metadata, error log
  when present). Wired through the redesigned job status
  page's "Show report" button (issue #131 follow-up). New
  `renderJobReport` handler in `jobs_handlers.go` and
  `templates.JobReportView` in `jobs.templ`. Regression
  net: `internal/appshell/jobs_report_handler_test.go`.
- `internal/templates/jobs.templ`: redesigned the terminal-state
  status card around a structured summary (issue #131). The new
  `jobSummaryCard` renders a kind-specific headline + size +
  duration detail lines, a primary Dismiss button that routes
  back to the page that kicked off the export
  (`jobs.Job.DismissTargetPath()`), a Show report button that
  links to `/jobs/{id}/report`, and demotes the artifact action
  (Open / Save) to a secondary link. `jobs.Job.Summary()`
  owns the structured payload so the template stays declarative;
  `formatBytes` rounds file sizes to a user-friendly unit.
- `internal/appshell`: .ddbak restore now runs as a background
  job (issue #133). The handler reads the local identity,
  enqueues the restore, and 303-redirects the user to
  /jobs/{id} so they see real progress during the multi-second
  restore instead of being blocked on the HTTP goroutine.
  Replaces the synchronous `X-DixieData-Redirect: /` flow that
  left the user on a blank /share tab for 10+ seconds on a
  500 MB archive. A new `a.importInFlight` atomic flag + an
  `importInFlightJobID` global coordinate the worker; a second
  click during a running restore redirects to the existing
  /jobs/{id} instead of opening a second dialog or crashing.
  The toast text now reads "Restoring backup: <name>" (info
  kind, issue #132) and the user lands on a real status page.
- `internal/appshell`: in-progress toasts (image import,
  shared-archive import, memorial-JSON import, Google Drive /
  Sheets exports, duplicate audit, bulk reviews, orphan
  cleanup) now emit `X-DixieData-Toast-Type: info` instead of
  the default `success` (issue #132). Combined with the
  existing `success || info` auto-dismiss branch in
  `frontend/app.js`'s `showToast`, every "X started…" toast
  fades out after 4 s on both the originating page and the
  page the user lands on after the 303 redirect. New
  `setInfoToastHeader` helper centralises the kind so future
  in-progress sites cannot regress to success-by-default.
  Error and warning toasts keep the manual-dismiss contract
  from issue #54. The 4 s and 320 ms timing values are now
  named constants (`toastAutoDismissMs`, `toastFadeOutMs`)
  at the top of `app.js` so future tuning is one edit.
- `internal/templates/jobs.templ`: non-viewable job artifacts
  (.ddbak, .ddshare, .zip, .csv, .ics) now render with a `download`
  attribute instead of `target="_blank"` (issue #129). The old
  combination opened a blank tab and triggered a silent download
  that the user couldn't see or find. PDFs, JPGs, PNGs, and other
  viewable extensions still open in a new tab as before. New
  `jobs.Job.IsViewableArtifact()` + `jobs.Job.ArtifactFilename()`
  helpers own the classification so the template stays declarative.
- `internal/templates/share.templ`: print-config modal renders
  with the centering classes required to display the dialog in
  the middle of the page (`justify-center`, `items-center` on
  `>=sm` viewports). Issue #128 reported the modal "loading on
  the left of the page" — root cause was a duplicate export
  click replacing the modal contents with the in-flight error
  body, fixed by the issue #130 redirect. The new
  `TestSharePrintConfigModalIsCentered` test pins down the
  CSS classes so a future refactor cannot silently remove
  them.

### Added

- `internal/routebuilder` package providing typed URL builders for
  every route templates reference (`ActiveJobs`, `JobStatus`,
  `JobStatusSlot`, `Anniversary`, `AnniversaryEdit`,
  `AnniversaryItemDelete`, `AnniversaryItemUpdate`,
  `AnniversaryItemCreate`, `FeedbackSubmit`, `DebugConsole`,
  `BrowseResults`, `SoldierSearch`). Templates call these via
  `templ.SafeURL(routebuilder.X(...))` instead of string literals.
  When a route moves, only `routes.go` and the matching builder need
  to change. 16 unit tests cover URL escaping, whitespace trimming,
  path-segment validation, and per-builder output stability.
- `github.com/go-chi/chi/v5` v5.3.0 added as a direct dep.

### Changed

- `internal/appshell/routes.go`: swapped `net/http.ServeMux` for
  `github.com/go-chi/chi/v5`. Chi provides explicit pattern routing,
  middleware composition (`middleware.Recoverer`,
  `middleware.RequestID`), and wildcard segments (`/*`) without
  changing handler signatures — every handler still reads
  `r.URL.Path` directly, so existing `strings.TrimPrefix` logic
  works unchanged. Wildcard routes register GET, POST, PUT, and
  DELETE methods where the handler dispatches by `r.Method` (soldier
  records, soldier display IDs).

### Added (continued)

- Persistent progress slot in the layout: a top-center progress bar
  (below the toast region) that polls `/jobs/active` every 3s and
  shows real progress for whatever background task the user kicked
  off most recently. The slot stays visible across page navigation
  so a user who starts an export from `/share` and navigates to
  `/soldiers` still sees the progress bar at the top of the page.
  Implemented as `JobStatusSlotFragment` in
  `internal/templates/job_slot_fragment.templ`.
- Toast kinds now have distinct CSS: success = warm cream + gold
  border (existing), error = warm red (existing), warning = amber
  (new), info = blue (new). `showToast()` in `frontend/app.js`
  switched to a header label matrix (Success/Heads up/Warning/
  Attention) and auto-dismisses `success` and `info` toasts after
  4 seconds. `error` and `warning` toasts remain manual-dismiss
  per the Issue #54 decision.
- Jobs registry hardening: `Registry.Shutdown(ctx)` cancels every
  running/queued job and waits on a new `workerWG` for worker
  goroutines to drain. Wired into `lifecycle.go` shutdown sequence
  before `database.Close()`, bounded by a 5s deadline. Prevents
  file-handle leaks on app exit (same family as the WJ-2 fix in
  `271149a`).
- New `openMultipleFilesDialogOverride` test hook on `*App`,
  mirroring the existing `openFileDialogOverride`. Required by the
  image-import migration so httptest can inject file paths.
- Migrated the following long-running handlers to the jobs registry
  (each now reports real progress via the persistent slot):
  JSON export, InsightsPDF export, Excel export, iCalendar export,
  Static web archive export, Printable database PDF export, Backup
  archive export, Shared archive export, Bug report bundle
  export, soldier PDF export (with and without images), soldier
  JPG export, monthly anniversary PDF export, image import on
  soldier detail and edit pages, shared archive import, memorial
  JSON import, duplicate audit, image orphan cleanup, review queue
  bulk-resolve and bulk-delete, Google Drive backup upload, Google
  Sheets export.
- Repaired the `JobStatusFragment` htmx polling: added the missing
  `hx-trigger="every 2s"` attribute so the `/jobs/{id}` page
  actually polls (previously the comment claimed 2s but no trigger
  was set, so htmx used the default `natural` trigger and never
  fired).
- **`audit/smoke.mjs`** — live Playwright regression net for
  click-driven surfaces. Boots a real Chromium against
  `dixiedata-web`, walks every button on the search / browse /
  share / insights / settings pages, asserts that each one
  fires the expected network request and that the swap target
  updates. 25 assertions. This is the test that finally
  caught the four bugs that PR #1 + PR #2 + PR #F1 shipped
  silently. Every commit that changes templ + htmx + JS +
  handler code must keep this green.
- Removed the unused SSE endpoint `/jobs/{id}/stream` and its
  handler (`streamJobProgress`, `writeJobEvent`,
  `isTerminalJobStatus`). No JS consumer in `app.js` opened an
  `EventSource` on the endpoint.

### Changed

- `data-progress-label` indeterminate spinner retained only on
  intentional carve-outs: image-import buttons (open native
  file picker), update-apply and recovery buttons (call
  `a.Quit()` 750ms after responding, cannot use the 303
  redirect pattern), and the six Google Calendar interaction
  buttons (OAuth popup, calendar picker UI).

### Fixed

- **16 chi-mis-registered routes** (PR #1 of the stabilization
  sprint set `r.Get` for every action endpoint whose handler
  rejected anything except `http.MethodPost`). Every export,
  share, insights, merge-review, and Google-connect button
  silently returned 405 Method Not Allowed when clicked.
  Flipped to `r.Post` for: `/export/{json,csv,ical,
  static-archive,backup,shared-archive,bug-report,feedback-log}`,
  `/insights/report/pdf`, `/merge-review/*`,
  `/integrations/google/{connect,disconnect,backup,
  sheets/export}`, `/images/screenshot`, `/open-link`. Two
  regression nets added so the class cannot recur:
  `routes_method_guard_test.go` (AST walk, flags any
  `r.Get` paired with a POST-only handler — pure compile-time
  check) and `route_integration_test.go` (runtime check that
  fires GET against every known POST-only path and asserts
  405 + `Allow: POST`). Plus a wildcard-shadowing test
  (`route_wildcard_test.go`) that fires GET at the more
  specific sibling of every `/parent/*` wildcard.

- **Broken `JobStatusFragment` htmx polling** — added the missing
  `hx-trigger` so the fragment actually re-fetches every 2s.

- **App.js hx-* attribute strip silently broke every click
  handler.** DOMContentLoaded stripped `hx-get`, `hx-post`,
  `hx-trigger`, etc. from the DOM to prevent htmx's auto-handler
  from double-firing alongside app.js's own `request()` /
  `queueRequest()`. But the same handlers READ those attrs to
  construct the fetch. After the strip, every read returned
  empty / null, so every click handler bailed out and the button
  did nothing. Fix: cache each `hx-*` attr to a `data-hx-*`
  mirror BEFORE stripping, then add `hxAttr(el, name)` /
  `hxHas(el, name)` helpers that prefer the live attr and fall
  back to the data-* mirror. Also added `input` to the
  `triggerInputRequest` regex so the quick-search trigger
  (`input changed delay:300ms`) actually fires.

- **htmxattr.Mux.Attrs() used `templ.SafeURL` for URL values
  — which templ.RenderAttributes silently drops.** This was
  the deepest bug in the chain: every `htmxattr.Mux{Get: ...}`
  call rendered the form/button without an `hx-get` attribute
  at all. The 16 unit tests in `internal/htmxattr/` passed
  because they only inspect the `templ.Attributes` map;
  nothing rendered the map through `templ.RenderAttributes`
  in a test. Fix: use plain `string` for URL values (not
  `templ.SafeURL`). The `SafeURL` wrapper is meaningful inside
  templ expression context but breaks in spread-attribute
  context.

- **Browse filter changes now auto-apply** (previously saved
  draft state only). The change handler in app.js calls
  `queueRequest(form)` after saving draft state, so the
  `/browse/results` request fires immediately. Updated the
  `TestBrowseFilterChangeSavesDraftWithoutAutoApplyingIt`
  Node-harness test (renamed to
  `TestBrowseFilterChangeAutoAppliesAndPersistsDraft`) to
  match the new behavior. The harness needed `window.setTimeout`
  added to the `windowMock` object so `queueRequest`'s
  `setTimeout(..., 0)` callback can drain.

- **`hxAttr` / `hxHas` duck-type the Element contract instead
  of `instanceof Element`.** `instanceof Element` is
  browser-only and broke the Node test harness for browse
  filter changes (the harness mocks `HTMLElement` but not
  `Element`). Now they check for `getAttribute` /
  `hasAttribute` method existence, which both real browsers
  and the mock satisfy.

- **Soldier PDF / JPG, image screenshot, and full database PDF
  exports no longer crash the app.** The 4 native `SaveFileDialog`
  call sites in `internal/appshell/app.go` (`handleSoldierPDF`,
  `handleSoldierPDFNoImages`, `handleSoldierJPG`,
  `handleImageScreenshot`) and `exportFullDatabasePDFPath` in
  `internal/appshell/exports_handlers.go` were the missing
  link in the issue #2807 guard net added by commit `162c353`.
  That commit routed 9 export handlers through
  `guardedSaveFileDialog` (or its inline equivalent) but the
  5 above called `a.SaveFileDialog` directly. A double-click
  on any of them queued a second native dialog on the Wails
  UI thread, both blocked, WebView2 lost focus during
  `MoveFocus`, and `errorCallback` killed the process with
  `Chrome_WidgetWin_0. Error = 1412`. All 5 call sites now
  carry the same `a.inFlight.LoadOrStore(...)` guard pattern
  as `handleCalendarPDF`; the database PDF helper returns a
  new `errExportInFlight` sentinel that the HTTP handler maps
  to a 429 and the Wails binding surfaces as a friendly toast.
  See `internal/appshell/save_dialog_guard_test.go` for the
  regression net.

- **Three modal dialogs reverted from native `<dialog>` back to
  the pre-issue-117 `<div role="dialog" aria-modal="true">`
  overlay** (feedback modal in layout, print-config and
  google-prefs in share). The native `<dialog>` swap was
  blamed for the crash but was a red herring — the real
  trigger was the unguarded `SaveFileDialog` race above.
  However, native `<dialog>` still carries a subtle WebView2
  interaction (showModal grabs host focus, which then routes
  through Wails' `onFocus` → `Chromium.Focus()` → `MoveFocus`
  at unexpected times), so reverting keeps the focus-event
  surface small while we wait for an upstream Wails fix.
  Manual focus trap and ESC close handlers live in
  `frontend/app.js` (`showOverlayModal` /
  `overlayModalKeydown`).

### Removed

- Developer visualizer overlay (orphan from v1; no current consumers).
  Removed `data-ui-id` template attributes (52 sites), `@SurfaceBadge`
  and `@InlineSurfaceBadge` calls (54 sites), `SurfaceBadge`/`InlineSurfaceBadge`/
  `uiDebugEnabled`/`uiDebugValue` helpers, `internal/uiids.DebugEnabled`/
  `EnableFromArgs`/`DebugEnvVar`/`DebugArg`/`truthy`, the
  `DIXIEDATA_DEBUG_UI_IDS` env var, the `--debug-ui-ids` flag, the
  `[data-debug-ui-ids=true] [data-ui-id]{...}` CSS outline rule,
  `.ui-debug-badge` / `.ui-debug-inline` styles, and
  `debugSurfaceIDsEnabled()` in `frontend/app.js`. The 78 surface
  constants in `internal/uiids/uiids.go` registry stay — they remain
  the canonical surface identifiers used by future HTMX typing work.
  The runtime log console at `/debug/console` (separate feature) is
  untouched.
- `/jobs/{id}/stream` route + `streamJobProgress`/`writeJobEvent`/
  `isTerminalJobStatus` handlers (dead code, no consumers).
- `enqueueStaticArchive` and `enqueueDatabasePDF` (replaced by
  the unified `enqueueExport` helper).

- Button primitive adopted in `calendar.templ` (Export Month PDF)
  and `jobs.templ` (Cancel x2) — these three sites were missed by
  the original grep pass that scoped to `class="primary-button"`
  with anchor instead of `<button` opening tag. Caught by the
  final verification sweep.
- Button primitive adopted in `share.templ` at all 33 sites:
  Export JSON/CSV/iCal/Static/Backup/Shared cards (ButtonContent
  variant for rich `<span>` children), Print config dialog
  (Close/Cancel/Generate Printable PDF), Import cards (Shared/
  Memorial JSON/Backup), Support & Diagnostics (Feedback Log/Bug
  Report Bundle), Merge Review (Inspect Diff/Keep Local/Keep
  Incoming/Keep Both), Google integration (Connect/Disconnect/
  Backup/Sheets), DixieData Calendar (Use/Sync/Unsync/Preferences
  + test variants), Calendar preferences (Close/Cancel/Save
  Preferences). `share.templ` now has zero raw button class
  strings. New `ButtonContent` variant added to the Button
  primitive for buttons with structured markup (bold title +
  muted description) — the existing string-only `Button` is for
  simple label buttons. Two `ButtonContent` regression tests
  cover the children render + type-not-duplicated invariants.
- Button primitive adopted in `entry_form.templ` at twenty-six
  sites (Fetch Data, Confirm/Cancel delete draft x2, Undo delete,
  Reapply older changes, Delete saved local draft, Add Source
  Record, Add Images From Computer x2, Save Changes / Create
  Person Record, Save Identity, Initialize Data, Back, Scan for
  Orphaned Images, Run Data Quality Scan, Save Update Source,
  Use Default GitHub Feed, Check for Updates, Export Backup,
  Download and Apply Latest Update, Move Listed Files to Temp
  Trash, Move Selected to Review Queue, Compare Selected, Quick
  View). The "Save Changes / Create Person Record" conditional-
  label pair was split into two primitive calls gated on `isEdit`.
  Test `TestEntryFormUsesMobileSafeSourceRecordAndActionLayouts`
  updated to accept both legacy `data-record-add` (bare) and
  primitive `data-record-add=""` (empty value) as semantically
  equivalent HTML. entry_form.templ now has zero raw button class
  strings.
- Button primitive adopted in `soldier_card.templ` at eleven sites
  (Browse Alphabetically, Run Advanced Search, Reset Filters, Export
  PDF, Export JPG, Send to Review Queue / Update Review Note, Mark
  as Resolved, Delete Person Record, Add Images From Computer,
  Download Selected Images, Delete Selected Images). The
  "Send to Review Queue / Update Review Note" pair required
  splitting the legacy conditional-label button into two
  primitive calls gated on `s.NeedsReview`. Anchors (Open Record,
  Compare, Open Unit Graph, etc.) and disclosure summaries stay
  unchanged — slated for Pill + future Disclosure primitives.
- Button primitive bug fix: the `{ attrs... }` spread previously
  duplicated the `type` attribute (rendered as `<button type="submit"
  ... type="submit">`). Added `buttonAttrsExcludingType` helper that
  strips `type` before the spread, so the primitive owns the type
  attribute end-to-end. Reordered attribute emission so caller attrs
  come before the kind `class=`, matching the legacy inline byte
  order (`<button type="submit" hx-post="..." class="...">Label`).
  New `TestButton_TypeNotDuplicatedFromAttrs` regression test
  asserts exactly one `type=` attribute in the rendered HTML.
- Button primitive adopted in `calendar_day.templ` at two sites
  (Save Changes, Add Item). The disclosure `<summary>` and `<a>`
  elements using button class strings remain — Button primitive
  targets `<button>` only; summary + anchor reuse is intentional
  CSS-level styling, slated for either the Pill primitive or a
  future Disclosure primitive migration.
- Button primitive adopted in `insights.templ` at five sites
  (Export Analytics Report, Audit Now, Back to Insights, Compare
  Selected, Quick View). `insights.templ` now has zero raw button
  class strings.
- Button primitive adopted in `research_collections.templ` at two
  sites (Create Collection, Add Current Person Record). The
  Compare Person Records anchor is left for the Pill migration.
- Button primitive adopted in `research_log.templ` at three sites
  (Add Research Task, Add to Research Log, Mark Resolved).
- Button primitive adopted in `layout.templ` at three sites
  (feedback modal Close, Cancel, Save Feedback). The two `<a>`
  anchors ("Add Person Record" in the top nav + floating nav panel)
  remain — they're anchor-styled-as-button, slated for the Pill
  primitive migration.
- Button primitive adopted in `browse.templ` at three sites
  (Apply Filters, Reset Browse, Clear Selection). The Print/Export
  Selected anchor is intentionally left untouched — it's an `<a>`
  styled with `.primary-button`, not a `<button>`, so it belongs to
  the future Pill primitive migration.
- Button primitive adopted in `review_queue.templ` at four sites
  (issue #74 Phase 1 migration). The bulk-action Ignore Selected /
  Delete Selected form buttons and the per-entry Mark as Resolved /
  Mark Match Resolved buttons now call `@components.Button` with
  the form attributes (`type="submit"`, `name`, `value`,
  `hx-confirm`, `hx-post`, `hx-target`, `hx-swap`) threaded
  through the `templ.Attributes` parameter. Rendered HTML is
  byte-stable against the legacy form; existing review-queue
  snapshot tests pass unchanged. `review_queue.templ` now has zero
  raw `class="primary-button"` / `class="secondary-button"` /
  `class="danger-button"` usages — a clean migration template for
  the remaining 110 button sites.
- EmptyState primitive adopted at six sites (issue #74 Phase 1.6
  migration): `/soldiers` (advanced filters, browse mode, quick
  search query, recent records, initial-state prompt) and
  `/browse` (no-results under active filter). Each call replaced a
  hand-rolled `<p class="rounded-2xl ...">` with
  `@components.EmptyState(title, body, "")`. The primitive emits
  `<div class="empty-state" data-empty-state="true">` so the audit
  harness picks up every migrated surface automatically. Existing
  entry-form + browse snapshot tests pass unchanged. Visually
  verified at 1280×800 — browse empty state renders with sepia
  dashed border + parchment surface (see
  `audit/screenshots/empty-state-browse.png`).
- Phase 1 component primitives (issue #74) continued:
  - **Field** (`internal/templates/components/field.templ`) —
    `templ Field(kind, attrs)` wraps `<input>` / `<textarea>` /
    `<select>` with the `.field-input` class. The primitive owns
    the class attribute so callers cannot double-emit it; callers
    who pass their own class string in attrs are silently ignored.
    Five golden-snapshot tests cover input default, input+class,
    input+type, textarea body, select with children.
  - **Pill** (`internal/templates/components/pill.templ`) —
    `templ Pill(label, href, extraClass, attrs)` renders an
    `<a class="pill-link" href="...">label</a>`. Three tests cover
    the default snapshot, extra-class append, and hx-* / aria-*
    pass-through (the browse pager uses these extensively).
  - **Toast** (`internal/templates/components/toast.templ`) —
    `templ Toast(kind, message)` documents the expected `toast-card`
    + `data-toast-kind` contract for future server-rendered toasts.
    The current toast rendering lives in `frontend/app.js`; this
    primitive is a contract, not a migration. One test asserts the
    class + data attribute + body content.
  - **EmptyState** (`internal/templates/components/empty_state.templ`)
    — `templ EmptyState(title, body, extraClass)` renders
    `<div class="empty-state" data-empty-state="true">` with title
    + body. The `data-empty-state` hook doubles as the audit
    harness signal so every migration lights up in round-3 reports.
    Companion CSS rule added to `frontend/tailwind.css`:
    `.empty-state` (1.2rem radius, sepia dashed border, parchment
    surface). Two tests cover default + extra-class.
- `internal/templates/components/card.templ` — Card primitive for
  issue #74 Phase 1.2. `templ Card(extraClass) { ... }` wraps the
  child content in `<div class="card ...">`. extraClass accepts the
  compound classes existing call sites use (`rounded-3xl p-6`,
  `rounded-2xl p-5 space-y-4`, etc.) so the byte-stable class string
  preserves every existing layout hook. Three golden-snapshot tests
  in `card_test.go` cover the default class, extra-class append,
  and child-content passthrough.
- `internal/templates/components/button.templ` — Button primitive
  for issue #74 Phase 1.1. `templ Button(label, kind, extraClass,
  attrs)` renders the legacy class strings (primary-button,
  secondary-button, ghost-link, danger-button) byte-stably; unknown
  kind values fall back to secondary. Layout template swaps the
  three floating-dock buttons (Scratch Pad, Feedback, Menu) to
  `@components.Button` as the proof-of-concept migration. Seven
  golden-snapshot tests in `button_test.go` cover all four kinds,
  extra-class merging, attr pass-through, and the unknown-kind
  fallback.

### Fixed

- CI: `.github/workflows/test.yml` "Restore Typst binary for render
  tests" step called `Restore-DixieDataTypstBinary` without the
  mandatory `-Root` parameter, causing the Windows runner to fail
  before `go test` could run on `internal/archive` and `pkg/render`.
  Resolve `$root` via `Get-DixieDataRoot` (already exported from
  `scripts/build-common.ps1`) and pass it through.
- CI: `nextGoogleAnniversaryDate` in both
  `internal/integrations/google_service.go` and
  `internal/archive/compat.go` built the anniversary `candidate`
  in `time.Local`. On UTC CI runners this produced a UTC midnight
  time that shifted to the previous calendar day when downstream
  callers converted to a non-UTC location (e.g. America/Chicago),
  surfacing as `start.DateTime = "2027-05-12T..."` instead of
  `"2027-05-13T..."` in the Google Calendar event. The Google
  Calendar test (`TestGoogleCalendarEventBuildsYearlyTimedEvent
  WithReminders`) failed on CI for this reason even though it
  passes locally where `time.Local = America/Chicago`. Added an
  explicit `location *time.Location` parameter so callers (and
  tests) build the candidate in the same location that will format
  the final event. Both function copies and three call sites
  (two production, three test) updated. Verified green under both
  `TZ=America/Chicago` (local) and `TZ=UTC` (CI).
- CI: `Restore-DixieDataTypstBinary` in `scripts/build-common.ps1`
  checked `$LASTEXITCODE -ne 0` after `Expand-Archive`, but
  `Expand-Archive` and `Invoke-WebRequest` are native pwsh cmdlets
  and do not set `$LASTEXITCODE`. In script scopes where no prior
  external command ran (the GitHub Actions test workflow is one),
  the read of `$LASTEXITCODE` threw `The variable '$LASTEXITCODE'
  cannot be retrieved because it has not been set` and failed
  CI. Switched to `$?` (success-of-last-command automatic variable,
  always defined) — the canonical pwsh idiom for catching cmdlet
  failures. Other `$LASTEXITCODE` checks in the file follow
  `& <external.exe>` calls (tar, npm, templ, wails) and remain
  correct.

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
- `Makefile` `help` target now lists every defined target. The regex
  `^[a-zA-Z_-]+:.*?## ` requires the `## doc` on the same line as the target
  name; multi-target rules like `build debug:` or `test test-quiet:`
  satisfied that for only the first token (and sometimes not at all when
  `##` sat on a recipe-body line). Three targets — `build`, `debug`, `test`
  — were silently hidden. Split each into single-target rules with their
  own `## doc` line. No behavior change; `make -n` confirms identical
  recipes.
- `internal/jobs/jobs_test.go` `TestSetResultBroadcastsSnapshot` had a race
  that surfaced intermittently under `go test ./...`: the worker goroutine
  from `Start` broadcasts StatusRunning before the test could call
  `Subscribe`, so the channel received the wrong snapshot on the next read
  and the assertion against `SetResult`'s broadcast saw
  `ReplacedRecords=0` / `MigrationRan=false`. Drain the channel until the
  `SetResult` snapshot arrives (identified by `MigrationRan=true`), with
  the same 1s deadline. Verified stable over 10 standalone runs and 3
  consecutive `go test ./...` invocations.

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
