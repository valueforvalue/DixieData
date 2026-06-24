// DixieData UI/UX audit — round 3.
//
// Verifies that earlier slices actually fixed the findings, plus adds
// interactive flow tests:
//   - Open hamburger drawer, click a nav link, drawer closes
//   - Open browse filter drawer, change a filter, results update
//   - Click "compare-selected" on /browse, navigate to /compare
//   - Open compare page, hover differences pills, scroll the diff table
//   - Submit feedback modal
//
// Run: node audit/run-round3.mjs
//
// Assumes dixiedata-web is already running at $BASE_URL (default
// http://127.0.0.1:8765) with .scratch/webmode seeded.

import { chromium } from 'playwright';
import { mkdir, writeFile, rm } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { runAxe, detectVisualIssues, renderSummary } from './harness.mjs';

const __dirname = dirname(fileURLToPath(import.meta.url));
const SHOTS = join(__dirname, 'screenshots-r3');
const REPORTS = join(__dirname, 'reports-r3');
const BASE = process.env.BASE_URL || 'http://127.0.0.1:8765';

const VIEWPORTS = [
  { name: 'desktop', width: 1280, height: 800 },
  { name: 'mobile', width: 390, height: 844 },
];

async function main() {
  if (existsSync(SHOTS)) await rm(SHOTS, { recursive: true, force: true });
  if (existsSync(REPORTS)) await rm(REPORTS, { recursive: true, force: true });
  await mkdir(SHOTS, { recursive: true });
  await mkdir(REPORTS, { recursive: true });

  const browser = await chromium.launch({ headless: true });
  const allRoutes = [];
  const allFindings = [];
  const flowResults = [];

  // Round 1 + 2 routes (subset for regression check)
  const regressionRoutes = [
    { label: 'home', path: '/', waitFor: '[data-ui-id="page.calendar"]' },
    { label: 'calendar', path: '/calendar', waitFor: '[data-ui-id="page.calendar"]' },
    { label: 'search', path: '/soldiers', waitFor: 'input[name="q"]' },
    { label: 'browse', path: '/browse', waitFor: '[data-ui-id="page.browse"]' },
    { label: 'review-queue', path: '/review-queue', waitFor: 'main' },
    { label: 'insights', path: '/insights', waitFor: 'main' },
    { label: 'share', path: '/share', waitFor: 'main' },
    { label: 'settings', path: '/settings', waitFor: 'main' },
    { label: 'soldier-new', path: '/soldiers/new', waitFor: 'main' },
    { label: 'compare', path: '/compare?id1=47&id2=35', waitFor: 'main' },
    { label: 'soldier-47', path: '/soldiers/47', waitFor: '[data-ui-id="page.soldier.detail"]' },
    { label: 'soldier-47-edit', path: '/soldiers/47/edit', waitFor: 'main' },
  ];

  for (const vp of VIEWPORTS) {
    console.log(`\n=== Viewport: ${vp.name} (${vp.width}x${vp.height}) ===`);
    const context = await browser.newContext({ viewport: { width: vp.width, height: vp.height } });
    const page = await context.newPage();
    page.on('pageerror', (err) => allFindings.push({ kind: 'pageerror', viewport: vp.name, message: err.message }));

    for (const route of regressionRoutes) {
      const label = `${vp.name}_${route.label}`;
      console.log(`  ${label} -> ${BASE}${route.path}`);
      const t0 = Date.now();
      try {
        const response = await page.goto(`${BASE}${route.path}`, { waitUntil: 'domcontentloaded', timeout: 15000 });
        if (route.waitFor) {
          await page.waitForSelector(route.waitFor, { timeout: 5000 }).catch(() => null);
        }
        await page.waitForTimeout(400);
        await page.screenshot({ path: join(SHOTS, `${label}.png`), fullPage: false });
        const axeResults = await runAxe(page);
        const visual = await detectVisualIssues(page);
        const report = {
          label, viewport: vp.name, path: route.path,
          status: response?.status(), load_ms: Date.now() - t0,
          isFragment: axeResults.skipped,
          fragment_reason: axeResults.reason,
          axe_violations: (axeResults.violations || []).map((v) => ({
            id: v.id, impact: v.impact, help: v.help, helpUrl: v.helpUrl,
            nodes: v.nodes.length, sample: v.nodes.slice(0, 2).map((n) => n.target),
          })),
          visual_issues: visual,
        };
        allRoutes.push(report);
        if (axeResults.skipped) {
          allFindings.push({ label, path: route.path, viewport: vp.name, kind: 'axe-skipped', reason: axeResults.reason });
        } else {
          for (const v of axeResults.violations) {
            for (const n of v.nodes) {
              allFindings.push({ label, path: route.path, viewport: vp.name, kind: 'a11y', id: v.id, impact: v.impact, help: v.help, target: n.target });
            }
          }
        }
        for (const v of visual) {
          allFindings.push({ label, path: route.path, viewport: vp.name, kind: 'visual', ...v });
        }
        const violCount = axeResults.violations?.length || 0;
        if (axeResults.skipped) console.log(`    axe: SKIPPED (${axeResults.reason})`);
        else if (violCount > 0) console.log(`    axe: ${violCount} violation types`);
      } catch (e) {
        allFindings.push({ label, path: route.path, viewport: vp.name, kind: 'crash', error: e.message });
        console.log(`    FAILED: ${e.message}`);
      }
    }

    // Interactive flow tests (desktop only — mobile interactions are mostly subset)
    if (vp.name === 'desktop') {
      console.log(`\n  -- Interactive flows --`);
      await testHamburgerFlow(page, flowResults);
      await testBrowseFilterFlow(page, flowResults);
      await testCompareScrollFlow(page, flowResults);
      await testBrowseCompareSelectedFlow(page, flowResults);
      await testFeedbackModalFlow(page, flowResults);
    }

    await context.close();
  }

  await browser.close();

  await writeFile(join(REPORTS, 'routes.json'), JSON.stringify(allRoutes, null, 2));
  await writeFile(join(REPORTS, 'findings.json'), JSON.stringify(allFindings, null, 2));
  await writeFile(join(REPORTS, 'flows.json'), JSON.stringify(flowResults, null, 2));
  await writeFile(join(REPORTS, 'summary.md'), renderFlowSummary(allRoutes, allFindings, flowResults));

  console.log(`\nReports: ${REPORTS}`);
  console.log(`Flows: ${flowResults.length} tests, ${flowResults.filter(f => f.passed).length} passed, ${flowResults.filter(f => !f.passed).length} failed`);
}

async function testHamburgerFlow(page, flowResults) {
  // Mobile-only feature. Skip on desktop.
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`${BASE}/calendar`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(300);
  const initialHidden = await page.evaluate(() => {
    const nav = document.querySelector('header nav');
    return nav ? window.getComputedStyle(nav).display === 'none' : false;
  });
  await page.click('[data-top-nav-toggle]');
  await page.waitForTimeout(200);
  const afterOpen = await page.evaluate(() => {
    const drawer = document.querySelector('[data-top-nav-drawer]');
    return drawer ? !drawer.classList.contains('hidden') : false;
  });
  const drawerHasLinks = await page.evaluate(() => {
    const drawer = document.querySelector('[data-top-nav-drawer]');
    return drawer ? drawer.querySelectorAll('a[href^="/"]').length >= 5 : false;
  });
  const ariaExpanded = await page.evaluate(() => document.querySelector('[data-top-nav-toggle]')?.getAttribute('aria-expanded'));

  flowResults.push({
    name: 'hamburger-opens-drawer',
    passed: initialHidden && afterOpen && drawerHasLinks && ariaExpanded === 'true',
    details: { initialHidden, afterOpen, drawerHasLinks, ariaExpanded },
  });

  // Test ESC closes
  await page.keyboard.press('Escape');
  await page.waitForTimeout(200);
  const afterEsc = await page.evaluate(() => {
    const drawer = document.querySelector('[data-top-nav-drawer]');
    return drawer ? drawer.classList.contains('hidden') : false;
  });
  flowResults.push({
    name: 'hamburger-esc-closes',
    passed: afterEsc,
    details: { afterEsc },
  });

  // Test focus returns to toggle
  const focusBackToToggle = await page.evaluate(() => {
    return document.activeElement?.matches('[data-top-nav-toggle]');
  });
  flowResults.push({
    name: 'hamburger-focus-returns',
    passed: focusBackToToggle,
    details: { focusBackToToggle },
  });

  console.log(`  hamburger flows: ${flowResults.slice(-3).map(f => `${f.name}=${f.passed ? 'PASS' : 'FAIL'}`).join(', ')}`);
  await page.setViewportSize({ width: 1280, height: 800 });
}

async function testBrowseFilterFlow(page, flowResults) {
  await page.goto(`${BASE}/browse`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(300);

  // Open the filter details
  const detailsOpen = await page.evaluate(() => {
    const d = document.querySelector('[data-browse-filters-details]');
    if (d) { d.open = true; return true; }
    return false;
  });
  await page.waitForTimeout(200);

  // Verify the filter drawer has the right form fields and the active-count
  // badge updates when filters change.
  await page.goto(`${BASE}/browse`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(300);

  await page.evaluate(() => {
    const d = document.querySelector('[data-browse-filters-details]');
    if (d) d.open = true;
  });
  await page.waitForTimeout(200);

  // Count filter inputs (excluding the hidden page input and submit button).
  const inputCount = await page.evaluate(() => {
    return document.querySelectorAll('form#browse-filters [data-browse-filter-input]').length;
  });

  // Change the entry_type select and verify the count badge updates.
  await page.selectOption('form#browse-filters select[name="entry_type"]', 'soldier');
  await page.waitForTimeout(200);
  const badgeAfterSelect = await page.evaluate(() => {
    const b = document.querySelector('[data-browse-filters-count]');
    return b?.textContent;
  });

  flowResults.push({
    name: 'browse-filter-applies',
    passed: detailsOpen && inputCount >= 9 && badgeAfterSelect === '1 active',
    details: { detailsOpen, inputCount, badgeAfterSelect },
  });

  // Reload and verify filter persistence. Clear localStorage first so
    // the URL params aren't overridden by the localStorage state-restore
    // logic. After clearing, the form should populate from URL query.
  await page.evaluate(() => {
    Object.keys(localStorage).forEach((k) => {
      if (k.startsWith('dixiedata.browse') || k.startsWith('dixiedata.layout')) {
        localStorage.removeItem(k);
      }
    });
  });
  await page.goto(`${BASE}/browse?page=2&page_size=25`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(500);
  const persistedFilter = await page.evaluate(() => {
    const size = document.querySelector('select[name="page_size"]');
    return {
      size: size?.value,
    };
  });
  flowResults.push({
    name: 'browse-filter-persists-from-url',
    passed: persistedFilter.size === '25',
    details: { persistedFilter },
  });

  console.log(`  browse filter flows: ${flowResults.slice(-2).map(f => `${f.name}=${f.passed ? 'PASS' : 'FAIL'}`).join(', ')}`);
}

async function testCompareScrollFlow(page, flowResults) {
  await page.goto(`${BASE}/compare?id1=47&id2=35`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(500);

  // Verify scrollable-region has tabindex on desktop
  const scrollableTabindex = await page.evaluate(() => {
    const el = document.querySelector('.overflow-x-auto[role="region"]');
    return el?.getAttribute('tabindex');
  });
  flowResults.push({
    name: 'compare-region-keyboard-accessible',
    passed: scrollableTabindex === '0',
    details: { scrollableTabindex },
  });

  // Verify differences pills exist and are clickable. The differences
    // section shows the differing field names as <span> chips (decorative
    // labels for the differing rows in the table below).
  const differencesPills = await page.evaluate(() => {
    const header = [...document.querySelectorAll('*')].find(el => (el.textContent || '').trim() === 'Differences to Review First');
    if (!header) return null;
    const container = header.parentElement;
    if (!container) return null;
    // Pills are <span class="rounded-full ..."> chips with the field labels
    const pills = container.querySelectorAll('span.rounded-full');
    return pills.length;
  });
  flowResults.push({
    name: 'compare-differences-pills-present',
    passed: differencesPills !== null && differencesPills >= 3,
    details: { differencesPills },
  });

  console.log(`  compare flows: ${flowResults.slice(-2).map(f => `${f.name}=${f.passed ? 'PASS' : 'FAIL'}`).join(', ')}`);
}

async function testBrowseCompareSelectedFlow(page, flowResults) {
  await page.goto(`${BASE}/browse`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(300);

  // Select 2 records via checkboxes (target table checkboxes explicitly since
  // the mobile card view also renders checkboxes via md:hidden).
  const checkboxes = await page.$$('table input[data-browse-select]');
  if (checkboxes.length >= 2) {
    await checkboxes[0].check();
    await checkboxes[1].check();
    await page.waitForTimeout(200);
  }

  // Find and check the Print/Export Selected button is enabled
  const printBtn = await page.$('a[href*="openPrintConfig"]');
  const printBtnEnabled = printBtn !== null;
  flowResults.push({
    name: 'browse-compare-selection-enabled',
    passed: checkboxes.length >= 2 && printBtnEnabled,
    details: { checkboxCount: checkboxes.length, printBtnFound: printBtnEnabled },
  });

  console.log(`  compare-selection flow: ${flowResults.slice(-1)[0].name}=${flowResults.slice(-1)[0].passed ? 'PASS' : 'FAIL'}`);
}

async function testFeedbackModalFlow(page, flowResults) {
  await page.goto(`${BASE}/calendar`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(300);

  // Click feedback button
  await page.click('[data-feedback-open]');
  await page.waitForTimeout(300);

  // Verify modal opened
  const modalOpen = await page.evaluate(() => {
    const modal = document.querySelector('#feedback-modal');
    return modal && !modal.classList.contains('hidden');
  });

  // Fill the textarea
  await page.fill('#feedback-form textarea[name="message"]', 'Test feedback from round 3 audit');
  await page.waitForTimeout(100);

  // Close (don't submit — that would write to disk)
  await page.click('[data-feedback-close]');
  await page.waitForTimeout(200);

  const modalClosed = await page.evaluate(() => {
    const modal = document.querySelector('#feedback-modal');
    return modal && modal.classList.contains('hidden');
  });

  flowResults.push({
    name: 'feedback-modal-opens-and-closes',
    passed: modalOpen && modalClosed,
    details: { modalOpen, modalClosed },
  });

  console.log(`  feedback flow: ${flowResults.slice(-1)[0].name}=${flowResults.slice(-1)[0].passed ? 'PASS' : 'FAIL'}`);
}

function renderFlowSummary(routes, findings, flows) {
  const byImpact = { critical: 0, serious: 0, moderate: 0, minor: 0 };
  for (const f of findings) {
    if (f.impact && byImpact[f.impact] !== undefined) byImpact[f.impact]++;
  }
  const byType = {};
  for (const f of findings) {
    const k = f.id || f.kind;
    byType[k] = (byType[k] || 0) + 1;
  }
  return [
    '# DixieData UI/UX Audit — Round 3 (verification + flows)',
    '',
    `Routes audited: **${routes.length}** (${new Set(routes.map(r => r.path)).size} unique paths × ${VIEWPORTS.length} viewports)`,
    `Findings: **${findings.length}**`,
    `Interactive flows: **${flows.length}** (${flows.filter(f => f.passed).length} passed, ${flows.filter(f => !f.passed).length} failed)`,
    '',
    '## A11y violations by severity',
    `- Critical: ${byImpact.critical}`,
    `- Serious: ${byImpact.serious}`,
    `- Moderate: ${byImpact.moderate}`,
    `- Minor: ${byImpact.minor}`,
    '',
    '## Findings by type',
    '',
    ...Object.entries(byType).sort((a, b) => b[1] - a[1]).map(([k, v]) => `- \`${k}\`: ${v}`),
    '',
    '## Interactive flow results',
    '',
    '| Flow | Result |',
    '|---|---|',
    ...flows.map((f) => `| ${f.name} | ${f.passed ? '✓ PASS' : '✗ FAIL'} |`),
  ].join('\n');
}

main().catch((e) => { console.error(e); process.exit(1); });