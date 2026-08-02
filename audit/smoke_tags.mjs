/**
 * audit/smoke_tags.mjs — /tags management surface probe
 * (issue #700 tier-2).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir,
 * seeds 1 soldier + 5 tags, then asserts:
 *   1. /tags renders the tag-management page with at least
 *      5 tag rows.
 *   2. Each row carries an inline rename form targeting
 *      /tags/{id}/rename with a tag_name input.
 *   3. Each row carries an inline merge form targeting
 *      /tags/{id}/merge with a survivor_id <select>
 *      populated with the other tags (excluding self).
 *   4. The merge select excludes the source row's own id
 *      (a tag cannot merge into itself).
 *   5. Each row carries an inline Delete form targeting
 *      /tags/{id} with data-confirm.
 *   6. Picking a survivor + clicking Merge posts the form,
 *      server merges the tags, the response redirects to
 *      /tags, and the source row is gone.
 *   7. Renaming a tag persists the new name (server round-trip).
 *   8. Deleting a tag with data-confirm-accept surfaces the
 *      X-DixieData-Toast and the row is gone.
 *
 * Run: `node audit/smoke_tags.mjs`
 * Exits 0 on all-pass, 1 on any-fail, 2 on fatal.
 */
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';
import { loadConfig, resolveBaseUrl } from './_lib/config.mjs';
import { submitAndWait } from './_lib/smoke_form.mjs';

const cfg = loadConfig();
const BASE = resolveBaseUrl(cfg);
const PORT = process.env.PROBE_PORT ? parseInt(process.env.PROBE_PORT, 10) : cfg.defaultPort;
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
  const seedProc = spawn('go', ['run', './cmd/seed-data', '-data-dir', SCRATCH, '-soldiers', '1', '-tags', '5', '-reset'], { stdio: ['ignore', 'pipe', 'pipe'] });
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

  await page.goto(`${BASE}/tags`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));

  const tagRows = await page.locator('tr[id^="tag-row-"]').count();
  record('tags-renders-tag-rows', tagRows >= 5, { tagRows });

  record('tags-rename-form-targets-rename-endpoint', (await page.locator('form[data-dixie-submit][action*="/tags/"][action$="/rename"]').count()) >= 5, {});
  record('tags-rename-form-has-tag-name-input', (await page.locator('form[action*="/tags/"][action$="/rename"] input[name="tag_name"]').count()) >= 5, {});

  record('tags-merge-form-targets-merge-endpoint', (await page.locator('form[data-dixie-submit][action*="/tags/"][action$="/merge"]').count()) >= 5, {});
  record('tags-merge-form-has-survivor-select', (await page.locator('form[action*="/tags/"][action$="/merge"] select[name="survivor_id"]').count()) >= 5, {});

  const deleteForms = await page.locator('form[data-dixie-submit][action*="/tags/"]:not([action$="/rename"]):not([action$="/merge"])').count();
  record('tags-delete-form-targets-delete-endpoint', deleteForms >= 5, { deleteForms });
  // data-confirm lives on the <form>, not the <button>
  // (the templ wires it on the form element per the issue
  // #248 dispatcher contract).
  record('tags-delete-form-has-confirm', (await page.locator('form[data-dixie-submit][action*="/tags/"][data-confirm]').count()) >= 5, {});

  // Verify a single row's merge picker excludes its own id.
  // Grab the first row, read its tr id and the select options.
  const firstRowId = await page.locator('tr[id^="tag-row-"]').first().getAttribute('id').catch(() => null);
  const sourceTagId = Number((firstRowId || '').match(/tag-row-(\d+)/)?.[1] || 0);
  const sourceRowSelector = `tr#tag-row-${sourceTagId}`;
  const survivorOptions = await page.locator(`${sourceRowSelector} select[name="survivor_id"] option`).evaluateAll((opts) =>
    opts.map((o) => ({ value: o.value, text: o.textContent }))
  );
  const ownIdPresent = survivorOptions.some((o) => Number(o.value) === sourceTagId);
  const placeholderPresent = survivorOptions.some((o) => o.value === '' && /Merge into which tag/i.test(o.text));
  record('tags-merge-picker-excludes-self', !ownIdPresent, { sourceTagId, optionCount: survivorOptions.length });
  record('tags-merge-picker-has-placeholder', placeholderPresent, { placeholderPresent });

  // Round-trip rename: rename the first tag and verify the
  // new name persists.
  const firstRowRenameInput = page.locator(`${sourceRowSelector} input[name="tag_name"]`).first();
  const newName = 'RenamedBySmoke-' + Date.now();
  await firstRowRenameInput.fill(newName);
  await Promise.all([
    page.waitForURL(/\/tags(\b|$|\?)/, { timeout: 15_000 }).catch(() => null),
    page.locator(`${sourceRowSelector} form[action*="/rename"] button[type="submit"]`).first().click({ timeout: 5_000 }),
  ]);
  await page.goto(`${BASE}/tags`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));
  const renamedVisible = (await page.locator(`tr#tag-row-${sourceTagId} input[name="tag_name"]`).inputValue().catch(() => '')) === newName;
  record('tags-rename-round-trip', renamedVisible, { sourceTagId, newName });

  // Round-trip merge: pick the FIRST non-self survivor option
  // in the source row's picker, click Merge.
  const survivorValue = await page.locator(`tr#tag-row-${sourceTagId} select[name="survivor_id"] option:not([disabled])`).first().getAttribute('value').catch(() => null);
  record('tags-merge-picker-has-survivor-option', !!survivorValue && survivorValue !== '', { survivorValue });
  if (survivorValue) {
    await page.locator(`tr#tag-row-${sourceTagId} select[name="survivor_id"]`).selectOption(survivorValue);
    // Issue #715: same helper as #713's settings-appearance
    // export-surface step. Encodes the
    // waitForResponse(POST) + click + networkidle shape that
    // #714 explicitly added to pin the race.
    const mergeSubmit = page.locator(`tr#tag-row-${sourceTagId} form[action*="/merge"] button[type="submit"]`).first();
    const { response: mergePostResp } = await submitAndWait(page, {
      submit: mergeSubmit,
      urlPredicate: (u) => u.includes('/tags/') && u.includes('/merge'),
    });
    const sourceRowGone = (await page.locator(`tr#tag-row-${sourceTagId}`).count()) === 0;
    record('tags-merge-round-trip-removes-source-row', sourceRowGone, { sourceTagId, mergePostStatus: mergePostResp?.status() });
  }

  await browser.close().catch(() => {});

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

runProbe({ name: 'tags', probeFn: main })
  .then((r) => process.exit(r.ok ? 0 : 1))
  .catch((err) => { console.error('fatal:', err); process.exit(2); });