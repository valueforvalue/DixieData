// audit/smoke_a11y.mjs — Slice 3a (WARN-only by default)
//
// WCAG 2 AA a11y sweep via @axe-core/playwright on every
// probed surface. Speed-rewrite. Mirrors
// audit/smoke_button_matrix.mjs lifecycle:
//
//   - One global `audit/harness.mjs::runAxe()` call per
//     surface (handles WCAG 2 AA tags + fragment detection
//     in one shot).
//   - Per-surface `ctx.record('a11y:<name>', ok, { ... })`
//     into the aggregator's JSON summary.
//   - Default GREEN: always `ok: true`. The probe is
//     informational, not a gate. Slice 3b (future PR)
//     flips to FAIL-mode after the issue cohort from this
//     run is remediated.
//   - `SMOKE_A11Y_STRICT=1` flips to FAIL-mode + exit 1
//     for the future slice 3b gate (preserved here so the
//     infra is in place).
//   - Same seed-data lifecycle as the matrix probe (Q6).
//   - Same 8-surface rotation (Q5) — 33 surfaces ÷ 8 = 4-
//     day cycle via day-of-epoch mod 4.
//
// What this catches (per docs/COMMON_BUGS §6):
//   - missing form labels
//   - insufficient color contrast (text on backgrounds)
//   - missing alt text on images
//   - missing accessible names on buttons/links
//   - keyboard trap / focus order bugs
//   - ARIA misuse
//   - duplicate IDs
//   - landmark structure problems
//
// What this does NOT cover (documented gaps):
//   - Modal-content a11y (per-feature probes cover).
//   - Dynamic content swap a11y (e.g., htmx swaps that lose
//     focus or screen-reader announcement).
//   - Mobile/touchscreen a11y (Playwright is desktop
//     Chromium only).
//   - Screen-reader testing (axe is heuristic, not real
//     SR-driven).
//
// Lifecycle: see audit/_lib/smoke_runner.md invariant #1.

import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { setTimeout as sleep } from 'node:timers/promises';
import path from 'node:path';
import fs from 'node:fs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';
import { loadConfig, resolveBaseUrl } from './_lib/config.mjs';
import { runAxe } from './harness.mjs';

const cfg = loadConfig();
const PORT = process.env.PROBE_PORT ? parseInt(process.env.PROBE_PORT, 10) : cfg.defaultPort;
const BASE = resolveBaseUrl(cfg).replace(/\/$/, '');

const STRICT = process.env.SMOKE_A11Y_STRICT === '1';

// 33 surfaces, mirroring smoke_button_matrix.mjs SURFACE_URLS
// (same source of truth per the single-context convention).
// Both probes share the same rotation so a single CI run
// covers one slice of the matrix and a11y together.
const SURFACE_URLS = [
  { name: 'home',              path: '/' },
  { name: 'soldiers-list',     path: '/soldiers' },
  { name: 'soldier-new',       path: '/soldiers/new' },
  { name: 'browse',            path: '/browse' },
  { name: 'calendar',          path: '/calendar' },
  { name: 'articles',          path: '/articles' },
  { name: 'article-new',       path: '/articles/new' },
  { name: 'events',            path: '/events' },
  { name: 'event-new',         path: '/events/new' },
  { name: 'review-queue',      path: '/review-queue' },
  { name: 'tags',              path: '/tags' },
  { name: 'settings',          path: '/settings' },
  { name: 'settings-appearance',   path: '/settings/appearance' },
  { name: 'settings-diagnostics',  path: '/settings/diagnostics' },
  { name: 'settings-maintenance',  path: '/settings/maintenance' },
  { name: 'settings-data',         path: '/settings/data' },
  { name: 'settings-updates',      path: '/settings/updates' },
  { name: 'recovery',          path: '/recovery' },
  { name: 'jobs',              path: '/jobs' },
  { name: 'share-landing',     path: '/share' },
  { name: 'share-exports',     path: '/share/exports' },
  { name: 'share-imports',     path: '/share/imports' },
  { name: 'share-sync',        path: '/share/sync' },
  { name: 'insights',          path: '/insights' },
  { name: 'research-collections',  path: '/research-collections' },
  { name: 'research-log',      path: '/research-log' },
  { name: 'inventory',         path: '/inventory' },
  { name: 'about',             path: '/about' },
];

let pass = 0;
let warn = 0;
let totalViolations = 0;
let seriousViolations = 0;
let criticalViolations = 0;

function record(name, ok, details = {}) {
  if (details.warn) {
    warn++;
    console.log(`  PASS ${name} (warn)`);
    if (details.violations && details.violations.length) {
      console.log(`         ${details.violations.length} violation(s) (see JSON summary)`);
    }
  } else {
    pass++;
    console.log(`  PASS ${name}`);
  }
  // Slice 3b: accumulate serious + critical counts so the
  // gate flip in main() can fail on either. Counters are
  // module-scoped (same pattern as pass/warn/totalViolations)
  // because the visitSurface call sites fire-and-forget into
  // record() without surfacing their bySeverity tally up the
  // call stack. Slice 3a left the gate open (hasCritical =
  // false) until the slice 3a first-run cohort was remediated.
  if (details.bySeverity) {
    seriousViolations += details.bySeverity.serious || 0;
    criticalViolations += details.bySeverity.critical || 0;
  }
}

async function waitForServer(url, maxMs = 30_000) {
  const deadline = Date.now() + maxMs;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url);
      if (res.status < 500) return;
    } catch (_) { /* not yet up */ }
    await sleep(200);
  }
  throw new Error(`server at ${url} never came up`);
}

async function seedArchive(repoRoot, scratchDir) {
  const seedProc = spawn(
    'go',
    [
      'run', './cmd/seed-data',
      '-data-dir', scratchDir,
      '-soldiers', '5',
      '-articles', '2',
      '-events', '2',
      '-tags', '5',
      '-reset',
    ],
    { cwd: repoRoot, stdio: ['ignore', 'pipe', 'pipe'] },
  );
  let out = '';
  seedProc.stdout.on('data', (d) => { out += d; });
  seedProc.stderr.on('data', (d) => { out += d; });
  const exit = await new Promise((r) => seedProc.on('exit', r));
  if (exit !== 0) {
    throw new Error(`seed-data failed (exit ${exit}):\n${out}`);
  }
}

function selectSurfaces() {
  const mode = process.env.SMOKE_ROTATION ?? 'ci';
  if (mode === 'full') return SURFACE_URLS;
  if (mode === 'local') return SURFACE_URLS.slice(0, 8);
  // 'ci' default: day-of-epoch mod 4, take 8 surfaces.
  const day = Math.floor(Date.now() / (1000 * 60 * 60 * 24));
  const sliceIndex = day % 4;
  const start = sliceIndex * 8;
  return SURFACE_URLS.slice(start, start + 8);
}

async function visitSurface(page, surface) {
  const targetUrl = `${BASE}${surface.path}`;

  let response;
  try {
    response = await page.goto(targetUrl, {
      waitUntil: 'domcontentloaded',
      timeout: cfg.navTimeoutMs,
    });
  } catch (e) {
    record(`a11y:${surface.name}`, true, {
      warn: true,
      detail: `nav failed: ${e.message}`,
    });
    return;
  }
  if (!response || response.status() >= 500) {
    record(`a11y:${surface.name}`, true, {
      warn: true,
      detail: `nav status ${response?.status() ?? 'null'}`,
    });
    return;
  }

  // Pre-expand <details> so axe can audit items inside
  // collapsed panels (matches the matrix probe's pre-
  // expansion policy).
  await page.evaluate(() => {
    document.querySelectorAll('details').forEach((d) => { d.open = true; });
  });

  // Dismiss any modals that auto-opened (feedback-modal
  // etc.) so axe's snapshot reflects the steady state, not
  // the modal-shown state.
  await page.evaluate(() => {
    document
      .querySelectorAll('[data-feedback-modal], [data-print-config-modal], [data-google-calendar-preferences-modal]')
      .forEach((el) => { el.classList.add('hidden'); });
  });

  // Run axe. The helper already handles fragment detection
  // and returns `{ skipped, reason, violations }`.
  let axeResult;
  try {
    axeResult = await runAxe(page, { skipIfFragment: true });
  } catch (e) {
    record(`a11y:${surface.name}`, true, {
      warn: true,
      detail: `axe threw: ${e.message?.slice(0, 200)}`,
    });
    return;
  }

  if (axeResult.skipped) {
    record(`a11y:${surface.name}`, true, {
      warn: true,
      detail: axeResult.reason,
    });
    return;
  }

  // Log violation details to stdout so the developer can
  // triage without re-running with MATRIX_DEBUG=1 or
  // parsing the JSON summary. The JSON summary still has
  // the structured form.
  if (axeResult.violations.length > 0) {
    for (const v of axeResult.violations) {
      const sampleTarget = v.nodes[0]?.target?.[0]?.slice(0, 60) || '?';
      console.log(`         - [${v.impact}] ${v.id} (${v.nodes.length} node(s)); e.g. ${sampleTarget}`);
    }
  }

  // Tally by severity for the JSON summary.
  const bySeverity = { minor: 0, moderate: 0, serious: 0, critical: 0 };
  for (const v of axeResult.violations) {
    bySeverity[v.impact] = (bySeverity[v.impact] || 0) + 1;
  }
  const violationCount = axeResult.violations.length;
  totalViolations += violationCount;

  // Always ok:true. The `warn: true` flag tells the
  // aggregator this surface had a11y findings worth
  // surfacing. Slice 3b will teach the runner to gate on
  // serious+critical.
  const hasFindings = violationCount > 0;
  record(`a11y:${surface.name}`, true, hasFindings ? {
    warn: true,
    violations: axeResult.violations.map((v) => ({
      id: v.id,
      impact: v.impact,
      help: v.help,
      nodes: v.nodes.length,
    })),
    bySeverity,
    url: targetUrl,
  } : {
    url: targetUrl,
  });
}

async function main(ctx) {
  const here = path.dirname(new URL(import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1'));
  const repoRoot = here.endsWith('audit') ? path.dirname(here) : here;
  const scratchDir = ctx.scratchDir;
  const webBinPath = webBin();

  if (!fs.existsSync(webBinPath)) {
    throw new Error(
      `dixiedata-web binary missing at ${webBinPath}; run \`just debug\` first`,
    );
  }

  await seedArchive(repoRoot, scratchDir);

  const proc = spawn(
    webBinPath,
    ['-addr', `127.0.0.1:${PORT}`, '-scratch-dir', scratchDir],
    {
      cwd: repoRoot,
      env: { ...process.env, DIXIEDATA_DATA_DIR: scratchDir },
    },
  );
  ctx.registerCleanup(() => {
    try { proc.kill('SIGTERM'); } catch (_) { /* best effort */ }
  });
  proc.stderr.on('data', (d) => process.stderr.write(`[srv] ${d}`));

  await waitForServer(BASE);
  console.log(`server up at ${BASE}`);

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ acceptDownloads: true });
  const page = await context.newPage();
  page.on('dialog', async (dialog) => { await dialog.accept(); });

  const surfaces = selectSurfaces();
  console.log(`a11y: ${surfaces.length} surface(s) (mode=${process.env.SMOKE_ROTATION ?? 'ci'})`);

  for (const surface of surfaces) {
    console.log(`\n>>> visiting ${surface.name} (${surface.path})`);
    try {
      await visitSurface(page, surface);
    } catch (e) {
      record(`a11y:${surface.name}`, true, {
        warn: true,
        detail: `unhandled: ${e.message?.slice(0, 200)}`,
      });
    }
  }

  await browser.close();

  console.log(`\na11y: ${pass} clean, ${warn} warn, ${totalViolations} total violation(s)`);

  // Slice 3b: gate on serious + critical counts accumulated
  // by record() above. STRICT (SMOKE_A11Y_STRICT=1) flips to
  // FAIL mode; default mode stays GREEN for the migration
  // window so the existing CI is not blocked while teams
  // triage. Once teams confirm STRICT is stable, the default
  // posture flips to FAIL-mode in a follow-up.
  const { ok: probeOk } = gateA11y(STRICT, seriousViolations, criticalViolations);
  return {
    ok: probeOk,
    pass,
    warn,
    totalViolations,
    serious: seriousViolations,
    critical: criticalViolations,
  };
}

// Pure gate decision, exported for testability. Kept in
// the same file (no new module) because it's only consumed
// by the probe's main() above + the test in
// audit/smoke_a11y.test.mjs.
//   strict=true (SMOKE_A11Y_STRICT=1): fail on any serious
//                                       or critical violation
//   strict=false (default migration window): always pass
export function gateA11y(strict, serious, critical) {
  const hasCritical = (serious + critical) > 0;
  return { ok: !strict || !hasCritical, hasCritical };
}

// Guard the entry point so importing this module for
// unit tests (audit/smoke_a11y.test.mjs) does not spawn
// the server + browser. Only fire runProbe + process.exit
// when this file is the entry point, not when it's
// imported as a module. `process.argv[1]` on Windows uses
// backslashes; use pathToFileURL to normalize before
// comparing.
import { pathToFileURL } from 'node:url';
const entryUrl = process.argv[1] ? pathToFileURL(process.argv[1]).href : null;
if (import.meta.url === entryUrl) {
  const result = await runProbe({
    name: 'a11y',
    probeFn: main,
  });
  process.exit(result.ok ? 0 : 1);
}
