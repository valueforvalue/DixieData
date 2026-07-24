# Plan: htmx-guard lint probes (issue #316)

## Decisions (locked after Critique phase)

| # | Decision | Choice |
|---|---|---|
| 1 | JS submit coexistence rule | **Attribute + marker allowlist** — `<form data-dixie-submit>` routes to `dispatchDixieDataForm`; bare `<form>` requires `// htmx-guard: utility-submit` annotation on the listener. Existing sites 3995/4066/5314 get the 1-line marker comment; no refactor. |
| 2 | Go function scope for toast-no-redirect | **Brace-walk per `func`** — regex finds `func ... {`, walks braces to matching close, scans body. ~30 LOC. |
| 3 | Slice 3 polling-stop scope | **Both `jobs.JobStatusView` + `jobs.JobStatusSlotFragment`** render-asserted against all 5 statuses. |
| 4 | Slice 4 reach | **`htmxattr.Mux{Target: ...}` panic only** — 1 LOC; templ-direct IDs are slice 2's job. |
| 5 | Architectural cleanup | **File follow-up issue** "Introduce `dispatchUtilitySubmit(form)` and migrate 3995/4066/5314" — linked from #316 closed summary. |

## Background

Issue #316 calls for 4 CI-failing lint probes that catch the
attribute-drift bug classes from `docs/COMMON_BUGS.md` §1.5, §1.8, §3.4,
§3.5 — today these are review-discipline-only. Recurring failure mode:
`70878ac → 3612dab` toast-no-redirect cycle, per AGENTS.md.

## Apply sites (per AGENTS.md "checklist rule") — TRACE from issue #316

- [ ] `audit/discover_htmx_guard.mjs` — toast-no-redirect walker (slice 1)
- [ ] `audit/discover_htmx_guard.mjs` — JS submit coexistence walker (slice 1)
- [ ] `audit/discover_htmx_guard.mjs` — orphan `hx-target` / `data-results-target` walker (slice 2)
- [ ] `audit/discover_htmx_guard.test.mjs` — sibling test mirroring `discover_orphan_handlers.test.mjs` (slice 1+2)
- [ ] `internal/templates/jobs_templ_test.go` — new polling-stop render assertions (slice 3)
- [ ] `internal/htmxattr/htmxattr.go` — uncomment `validateTarget` panic (slice 4 only)
- [ ] `Makefile` — `lint-htmx-guard` target; add to `RELEASE_PIPELINE_GATES` (slice 1+2)
- [ ] `frontend/app.js` — add `// htmx-guard: utility-submit` markers at lines 3995, 4066, 5314 (slice 1 prerequisite; no behavior change)
- [ ] `frontend/app.js:5225` — confirm the `dispatchDixieDataForm` delegate has the marker (slice 1 prerequisite)
- [ ] `docs/agents/htmx-guard-conventions.md` — short doc describing the marker convention (slice 1, lives with the probe)
- [ ] File follow-up issue: "Introduce `dispatchUtilitySubmit(form)` and migrate 3995/4066/5314" — separate from #316; link from #316 close comment

The work is not "shipped" until every box is checked. The marker
annotations on app.js and the conventions doc are part of slice 1's
reviewable scope; they are not optional.

## Tracer-bullet Slice 1 — toast-no-redirect + JS submit coexistence

**Why this slice first:** highest-ROI per AGENTS.md ("recurring failure"
class), models the probe shape on `discover_orphan_handlers.mjs`, and the
markers + conventions doc establish vocabulary that slices 2-4 reuse.

### Files

- `audit/discover_htmx_guard.mjs` (new, ~120 LOC)
- `audit/discover_htmx_guard.test.mjs` (new, ~70 LOC; mirrors `discover_orphan_handlers.test.mjs`)
- `frontend/app.js` — add `// htmx-guard: utility-submit` markers at lines 3995, 4066, 5314; one marker at the 5225/5314 doc-level delegates confirming they're `data-dixie-submit` or utility
- `docs/agents/htmx-guard-conventions.md` (new, ~25 LOC) — the marker convention documented
- `justfile` — add `lint-htmx-guard` recipe, wire into `audit` and `RELEASE_PIPELINE_GATES`

### Success criteria (observable in 5 min)

1. `node audit/discover_htmx_guard.mjs --strict` exits 0 on current `dev` HEAD.
2. With a synthetic regression (added `setInfoToastHeader(w, "saved")` to a function whose body lacks all of `X-DixieData-Redirect | writeExportRedirect | enqueueExport(` | `respondDuplicateInFlight(`), probe exits 1 with file:line citation.
3. With a synthetic regression (added `form.addEventListener("submit", ...)` inside a `<form data-dixie-submit>` body, NOT routing through `dispatchDixieDataForm`), probe exits 1.
4. With a synthetic regression (added bare `<form>` `addEventListener("submit", ...)` without a `// htmx-guard: utility-submit` marker), probe exits 1.
5. `just lint-htmx-guard` runs probe + reports pass/fail summary; exits non-zero on regression.

### Regression net

- `audit/discover_htmx_guard.test.mjs` — synthetic-file fixtures under `audit/_lib/fixtures/htmx_guard/` covering all 4 cases. Pattern matches `discover_orphan_handlers.test.mjs` (which exists as a sibling model).
- `just lint-htmx-guard` wired into CI (GHA audit step already runs `node audit/*.mjs --strict`-style probes; this just adds one more).

### Commit shape (per AGENTS.md)

- Commit 1: probe + tests + justfile recipe. Title: `audit: add discover_htmx_guard probe for toast-no-redirect + JS submit coexistence`.
- Commit 2 (PREREQ, can co-land): app.js marker annotations + `htmx-guard-conventions.md`. Title: `frontend: mark utility-submit handlers for htmx-guard lint`. This is the slice 1 prerequisite that confirms the marker convention works on real code.

Both commits land in the same PR (single feature, single review).

---

## Slice 2 — orphan `hx-target` / `data-results-target` IDs

### Files

- `audit/discover_htmx_guard.mjs` (extend, +40 LOC)
- `audit/discover_htmx_guard.test.mjs` (extend, +20 LOC synthetic fixtures)
- `internal/appshell/app_orphan_scan_test.go` — add sibling test `TestSettingsQualityScanEndpointRendersResults` matching the existing settings-orphan pattern (already exists per issue; verify the quality-results case is also covered)
- `Makefile` (no change)

### Success criteria

1. Probe walks all `internal/templates/**/*.templ`, collects every `hx-target="#X"`, `data-results-target="#X"`, `data-status-target="#X"`, and verifies each `X` has a matching `id="X"` somewhere in the same glob.
2. `--strict` mode exits 1 on a synthetic templ introducing `hx-target="#nonexistent"` with no matching `id`.
3. Existing IDs from recon (`#browse-results`, `#details-pane`, `#settings-quality-results`, `#settings-orphan-results`, `#soldier-list`, `#job-status-body`, `[data-jobs-progress-region]`, `this`, `#layout-review-count`) all pass — confirmed by step 1.
4. Adhoc element-id selectors like `[data-jobs-progress-region]` (not in `uiids.Registry`) do NOT cause panic — explicitly handled by allowing any non-`#`-prefix selector to pass.

### Regression net

- The probe itself (synthetic templ fixtures)
- Sibling test in `app_orphan_scan_test.go` confirming #settings-quality-results stays present

### Commit shape

- `audit: extend discover_htmx_guard probe with orphan hx-target walker`. Single commit.

---

## Slice 3 — polling-stop render assertions (templ test, not lint)

### Files

- `internal/templates/jobs_templ_test.go` (new, ~50 LOC)

### Success criteria

1. Test renders `jobs.JobStatusView` and `jobs.JobStatusSlotFragment` against `{StatusDone, StatusError, StatusCancelled, StatusInterrupted, StatusRunning}` using templ's test render helpers.
2. For the 4 terminal statuses, rendered output contains `hx-trigger="none"`.
3. For `StatusRunning`, rendered output contains `hx-trigger="every 2s"`.
4. Test fails clearly when the `if job.Status == ...` branch is changed in either templ file.

### Why a templ test, not a lint

The branch condition lives in templ `if job.Status == jobs.StatusDone || ...` and is consumed by code generation — a JS regex cannot reach it. Per locked decision 3 in issue #316.

### Regression net

- The test itself

### Commit shape

- `templ: assert polling stops on terminal state in job fragments`. Single commit.

---

## Slice 4 — enable `validateTarget` panic in dev builds

### Files

- `internal/htmxattr/htmxattr.go` — uncomment the 1-line panic at lines 165-167
- `internal/htmxattr/htmxattr_test.go` — verify `TestMuxAdHocTargetDoesNotPanic` (or equivalent) still passes for `#feedback-form` style adhoc IDs; add new test `TestMuxPanicsOnUnknownRegistryTarget` for `#typo`

### Success criteria

1. `htmxattr.Mux{Target: "#typo"}` panics in dev/test builds when `#typo` is not in `uiids.Registry`.
2. `htmxattr.Mux{Target: "#feedback-form"}` does NOT panic (non-registry IDs allowed).
3. `htmxattr.Mux{Target: ""}` does NOT panic (early return).
4. `htmxattr.Mux{Target: ".cls"}` does NOT panic (non-ID selector allowed).
5. Existing `internal/templates/hx_guard_test.go::TestNoPostThenNavigateHXXAttrs` continues to pass; slice 2 probe continues to pass.

### Regression net

- New + existing `htmxattr_test.go` tests

### Commit shape

- `htmxattr: panic on unknown registry target in dev builds`. Single commit, 1-line code change + 1-line test.

---

## Cross-cutting

- **CHANGELOG:** one bullet per slice under `### Added` (or `### Maintenance` if user-invisible). Issue #316 is user-invisible infra; all bullets land under `### Maintenance`.
- **Makefile:** `RELEASE_PIPELINE_GATES` already documented; new target slots in next to existing `lint-*` entries.
- **Smoke probe:** not needed — `audit/smoke.mjs` covers click-driven surfaces; the lint probes are static, no WebView.
- **AGENTS.md cross-references:** the probe name `discover_htmx_guard.mjs` becomes a documented sibling in AGENTS.md `audit/` file map (optional maintenance commit, can skip).

## Out of scope (per locked decision 5)

§4.1, §4.6, §4.11, §4.13, §4.16, §4.20 of `docs/COMMON_BUGS.md` — already enforced.

## Files touched (all)

- `audit/discover_htmx_guard.mjs` (new)
- `audit/discover_htmx_guard.test.mjs` (new)
- `audit/_lib/fixtures/htmx_guard/*` (new, synthetic fixtures)
- `frontend/app.js` (4-line marker annotations, no behavior change)
- `docs/agents/htmx-guard-conventions.md` (new)
- `internal/templates/jobs_templ_test.go` (new)
- `internal/htmxattr/htmxattr.go` (1-line change)
- `internal/htmxattr/htmxattr_test.go` (1-line test extension)
- `internal/appshell/app_orphan_scan_test.go` (sibling test extension)
- `Makefile` (1 new target)
- `CHANGELOG.md` (per-slice bullets)
- 1 new follow-up GitHub issue filed at slice-1 close

**Net: ~280 LOC across 6 commits (4 functional + 2 setup), 1 follow-up issue.**

## RPCI sign-off gate

Critique phase complete per the 6 locked decisions above.
**Open question for user:** approve the slice 1 commit shape — should the
marker annotations on app.js + conventions doc land as a separate
prereq commit (recommended), or co-land with the probe in one commit?
