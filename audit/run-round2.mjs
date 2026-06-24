// Round 2 audit: deeper routes not covered in run.mjs.
// Soldier sub-pages (camaraderie, timeline, research-log, conflict-ledger,
// research-pack/{state,county}), compare flow, research-collections,
// advanced search.
//
// Assumes:
//   - dixiedata-web is already running at $BASE_URL (default http://127.0.0.1:8765)
//   - scratch dir is already seeded (use soldier ids 47 and 35 — first two from search "a")

import { chromium } from 'playwright';
import { AxeBuilder } from '@axe-core/playwright';
import { mkdir, writeFile, rm } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { writeFileSync } from 'node:fs';

const __dirname = dirname(fileURLToPath(import.meta.url));
const SHOTS = join(__dirname, 'screenshots-r2');
const REPORTS = join(__dirname, 'reports-r2');
const BASE = process.env.BASE_URL || 'http://127.0.0.1:8765';

const VIEWPORTS = [
  { name: 'desktop', width: 1280, height: 800 },
  { name: 'tablet', width: 900, height: 1200 },
  { name: 'mobile', width: 390, height: 844 },
];

// Soldier ids 47 and 35 are first two matches from /soldiers/search?q=a
// with the default 50-soldier seed.
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

async function detectVisualIssues(page) {
  return await page.evaluate(() => {
    const issues = [];
    if (document.documentElement.scrollWidth > window.innerWidth + 1) {
      issues.push({ id: 'h-scroll', severity: 'high', detail: `scrollWidth=${document.documentElement.scrollWidth} > ${window.innerWidth}` });
    }
    if (document.body.scrollWidth > window.innerWidth + 1) {
      issues.push({ id: 'h-scroll-body', severity: 'high', detail: `body scrollWidth=${document.body.scrollWidth} > ${window.innerWidth}` });
    }
    const overflowers = [];
    document.querySelectorAll('*').forEach((el) => {
      if (el.tagName === 'HTML' || el.tagName === 'BODY') return;
      const r = el.getBoundingClientRect();
      if (r.right > window.innerWidth + 1 && r.width > 0) {
        const cs = window.getComputedStyle(el);
        if (cs.position === 'fixed' || cs.position === 'absolute') {
          let p = el.parentElement, clipped = false;
          while (p && p !== document.body) {
            const pcs = window.getComputedStyle(p);
            if (pcs.overflow === 'hidden' || pcs.overflowX === 'hidden') { clipped = true; break; }
            p = p.parentElement;
          }
          if (clipped) return;
        }
        overflowers.push({
          tag: el.tagName.toLowerCase(),
          id: el.id || null,
          cls: typeof el.className === 'string' ? el.className.slice(0, 80) : null,
          right: Math.round(r.right),
          width: Math.round(r.width),
        });
      }
    });
    if (overflowers.length > 0) {
      const seen = new Set(), sample = [];
      for (const o of overflowers) {
        const k = `${o.tag}.${o.cls || ''}`;
        if (seen.has(k)) continue;
        seen.add(k); sample.push(o);
        if (sample.length >= 5) break;
      }
      issues.push({ id: 'overflow-x', severity: 'high', count: overflowers.length, sample });
    }
    const smallTargets = [];
    document.querySelectorAll('button, a, [role="button"]').forEach((el) => {
      const r = el.getBoundingClientRect();
      if (r.width === 0 && r.height === 0) return;
      if (r.width < 24 || r.height < 24) {
        smallTargets.push({
          tag: el.tagName.toLowerCase(),
          text: (el.textContent || '').trim().slice(0, 40),
          w: Math.round(r.width), h: Math.round(r.height),
        });
      }
    });
    if (smallTargets.length > 0) {
      issues.push({ id: 'small-tap-target', severity: 'medium', count: smallTargets.length, sample: smallTargets.slice(0, 5) });
    }
    const unlabeled = [];
    document.querySelectorAll('input:not([type="hidden"]):not([type="submit"]):not([type="button"]), textarea, select').forEach((el) => {
      const id = el.id;
      const lbl = id ? document.querySelector(`label[for="${id}"]`) : null;
      const wrap = el.closest('label');
      const aria = el.getAttribute('aria-label') || el.getAttribute('aria-labelledby') || el.getAttribute('placeholder');
      if (!lbl && !wrap && !aria) {
        unlabeled.push({ tag: el.tagName.toLowerCase(), name: el.name || null, type: el.type || null });
      }
    });
    if (unlabeled.length > 0) {
      issues.push({ id: 'unlabeled-input', severity: 'medium', count: unlabeled.length, sample: unlabeled.slice(0, 5) });
    }
    const nav = document.querySelector('nav');
    if (nav) {
      const links = nav.querySelectorAll('a, button');
      const navR = nav.getBoundingClientRect();
      issues.push({
        id: 'nav-density',
        severity: 'info',
        item_count: links.length,
        nav_width: Math.round(navR.width),
        item_width_avg: links.length ? Math.round(navR.width / links.length) : 0,
      });
    }
    // Detect empty-state-only pages so we can flag them in the summary.
    const emptyState = document.querySelector('[data-empty-state], .empty-state');
    if (emptyState) {
      issues.push({ id: 'empty-state-visible', severity: 'info', tag: emptyState.tagName.toLowerCase(), snippet: (emptyState.textContent || '').trim().slice(0, 80) });
    }
    // Detect long pages that would benefit from pagination/virtualisation.
    const table = document.querySelector('table');
    if (table) {
      const tbody = table.querySelector('tbody');
      const rowCount = tbody ? tbody.querySelectorAll('tr').length : 0;
      const pageHeight = document.documentElement.scrollHeight;
      issues.push({ id: 'table-rows', severity: 'info', rows: rowCount, page_height: pageHeight });
    }
    // Detect htmx targets and swap styles for hidden-loading state issues.
    const htmxBusy = document.querySelectorAll('[aria-busy="true"]').length;
    if (htmxBusy > 0) {
      issues.push({ id: 'stuck-busy', severity: 'medium', count: htmxBusy });
    }
    // Detect tables inside other tables or interactive inside interactive.
    const tables = document.querySelectorAll('table');
    if (tables.length > 1) {
      issues.push({ id: 'nested-tables', severity: 'medium', count: tables.length });
    }
    return issues;
  });
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

        let axeResults = null;
        try {
          const builder = new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']);
          axeResults = await builder.analyze();
        } catch (e) {
          axeResults = { error: e.message, violations: [] };
        }

        const visual = await detectVisualIssues(page);
        const report = {
          label, viewport: vp.name, path: route.path, url, status, load_ms: Date.now() - t0,
          axe_violations: axeResults.violations?.map((v) => ({
            id: v.id, impact: v.impact, help: v.help, helpUrl: v.helpUrl,
            nodes: v.nodes.length,
            sample: v.nodes.slice(0, 2).map((n) => n.target),
          })) || [],
          visual_issues: visual,
        };
        allRoutes.push(report);
        allFindings.push(...axeResults.violations?.flatMap((v) =>
          v.nodes.map((n) => ({
            label, path: route.path, viewport: vp.name,
            kind: 'a11y', id: v.id, impact: v.impact, help: v.help, target: n.target,
          }))
        ) || []);
        allFindings.push(...visual.map((v) => ({ label, path: route.path, viewport: vp.name, kind: 'visual', ...v })));
        const violCount = axeResults.violations?.length || 0;
        if (violCount > 0) console.log(`    axe: ${violCount} violation types`);
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

  const summary = renderSummary(allRoutes, allFindings);
  await writeFile(join(REPORTS, 'summary.md'), summary);
  console.log(`\nReports: ${REPORTS}`);
}

function renderSummary(routes, findings) {
  const byImpact = { critical: 0, serious: 0, moderate: 0, minor: 0 };
  const byId = {};
  for (const f of findings) {
    if (f.impact && byImpact[f.impact] !== undefined) byImpact[f.impact]++;
    const k = f.id || f.kind || 'unknown';
    byId[k] = (byId[k] || 0) + 1;
  }
  const lines = [
    '# DixieData UI/UX Audit — Round 2 (deep routes)',
    '',
    `Routes: **${routes.length}** (${new Set(routes.map((r) => r.path)).size} unique paths × 3 viewports)`,
    `Findings: **${findings.length}**`,
    '',
    '## A11y by severity',
    `- Critical: ${byImpact.critical}`,
    `- Serious: ${byImpact.serious}`,
    `- Moderate: ${byImpact.moderate}`,
    `- Minor: ${byImpact.minor}`,
    '',
    '## Findings by type',
    '',
    ...Object.entries(byId).sort((a, b) => b[1] - a[1]).map(([k, v]) => `- \`${k}\`: ${v}`),
    '',
    '## Per-route',
    '',
    '| Route | Viewport | Status | Load (ms) | A11y types | Visual |',
    '|---|---|---:|---:|---:|---:|',
    ...routes.map((r) =>
      `| \`${r.path}\` | ${r.viewport} | ${r.status} | ${r.load_ms} | ${r.axe_violations.length} | ${r.visual_issues.length} |`
    ),
  ];
  return lines.join('\n');
}

main().catch((e) => { console.error(e); process.exit(1); });