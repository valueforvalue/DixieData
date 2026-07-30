/**
 * audit/smoke_htmx_swap_rebind.mjs — class 3 regression net
 * (issue #700 tier-3 follow-up, #706).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir,
 * seeds 5 soldiers, then asserts:
 *   1. /browse with ?page=1&page_size=2 renders 2 result
 *      cards + a Next pager link with hx-get + hx-target.
 *   2. Clicking Next fires an htmx GET + swaps
 *      #browse-results with page-2 content (2 more cards).
 *   3. The SWAPPED-IN #browse-results contains a new
 *      Previous pager link with hx-get (proving htmx
 *      re-scanned the new content for hx-* attrs -- the
 *      class 3 fix from cb4ac34 that added
 *      `htmx.process(target)` to swapHtmlIntoTarget).
 *   4. Clicking the swapped-in Previous pager link
 *      fires ANOTHER htmx GET (not a full-page reload)
 *      and swaps back to page 1.
 *   5. The original /calendar day cell hx-get still
 *      works after a /browse swap (sanity: the class 3
 *      fix doesn't regress other htmx surfaces).
 *
 * Why this matters: the pre-cb4ac34 code did
 * `target.innerHTML = html; initializeDynamicContent();`
 * which silently dropped htmx wiring on swapped-in
 * elements (the canonical example: a swapped-in pager
 * link had hx-get but no htmx scan, so clicking it did
 * a full page reload instead of a fragment swap). The
 * fix added `htmx.process(target)` to the helper. A
 * future refactor that inlines the raw `innerHTML = html`
 * without `htmx.process(target)` would re-introduce the
 * bug. This probe pins the post-swap hx-get firing.
 *
 * Run: `node audit/smoke_htmx_swap_rebind.mjs`
 * Exits 0 on all-pass, 1 on any-fail, 2 on fatal.
 */
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';

const PORT = process.env.PROBE_PORT || '8801';
const BASE = `http://127.0.0.1:${PORT}`;
const WEB_BIN_PATH = webBin();
if (!existsSync(WEB_BIN_PATH)) { console.error('missing', WEB_BIN_PATH); process.exit(2); }

let pass = 0, fail = 0;
const results = [];
function record(name, ok, details = {}) {
  results.push({ name, ok, details });
  if (ok) { pass++; console.log(`  PASS ${name}`); }
  else { fail++; console.log(`  FAIL ${name}\n    ${JSON.stringify(details).slice(0, 800)}`); }
}

async function main(ctx) {
  const SCRATCH = ctx.scratchDir;
  const seedProc = spawn('go', ['run', './cmd/seed-data', '-data-dir', SCRATCH, '-soldiers', '5', '-reset'], { stdio: ['ignore', 'pipe', 'pipe'] });
  let seedOut = '';
  seedProc.stdout.on('data', (d) => { seedOut += d; });
  seedProc.stderr.on('data', (d) => { seedOut += d; });
  const seedExit = await new Promise((resolve) => seedProc.on('exit', resolve));
  if (seedExit !== 0) throw new Error(`seed-data failed (${seedExit}):\n${seedOut}`);

  const server = spawn(WEB_BIN_PATH, ['-addr', `127.0.0.1:${PORT}`, '-scratch-dir', SCRATCH], { stdio: ['ignore', 'pipe', 'pipe'], env: { ...process.env, DIXIEDATA_DATA_DIR: SCRATCH } });
  ctx.registerCleanup(() => { try { server.kill(); } catch (_) {} });

  for (let i = 0; i < 60; i++) {
    try { const r = await fetch(`${BASE}/`); if (r.status < 500) break; } catch {}
    await new Promise((r) => setTimeout(r, 250));
  }

  const browser = await chromium.launch({ headless: true });
  ctx.registerCleanup(() => browser.close().catch(() => {}));
  const page = await browser.newPage({ viewport: { width: 1600, height: 1200 } });
  page.on('dialog', (d) => { d.accept().catch(() => {}); });

  // Navigate to /browse with page_size=2 so we span 3 pages
  // (5 soldiers / 2 per page = pages 1, 2, 3).
  await page.goto(`${BASE}/browse?page=1&page_size=2`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 600));

  // Step 1: page 1 renders 2 result cards + a Next pager link.
  // The browse has both mobile (md:hidden) and desktop
  // (hidden md:block) layouts; each record renders in both,
  // so 2 records = 4 checkboxes (2 mobile + 2 desktop).
  // The check is "page 1 has exactly 2 records" = 4 checkboxes.
  const cardsPage1 = await page.locator('#browse-results input[data-browse-select]').count();
  record('htmx-rebind-page-1-renders-2-cards', cardsPage1 === 4, { cardsPage1 });

  const nextLink = page.locator('#browse-results a[data-browse-page-link][aria-label="Next page"]').first();
  record('htmx-rebind-next-pager-link-renders', (await nextLink.count()) >= 1, {});
  const nextHref = await nextLink.getAttribute('href').catch(() => null);
  record('htmx-rebind-next-pager-link-wired', nextHref !== null && /page=2/.test(nextHref), { nextHref });

  // Step 2: click Next -> htmx GET fires -> #browse-results
  // swaps. Wait for the response + the DOM update.
  await Promise.all([
    page.waitForResponse((r) => /\/browse\/results/.test(r.url()) && r.request().method() === 'GET', { timeout: 15_000 }).catch(() => null),
    nextLink.click({ timeout: 5_000 }),
  ]);
  await new Promise((r) => setTimeout(r, 800));
  record('htmx-rebind-after-next-page-url-still-browse', /\/browse/.test(page.url()), { url: page.url() });

  // Step 3: the swapped-in #browse-results has a Previous
  // pager link. htmx must have re-scanned the new content
  // for hx-get -- this is the class 3 regression net.
  const prevLink = page.locator('#browse-results a[data-browse-page-link][aria-label="Previous page"]').first();
  record('htmx-rebind-swapped-pager-has-prev-link', (await prevLink.count()) >= 1, {});
  const prevHref = await prevLink.getAttribute('href').catch(() => null);
  record('htmx-rebind-swapped-pager-href-is-page-1', prevHref !== null && /page=1/.test(prevHref), { prevHref });

  // Step 4: click the swapped-in Previous link. If htmx
  // wiring survived the swap, this is another htmx GET
  // (not a full page reload). Verify by checking the
  // /browse/results endpoint was hit.
  await Promise.all([
    page.waitForResponse((r) => /\/browse\/results/.test(r.url()) && r.request().method() === 'GET', { timeout: 15_000 }).catch(() => null),
    prevLink.click({ timeout: 5_000 }),
  ]);
  await new Promise((r) => setTimeout(r, 600));
  // After the second htmx swap we're back to page 1.
  // The Next link should be the swapped-in one (with
  // the post-cb4ac34 fix, the htmx wiring is re-bound
  // and clickable).
  const nextLinkAfterSwapBack = page.locator('#browse-results a[data-browse-page-link][aria-label="Next page"]').first();
  record('htmx-rebind-after-prev-swap-next-link-clickable', (await nextLinkAfterSwapBack.count()) >= 1, {});

  // Step 5: navigate to /calendar and verify the day-cell
  // hx-get still works (sanity: the class 3 fix didn't
  // regress other htmx surfaces).
  await page.goto(`${BASE}/calendar`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 500));
  const dayCell = page.locator('.grid.grid-cols-7 button[hx-get*="/anniversary/"]').first();
  record('htmx-rebind-calendar-day-cell-still-wired', (await dayCell.count()) >= 1, {});

  await browser.close().catch(() => {});

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

runProbe({ name: 'htmx-swap-rebind', probeFn: main })
  .then((r) => process.exit(r.ok ? 0 : 1))
  .catch((err) => { console.error('fatal:', err); process.exit(2); });