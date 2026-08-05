# UI Probe Report — rc/v1.1 @ 4e07c67a

**Date:** 2026-08-05
**Branch:** `rc/v1.1` @ 4e07c67a
**Operator:** Jeremy Morris (LLM-assisted)

## Build & static gates

- `just debug` — OK (42s compile + 4 sibling binaries)
- `go test -short ./...` — all packages pass
- Lint gates (7) — all clean:
  - `lint-dispatcher-patch-method`
  - `lint-htmx-guard`
  - `lint-button-actions-resolve`
  - `lint-foldout-trigger-marker`
  - `lint-bake-bootstrap`
  - `lint-no-nested-forms`
  - `lint-dialog-guard`
- Orphan handlers — 0 (532 invokers across 202 routes)
- Informational: `stale marker: data-article-md-cheatsheet-open` (not failure)

## Aggregated results

| Probe suite | Result |
|---|---|
| `smoke.mjs` (headline) | 56 pass, 2 fail, 1 skip |
| `smoke_aggregator.mjs` | 34 pass / 1 fail (`a11y`) |
| `smoke_a11y.mjs` | 6 clean, 2 warn, 1 violation |
| `smoke_button_matrix.mjs` | 151 pass / 6 fail (probe bug — see below) |
| `smoke_articles.mjs` | 7 pass / 20 fail (probe bug + setup ordering — see below) |
| `smoke_events.mjs` | 0 pass / 1 fail (probe assumption drift — see below) |
| `smoke_swallowed_errors.mjs` | 85 pass / 21 fail (hygiene list — see below) |
| `smoke_research_picker.mjs` | 4 pass / 1 fail (template gap — see below) |
| 30+ other `smoke_*.mjs` | all pass |

## Real findings (filed as issues)

### #724 — `fix(share): /share/export-options returns 405 on POST` (medium)
`POST /share/export-options` returns `405 Method Not Allowed` (`Allow: PATCH`). Smoke probe comment claims "accepts both POST/PATCH"; server only accepts PATCH. Likely PATCH-only is the intent (Wails dispatcher rewrites PATCH→POST via `X-HTTP-Method-Override`, so plain Chromium POST in the audit harness is a separate path that does not exist). Either fix the registration to also accept POST, or update the probe comment.

Repro:
```bash
curl -X POST http://127.0.0.1:8765/share/export-options \
  -d "include_tags=1" \
  -H "Content-Type: application/x-www-form-urlencoded" -i
# HTTP/1.1 405 Method Not Allowed
# Allow: PATCH
```

### #725 — `bug(research): research picker sub-screen fragment missing or selector drift` (medium)
`audit/smoke_research_picker.mjs` `research-pack-sub-screen-renders` step fails. Click into any pack from `/research` and the sub-screen fragment does not render (or the probe selector has drifted). See `docs/ui-map/wireframes/research-picker.md` `panel.research.picker.pack-sub-screen` for the spec.

### #726 — `fix(a11y): /recovery page color-contrast violation (serious, axe)` (low)
`smoke_a11y.mjs` flags 1 serious WCAG 1.4.3 violation:
- Selector: `.text-base.mt-3.text-\[var\(--theme-sepia-300\)\]`
- Color: `var(--theme-sepia-300)`
- Bump the token or change parent background. Sibling to #720.

## Probe-bug failures (NOT real bugs — skip)

- `smoke_articles.mjs` × 20 — probe spins its own ephemeral server with empty DB, hits `/articles/new` before completing setup, so editor-attr checks land on `/setup`. **Editor attrs verified manually via curl** — present and correct:
  - `data-draft-key="new-article"` on `<form>`
  - `data-record-persistence` + `data-record-persistence-kind="new"`
  - `data-article-editor-source` on the textarea
- `smoke_articles.mjs` `preview-sanitizes-script` — **XSS sanitizer works.** Verified directly:
  ```
  POST /articles/preview body=Hello <script>alert(1)</script> world.
  → <p>Hello alert(1) world.</p>   (no <script>)
  ```
- `smoke_button_matrix.mjs` × 6 — closure bug: `checkOneButton()` references `surface.path` at lines 445/447 but `surface` is not in scope (only `visitSurface()` has it). Affects surfaces `settings-maintenance`, `settings-data`, `settings-updates`, `recovery`, `share-landing`, `share-exports`.
- `smoke.mjs` `share-print-modal-openable` — probe navigates `/share` and finds `[data-print-config-open]`, but the submit-button text selector misses the actual button. `data-print-config-open` is present on `/share` (verified via curl).
- `smoke.mjs` `memorial-import-flow` — fixture env (`MEMORIAL_JSON_PATH`) not seeded; expected in CI flow.
- `smoke_events.mjs` `step-01 /events empty-state` — probe expects empty `/events`; current seed yields 2 events (`EVT-00001`, `EVT-00002`). Either seed is over-eager or probe should expect non-empty.

## Static-hygiene findings (`swallowed_errors`)

21 surfaces where handler is not wrapped with `respondErrorFragment` (toast-on-error pattern incomplete). Errors still log; user sees no toast. Not blocking.

Functions missing the wrapper:
- `events_handlers.go ResearchLogView`
- `research_handlers.go`: `UnitCamaraderieEmpty`, `UnitCamaraderieView`, `MergeReviewLedgerView`, `ResearchPackCountyEmpty`, `ResearchPackView`
- `app_update.go SettingsUpdateApplyStarted`
- `settings_handlers.go SettingsView`
- `reviews_handlers.go ReviewQueueView`

Plus `DeferCloseLog` convention check across `backup_service.go` (25 sites), `soldier_service.go` (22), `event_service.go` (6), `audit_service.go` (5), `quality_scan.go` (4), `article_service.go` (4). Cosmetic — convention enforcement, not runtime bugs.

## Surfaces NOT probed (manual test needed)

From `docs/ui-map/INDEX.md` (36 screens); aggregator covered 35. The probe suite does not exercise:

- **Calendar day** (`02`) — month view covered, day-detail drilldown not
- **Soldier detail** (`05`) — `soldier-images` covers images panel; `summary` and `records` panels not deeply exercised
- **Soldier edit** (`07`) — `soldier-new` covered; edit (PATCH to `/soldiers/{id}`) not specifically probed (Wails vs Chromium PATCH override difference)
- **Insights drilldown** (`10`) — overview probed; drilldown links not clicked through
- **Review Queue Compare** (`12`) — bulk list probed; side-by-side compare not
- **Research Collections Hub** (`13`) / **Detail** (`14`) — picker probed; hub + collection-detail not
- **Service Timeline** (`17`) — not probed
- **Article detail** (`29`) — `/articles/{id}` renders, but citation/refs panels not walked
- **Initial Setup** (`22`) — flow probed but most steps assumed
- **Recovery** (`23`) — page loads; restore-from-`.ddbak` flow not walked through end-to-end

## Priority manual checks

1. **`/share` → click any export button → /jobs/{id}?** (covers smoke 5a–5d)
2. **`/soldiers/{id}` → click edit → save → record unchanged?** (#691 empty-body regression net)
3. **`/articles/{id}` → cite a person record → preview modal** (citation insertion path)
4. **Drag-drop image upload on `/soldiers/{id}/edit`** (#682/#689 hazard)
5. **`/recovery` restore flow** (full round-trip from .ddbak)
6. **`/settings/updates` → apply update** (real RC validation per #658)
7. **`/share/exports` → Printable PDF modal submit → /jobs/{id}** (smoke 5b fail — narrow down trigger)
8. **Theme toggle + Classic → reload** (#662 flash bug)
9. **`/research` → click pack → sub-screen renders** (per #725)
10. **Native dialogs (Open/Save File)** in actual Wails window — dialog-guard race only verifiable via real concurrent dialogs

## Reproduction

```bash
just debug
# in a second shell:
cd build/bin && ./dixiedata-web.exe -addr 127.0.0.1:8765
# in a third:
node audit/smoke.mjs
node audit/smoke_aggregator.mjs
node audit/smoke_a11y.mjs
node audit/smoke_research_picker.mjs
node audit/smoke_swallowed_errors.mjs
node audit/smoke_articles.mjs  # probe bug — context only
node audit/smoke_button_matrix.mjs  # probe bug — context only
```

## Files filed

- #724 — `/share/export-options` POST 405
- #725 — research picker sub-screen fragment
- #726 — `/recovery` color-contrast (a11y)