# DixieData UI/UX Audit Harness

This directory contains the playwright-based audit harness for DixieData.
It drives the real web-mode UI in a headless browser, runs axe-core
accessibility scans, applies custom DOM heuristics for visual issues, and
runs interactive flow tests. Every slice of the [#74 UI/UX sprint](./reports/SLICES.md)
landed with the harness as the regression net.

---

## Quick start

```bash
# 1. Build the web-mode entrypoint (one-time)
go build -o build/bin/dixiedata-web.exe ./cmd/dixiedata-web

# 2. Seed a clean test dataset (one-time, repeat only to reset state)
build/bin/seed-data.exe -data-dir .scratch/webmode -soldiers 50 -reset

# 3. Start the server (background)
cmd //c "start /B build\bin\dixiedata-web.exe -scratch-dir .scratch\webmode -addr 127.0.0.1:8765 > build\log\webmode.log 2>&1"

# 4. Run any round of audits
node audit/run.mjs          # 13 top-level routes × 3 viewports
node audit/run-round2.mjs   # 10 deep routes (soldier sub-pages, compare, research)
node audit/run-round3.mjs   # 12 regression routes × 2 viewports + 9 interactive flow tests
```

Each script writes to `audit/reports*/{routes,findings,flows,summary}.{json,md}`.
Screenshots go to `audit/screenshots*/` (gitignored — re-generate with the
harness scripts).

To stop the server when done:

```bash
netstat -ano | grep 8765 | head -1 | awk '{print $5}' | xargs -I{} cmd //c "taskkill /F /PID {}"
```

---

## File layout

```
audit/
├── README.md                          # this file
├── package.json                        # playwright + @axe-core/playwright
├── run.mjs                             # round 1 — top-level routes
├── run-round2.mjs                      # round 2 — deep routes
├── run-round3.mjs                      # round 3 — verification + interactive flows
├── harness.mjs                         # shared helpers (runAxe, detectVisualIssues, renderSummary)
├── reports/
│   ├── SLICES.md                       # historical record of every slice in the sprint
│   ├── audit-v1.md                     # round 1 narrative (originally written 2026-06-24)
│   ├── audit-v2.md                     # round 2 narrative
│   ├── comprehensive-plan.md            # original 12-slice plan
│   ├── routes.json                     # per-route detail for every audited page
│   ├── findings.json                   # flat findings list
│   └── summary.md                      # machine-generated summary
├── reports-r2/                         # round 2 output
├── reports-r3/                         # round 3 output
├── screenshots/                         # round 1 PNGs (gitignored)
├── screenshots-r2/                     # round 2 PNGs (gitignored)
└── screenshots-r3/                     # round 3 PNGs (gitignored)
```

---

## Architecture

### Server (`cmd/dixiedata-web`)

`cmd/dixiedata-web/main.go` boots the existing `appshell.App` on plain HTTP
at `127.0.0.1:8765`. It pins `DIXIEDATA_DATA_DIR` to a sandbox directory
(`./.scratch/webmode` by default) so audits never touch the real
`.dixiedata/` user data. The Wails-specific dialog handlers panic on call,
so the audit is intentionally read-only — it never triggers
`runtime.OpenFileDialog` or `runtime.SaveFileDialog`.

The server reads `frontend/{app.js, app.css}` directly from disk when no
embedded assets are provided, so template and CSS changes take effect on
the next request without a rebuild.

### Audit harness (`audit/harness.mjs`)

Three exports used by all round scripts:

#### `runAxe(page, opts?) → { skipped, reason, violations }`

Runs `@axe-core/playwright` with `wcag2a`, `wcag2aa`, `wcag21aa` tags.
Returns `{ skipped: true, reason }` when the page is an HTMX fragment
(detected by re-fetching the URL via `page.request.get` and looking for
`<html>` in the response body). Otherwise returns `{ skipped: false, violations }`
where violations is the raw axe-core output.

#### `detectVisualIssues(page) → Issue[]`

Runs in-page DOM heuristics that axe-core doesn't catch. Returns an array
of `{ id, severity, count?, sample? }`:

| id | severity | what it finds |
|---|---|---|
| `h-scroll` / `h-scroll-body` | high | document/body scrollWidth exceeds viewport |
| `overflow-x` | high | elements with bounding rect right > viewport + 1px |
| `small-tap-target` | medium | buttons/links/role=button <24px on either axis |
| `unlabeled-input` | medium | inputs/selects/textareas without label/aria/placeholder |
| `nav-density` | info | number of nav items and average width per item |
| `empty-state-visible` | info | `[data-empty-state]` or `.empty-state` is rendered |
| `table-rows` | info | row count + page height for pagination planning |
| `stuck-busy` | medium | `[aria-busy="true"]` elements remaining after load |
| `nested-tables` | medium | more than one `<table>` on the page |
| `dead-onclick` | low | `<element onclick="">` with empty handler (no longer fires — leftover) |
| `truncated-with-title` | low | `[title]` element with scrollWidth > clientWidth |

#### `renderSummary({ title, routes, findings }) → string`

Markdown formatter. Emits severity counts, finding types, and a per-route
table. Reused by all three round scripts so the markdown structure stays
consistent across rounds.

### Round scripts

`run.mjs` (round 1) — 13 top-level routes including home/calendar, search,
browse, review-queue, insights, share, settings, soldier-new, and a
discovery pass that finds 2 more soldier IDs to add to the route list.

`run-round2.mjs` (round 2) — 10 deep routes: soldier sub-pages
(camaraderie, timeline, research-log, conflict-ledger, research-pack state +
county), compare, research-collections, search-advanced, and a second
soldier detail.

`run-round3.mjs` (round 3) — 12 regression routes plus 9 interactive
flow tests. The regression routes overlap with round 1 so you can
diff before/after. The flow tests live in `main()` after the route loop:

- `hamburger-opens-drawer`
- `hamburger-esc-closes`
- `hamburger-focus-returns`
- `browse-filter-applies`
- `browse-filter-persists-from-url`
- `compare-region-keyboard-accessible`
- `compare-differences-pills-present`
- `browse-compare-selection-enabled`
- `feedback-modal-opens-and-closes`

---

## Adding a new audit

### Add a route to round 1

Edit `audit/run.mjs`, find the `ROUTES` array, append a new entry:

```js
{ label: 'my-new-route', path: '/my-route', waitFor: '[data-ui-id="page.my-route"]' },
```

If your route doesn't render any known page identifier, use `'main'` as
the `waitFor` (always present) or a more specific selector. Run:

```bash
node audit/run.mjs
```

The output `audit/reports/routes.json` includes the new route. Findings
appear in `audit/reports/findings.json` with `path: '/my-route'`.

### Add a route to round 2 (deep routes)

Same as above, but edit `audit/run-round2.mjs` and use the same array shape.

### Add a flow test to round 3

Edit `audit/run-round3.mjs`. Each flow is an `async function testXxxFlow(page, flowResults)`
that:

1. Navigates to the relevant page (or uses the current page state)
2. Performs some user gesture (click, type, keypress)
3. Calls `page.evaluate(...)` or `page.locator(...).count()` to verify
4. Pushes a result onto `flowResults`: `{ name, passed, details }`

Example skeleton:

```js
async function testMyFlow(page, flowResults) {
  await page.goto(`${BASE}/my-route`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(300);
  // do the gesture
  await page.click('button');
  // verify
  const ok = await page.evaluate(() => /* check something */);
  flowResults.push({
    name: 'my-flow',
    passed: ok,
    details: { /* diagnostic info */ },
  });
}
```

Register the flow at the bottom of `main()`:

```js
if (vp.name === 'desktop') {
  await testMyFlow(page, flowResults);
}
```

(Flows only run on desktop by convention — mobile interactions are mostly
a subset. Add them inside the mobile viewport block if you need them.)

### Add a new visual detector

Edit `audit/harness.mjs`, find `detectVisualIssues`, add your heuristic
inline. Return issues in the same shape `{ id, severity, ... }` so the
existing renderer and CSV-style summary code picks them up automatically.

### Add a new axe check

axe-core tags (`wcag2a`, `wcag2aa`, `wcag21aa`) are already enabled in
`runAxe`. To enable a specific rule set, edit `harness.mjs`:

```js
const builder = new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa', 'best-practice']);
```

If you want to disable a specific rule:

```js
const builder = new AxeBuilder({ page }).disableRules(['color-contrast']);
```

---

## Reading the output

### `reports/summary.md`

Markdown overview. Look at:
- "A11y violations by severity" — should be 0 critical / 0 serious for any release
- "Findings by type" — high counts of any type indicate a systematic issue
- "Per-route snapshot" — which routes are accumulating violations

### `reports/findings.json`

Flat array of findings. Each entry has either `kind: 'a11y'` (axe result)
or `kind: 'visual'` (heuristic result). Filter with:

```bash
node -e "
const f = require('./audit/reports/findings.json');
f.filter(x => x.id === 'overflow-x').forEach(x => console.log(x.label, x.path, x.viewport));
"
```

### `reports-r3/flows.json`

9 flow results. `passed: false` entries include `details` with diagnostic
info — read these to debug failing flows.

### Screenshots

`audit/screenshots/{desktop|mobile|tablet}_{route}.png`. Full-page screenshots
from the audit runs. Use these for visual regression review.

---

## CI integration (suggested)

The harness is designed to run unattended in CI. Recommended setup:

```yaml
# .github/workflows/audit.yml
name: UI/UX audit
on: [push, pull_request]
jobs:
  audit:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.25' }
      - uses: actions/setup-node@v4
        with: { node-version: '20' }
      - run: npm ci
      - run: go build -o build/bin/dixiedata-web ./cmd/dixiedata-web
      - run: go build -o build/bin/seed-data ./cmd/seed-data
      - run: ./build/bin/seed-data -data-dir .scratch/webmode -soldiers 50 -reset
      - run: ./build/bin/dixiedata-web -scratch-dir .scratch/webmode &
      - run: sleep 2
      - run: node audit/run.mjs
      - run: node audit/run-round2.mjs
      - run: node audit/run-round3.mjs
      - uses: actions/upload-artifact@v4
        with:
          name: audit-reports
          path: |
            audit/reports/
            audit/reports-r2/
            audit/reports-r3/
            audit/screenshots*/
```

Suggested failure thresholds:
- Critical a11y violations > 0 → fail
- Any flow test `passed: false` → fail
- Total findings more than 10% over previous round → warn

---

## Known limitations

- **htmx never loads in web mode.** All `hx-*` attributes in templ files
  are inert when served by `dixiedata-web`. Round 3 documented a workaround
  for the `browse-filter-applies` flow test that uses the badge counter
  instead of waiting for an htmx-triggered URL change. The Wails desktop
  build presumably bundles htmx.
- **Headless Chromium content-visibility quirks.** `content-visibility:
  auto` causes Playwright's `fullPage: true` screenshots to skip rendering
  off-screen rows in headless mode. Don't add this to table rows.
- **Web-mode lacks the Wails CSS env var.** Templates that rely on
  `templ.SafeURL("wails://...")` will produce broken links in web mode.
  None of the audited templates do this.

---

## Extending the harness

### Custom DOM selector for fragment detection

`detectFragment` in `harness.mjs` checks for `<html>` tag presence in the
response body. Add more rules if you encounter new fragment endpoints:

```js
export function detectFragment({ contentType, body }) {
  if (!contentType || !contentType.toLowerCase().includes('text/html')) {
    return { isFragment: true, reason: `content-type=${contentType || 'unset'}` };
  }
  if (typeof body !== 'string' || body.length < 200) {
    return { isFragment: true, reason: `body length ${body?.length ?? 0} < 200` };
  }
  if (!/<html[\s>]/i.test(body)) {
    return { isFragment: true, reason: 'no <html> tag in body' };
  }
  // ADD CUSTOM RULES HERE:
  // if (body.includes('marker-for-our-fragment')) {
  //   return { isFragment: true, reason: 'custom marker detected' };
  // }
  return { isFragment: false, reason: null };
}
```

### Adding axe rules to run by default

Edit `runAxe` in `harness.mjs`:

```js
const builder = new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']);
// Add specific rules:
// const builder = new AxeBuilder({ page }).withRules(['color-contrast', 'image-alt']);
```

### Different viewports

The `VIEWPORTS` array in each round script defines which sizes to test.
Add new sizes by appending:

```js
{ name: 'wide-desktop', width: 1920, height: 1080 },
```

The audit harness picks up any viewport automatically — the label is used
in screenshot filenames and finding identifiers.

---

## Quick troubleshooting

**`net::ERR_CONNECTION_REFUSED` at start:** the server isn't running. Start
it before running any audit script.

**All routes show 0 findings:** double-check the seed step actually ran
and populated records. `build/bin/seed-data -data-dir .scratch/webmode -reset`
to reset.

**Findings count jumps unexpectedly:** check whether localStorage state
from a previous run is bleeding into the new run. The audit doesn't clear
localStorage between route visits. `await page.context().clearCookies()`
and `await page.evaluate(() => localStorage.clear())` if you need a clean
state — extend `run-round3.mjs` to do this for flows that test URL-persisted
state (see `browse-filter-persists-from-url`).

**Axe reports `document-title` or `html-has-lang` on a non-fragment route:**
that means the new route is actually returning a fragment body. Investigate
the handler — it should return a full page.

**Screenshots show a blank area:** the dock or another fixed-position
element is covering the content. The audit detects this as `overflow-x`
or `h-scroll`. Re-run with a viewport change to confirm.

---

## See also

- [`audit/reports/SLICES.md`](./reports/SLICES.md) — historical record of
  every slice that landed during the audit sprint
- [`audit/reports/comprehensive-plan.md`](./reports/comprehensive-plan.md)
  — original 12-slice plan with sequencing dependency graph
- [`audit/reports/audit-v1.md`](./reports/audit-v1.md) — round 1 narrative
- [`audit/reports/audit-v2.md`](./reports/audit-v2.md) — round 2 narrative
- Parent epic: <https://github.com/valueforvalue/DixieData/issues/74>
- Wails docs: <https://wails.io/docs/introduction>
- Playwright: <https://playwright.dev/docs/intro>
- axe-core: <https://github.com/dequelabs/axe-core>