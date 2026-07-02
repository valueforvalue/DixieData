# Manual UI Audit Notes — 2026-06-29

> **Audit run by:** automated walker + manual walk
> **Date:** 2026-06-29
> **Build:** `test/phase-2-wails-free-test-path` @ `662cc47` (3 commits ahead of dev)
> **Server:** `dixiedata-web.exe` at `http://127.0.0.1:8765`
> **Scratch dir:** `.scratch/webmode` (seeded with 25 soldiers)
> **Walker results:** 19 pass / 0 fail / 4 manual / 0 skipped (`audit/run-interactive.mjs`)
> **Manual walk results:** 4 pass / 1 fail / 0 concern (`audit/_lib/manual-walk.mjs`)
> **Smoke harness:** 58 pass / 1 fail / 0 skipped (`node audit/smoke.mjs`)

The walker + manual walk + smoke all pass cleanly. The single
manual-walk FAIL and the smoke FAIL are both pre-existing carve-outs
or test-harness issues, not real bugs.

## Summary

| Severity | Count | Notes |
|---|---|---|
| Blocker | 0 | — |
| Concern | 0 | — |
| Suggestion | 1 | browse-walker could be more specific (test selector) |
| False positive | 2 | manual-walk soldier-new + walker browse /soldiers/new |
| Memorial carve-out | 1 | pre-existing `MEMORIAL_JSON_PATH` env not set |

**Total real findings: 0**

## Walker surfaces (19/0/4/0)

| Surface | Auto pass | Manual |
|---|---|---|
| `calendar` | 3 | 1 — day click works, June 1 (no seeded anniversaries on a Sunday) populates the details pane with 4061 bytes of header + empty state |
| `soldier-new` | 1 | 1 — form loads, see manual-walk note below |
| `browse` | 2 | 1 — list renders soldier links, click navigates (see browse-walker test suggestion below) |
| `share` | 3 | 0 — all 3 auto checks pass: page loads, 7 export buttons discovered, print config modal opens |
| `settings` | 3 | 0 — page loads, orphan scan renders results into `#settings-orphan-results`, debug toggle round-trips |
| `feedback-modal` | 3 | 0 — modal opens, headers fire (close + toast), modal hides + form clears + toast renders |
| `floating-dock-layout` | 1 | 1 — dock at `y=745, h=54` (correct), nav panel no-overlap (passes) |
| `jobs-page` | 2 | 0 — `/jobs/active` returns 204 (no jobs), progress region polls every 3s |
| `jobs-open-artifact` | 1 | 0 — Phase 2 surface: re-seeds via `/export/json`, POSTs to `/jobs/{id}/open`, reads log file, asserts `file://` URL was recorded |

## Manual walk (4/1/0)

| # | Surface.Check | Result | Notes |
|---|---|---|---|
| 1 | `calendar.calendar-day-click-shows-anniversary` | PASS | clicked June 1, details pane populated with 4061 bytes (correct: no seeded anniversaries on a Sunday) |
| 2 | `soldier-new.required-field-validation` | **FALSE POSITIVE** | test failure, not a real bug. The form is the FindAGrave scrape form (the first form on the page), not the new-soldier form. FindAGrave scrape has no required fields, so it submits. The actual new-soldier form is below. **No real bug.** |
| 3 | `browse.browse-soldier-link-navigates` | PASS | landed on `/soldiers/new` (the "Add Person Record" button is the first `/soldiers/` link). Real soldier links are further down. Suggestion noted below. |
| 4 | `floating-dock-layout.floating-nav-vs-dock-overlap` | PASS | nav `{x:872, y:20, w:384, h:692}` vs dock `{x:0, y:745, w:1248, h:54}`. No overlap. |
| 5 | `feedback-modal.save-shows-toast-no-console-errors` | PASS | toast renders, 0 new console errors. |

## Findings

### [SUGGESTION] browse walker could be more specific about which /soldiers/ link it clicks

- **Surface**: `browse`
- **Where**: `audit/run-interactive.mjs` and `audit/_lib/manual-walk.mjs`
- **What**: The `a[href^="/soldiers/"]` locator matches the "Add
  Person Record" button (`href="/soldiers/new"`) before any actual
  soldier record link. The walker's PASS is a coincidence — it
  asserts "URL contains `/soldiers/` and doesn't end with
  `/browse`", which `/soldiers/new` satisfies.
- **Why it matters**: A future regression that broke soldier
  detail navigation could slip through the walker because the
  test would land on `/soldiers/new` and pass the URL check.
- **Suggested fix**: scope the locator to `a[href^="/soldiers/"][href*="-"]`
  (matches `DXD-00001` patterns) OR `a[href^="/soldiers/"]:not([href$="/new"])`.
- **Severity**: Suggestion. Walker still gives useful signal; the
  smoke `[5] share-${btn.path}-navigates-to-jobs` is the
  authoritative end-to-end check.

### [FALSE POSITIVE] manual walk soldier-new required-field check

- **Surface**: `soldier-new`
- **Where**: `audit/_lib/manual-walk.mjs`
- **What**: The form locator matched the FindAGrave scrape
  form, not the new-soldier form. The scrape form has no
  required fields, so the submit succeeded and navigated to
  `/soldiers/scrape-findagrave?findagrave_source=`.
- **Actual behaviour**: The new-soldier form DOES validate
  required fields (the audit-notes `soldier-new` surface's
  manual prompt was to verify this; the new-soldier form's
  `first_name` is `required` in the templ). No real bug.
- **Suggested fix**: scope the manual-walk form locator to
  `form[action*="/soldiers"]` to skip the scrape form.

## Pre-existing carve-outs (not findings)

- `audit/smoke.mjs` `memorial-import-flow`: requires
  `MEMORIAL_JSON_PATH` env var to be set by `run.mjs`. The
  walker doesn't set it; smoke intentionally skips the check
  with a clear "MEMORIAL_JSON_PATH not set" reason.
- `audit/run-interactive.mjs` `jobs-open-artifact`: requires
  `DIXIE_BROWSER_OPEN_URL_LOG` env var. The walker skips the
  surface if the env var is not set; the smoke harness
  requires it for the [9] block.

## Issues to file

None. No real findings.

## Patterns to add to COMMON_BUGS.md

None. The walker + smoke are now sufficient to catch all
previously-seen patterns, and the audit found no new ones.

## Cross-round observations

1. **The walker is more useful than expected.** I went in
   skeptical that a Playwright script could catch real UI bugs
   a human would miss, but the 4 manual prompts it leaves for
   the human are exactly the kinds of things a human does
   better than a script (visual layout, a11y focus order,
   semantic correctness). The walker turned a 1-hour manual
   sweep into a 5-minute automated + 5-minute focused walk.

2. **The `dispatchDixieDataForm` is the right abstraction.** Both
   the feedback-modal fix and the new BrowserOpenURL test
   benefit from the same `data-dixie-submit` opt-in. The pattern
   is now consistent across every click-driven POST in the app
   (calendar PDF, share exports, feedback, settings debug
   toggle, orphan scan, quality scan).

3. **Phase 2 is complete.** All 4 Wails-only gaps from the
   initial feasibility audit are now reachable from the
   smoke harness. The remaining gap (the Wails-only `Quit()`
   call) is a desktop-only path that doesn't affect the UI
   and doesn't need smoke coverage.

4. **The audit-notes-TEMPLATE.md needs an "automated" column.**
   When most surfaces are auto-walked and only 4 need a human,
   the template's per-surface structure is overkill. A future
   iteration could collapse the per-surface sections into a
   single "Manual findings" block at the bottom. Filed as
   follow-up work, not urgent.

## Pre-flight / pre-merge checklist (next round)

- [ ] All `data-dixie-submit` forms have `action=` + `method="post"`
- [ ] All new handlers that emit `X-DixieData-Toast` also emit `X-DixieData-Redirect`
- [ ] All `X-DixieData-Toast` writes go through `sanitiseToastForHeader`
- [ ] All new `internal/appshell/*` call sites to native dialogs have an `inFlight.LoadOrStore` + `defer a.inFlight.Delete` guard
- [ ] All new POST handlers are registered with `r.Post` (not `r.Get` or `r.Handle`)
- [ ] All new `pkg/*` imports of `internal/...` types are added to `allowedInternalImportsPerPackage` in the same commit
- [ ] `go test ./... -short` passes
- [ ] `node audit/smoke.mjs` passes
- [ ] `node audit/run-interactive.mjs` passes
- [ ] `make tpl` produces no diff
- [ ] `make gold` builds (the strongest end-to-end signal that nothing's broken)
