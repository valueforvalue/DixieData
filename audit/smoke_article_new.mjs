/**
 * audit/smoke_article_new.mjs — /articles/new surface probe (issue #700).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir,
 * seeds 3 soldiers, then asserts:
 *   1. /articles/new renders the form, title + body inputs,
 *      and Save button.
 *   2. The form action is /articles/new (the canonical
 *      ArticleNew routebuilder URL).
 *   3. Filling title + body + clicking Save posts the form,
 *      server creates the article, response redirects to
 *      /articles/{id}, and the detail page shows the saved
 *      title.
 *   4. /articles/{id} renders the new article body via the
 *      goldmark+bluemonday pipeline.
 *   5. The /articles/{id}/pdf button + /articles/{id}/delete
 *      form anchor exist on the detail page.
 *
 * Run: `node audit/smoke_article_new.mjs`
 * Exits 0 on all-pass, 1 on any-fail, 2 on fatal.
 */
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';

const PORT = process.env.PROBE_PORT || '8780';
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

  await page.goto(`${BASE}/articles/new`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));

  const formAction = await page.locator('form[data-dixie-submit]').first().getAttribute('action');
  record('article-new-form-targets-articles-new', formAction === '/articles/new', { formAction });
  record('article-new-form-has-title-input', (await page.locator('input[name="title"]').count()) >= 1, {});
  record('article-new-form-has-body-input', (await page.locator('textarea[name="body"]').count()) >= 1, {});
  record('article-new-form-has-save-button', (await page.locator('form[data-dixie-submit] button[type="submit"]').count()) >= 1, {});

  const uniqueTitle = 'SmokeArticle-' + Date.now();
  await page.locator('input[name="title"]').fill(uniqueTitle);
  await page.locator('textarea[name="body"]').fill('First paragraph of the smoke article body.\n\n## Subheading\n\n* bullet one\n* bullet two');
  await Promise.all([
    page.waitForURL(/\/articles\/\d+/, { timeout: 10_000 }).catch(() => null),
    page.locator('form[data-dixie-submit] button[type="submit"]').first().click(),
  ]);
  const detailUrl = page.url();
  const articleId = Number((detailUrl.match(/\/articles\/(\d+)/) || [])[1]);
  record('article-new-save-redirects-to-detail', Number.isFinite(articleId) && articleId > 0, { detailUrl, articleId });

  await new Promise((r) => setTimeout(r, 400));
  const renderedTitle = await page.locator('h1').first().textContent();
  record('article-detail-renders-saved-title', (renderedTitle || '').includes(uniqueTitle), { renderedTitle });

  const bodyParagraph = await page.locator('[data-article-body] p').first().textContent().catch(() => '');
  record('article-detail-renders-goldmark-body', (bodyParagraph || '').includes('First paragraph'), { bodyParagraph });

  const pdfAction = await page.locator('form[data-dixie-submit][action*="/articles/"][action$="/pdf"]').first().getAttribute('action').catch(() => null);
  record('article-detail-has-pdf-export-form', !!pdfAction, { pdfAction });
  const deleteAction = await page.locator('form[data-dixie-submit][action*="/articles/"][action$="/delete"]').first().getAttribute('action').catch(() => null);
  record('article-detail-has-delete-form', !!deleteAction, { deleteAction });

  await browser.close().catch(() => {});

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

runProbe({ name: 'article-new', probeFn: main })
  .then((r) => process.exit(r.ok ? 0 : 1))
  .catch((err) => { console.error('fatal:', err); process.exit(2); });
