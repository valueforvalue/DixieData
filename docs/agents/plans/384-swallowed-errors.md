# Plan: Swallowed errors audit + remediation (#384)

## Scope reality check (from Research phase)

The issue body estimates 50+ JS + 80+ Go + 30+ templ sites. The
scope-tracer recon counts:

- **JS: ~7 truly swallowed** (most "catches" log or write a toast
  already; the debug-toolbox LS wrappers + `app.js:3601` print-
  records fragment fetch are the user-visible subset)
- **Go: ~10 high-leverage** (~120 `defer X.Close()` sites exist
  but are textbook pool-leak patterns, NOT error-hiding; the 10
  that matter are bare `templ.Render(...)w` calls in handler
  bodies that discard the returned err)
- **Templ: ~5 high-leverage** (overlap with Go's bare-Render set
  + the print_records_fragment_handler that logs `trace.Log` AFTER
  the headers are flushed)

The 50+/80+/30+ estimates include patterns that are intentional
(ADR 0006 codifies `trace.Log + return nil` as designed silence)
and patterns that have different semantics (defer-Close leaks
vs error-hiding). The Plan below targets the **user-visible**
subset, not the inflated body count.

## Decisions to confirm

- **Q1 — Scope of "fix":** (a) inline patches only on the
  ~22 high-leverage sites; (b) inline patches + lint rules
  (errcheck for Go, ESLint rule for empty catch); (c) catalog
  only (no code changes). **Recommendation: (a) inline patches
  only for Slice 1**. Lint scaffolding is a separate cross-cutting
  piece (#318 covers test infra; lint is orthogonal and deserves
  its own issue if the user wants it).
- **Q2 — Toast surface for templ-render failures:** when
  `templ.Render(ctx, w)` fails mid-write (headers already flushed)
  the client gets partial HTML with no error. Options:
  (a) swap to `<EmptyState kind="error">` (visible fragment, no
  toast round-trip); (b) `data-error` attribute + JS listener that
  fires `showToast` (JS-side detection); (c) write a sentinel
  comment in the partial HTML + JS pickup. **Recommendation:
  (a) EmptyState kind="error"** — visible, works with no JS
  changes, matches the existing component vocabulary.
- **Q3 — `defer X.Close()` policy:** (a) wrap every defer-Close
  with `slog.Debug(...)` so close errors leave a trail; (b) only
  fix defer-Closes inside handler bodies that return multiple
  errors via `issues []string`; (c) leave defer-Closes alone (they
  are pool-leak patterns, not error-hiding; issue body conflates
  the two). **Recommendation: (c) leave defer-Closes alone for
  Slice 1** — they are a separate concern (resource leak, not
  error visibility) and tackling them in #384 scope-creeps the
  audit. File a separate issue if the user wants the pool-leak
  remediation.
- **Q4 — ADR 0006 sites (trace harness):** `internal/debug/trace/
  trace.go:33` is documented in ADR 0006 as deliberate silence.
  (a) revoke the ADR and surface all trace sites; (b) acknowledge
  the ADR and leave trace harness as designed. **Recommendation:
  (b) leave alone** — ADR codifies the design; flipping it would
  invalidate every existing trace site across the repo.
- **Q5 — Backward compat for EmptyState kind="error":** (a) add
  `kind="error"` to the existing `components/empty_state.templ`
  component (new variant, default stays empty); (b) create a new
  separate `components/error_banner.templ`. **Recommendation: (a)
  reuse EmptyState** — DRY, same visual language, fewer files.

## Slice 1 (tracer bullet — fully detailed)

**Scope:** Fix the top-3 user-visible JS silent catches
(debug-toolbox LS wrappers + print-records fragment fetch) +
the top-3 user-visible Go templ-render swallows (print_records
fragment, jobs status fragment, soldiers list/detail handlers).
Add EmptyState `kind="error"` variant. One regression test per
fixed site asserting the user sees an error message.

### Files

- `frontend/debug-toolbox.js:198-225` — three LS read/write
  helpers (`readShareQueueFromLocalStorage`, `writeShareQueueTo
  LocalStorage`, `readPresetsFromLocalStorage`) gain a
  `dixie.logger.warn('localStorage error', err)` call inside the
  catch + return the existing fallback. The `window.__dixieDebug.
  push` infrastructure (debug.js:148) is already in place to
  surface the warning in the debug-console.
- `frontend/app.js:3601` — `.catch(() => {})` on the
  printRecordsFragment fetch gains an error logger call AND a
  toast: `catch (err) { dixie.logger.warn('print fragment', err);
  showToast('Could not load print preview', 'error'); }`. Uses
  the existing `showToast` helper (app.js:2732).
- `internal/appsshell/print_records_fragment_handlers.go:47-48` —
  wrap `templ.Render(...)` so a render error after the headers
  are flushed still produces a visible `<EmptyState kind="error">`
  fragment. Either (a) check the err from Render and write a
  fallback fragment, or (b) move the Render BEFORE the
  Content-Type header write so the err can be served via the
  normal `respondError` toast path.
- `internal/appsshell/jobs_handlers.go:95-149` — wrap the three
  bare `Render(r.Context(), w)` calls in `renderJobStatus` with
  err checks that write a `<EmptyState kind="error">` fallback
  fragment via `fmt.Fprintf(w, ...)`.
- `internal/appsshell/soldiers_handlers.go:48-540` — wrap the
  bare `Render` calls in `SoldierList`, `SearchResults`,
  `BrowseView`, `BrowseResults`, `SoldierDetailWithCitedIn` with
  the same fallback pattern. The handler is the most-trafficked
  page surface so a regression here is the highest user-impact.
- `internal/templates/components/empty_state.templ` — add
  `kind="error"` variant (red border, error icon, "Something went
  wrong loading this view" copy). Defaults to existing empty
  styling for backwards compat.
- `internal/appsshell/respond.go` — add a helper
  `respondErrorFragment(w, err)` that writes a visible error
  fragment (`<div data-error="true"><EmptyState kind="error">
  ...</div>`) when a templ render fails after headers are flushed.
  Mirrors the existing `respondError` toast path but for the
  fragment-swap case.
- `audit/smoke_swallowed_errors.mjs` (new) — browser probe that
  simulates the failure mode for each fixed site (kill the
  endpoint via route stub, click the triggering button, assert
  the user sees an error message — toast for JS fixes, visible
  EmptyState fragment for Go fixes).
- `internal/appsshell/soldiers_handlers_test.go` — add tests:
  - `TestHandleSoldierList_RenderErrorProducesErrorFragment`
  - `TestHandleSoldierDetail_RenderErrorProducesErrorFragment`
  - `TestHandleBrowseView_RenderErrorProducesErrorFragment`
- `internal/appsshell/jobs_handlers_test.go` — add:
  - `TestRenderJobStatus_RenderErrorProducesErrorFragment`
- `internal/appsshell/print_records_fragment_handlers_test.go` —
  add:
  - `TestHandlePrintRecordsFragment_RenderErrorProducesErrorFragment`
- `docs/agents/error-handling.md` (new) — house style doc
  covering: toast vs banner vs inline choices; `respondError` vs
  `respondErrorFragment`; defer-Close policy (intentional ADR
  reference); trace harness policy (ADR 0006 reference); the
  EmptyState `kind="error"` variant contract.
- `CHANGELOG.md` — bullet under `### Fixed`.

### Success criteria

1. JS catches in debug-toolbox.js:198-225 log via `__dixieDebug`
   on every error path (parse error, quota error, etc.).
2. `frontend/app.js:3601` printRecordsFragment fetch failure
   shows an error toast via `showToast('Could not load print
   preview', 'error')`.
3. Go handler `templ.Render(ctx, w)` failures after header flush
   produce a visible `<EmptyState kind="error">` fragment the
   user can see (not silent truncation).
4. The `respondErrorFragment` helper exists + handles the
   `Content-Type` already-set case correctly (writes a fragment
   body, not a toast header).
5. `audit/smoke_swallowed_errors.mjs` exercises each fixed site
   + asserts the user-visible error message renders.
6. `docs/agents/error-handling.md` covers the locked house style
   decisions.
7. `audit/discover_orphan_handlers.mjs` shows no new orphans.
8. ADR 0006 + defer-Close scope-deferral decisions documented
   in the new `error-handling.md` so the next agent doesn't
   re-litigate.

### Regression net

- `TestRespondErrorFragment_WritesErrorStateBody` — helper
  produces correct fragment markup.
- `TestHandleSoldierList_RenderErrorProducesErrorFragment` —
  forced templ render error → visible EmptyState fragment.
- `TestHandleSoldierDetail_RenderErrorProducesErrorFragment` —
  same for detail handler.
- `TestHandleBrowseView_RenderErrorProducesErrorFragment`.
- `TestRenderJobStatus_RenderErrorProducesErrorFragment`.
- `TestHandlePrintRecordsFragment_RenderErrorProducesErrorFragment`.
- `audit/smoke_swallowed_errors.mjs` steps:
  - Step 1: corrupt the localStorage share-queue payload → click
    "Open Share Queue" → assert the debug console surfaces a
    "localStorage error" warning (via `window.__dixieDebug.buffer`).
  - Step 2: stub `/soldiers/{id}/print-fragment` to return 500 →
    click Print → assert a red toast appears (`showToast('Could
    not load print preview', 'error')`).
  - Step 3: stub `/jobs/{id}/status` to return 500 → poll the
    job-status fragment → assert the page shows
    `<EmptyState kind="error">`, not silent truncation.

### Commit

Single commit. Layer coupling (JS + Go + new component variant +
new helper + new tests + new smoke probe + new docs) only makes
sense together — the tracer-bullet slice proves the
respondErrorFragment helper + EmptyState kind="error" variant +
JS toast surface all work in concert before any further site
gets a similar treatment.

## Subsequent slices (stub only — detail when their turn arrives)

- **Slice 2** — Apply the `respondErrorFragment` pattern to the
  remaining bare-Render handler sites (~50 more across
  app.go, calendar_handlers.go, events_handlers.go, insights_
  handlers.go, research_handlers.go, reviews_handlers.go,
  settings_handlers.go, share_queue_handlers.go, share_subpages_
  handlers.go). Slice 1 picks the 3 highest-leverage + proves
  the pattern; Slice 2 generalizes.
- **Slice 3** — Frontend JS sweep: fix the remaining
  ~5 silent catches in app.js (template export PATCH,
  debug.js Open Folder / Clear Console fetches, save/draft
  restoration paths).
- **Slice 4** — Lint scaffolding: introduce `golangci-lint` +
  `errcheck` config + ESLint rule forbidding empty catch.
  Cross-cutting infra piece; benefits from its own RPCI
  separate from the inline patches.
- **Slice 5** — `defer X.Close()` pool-leak remediation
  (separate concern from error visibility; out of #384 scope
  per Decision 3; opens as a new issue if the user wants it).

## Out of scope (locked decisions)

- ADR 0006 sites (`trace.Log + return nil`) — by design.
- `defer X.Close()` patterns — resource leak, not error hiding;
  separate concern.
- Cross-feature fixes that surface swallowed errors discovered
  while touching this area — those land in #316 (htmx-guard lint)
  or as ad-hoc follow-ups.
- golangci-lint + errcheck + ESLint configuration — separate
  cross-cutting infra piece (Slice 4 above), tracked in a
  follow-up issue if the user wants it.