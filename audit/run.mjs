// DixieData UI/UX audit harness.
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
import { AxeBuilder } from '@axe-core/playwright';
import { mkdir, writeFile, rm } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

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

    // Catch page errors so we know about broken JS.
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
        // Give htmx a beat to settle.
        await page.waitForTimeout(400);

        const shot = join(SHOTS, `${label}.png`);
        await page.screenshot({ path: shot, fullPage: true });

        // axe-core a11y scan.
        let axeResults = null;
        try {
          const builder = new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']);
          axeResults = await builder.analyze();
        } catch (e) {
          axeResults = { error: e.message, violations: [] };
        }

        // Detect visual / layout problems that axe doesn't catch.
        const visual = await detectVisualIssues(page);

        const report = {
          label,
          viewport: vp.name,
          path: route.path,
          url,
          status,
          load_ms: Date.now() - t0,
          axe_violations: axeResults.violations?.map((v) => ({
            id: v.id,
            impact: v.impact,
            help: v.help,
            helpUrl: v.helpUrl,
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
        if (violCount > 0) {
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

  // Write reports.
  await writeFile(join(REPORTS, 'routes.json'), JSON.stringify(allRoutes, null, 2));
  await writeFile(join(REPORTS, 'findings.json'), JSON.stringify(allFindings, null, 2));

  // Markdown summary.
  const summary = renderSummary(allRoutes, allFindings);
  await writeFile(join(REPORTS, 'summary.md'), summary);

  console.log(`\nReports written to ${REPORTS}`);
  console.log(`  - routes.json   (per-route axe + visual)`);
  console.log(`  - findings.json (flat finding list)`);
  console.log(`  - summary.md    (human-readable overview)`);
  console.log(`\nScreenshots in ${SHOTS}`);
}

async function detectVisualIssues(page) {
  return await page.evaluate(() => {
    const issues = [];

    // 1. Horizontal scroll on the main viewport.
    if (document.documentElement.scrollWidth > window.innerWidth + 1) {
      issues.push({
        id: 'h-scroll',
        severity: 'high',
        detail: `document scrollWidth=${document.documentElement.scrollWidth} > viewport=${window.innerWidth}`,
      });
    }
    if (document.body.scrollWidth > window.innerWidth + 1) {
      issues.push({
        id: 'h-scroll-body',
        severity: 'high',
        detail: `body scrollWidth=${document.body.scrollWidth} > viewport=${window.innerWidth}`,
      });
    }

    // 2. Elements overflowing the viewport horizontally.
    const overflowers = [];
    document.querySelectorAll('*').forEach((el) => {
      const r = el.getBoundingClientRect();
      if (r.right > window.innerWidth + 1 && r.width > 0 && el.tagName !== 'HTML' && el.tagName !== 'BODY') {
        // Skip fixed-position elements that intentionally extend off-screen
        // (e.g. tooltips), and skip elements that are children of overflow:hidden ancestors.
        const cs = window.getComputedStyle(el);
        if (cs.position === 'fixed' || cs.position === 'absolute') {
          // Only flag absolute/fixed if parent clips aren't hiding it.
          let p = el.parentElement;
          let clipped = false;
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
          cls: el.className && typeof el.className === 'string' ? el.className.slice(0, 80) : null,
          right: Math.round(r.right),
          width: Math.round(r.width),
        });
      }
    });
    if (overflowers.length > 0) {
      // Dedupe by tag+class, keep first 5.
      const seen = new Set();
      const sample = [];
      for (const o of overflowers) {
        const k = `${o.tag}.${o.cls || ''}`;
        if (seen.has(k)) continue;
        seen.add(k);
        sample.push(o);
        if (sample.length >= 5) break;
      }
      issues.push({
        id: 'overflow-x',
        severity: 'high',
        count: overflowers.length,
        sample,
      });
    }

    // 3. Tiny tap targets (mobile a11y).
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
      issues.push({
        id: 'small-tap-target',
        severity: 'medium',
        count: smallTargets.length,
        sample: smallTargets.slice(0, 5),
      });
    }

    // 4. Text contrast on body.
    // (axe handles this more thoroughly; skip custom heuristic to avoid duplication)

    // 5. Dead / no-op clickable things.
    const dead = [];
    document.querySelectorAll('[onclick]').forEach((el) => {
      if (typeof el.onclick === 'function' && el.onclick.toString().trim() === 'function onclick(event) {}') {
        dead.push({ tag: el.tagName.toLowerCase(), text: (el.textContent || '').trim().slice(0, 30) });
      }
    });
    if (dead.length > 0) {
      issues.push({ id: 'dead-onclick', severity: 'low', count: dead.length, sample: dead });
    }

    // 6. Inputs without associated label.
    const unlabeled = [];
    document.querySelectorAll('input:not([type="hidden"]):not([type="submit"]):not([type="button"]), textarea, select').forEach((el) => {
      const id = el.id;
      if (id) {
        const lbl = document.querySelector(`label[for="${id}"]`);
        if (!lbl) {
          // Check for wrapping label or aria-label.
          const wrap = el.closest('label');
          const aria = el.getAttribute('aria-label') || el.getAttribute('aria-labelledby') || el.getAttribute('placeholder');
          if (!wrap && !aria) {
            unlabeled.push({ tag: el.tagName.toLowerCase(), name: el.name || null, type: el.type || null });
          }
        }
      } else {
        const wrap = el.closest('label');
        const aria = el.getAttribute('aria-label') || el.getAttribute('aria-labelledby');
        if (!wrap && !aria) {
          unlabeled.push({ tag: el.tagName.toLowerCase(), name: el.name || null, type: el.type || null });
        }
      }
    });
    if (unlabeled.length > 0) {
      issues.push({
        id: 'unlabeled-input',
        severity: 'medium',
        count: unlabeled.length,
        sample: unlabeled.slice(0, 5),
      });
    }

    // 7. Truncated/ellipsised long text (heuristic: title attr present, scrollWidth > clientWidth).
    const truncated = [];
    document.querySelectorAll('[title]').forEach((el) => {
      if (el.scrollWidth > el.clientWidth + 1) {
        truncated.push({
          tag: el.tagName.toLowerCase(),
          title: el.getAttribute('title').slice(0, 40),
        });
      }
    });
    if (truncated.length > 0) {
      issues.push({ id: 'truncated-with-title', severity: 'low', count: truncated.length, sample: truncated.slice(0, 3) });
    }

    // 8. Heuristic: menu/nav density.
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

    return issues;
  });
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
    '# DixieData UI/UX Audit — Summary',
    '',
    `Routes audited: **${routes.length}** (${new Set(routes.map((r) => r.path)).size} unique paths × 3 viewports)`,
    `Total findings: **${findings.length}**`,
    '',
    '## A11y violations by severity',
    '',
    `- Critical: ${byImpact.critical}`,
    `- Serious: ${byImpact.serious}`,
    `- Moderate: ${byImpact.moderate}`,
    `- Minor: ${byImpact.minor}`,
    '',
    '## Findings by type',
    '',
    ...Object.entries(byId).sort((a, b) => b[1] - a[1]).map(([k, v]) => `- \`${k}\`: ${v}`),
    '',
    '## Per-route snapshot',
    '',
    '| Route | Viewport | Status | Load (ms) | A11y types | Visual issues |',
    '|---|---|---:|---:|---:|---:|',
    ...routes.map((r) =>
      `| \`${r.path}\` | ${r.viewport} | ${r.status} | ${r.load_ms} | ${r.axe_violations.length} | ${r.visual_issues.length} |`
    ),
    '',
    '## Worst offenders (routes with most violations)',
    '',
    ...routes
      .slice()
      .sort((a, b) => (b.axe_violations.length + b.visual_issues.length) - (a.axe_violations.length + a.visual_issues.length))
      .slice(0, 10)
      .map((r) => `- **${r.label}** — ${r.axe_violations.length} a11y types, ${r.visual_issues.length} visual issues`),
  ];
  return lines.join('\n');
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
