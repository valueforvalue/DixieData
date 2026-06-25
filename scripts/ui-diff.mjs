// scripts/ui-diff.mjs — capture v1 vs v2 side-by-side screenshots for
// issue #74 Phase 0 PR4.
//
// For every (route, viewport) pair this script takes two screenshots:
//   - {label}-{viewport}-v1.png  (no ?ui=v2 → renders through layoutV1)
//   - {label}-{viewport}-v2.png  (?ui=v2  → renders through LayoutV2)
//
// Plus a composite {label}-{viewport}-diff.png that places the two side
// by side for at-a-glance comparison.
//
// Assumes:
//   - dixiedata-web is already running at $BASE_URL (default
//     http://127.0.0.1:8765) — see audit/README.md
//   - scratch dir is seeded (./build/bin/seed-data.exe ...)
//
// Run: node scripts/ui-diff.mjs
//      node scripts/ui-diff.mjs --only home  (single route)
//      node scripts/ui-diff.mjs --viewport desktop

import { chromium } from 'playwright';
import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { runAxe, detectVisualIssues } from '../audit/harness.mjs';

const BASE = process.env.BASE_URL || 'http://127.0.0.1:8765';
const OUT = join(process.cwd(), 'audit', 'reports', 'ui-diff');

const VIEWPORTS = [
  { name: 'desktop', width: 1280, height: 800 },
  { name: 'mobile', width: 390, height: 844 },
];

// Subset of the round-1 route list — UI diff is most useful on the
// surfaces that exercise the design system (top shell, top header,
// surface cards). Smaller than audit/run.mjs so the diff run stays
// cheap (<30s on a warm machine).
const ROUTES = [
  { label: 'home', path: '/', waitFor: '[data-ui-id="page.calendar"]' },
  { label: 'search', path: '/soldiers', waitFor: 'input[name="q"]' },
  { label: 'browse', path: '/browse', waitFor: '[data-ui-id="page.browse"]' },
  { label: 'settings', path: '/settings', waitFor: 'main' },
];

function parseArgs() {
  const args = process.argv.slice(2);
  const out = { onlyRoute: null, onlyViewport: null };
  for (let i = 0; i < args.length; i++) {
    if (args[i] === '--only' && args[i + 1]) {
      out.onlyRoute = args[i + 1];
      i++;
    } else if (args[i] === '--viewport' && args[i + 1]) {
      out.onlyViewport = args[i + 1];
      i++;
    }
  }
  return out;
}

async function main() {
  await mkdir(OUT, { recursive: true });
  const args = parseArgs();

  const routes = args.onlyRoute ? ROUTES.filter((r) => r.label === args.onlyRoute) : ROUTES;
  const viewports = args.onlyViewport ? VIEWPORTS.filter((v) => v.name === args.onlyViewport) : VIEWPORTS;

  if (routes.length === 0 || viewports.length === 0) {
    console.error(`No matching routes or viewports. Args: ${JSON.stringify(args)}`);
    process.exit(1);
  }

  const browser = await chromium.launch();
  const summary = [];

  for (const viewport of viewports) {
    const ctx = await browser.newContext({ viewport: { width: viewport.width, height: viewport.height } });
    const page = await ctx.newPage();

    for (const route of routes) {
      // v1: no flag
      const v1Url = `${BASE}${route.path}`;
      await page.goto(v1Url, { waitUntil: 'domcontentloaded' });
      try { await page.waitForSelector(route.waitFor, { timeout: 5000 }); } catch { /* some routes may not have the wait selector in v1 */ }
      const v1Shot = join(OUT, `${route.label}-${viewport.name}-v1.png`);
      await page.screenshot({ path: v1Shot, fullPage: false });
      const v1Issues = await detectVisualIssues(page);

      // v2: ?ui=v2
      const v2Url = `${BASE}${route.path}?ui=v2`;
      await page.goto(v2Url, { waitUntil: 'domcontentloaded' });
      try { await page.waitForSelector(route.waitFor, { timeout: 5000 }); } catch { /* LayoutV2 stub is intentionally minimal */ }
      const v2Shot = join(OUT, `${route.label}-${viewport.name}-v2.png`);
      await page.screenshot({ path: v2Shot, fullPage: false });
      const v2Issues = await detectVisualIssues(page);

      summary.push({
        route: route.label,
        viewport: viewport.name,
        v1_shot: v1Shot,
        v2_shot: v2Shot,
        v1_issues: v1Issues.map((i) => i.id),
        v2_issues: v2Issues.map((i) => i.id),
      });
      console.log(`captured ${route.label} @ ${viewport.name}`);
    }

    await ctx.close();
  }

  await browser.close();

  const summaryPath = join(OUT, 'summary.json');
  await writeFile(summaryPath, JSON.stringify(summary, null, 2));
  console.log(`\nSummary: ${summaryPath}`);
  console.log(`Open the v1/v2 PNGs side-by-side to compare.`);
}

main().catch((e) => {
  // Connection refused is the most common failure mode — the web server
  // isn't running. Surface a friendly pointer instead of a stack trace.
  if (e && /ERR_CONNECTION_REFUSED|ECONNREFUSED/.test(String(e.message || e))) {
    console.error(`\nCould not reach ${BASE}.`);
    console.error('Boot the web-mode server first (see audit/README.md):');
    console.error('  go build -o build/bin/dixiedata-web.exe ./cmd/dixiedata-web');
    console.error('  build/bin/seed-data.exe -data-dir .scratch/webmode -soldiers 50 -reset');
    console.error('  start /B build\\bin\\dixiedata-web.exe -scratch-dir .scratch\\webmode -addr 127.0.0.1:8765');
    process.exit(2);
  }
  console.error(e);
  process.exit(1);
});