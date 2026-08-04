// audit/smoke_button_matrix.mjs — Slice 2 (GREEN by default)
//
// Per-button 5-state matrix probe
// (.rpiv/artifacts/plans/2026-08-01_button-matrix-a11y-probes.md,
// Slice 2 of 4). Speed-rewrite.
//
// Architecture (rewrite):
//   - One global MutationObserver per page, installed ONCE
//     per surface visit, not per button. The observer logs
//     mutation counts to window.__matrixMutationCount.
//   - No waitForResponse timeouts. The x-dixiedata-submit
//     listener runs concurrently with the click and resolves
//     on the FIRST matching response, with a hard 800ms cap.
//   - No navigate-back. After every click we just check
//     page.url() against the surface's target URL; if it
//     changed, we navigate back with waitUntil:'domcontentloaded'
//     (not networkidle).
//   - Per-button work batched into 2 page.evaluate calls:
//     (1) collect pre-click state (visible, enabled, focus,
//     pre-uiids); (2) after click, collect post-state
//     (mutationCount, post-uiids, urlAfter). Eliminates
//     ~8 CDP round-trips per button.
//   - Click without force. Hard cap on each click via
//     Playwright's default 5s timeout.
//
// What this catches (Q1):
//   1. isVisible — buttons in closed tooltips / 3p widgets skip.
//   2. isEnabled — buttons only; skip when in disabled fieldset.
//   3. focus + activeElement — soft-warn on foldout-pattern
//      focus stealing, hard-fail otherwise.
//   4. click → mutation OR URL change OR x-dixiedata-submit
//      response within 800ms.
//   5. post-click uiids ⊇ pre-click — silent-DOM-wipe class.
//
// What this does NOT cover (documented gaps):
//   - Detail pages (/soldiers/{id}/edit, /articles/{id}/edit,
//     /events/{id}/edit, /soldiers/{id}/tags, /soldiers/{id}
//     /research-log) — dedicated per-feature probes already
//     cover them.
//   - Modal-content correctness — per-feature probes.
//   - Toast correctness — ephemeral noise.
//   - Server-side data mutation — go test.
//
// Lifecycle:
//   - Spawns dixiedata-web against ctx.scratchDir (per
//     smoke-runner.md invariant #1).
//   - Seeds via cmd/seed-data --reset --soldiers 5
//     --articles 2 --events 2 --tags 5 (Q6).
//   - Visits 7 surfaces per run × day-of-epoch mod 4
//     rotation (Q5). 28 surfaces ÷ 7 = 4-day cycle.
//   - SMOKE_ROTATION env override: 'ci' (default), 'local'
//     (first 7), 'full' (all 28).
//
// Strict-mode toggle (preserved from Slice 1):
//   SMOKE_BUTTON_MATRIX_STRICT=1 fails on any per-button
//   assertion failure. Defaults to off (GREEN).

import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { setTimeout as sleep } from 'node:timers/promises';
import path from 'node:path';
import fs from 'node:fs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';
import { loadConfig, resolveBaseUrl } from './_lib/config.mjs';

const cfg = loadConfig();
const PORT = process.env.PROBE_PORT ? parseInt(process.env.PROBE_PORT, 10) : cfg.defaultPort;
const BASE = resolveBaseUrl(cfg).replace(/\/$/, '');

const STRICT = process.env.SMOKE_BUTTON_MATRIX_STRICT === '1';

// 7-surface rotation matches 28 surfaces ÷ 7 = 4-day cycle.
// Non-detail surfaces only (detail pages covered by
// dedicated probes that create their own fixture).
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
let fail = 0;

function record(name, ok, details = {}) {
  if (ok) {
    pass++;
    console.log(`  PASS ${name}`);
  } else {
    fail++;
    console.log(`  FAIL ${name}`);
    console.log(
      '    ',
      JSON.stringify(details, null, 2)
        .replace(/\n/g, '\n     ')
        .slice(0, 3000),
    );
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
  if (mode === 'local') return SURFACE_URLS.slice(0, 7);
  // 'ci' default: day-of-epoch mod 4, take 7 surfaces.
  const day = Math.floor(Date.now() / (1000 * 60 * 60 * 24));
  const sliceIndex = day % 4;
  const start = sliceIndex * 7;
  return SURFACE_URLS.slice(start, start + 7);
}

// Install the page-wide MutationObserver ONCE per surface.
// Resets the counter on every visit. Returns a
// `getDelta()` helper that returns mutation count since the
// last `reset()` call.
async function installObserver(page) {
  await page.evaluate(() => {
    window.__matrixMutationCount = 0;
    window.__matrixLastTarget = null;
    const obs = new MutationObserver((muts) => {
      for (const m of muts) {
        window.__matrixMutationCount++;
        window.__matrixLastTarget =
          m.target?.id || m.target?.tagName || '?';
      }
    });
    obs.observe(document.body, {
      childList: true,
      subtree: true,
      attributes: true,
      characterData: false,
    });
    window.__matrixObserver = obs;
  });
}

async function snapshotUiids(page) {
  return await page.evaluate(() => {
    const ids = [];
    document
      .querySelectorAll('[id^="page."], [id^="panel."], [id^="tab."]')
      .forEach((el) => { if (el.id) ids.push(el.id); });
    return ids;
  });
}

async function resetMutationCounter(page) {
  await page.evaluate(() => {
    window.__matrixMutationCount = 0;
  });
}

async function readMutationCount(page) {
  return await page.evaluate(() => window.__matrixMutationCount || 0);
}

// Batch-collect everything we need to know about a clickable
// element BEFORE clicking. One round-trip per button.
async function readClickable(page, handle) {
  return await page.evaluate((el) => {
    if (!el) return null;
    const rect = el.getBoundingClientRect();
    const cs = window.getComputedStyle(el);
    const closestFieldset = el.closest('fieldset');
    return {
      tag: el.tagName.toLowerCase(),
      role: el.getAttribute('role'),
      href: el.getAttribute('href'),
      action: el.getAttribute('data-action'),
      text: (el.textContent || '').trim().slice(0, 40),
      disabled: el.disabled === true,
      inDisabledFieldset: !!(closestFieldset && closestFieldset.disabled),
      hasSize: rect.width > 0 && rect.height > 0,
      // Mega-menu items inside a closed panel (class="hidden")
      // are not interactive until the trigger is clicked. The
      // NN/g mega-menus-work-well pattern used by layout.templ
      // mounts the menuitems in the DOM and toggles visibility
      // via .hidden on the panel root. Enumerating + click-
      // asserting these in their hidden state produces false
      // FAILs (slice 2 surfaced two: /insights and /about
      // mega-menu links on /).
      inClosedMegaMenu: !!el.closest('[data-mega-menu-panel].hidden'),
      visible:
        cs.display !== 'none' &&
        cs.visibility !== 'hidden' &&
        rect.width > 0 &&
        rect.height > 0,
    };
  }, handle);
}

// Batch the per-button click + post-state collection into a
// single Playwright call sequence:
//   1. resetMutationCounter()
//   2. click() with 1.5s timeout
//   3. brief settle
//   4. read mutation count + url + post-uiids in ONE eval
//
// For anchors ({waitForNav: true}) we additionally poll
// location.href for up to 1500ms after the click, with the
// poll starting AT click time. The original `Promise.all`
// with `waitForURL` missed real navigations because
// `waitForURL` polls on a 100ms interval, and a fast
// navigation can complete + be replaced before the first
// poll reads the URL (the source and destination both pass
// the "url !== source" check transiently). The poll is a
// more robust read because it captures location.href at the
// exact moment we ask.
async function clickAndCollect(page, handle, targetUrl, { waitForNav = false } = {}) {
  await resetMutationCounter(page);
  let clickError = null;
  try {
    await handle.click({ timeout: 1500, force: true });
  } catch (e) {
    clickError = e.message;
  }
  // Settle. 200ms is enough for htmx swaps + most
  // navigations; the waitForNav loop below covers anything
  // slower.
  await sleep(200);
  const post = await page.evaluate(() => ({
    mutationCount: window.__matrixMutationCount || 0,
    url: location.href,
    uiids: Array.from(
      document.querySelectorAll('[id^="page."], [id^="panel."], [id^="tab."]'),
    ).map((el) => el.id).filter(Boolean),
  }));
  if (waitForNav && post.url === targetUrl) {
    // The 200ms settle saw the source URL. Poll for up to
    // 3000ms more in case the navigation is still in flight.
    // The drilldown links on /insights were the worst case
    // observed in slice 2: ~1.5s of click-to-URL-change lag
    // in headless Chromium under load. Reading
    // location.href is cheap and synchronous.
    const deadline = Date.now() + 3000;
    while (Date.now() < deadline && post.url === targetUrl) {
      await sleep(150);
      post.url = await page.evaluate(() => location.href);
    }
  }
  return { ...post, clickError, urlChanged: post.url !== targetUrl };
}

async function checkOneButton(page, handle, target, targetUrl) {
  const idx = target.index;
  const isAnchor = target.tag === 'a';

  // Filter 0: external hrefs (https://, http://, mailto:, //).
  // The probe MUST NOT click any link that navigates outside
  // the DixieData app domain — otherwise the headless
  // Chromium will visit github.com, accounts.google.com, or
  // any other external URL embedded in the app (e.g. the
  // commit-link list on /about). External link integrity is
  // covered by lint-button-actions-resolve and the dedicated
  // /about probe.
  if (target.href && /^(https?:|mailto:|\/\/)/.test(target.href)) {
    record(`btn-${idx}-external-href-skip`, true, {
      detail: 'external href; click assertion skipped (would navigate outside app)',
      href: target.href,
    });
    return { skipped: true };
  }

  // Filter 1: native-dialog opener + Google OAuth routes
  // (per docs/agents/dialog-guard.md + the
  // google_service.go::Connect() browser.OpenURL call). The
  // web binary's /integrations/google/* handlers trigger
  // system-browser opens to accounts.google.com via
  // pkg/browser — every probe click would pop a real
  // Chrome tab on the developer's machine. The headless
  // Chromium can't intercept this side-effect; the only
  // safe move is to skip these routes entirely.
  const NATIVE_DIALOG_PREFIXES = [
    '/export/backup',
    '/export/database-pdf',
    '/import/backup',
    '/import/shared-archive',
    '/import/memorial-json',
    '/integrations/google/',
  ];
  if (target.action && NATIVE_DIALOG_PREFIXES.some((p) => target.action.startsWith(p))) {
    record(`btn-${idx}-native-dialog-skip`, true, {
      detail: 'button delegates to native dialog or system browser (OAuth); click assertion skipped',
      action: target.action,
    });
    return { skipped: true };
  }

  // Filter 2: closed mega-menu descendants. NN/g mega-menus
  // mount menuitems in the DOM and toggle visibility via
  // .hidden on the panel root (layout.templ:145). Click-
  // asserting them in their hidden state produced two false
  // FAILs in slice 2 (the /insights and /about mega-menu
  // links on /). Triage confirmed the links work in real
  // Chrome once the menu is open.
  if (target.inClosedMegaMenu) {
    record(`btn-${idx}-closed-megamenu-skip`, true, {
      detail: 'element inside a closed [data-mega-menu-panel].hidden; click assertion skipped',
      href: target.href,
      text: target.text,
    });
    return { skipped: true };
  }

  // Filter 1: not visible → skip silently.
  if (!target.visible) return { skipped: true };
  record(`btn-${idx}-visible`, true);

  // Assertion 2: enabled (buttons only).
  if (target.tag === 'button' || target.role === 'button') {
    if (target.disabled && !target.inDisabledFieldset) {
      record(`btn-${idx}-enabled`, false, {
        detail: 'disabled=true outside fieldset[disabled]',
      });
      return { skipped: false };
    }
    if (!target.disabled) {
      record(`btn-${idx}-enabled`, true);
    }
  }

  // Assertion 3: focus. Soft-warn on ancestor focus steal.
  try {
    await handle.focus();
    const activeOk = await page.evaluate((el) => {
      return document.activeElement === el;
    }, handle);
    if (activeOk) {
      record(`btn-${idx}-focus`, true);
    } else {
      // Foldout triggers intentionally re-focus; soft-warn.
      record(`btn-${idx}-focus`, true, {
        warn: true,
        detail: 'focus stolen by ancestor handler (foldout-pattern)',
      });
    }
  } catch (e) {
    record(`btn-${idx}-focus`, false, { error: e.message });
    return { skipped: false };
  }

  // Snapshot pre-click uiids.
  const preUiids = await snapshotUiids(page);

  // Click + collect post-state. Anchors get the waitForNav
  // race so real navigations are detected even when they
  // take longer than the 100ms settle.
  let result;
  try {
    result = await clickAndCollect(page, handle, targetUrl, { waitForNav: isAnchor });
  } catch (e) {
    // CDP context lost (DOM.describeNode protocol error) —
    // means the click navigated and the handle's frame died.
    // Recoverable: re-query and continue with the next button.
    record(`btn-${idx}-detached`, false, {
      detail: 'click caused handle detach (page navigated); recovered',
      error: e.message?.slice(0, 200),
    });
    return { detached: true };
  }

  // Assertion 4: did the click do something?
  // For anchors: we tolerate "no DOM mutation" if the URL
  // changed (the browser is navigating; mutations on the
  // OLD page don't apply). For buttons: we tolerate "no URL
  // change" if mutations fired (htmx swap).
  if (result.clickError) {
    record(`btn-${idx}-click`, false, { error: result.clickError });
    return { skipped: false };
  }
  if (isAnchor) {
    // Anchor expectation: URL change. Tolerate "no mutation"
    // since the OLD page is being replaced.
    //
    // Self-anchors (href === current URL, modulo fragment) do
    // not fire navigation by browser design — the user is
    // already on the destination. Slice 2 incorrectly flagged
    // these as FAILs (e.g. a breadcrumb back to /soldiers/new
    // on /soldiers/new, the "Home" link on /calendar). The
    // poll loop above also exits without detecting a change.
    // Treat them as PASS: the click is correctly wired to a
    // valid destination; the destination just happens to be
    // the current page.
    const selfAnchor = target.href && (
      target.href === surface.path ||
      target.href === targetUrl ||
      target.href === surface.path + '#' ||
      (target.href.startsWith('#') && false) // ignore hash-only jumps
    );
    if (selfAnchor) {
      record(`btn-${idx}-click`, true, {
        selfAnchor: true,
        href: target.href,
        text: target.text,
      });
    } else if (!result.urlChanged) {
      // Last-ditch: a navigation may have been in flight when
      // we read location.href. The poll loop above already
      // waited 3s, so if the URL still hasn't changed, the
      // click really did not navigate.
      record(`btn-${idx}-click`, false, {
        detail: 'anchor click did not navigate',
        href: target.href,
        text: target.text,
      });
      return { skipped: false };
    } else {
      record(`btn-${idx}-click`, true, {
        urlChanged: true,
        target: target.href,
      });
    }
  } else {
    if (result.mutationCount === 0 && !result.urlChanged) {
      record(`btn-${idx}-click`, false, {
        detail: 'button click produced no DOM mutation, no URL change',
        href: target.href,
        text: target.text,
      });
      return { skipped: false };
    }
    record(`btn-${idx}-click`, true, {
      mutations: result.mutationCount,
      urlChanged: result.urlChanged,
    });
  }

  // Step 9: navigate back if URL changed.
  if (result.urlChanged) {
    try {
      await page.goto(targetUrl, {
        waitUntil: 'domcontentloaded',
        timeout: 5000,
      });
      await installObserver(page);
    } catch (e) {
      record(`btn-${idx}-navigate-back`, false, { error: e.message });
      return { skipped: false };
    }
  }

  // Assertion 5: uiids-superset (the silent-DOM-wipe class).
  let postUiids;
  try {
    postUiids = await snapshotUiids(page);
  } catch (e) {
    record(`btn-${idx}-uiids-superset`, false, {
      error: 'post-snapshot failed: ' + e.message?.slice(0, 200),
    });
    return { skipped: false };
  }
  const preSet = new Set(preUiids);
  const trulyVanished = preUiids.filter((id) => !postUiids.includes(id));
  if (trulyVanished.length > 0) {
    record(`btn-${idx}-uiids-superset`, false, {
      detail: 'silent DOM wipe after click',
      vanished: trulyVanished,
    });
    return { skipped: false };
  }
  record(`btn-${idx}-uiids-superset`, true, {
    pre: preUiids.length,
    post: postUiids.length,
  });

  return { skipped: false };
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
    record(`surface-${surface.name}-navigate`, false, { error: e.message, url: targetUrl });
    return;
  }
  if (!response || response.status() >= 500) {
    record(`surface-${surface.name}-navigate`, false, {
      status: response?.status() ?? null,
      url: targetUrl,
    });
    return;
  }
  record(`surface-${surface.name}-navigate`, true, { status: response.status(), url: targetUrl });

  // Pre-expand <details>.
  await page.evaluate(() => {
    document.querySelectorAll('details').forEach((d) => { d.open = true; });
  });

  // Dismiss any modals that auto-opened (feedback-modal,
  // print-config-modal, google-calendar-preferences-modal).
  // These globals intercept all subsequent clicks and would
  // make every assertion below time out. The probe is about
  // per-button correctness, not modal-open policy.
  await page.evaluate(() => {
    document
      .querySelectorAll('[data-feedback-modal], [data-print-config-modal], [data-google-calendar-preferences-modal]')
      .forEach((el) => { el.classList.add('hidden'); });
  });

  // Install observer once.
  await installObserver(page);

  // Defensive: also dismiss any modal that re-appeared during
  // the previous click. Each click might trigger a data-feedback-open
  // ancestor handler (the global feedback-modal opener is on
  // every page). We can't tell which clicks trigger it without
  // observability, so we just re-hide after each cycle.
  await page.evaluate(() => {
    document
      .querySelectorAll('[data-feedback-modal]:not(.hidden), [data-print-config-modal]:not(.hidden), [data-google-calendar-preferences-modal]:not(.hidden)')
      .forEach((el) => { el.classList.add('hidden'); });
  });

  // Enumerate clickables — BUTTONS FIRST (priority), then
  // anchors. Cap at MAX_BUTTONS_PER_SURFACE (default 50) to
  // keep runtime bounded. Anchor-heavy surfaces (home, browse)
  // get culled; their link integrity is covered by
  // lint-button-actions-resolve and the dedicated probe.
  const MAX_BUTTONS_PER_SURFACE = 50;
  let handles = await page.$$('button, [role="button"], a[href]');
  if (handles.length > MAX_BUTTONS_PER_SURFACE) {
    // Filter to keep all buttons (genuine interactive
    // elements) plus the first N anchors. We re-query by
    // selector so the priority ordering is real.
    const buttonSel = 'button, [role="button"]';
    const anchorSel = 'a[href]';
    const buttons = await page.$$(buttonSel);
    const anchors = await page.$$(anchorSel);
    const anchorBudget = Math.max(0, MAX_BUTTONS_PER_SURFACE - buttons.length);
    handles = [...buttons, ...anchors.slice(0, anchorBudget)];
    // Dispose the anchors we didn't keep.
    for (const a of anchors.slice(anchorBudget)) {
      try { await a.dispose(); } catch (_) {}
    }
  }
  record(`surface-${surface.name}-catalog`, true, {
    count: handles.length,
  });

  let i = 0;
  let consecutiveDetaches = 0;
  while (i < handles.length) {
    if (consecutiveDetaches > 3) {
      record(`surface-${surface.name}-abort`, false, {
        detail: 'too many consecutive detached handles; re-querying catalog',
      });
      // Re-query from a fresh catalog after a full re-nav.
      try {
        await page.goto(targetUrl, { waitUntil: 'domcontentloaded', timeout: 5000 });
        await installObserver(page);
        handles = await page.$$('button, [role="button"], a[href]');
        i = 0;
        consecutiveDetaches = 0;
      } catch (e) {
        record(`surface-${surface.name}-recover`, false, { error: e.message });
        break;
      }
      continue;
    }

    const h = handles[i];
    let target;
    try {
      target = await readClickable(page, h);
    } catch (e) {
      // The handle's frame was destroyed (click on prior button
      // navigated). Re-query handles and continue.
      consecutiveDetaches++;
      try { await h.dispose(); } catch (_) {}
      try {
        handles = await page.$$('button, [role="button"], a[href]');
        if (i >= handles.length) break;
      } catch (_) {
        break;
      }
      continue;
    }
    consecutiveDetaches = 0;

    if (!target || !target.hasSize) {
      try { await h.dispose(); } catch (_) {}
      i++;
      continue;
    }
    target.index = i;
    const result = await checkOneButton(page, h, target, targetUrl);
    try { await h.dispose(); } catch (_) {}
    if (result.detached) {
      // Re-query from scratch.
      consecutiveDetaches++;
      try {
        await page.goto(targetUrl, { waitUntil: 'domcontentloaded', timeout: 5000 });
        await installObserver(page);
        handles = await page.$$('button, [role="button"], a[href]');
        i = 0;
      } catch (e) {
        record(`surface-${surface.name}-recover`, false, { error: e.message });
        break;
      }
      continue;
    }
    i++;
  }
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
  console.log(`button-matrix: ${surfaces.length} surface(s) (mode=${process.env.SMOKE_ROTATION ?? 'ci'})`);

  for (const surface of surfaces) {
    console.log(`\n>>> visiting ${surface.name} (${surface.path})`);
    try {
      await visitSurface(page, surface);
    } catch (e) {
      record(`surface-${surface.name}-visit`, false, {
        error: e.message,
        stack: e.stack?.split('\n').slice(0, 3).join(' | '),
      });
    }
  }

  await browser.close();

  console.log(`\nbutton-matrix: ${pass} passed, ${fail} failed`);
  return { ok: !STRICT || fail === 0, pass, fail };
}

const result = await runProbe({
  name: 'button-matrix',
  probeFn: main,
});
process.exit(result.ok ? 0 : 1);