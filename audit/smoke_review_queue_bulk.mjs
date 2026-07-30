/**
 * audit/smoke_review_queue_bulk.mjs — /review-queue bulk-action
 * surface probe (issue #700 tier-2).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir,
 * seeds 3 soldiers, then asserts:
 *   1. /review-queue renders the review-queue page (no auth
 *      gate on this surface).
 *   2. Empty queue path renders the "review queue is clear"
 *      copy when no NeedsReview rows exist.
 *   3. After seeding 2 NeedsReview rows via direct POST to
 *      /soldiers (empty name + confirm_empty_name=1 triggers
 *      the soft-confirm-and-mark-for-review path per
 *      issue #151), the queue renders 2 entry cards with
 *      checkboxes + the bulk-action toolbar.
 *   4. The bulk-action form action targets /review-queue/bulk
 *      (the handleReviewQueueBulk endpoint).
 *   5. The "Select all" checkbox carries data-select-all.
 *   6. The Ignore Selected submit button carries
 *      name="bulk_action" value="ignore" + data-confirm.
 *   7. The Delete Selected submit button carries
 *      name="bulk_action" value="delete" + data-confirm.
 *   8. The Mark as Resolved per-row button carries
 *      data-action="/soldiers/{id}/review/resolve?context=queue"
 *      and data-dixie-submit (issue #248 fix regression net).
 *
 * Note: the bulk Ignore / Delete round-trip itself is
 * covered by the legacy probe
 * audit/smoke_review_queue_bulk.mjs (which injects a
 * synthetic form and asserts the dispatch body includes
 * bulk_action). This probe pins the templ-rendered form
 * contract on a real /review-queue page load.
 *
 * Run: `node audit/smoke_review_queue_bulk.mjs`
 * Exits 0 on all-pass, 1 on any-fail, 2 on fatal.
 */
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';

const PORT = process.env.PROBE_PORT || '8789';
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
  const seedProc = spawn('go', ['run', './cmd/seed-data', '-data-dir', SCRATCH, '-soldiers', '3', '-reset'], { stdio: ['ignore', 'pipe', 'pipe'] });
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

  await page.goto(`${BASE}/review-queue`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));

  const reviewQueueHeading = await page.locator('h2:has-text("Review Queue")').count();
  record('review-queue-renders-page', reviewQueueHeading >= 1, { reviewQueueHeading });
  const emptyText = await page.locator('body').innerText().catch(() => '');
  record('review-queue-empty-state-when-no-needs-review', /review queue is clear/i.test(emptyText), { emptyText: emptyText.slice(0, 200) });

  // Seed 2 NeedsReview rows via the empty-name confirmation
  // path. handleCreateSoldier sets NeedsReview=true when
  // first_name + last_name are empty AND confirm_empty_name=1.
  // POST directly with FormData so we bypass the JS-side
  // data-confirm dialog (the probe is verifying the queue
  // rendering, not the JS confirm path).
  for (let i = 0; i < 2; i++) {
    const res = await fetch(`${BASE}/soldiers`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({
        first_name: '',
        last_name: '',
        confirm_empty_name: '1',
        entry_type: 'soldier',
      }).toString(),
    });
    if (res.status >= 400) {
      throw new Error(`seed NeedsReview soldier #${i} failed (${res.status})`);
    }
  }

  await page.goto(`${BASE}/review-queue`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));

  const entryCards = await page.locator('[id^="review-queue-item-"]').count();
  record('review-queue-renders-entry-cards-when-needs-review', entryCards === 2, { entryCards });

  const bulkActionForm = await page.locator('form[data-dixie-submit][action="/review-queue/bulk"]').first().getAttribute('action').catch(() => null);
  record('review-queue-bulk-form-targets-endpoint', bulkActionForm === '/review-queue/bulk', { bulkActionForm });

  record('review-queue-select-all-checkbox-renders', (await page.locator('input[data-select-all="review-queue"]').count()) >= 1, {});

  const ignoreButton = page.locator('button[name="bulk_action"][value="ignore"][data-confirm]').first();
  record('review-queue-ignore-button-has-confirm', await ignoreButton.getAttribute('data-confirm').then((v) => !!v && v.length > 0), {});

  const deleteButton = page.locator('button[name="bulk_action"][value="delete"][data-confirm]').first();
  record('review-queue-delete-button-has-confirm', await deleteButton.getAttribute('data-confirm').then((v) => !!v && v.length > 0), {});

  const resolveAction = await page.locator('[data-action*="/review/resolve?context=queue"][data-dixie-submit="true"]').first().getAttribute('data-action').catch(() => null);
  record('review-queue-mark-as-resolved-data-action-attr', resolveAction !== null && /\/soldiers\/\d+\/review\/resolve\?context=queue/.test(resolveAction), { resolveAction });

  await browser.close().catch(() => {});

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

runProbe({ name: 'review-queue-bulk', probeFn: main })
  .then((r) => process.exit(r.ok ? 0 : 1))
  .catch((err) => { console.error('fatal:', err); process.exit(2); });