// DixieData UI/UX audit harness — round 1 (top-level routes).
// Boots a headless browser against the dixiedata-web server, walks every
// discoverable route, screenshots at desktop/tablet/mobile widths, runs
// axe-core for accessibility, and writes findings to audit/reports/.
//
// Assumes:
//   - dixiedata-web is already running at $BASE_URL (default http://127.0.0.1:8765)
//   - scratch dir is already seeded (./build/bin/seed-data.exe -data-dir .scratch/webmode -reset)
//
// Run: node audit/run.mjs

import { chromium } from 'playwright';
import { mkdir, writeFile, rm } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { runAxe, detectVisualIssues, renderSummary } from './harness.mjs';

const __dirname = dirname(fileURLToPath(import.meta.url));
const SHOTS = join(__dirname, 'screenshots');
const REPORTS = join(__dirname, 'reports');
const BASE = process.env.BASE_URL || 'http://127.0.0.1:8765';

const VIEWPORTS = [
  { name: 'desktop', width: 1280, height: 800 },
  { name: 'tablet', width: 900, height: 1200 },
  { name: 'mobile', width: 390, height: 844 },
];

// Routes to walk. Each is a label and the URL path.
const ROUTES = [
  { label: 'home', path: '/', waitFor: '[data-ui-id="page.calendar"]' },
  { label: 'calendar', path: '/calendar', waitFor: '[data-ui-id="page.calendar"]' },
  { label: 'search', path: '/soldiers', waitFor: 'form input[name="q"]' },
  { label: 'browse', path: '/browse', waitFor: '[data-ui-id="page.browse"]' },
  { label: 'review-queue', path: '/review-queue', waitFor: 'main' },
  { label: 'insights', path: '/insights', waitFor: 'main' },
  { label: 'share', path: '/share', waitFor: 'main' },
  { label: 'settings', path: '/settings', waitFor: 'main' },
  { label: 'soldier-new', path: '/soldiers/new', waitFor: 'main' },
];

// Discover a single soldier detail/edit pair to audit record-level UX.
async function discoverSoldierRoutes(page) {
  const out = [];
  try {
    const response = await page.request.get(`${BASE}/soldiers/search?q=a`);
    const html = await response.text();
    const matches = [...new Set([...html.matchAll(/href="\/soldiers\/(\d+)(?:\/edit)?"/g)].map((m) => m[1]))];
    for (const id of matches.slice(0, 2)) {
      out.push({ label: `soldier-${id}`, path: `/soldiers/${id}`, waitFor: '[data-ui-id="page.soldier.detail"]' });
      out.push({ label: `soldier-${id}-edit`, path: `/soldiers/${id}/edit`, waitFor: 'main' });
    }
  } catch (e) {
    console.warn('soldier discovery failed:', e.message);
  }
  return out;
}

async function main() {
  if (existsSync(SHOTS)) await rm(SHOTS, { recursive: true, force: true });
  if (existsSync(REPORTS)) await rm(REPORTS, { recursive: true, force: true });
  await mkdir(SHOTS, { recursive: true });
  await mkdir(REPORTS, { recursive: true });

  const browser = await chromium.launch({ headless: true });
  const allRoutes = [];
  const allFindings = [];

  // First pass: discover dynamic routes using a generic context.
  const discovery = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  const discoveryPage = await discovery.newPage();
  const soldierRoutes = await discoverSoldierRoutes(discoveryPage);
  await discovery.close();
  console.log(`Discovered ${soldierRoutes.length} soldier routes`);

  const routes = [...ROUTES, ...soldierRoutes];

  for (const vp of VIEWPORTS) {
    console.log(`\n=== Viewport: ${vp.name} (${vp.width}x${vp.height}) ===`);
    const context = await browser.newContext({ viewport: { width: vp.width, height: vp.height } });
    const page = await context.newPage();

    const pageErrors = [];
    page.on('pageerror', (err) => pageErrors.push({ viewport: vp.name, message: err.message }));
    page.on('requestfailed', (req) => {
      const url = req.url();
      if (url.startsWith(BASE)) {
        pageErrors.push({ viewport: vp.name, kind: 'request-failed', url, error: req.failure()?.errorText });
      }
    });

    for (const route of routes) {
      const url = `${BASE}${route.path}`;
      const label = `${vp.name}_${route.label}`;
      console.log(`  ${label} -> ${url}`);
      const t0 = Date.now();
      try {
        const response = await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 15000 });
        const status = response?.status();
        if (route.waitFor) {
          await page.waitForSelector(route.waitFor, { timeout: 5000 }).catch(() => null);
        }
        await page.waitForTimeout(400);

        const shot = join(SHOTS, `${label}.png`);
        await page.screenshot({ path: shot, fullPage: true });

        const axeResults = await runAxe(page);
        const visual = await detectVisualIssues(page);

        const report = {
          label,
          viewport: vp.name,
          path: route.path,
          url,
          status,
          load_ms: Date.now() - t0,
          isFragment: axeResults.skipped,
          fragment_reason: axeResults.reason,
          axe_violations: (axeResults.violations || []).map((v) => ({
            id: v.id,
            impact: v.impact,
            help: v.help,
            helpUrl: v.helpUrl,
            nodes: v.nodes.length,
            sample: v.nodes.slice(0, 2).map((n) => n.target),
          })),
          visual_issues: visual,
        };
        allRoutes.push(report);
        if (axeResults.skipped) {
          allFindings.push({ label, path: route.path, viewport: vp.name, kind: 'axe-skipped', reason: axeResults.reason });
        } else {
          allFindings.push(...axeResults.violations.flatMap((v) =>
            v.nodes.map((n) => ({
              label, path: route.path, viewport: vp.name,
              kind: 'a11y', id: v.id, impact: v.impact, help: v.help, target: n.target,
            }))
          ));
        }
        allFindings.push(...visual.map((v) => ({ label, path: route.path, viewport: vp.name, kind: 'visual', ...v })));

        const violCount = axeResults.violations?.length || 0;
        if (axeResults.skipped) {
          console.log(`    axe: SKIPPED (${axeResults.reason})`);
        } else if (violCount > 0) {
          console.log(`    axe: ${violCount} violation types`);
        }
      } catch (e) {
        allFindings.push({ label, path: route.path, viewport: vp.name, kind: 'crash', error: e.message });
        console.log(`    FAILED: ${e.message}`);
      }
    }

    if (pageErrors.length > 0) {
      console.log(`  page errors: ${pageErrors.length}`);
      allFindings.push(...pageErrors.map((e) => ({ kind: 'pageerror', ...e })));
    }
    await context.close();
  }

  await browser.close();

  await writeFile(join(REPORTS, 'routes.json'), JSON.stringify(allRoutes, null, 2));
  await writeFile(join(REPORTS, 'findings.json'), JSON.stringify(allFindings, null, 2));

  const summary = renderSummary({
    title: 'DixieData UI/UX Audit — Summary',
    routes: allRoutes,
    findings: allFindings,
  });
  await writeFile(join(REPORTS, 'summary.md'), summary);

  console.log(`\nReports written to ${REPORTS}`);
  console.log(`  - routes.json   (per-route axe + visual)`);
  console.log(`  - findings.json (flat finding list)`);
  console.log(`  - summary.md    (human-readable overview)`);
  console.log(`\nScreenshots in ${SHOTS}`);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});