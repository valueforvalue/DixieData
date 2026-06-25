// Round 2 audit: deeper routes not covered in run.mjs.
// Soldier sub-pages (camaraderie, timeline, research-log, conflict-ledger,
// research-pack/{state,county}), compare flow, research-collections,
// advanced search.
//
// Assumes:
//   - dixiedata-web is already running at $BASE_URL (default http://127.0.0.1:8765)
//   - scratch dir is already seeded (use soldier ids 47 and 35 — first two from search "a")

import { chromium } from 'playwright';
import { mkdir, writeFile, rm } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { runAxe, detectVisualIssues, renderSummary } from './harness.mjs';

const __dirname = dirname(fileURLToPath(import.meta.url));
const SHOTS = join(__dirname, 'screenshots-r2');
const REPORTS = join(__dirname, 'reports-r2');
const BASE = process.env.BASE_URL || 'http://127.0.0.1:8765';

const VIEWPORTS = [
  { name: 'desktop', width: 1280, height: 800 },
  { name: 'tablet', width: 900, height: 1200 },
  { name: 'mobile', width: 390, height: 844 },
];

const SOLDIER_A = 47;
const SOLDIER_B = 35;

function makeRoutes() {
  const s = SOLDIER_A;
  return [
    { label: 'camaraderie', path: `/soldiers/${s}/camaraderie`, waitFor: 'main' },
    { label: 'timeline', path: `/soldiers/${s}/timeline`, waitFor: 'main' },
    { label: 'research-log', path: `/soldiers/${s}/research-log`, waitFor: 'main' },
    { label: 'conflict-ledger', path: `/soldiers/${s}/conflict-ledger`, waitFor: 'main' },
    { label: 'research-pack-state', path: `/soldiers/${s}/research-pack/state`, waitFor: 'main' },
    { label: 'research-pack-county', path: `/soldiers/${s}/research-pack/county`, waitFor: 'main' },
    { label: 'compare', path: `/compare?id1=${SOLDIER_A}&id2=${SOLDIER_B}`, waitFor: 'main' },
    { label: 'research-collections', path: '/research-collections', waitFor: '[data-ui-id="page.research-collections.hub"]' },
    { label: 'search-advanced', path: '/soldiers/search/advanced?q=test', waitFor: 'main' },
    { label: 'soldier-detail-b', path: `/soldiers/${SOLDIER_B}`, waitFor: '[data-ui-id="page.soldier.detail"]' },
  ];
}

async function main() {
  if (existsSync(SHOTS)) await rm(SHOTS, { recursive: true, force: true });
  if (existsSync(REPORTS)) await rm(REPORTS, { recursive: true, force: true });
  await mkdir(SHOTS, { recursive: true });
  await mkdir(REPORTS, { recursive: true });

  const browser = await chromium.launch({ headless: true });
  const routes = makeRoutes();
  const allRoutes = [];
  const allFindings = [];

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
        await page.waitForTimeout(500);
        const shot = join(SHOTS, `${label}.png`);
        await page.screenshot({ path: shot, fullPage: true });

        const axeResults = await runAxe(page);
        const visual = await detectVisualIssues(page);

        const report = {
          label, viewport: vp.name, path: route.path, url, status, load_ms: Date.now() - t0,
          isFragment: axeResults.skipped,
          fragment_reason: axeResults.reason,
          axe_violations: (axeResults.violations || []).map((v) => ({
            id: v.id, impact: v.impact, help: v.help, helpUrl: v.helpUrl,
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
        if (axeResults.skipped) {
          console.log(`    axe: SKIPPED (${axeResults.reason})`);
        } else {
          const violCount = axeResults.violations?.length || 0;
          if (violCount > 0) console.log(`    axe: ${violCount} violation types`);
        }
      } catch (e) {
        allFindings.push({ label, path: route.path, viewport: vp.name, kind: 'crash', error: e.message });
        console.log(`    FAILED: ${e.message}`);
      }
    }
    if (pageErrors.length > 0) {
      allFindings.push(...pageErrors.map((e) => ({ kind: 'pageerror', ...e })));
    }
    await context.close();
  }

  await browser.close();
  await writeFile(join(REPORTS, 'routes.json'), JSON.stringify(allRoutes, null, 2));
  await writeFile(join(REPORTS, 'findings.json'), JSON.stringify(allFindings, null, 2));

  const summary = renderSummary({
    title: 'DixieData UI/UX Audit — Round 2 (deep routes)',
    routes: allRoutes,
    findings: allFindings,
  });
  await writeFile(join(REPORTS, 'summary.md'), summary);
  console.log(`\nReports: ${REPORTS}`);
}

main().catch((e) => { console.error(e); process.exit(1); });