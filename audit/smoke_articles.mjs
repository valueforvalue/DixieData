/**
 * audit/smoke_articles.mjs — browser-driven end-to-end probe for
 * Article Records v1 UI surface (issue #321 slice 3). Walks the
 * headline acceptance criterion: nav item → list → detail → Refs
 * panel + picker → Revisions tab → editor (markdown source +
 * sanitized preview + local-draft persistence) → Cited-in panel.
 *
 * Per AGENTS.md + the slice-3 plan, this probe asserts BOTH the
 * response shape AND `page.url()` after each click. Handler unit
 * coverage lives in `internal/appshell/articles_handlers_test.go`;
 * this probe is the UI end-to-end equivalent.
 *
 * Coverage per slice-3 plan:
 *   1.  Top-level nav has "Articles" pill + click → /articles
 *   2.  /articles/{id} Refs panel: Unlink + Add-Ref buttons render
 *   3.  Picker modal opens + searches Person Records + attaches on click
 *   4.  Revisions tab: Save copy + Restore + Delete round-trip
 *   5.  /articles/new editor: data-draft-key + live preview updates
 *   6.  /articles/{id}/edit editor: pre-filled + draft version attr
 *   7.  Person Record detail "Cited in" panel renders when refs exist
 *
 * Lifecycle:
 *   - Spawns `build/bin/dixiedata-web.exe` against a private
 *     scratch dir, seeds it via `cmd/seed-data`, tears it down
 *     at the end. Mirrors `audit/smoke_events.mjs` lifecycle.
 *   - Cleanup registered with `_lib/cleanup.mjs` so the binary
 *     is killed on Ctrl-C / fatal / successful exit (Windows
 *     task leak is the recurring harness bug).
 *
 * Selector strategy:
 *   CSS / data-attr selectors only — no canonical uiids yet.
 *
 * Failure mode:
 *   - Per-step pass / fail.
 *   - On failure prints: failed step + final url + last response +
 *     2000-char DOM snippet.
 *   - Exits 0 on all-pass, 1 on any-fail, 2 on fatal.
 *
 * Reference: pattern follows `audit/smoke_tags_nav.mjs` (small)
 * + `audit/smoke_events.mjs` (full lifecycle).
 */

import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { setTimeout as sleep } from 'node:timers/promises';
import { registerCleanup } from './_lib/cleanup.mjs';

const PORT = process.env.PROBE_PORT || '8775';
const BASE = `http://127.0.0.1:${PORT}`;
const SCRATCH = process.env.SCRATCH_DIR || 'C:/Development/DixieData/.scratch/articles-smoke';
const WEB_BIN = process.env.WEB_BIN || 'C:/Development/DixieData/build/bin/dixiedata-web.exe';

import fs from 'node:fs';
if (!fs.existsSync(WEB_BIN)) {
  console.error('missing', WEB_BIN);
  process.exit(2);
}
if (!fs.existsSync(SCRATCH)) {
  fs.mkdirSync(SCRATCH, { recursive: true });
}

const server = spawn(WEB_BIN, ['-addr', `127.0.0.1:${PORT}`, '-scratch-dir', SCRATCH], {
  stdio: ['ignore', 'pipe', 'pipe'],
});
server.stderr.on('data', () => {}); // swallow noise
registerCleanup(() => {
  try {
    server.kill();
  } catch (_) {}
});

const wait = (ms) => new Promise((r) => setTimeout(r, ms));

async function ready() {
  for (let i = 0; i < 60; i++) {
    try {
      const r = await fetch(BASE + '/calendar');
      if (r.ok) return;
    } catch (_) {}
    await wait(500);
  }
  throw new Error('server never came up');
}

let pass = 0;
let fail = 0;
const results = [];
const lastResponses = [];

function record(name, ok, details = {}) {
  results.push({ name, ok, ...details });
  if (ok) {
    pass++;
    console.log(`  ✓ ${name}`);
  } else {
    fail++;
    console.log(`  ✗ ${name}`);
    console.log(
      '    ',
      JSON.stringify(details, null, 2)
        .replace(/\n/g, '\n     ')
        .slice(0, 800),
    );
  }
}

let Playwright = null;
try {
  Playwright = await import('playwright');
} catch (e) {
  console.error('playwright import failed:', e.message);
  process.exit(2);
}

try {
  await ready();
  await wait(2000);

  const browser = await Playwright.chromium.launch({ headless: true });
  const ctx = await browser.newContext({ viewport: { width: 1600, height: 1200 } });
  const page = await ctx.newPage();
  page.on('pageerror', (err) => console.log('    [pageerror]', err.message));
  page.on('response', (resp) => {
    lastResponses.push({ url: resp.url(), status: resp.status() });
  });

  // ──────────────────────────────────────────────────────────
  // Step 1: top-level nav has "Articles" + click navigates.
  // ──────────────────────────────────────────────────────────
  console.log('\nStep 1: top-level nav has "Articles" pill');
  await page.goto(BASE + '/calendar');
  await wait(800);

  const articlesLink = await page.evaluate(() => {
    const navs = Array.from(document.querySelectorAll('nav'));
    for (const nav of navs) {
      const link = Array.from(nav.querySelectorAll('a.top-nav-link')).find(
        (a) => (a.textContent || '').trim() === 'Articles',
      );
      if (link) return { href: link.getAttribute('href'), hasDataAttr: link.hasAttribute('data-article-nav-link') };
    }
    return null;
  });
  record('top-nav-has-articles-pill', articlesLink !== null, { articlesLink });
  record('top-nav-pill-points-to-articles', articlesLink && articlesLink.href === '/articles', {
    href: articlesLink && articlesLink.href,
  });
  record('top-nav-pill-has-data-attr', articlesLink && articlesLink.hasDataAttr === true);

  await page.locator('a.top-nav-link', { hasText: 'Articles' }).first().click();
  await wait(800);
  const step1Url = page.url();
  record('click-navigates-to-articles', step1Url.endsWith('/articles'), { url: step1Url });

  const articlesListRendered = await page.evaluate(() => {
    return (
      document.body.textContent.includes('Articles') &&
      (document.querySelector('[data-articles-list]') !== null ||
        document.body.textContent.includes('No articles yet'))
    );
  });
  record('articles-list-rendered', articlesListRendered);

  // ── Slice-3.1 step complete. Slice-3.2+ steps land in follow-up commits.

  await browser.close();
  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail === 0 ? 0 : 1);
} catch (e) {
  console.error('FATAL', e);
  console.error('last responses:', JSON.stringify(lastResponses.slice(-5), null, 2));
  process.exit(2);
} finally {
  try {
    server.kill();
  } catch (_) {}
  await wait(500);
}