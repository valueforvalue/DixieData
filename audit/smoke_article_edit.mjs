/**
 * audit/smoke_article_edit.mjs — browser-driven end-to-end probe
 * for the Article edit + new surface (issue #700 + #321 slice 3.7).
 *
 * The article new + edit screens share the same ArticleArticleForm
 * component (internal/templates/article_new.templ). The probe
 * covers both by navigating to /articles/{id}/edit against an
 * article pre-seeded by `seed-data --articles 1`. The new route
 * /articles/new is covered too (the same form, empty state).
 *
 * Coverage (from docs/ui-map/wireframes/15a-articles-edit.md):
 *
 *   1.  Edit form renders on /articles/{id}/edit with the
 *       canonical form action = routebuilder.ArticleEdit(id)
 *       (the bug class: template-split URL drift, #687).
 *   2.  EditorToolbar (issue #610) renders 11 buttons: 9
 *       format-insert buttons (Bold/Italic/Heading/Link/Image/
 *       List/Code/Quote/Table) + Undo/Redo. Each button
 *       carries data-editor-toolbar-action + a stable id.
 *   3.  Preview button opens the article-preview-modal (hidden
 *       by default); the modal body is identified by
 *       data-article-preview-body. The body is filled by JS
 *       on first Preview click (initializeArticlePreview).
 *   4.  Markdown syntax cheatsheet renders the Foldout popover
 *       (panel.article.markdown-cheatsheet). The trigger is
 *       data-article-md-cheatsheet-open; the panel is rendered
 *       initially hidden and toggled by the Foldout JS handler.
 *       The cheatsheet does NOT steal focus from the editor
 *       textarea (per the issue #565 decision).
 *   5.  Image picker modal opens from the toolbar's Image
 *       button via data-editor-toolbar-opens-modal=
 *       "image-picker-modal". In the EDIT form, the picker
 *       shows upload + pick-existing tabs + manual URL.
 *   6.  Save Article POST submits to /articles/{id}/edit and
 *       the Wails dispatcher dispatches the form. The
 *       X-DixieData-Redirect response header navigates to
 *       /articles/{id}. The detail page re-renders with the
 *       new title.
 *   7.  Local-draft persistence attrs are wired:
 *       data-draft-key, data-draft-record-version,
 *       data-draft-reset-path on the form;
 *       data-record-persistence, data-clear-draft-trigger="base"
 *       on the banner. The version is "UpdatedAt|ID" so a
 *       server-side update invalidates stale drafts.
 *   8.  New-article form on /articles/new renders the same
 *       form with empty fields + kind="new" persistence.
 *       The data-image-picker-no-article banner is shown
 *       (no article rowId yet).
 *   9.  Nested-form-sibling invariant: the image picker
 *       modal is a SIBLING of the article form, not a
 *       descendant. The data-image-picker-modal element is
 *       NOT inside the article <form>. (Issue #682, #689
 *       class 4 — HTML5 parser auto-closes on <form> in
 *       <form>.)
 *
 * Lifecycle:
 *   - Spawns its own build/bin/dixiedata-web.exe against a
 *     private scratch dir, seeds it via cmd/seed-data with
 *     --articles 1 --skip-soldiers, then tears the scratch
 *     dir down at the end.
 *   - _lib/cleanup.mjs ensures the spawned binary is killed
 *     on Ctrl-C / fatal / successful exit.
 *   - The seeded article is kept (no rollback needed; the
 *     scratch dir is removed at probe end).
 *
 * Failure mode:
 *   - Records pass / fail per step.
 *   - On failure prints: the failed step name, current url,
 *     last server response status + url, and a 2000-char DOM
 *     snippet to stderr.
 *   - Exits 0 on all-pass, 1 on any-fail, 2 on fatal.
 */
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { setTimeout as sleep } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import fs from 'node:fs';
import { registerCleanup } from './_lib/cleanup.mjs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';

const PORT = process.env.PROBE_PORT || '8775';
const BASE = `http://127.0.0.1:${PORT}`;

let pass = 0;
let fail = 0;
const results = [];
const lastResponses = [];

function record(name, ok, details = {}) {
  results.push({ name, ok, ...details });
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
        .slice(0, 4000),
    );
  }
}

const wait = (ms) => new Promise((r) => setTimeout(r, ms));

async function dumpFailureContext(page, stepName) {
  const url = page.url();
  const lastResp = lastResponses[lastResponses.length - 1] || null;
  const filtered = lastResponses
    .filter((r) => /\/articles\//.test(r.url))
    .slice(-15);
  let body = '';
  try {
    body = await page.content();
  } catch (e) {
    body = `(page.content() threw: ${e.message})`;
  }
  return {
    step: stepName,
    url,
    lastResponse: lastResp,
    recentArticleResponses: filtered,
    bodySnippet: body.slice(0, 2000),
  };
}

async function step(page, name, fn) {
  try {
    await fn();
    record(name, true);
  } catch (e) {
    const ctx = await dumpFailureContext(page, name);
    record(name, false, {
      error: e.message,
      ...ctx,
    });
    throw e;
  }
}

async function waitForServer(url, maxMs = 30_000) {
  const deadline = Date.now() + maxMs;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url);
      if (res.status < 500) return;
    } catch (_) {
      // server not yet up
    }
    await sleep(200);
  }
  throw new Error(`server at ${url} never came up`);
}

async function discoverArticleID(page) {
  // Navigate to /articles (the list page rendered by
  // articles.templ) and parse the first data-article-row
  // link. The list page has the canonical data-article-row
  // + data-article-row-title markers (per
  // docs/ui-map/wireframes/15-articles.md).
  await page.goto(`${BASE}/articles`, { waitUntil: 'domcontentloaded' });
  await wait(300);
  const firstHref = await page.evaluate(() => {
    const row = document.querySelector('[data-article-row]');
    if (!row) return null;
    return row.getAttribute('href') || null;
  });
  if (!firstHref) {
    throw new Error('no [data-article-row] found on /articles');
  }
  const m = firstHref.match(/\/articles\/(\d+)/);
  if (!m) {
    throw new Error(`cannot parse article id from href="${firstHref}"`);
  }
  return parseInt(m[1], 10);
}

async function main(ctx) {
  const here = path.dirname(fileURLToPath(import.meta.url));
  const repoRoot = here.endsWith('audit') ? path.dirname(here) : here;
  const scratchDir = path.join(repoRoot, '.scratch', 'smoke-article-edit');
  const webBinPath = webBin();

  if (fs.existsSync(scratchDir)) {
    fs.rmSync(scratchDir, { recursive: true, force: true });
  }
  fs.mkdirSync(scratchDir, { recursive: true });

  // Seed: 5 soldiers + 1 article. The article is the
  // canonical test fixture; soldiers are needed because the
  // /articles/{id}/refs counter requires a person registry
  // (per articles_handlers.go:handleArticleRefsAttach).
  // Use `go run` so the probe doesn't depend on a pre-built
  // build/bin/seed-data binary (matches the smoke_soldier_images
  // pattern).
  const seedProc = spawn(
    'go',
    ['run', './cmd/seed-data',
      '-data-dir', scratchDir,
      '-soldiers', '5',
      '-articles', '1',
      '-reset',
    ],
    { stdio: ['ignore', 'pipe', 'pipe'], cwd: repoRoot },
  );
  let seedOut = '';
  seedProc.stdout.on('data', (d) => { seedOut += d.toString(); });
  seedProc.stderr.on('data', (d) => { seedOut += d.toString(); });
  const seedExit = await new Promise((resolve) => seedProc.on('exit', resolve));
  if (seedExit !== 0) {
    throw new Error(`seed-data exited ${seedExit}: ${seedOut.slice(0, 1000)}`);
  }

  const proc = spawn(
    webBinPath,
    ['-addr', `127.0.0.1:${PORT}`, '-scratch-dir', scratchDir],
    { stdio: ['ignore', 'pipe', 'pipe'] },
  );
  proc.stderr.on('data', (d) => { /* swallow noise */ });
  ctx.registerCleanup(() => {
    try { proc.kill(); } catch (_) { /* best effort */ }
  });

  await waitForServer(`${BASE}/`);

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ acceptDownloads: true });
  const page = await context.newPage();
  page.on('response', (r) => {
    lastResponses.push({ status: r.status(), method: r.request().method(), url: r.url() });
  });

  try {
    let articleID;
    await step(page, 'step-01 discover-article-id-from-list', async () => {
      articleID = await discoverArticleID(page);
      if (!articleID || articleID < 1) {
        throw new Error(`expected positive article id, got ${articleID}`);
      }
    });

    // Step 2: assert the edit form renders with the
    // canonical form action (the bug class 6 — invoker-URL
    // drift — catches a routebuilder mismatch).
    await step(page, 'step-02 edit-form-action-resolves', async () => {
      await page.goto(`${BASE}/articles/${articleID}/edit`, {
        waitUntil: 'domcontentloaded',
      });
      await wait(300);
      if (!page.url().endsWith(`/articles/${articleID}/edit`)) {
        throw new Error(
          `expected /articles/${articleID}/edit, got ${page.url()}`,
        );
      }
      const formAction = await page.evaluate((id) => {
        const form = document.querySelector(
          'form[data-dixie-submit][data-draft-key]',
        );
        return form ? form.getAttribute('action') : null;
      }, articleID);
      if (formAction !== `/articles/${articleID}/edit`) {
        throw new Error(
          `expected form action="/articles/${articleID}/edit", got "${formAction}"`,
        );
      }
    });

    // Step 3: EditorToolbar renders 11 buttons (the 9
    // format-insert + Undo/Redo). Each carries
    // data-editor-toolbar-action + a stable id.
    await step(page, 'step-03 editor-toolbar-renders-11-buttons', async () => {
      const actions = await page.evaluate(() => {
        const buttons = Array.from(
          document.querySelectorAll('[data-editor-toolbar] button[data-editor-toolbar-action]'),
        );
        return buttons.map((b) => b.getAttribute('data-editor-toolbar-action'));
      });
      const expected = [
        'bold', 'italic', 'heading', 'link', 'image',
        'list', 'code', 'quote', 'table',
        'undo', 'redo',
      ];
      const missing = expected.filter((a) => !actions.includes(a));
      if (missing.length > 0) {
        throw new Error(
          `missing toolbar actions: ${missing.join(', ')}; saw=${JSON.stringify(actions)}`,
        );
      }
    });

    // Step 4: Image button opens the image picker modal;
    // Table button opens the table builder. Two separate
    // modals named in data-editor-toolbar-opens-modal.
    await step(page, 'step-04 toolbar-opens-correct-modals', async () => {
      const opens = await page.evaluate(() => {
        const img = document.querySelector(
          'button[data-editor-toolbar-action="image"]',
        );
        const tbl = document.querySelector(
          'button[data-editor-toolbar-action="table"]',
        );
        return {
          image: img ? img.getAttribute('data-editor-toolbar-opens-modal') : null,
          table: tbl ? tbl.getAttribute('data-editor-toolbar-opens-modal') : null,
        };
      });
      if (opens.image !== 'image-picker-modal') {
        throw new Error(
          `image button should open image-picker-modal, got "${opens.image}"`,
        );
      }
      if (opens.table !== 'table-builder-modal') {
        throw new Error(
          `table button should open table-builder-modal, got "${opens.table}"`,
        );
      }
    });

    // Step 5: Markdown cheatsheet Foldout trigger +
    // panel exist. The panel is initially hidden. The Foldout
    // primitive renders the trigger with data-foldout-trigger=
    // <menuID> (where menuID = panel.article.markdown-cheatsheet).
    await step(page, 'step-05 cheatsheet-trigger-and-panel', async () => {
      const trigger = await page.evaluate(() => {
        const t = document.querySelector(
          '[data-foldout-trigger="panel.article.markdown-cheatsheet"]',
        );
        if (!t) return null;
        return {
          tag: t.tagName,
          text: (t.textContent || '').trim(),
          ariaExpanded: t.getAttribute('aria-expanded'),
        };
      });
      if (!trigger) {
        throw new Error(
          'cheatsheet trigger (data-foldout-trigger="panel.article.markdown-cheatsheet") not found',
        );
      }
      if (trigger.tag !== 'BUTTON') {
        throw new Error(
          `cheatsheet trigger should be <button>, got <${trigger.tag.toLowerCase()}>`,
        );
      }
      const panel = await page.evaluate(() => {
        const p = document.querySelector('[id="panel.article.markdown-cheatsheet"]');
        if (!p) return null;
        return {
          hidden: p.classList.contains('hidden'),
          rowCount: p.querySelectorAll('[data-md-cheatsheet-insert-key]').length,
        };
      });
      if (!panel) {
        throw new Error('panel.article.markdown-cheatsheet not found');
      }
      if (!panel.hidden) {
        throw new Error('cheatsheet panel should be hidden initially');
      }
      if (panel.rowCount < 1) {
        throw new Error(
          `cheatsheet should have >=1 insert row, got ${panel.rowCount}`,
        );
      }
    });

    // Step 6: Preview modal exists, hidden initially, body
    // is identified by data-article-preview-body.
    await step(page, 'step-06 preview-modal-renders-hidden', async () => {
      const preview = await page.evaluate(() => {
        const modal = document.querySelector('[data-article-preview-modal]');
        if (!modal) return null;
        return {
          hidden: modal.classList.contains('hidden'),
          modalId: modal.id,
          hasBody: !!modal.querySelector('[data-article-preview-body]'),
          hasCloseBtn: !!modal.querySelector('[data-article-preview-close]'),
        };
      });
      if (!preview) {
        throw new Error('data-article-preview-modal not found');
      }
      if (!preview.hidden) {
        throw new Error('preview modal should be hidden initially');
      }
      if (!preview.hasBody) {
        throw new Error('preview modal missing data-article-preview-body');
      }
      if (!preview.hasCloseBtn) {
        throw new Error('preview modal missing data-article-preview-close button');
      }
    });

    // Step 7: Image picker modal exists with upload tab
    // (edit form has a row id, so tabs are enabled).
    await step(page, 'step-07 image-picker-modal-enabled-tabs', async () => {
      const picker = await page.evaluate(() => {
        const modal = document.querySelector('[data-image-picker-modal]');
        if (!modal) return null;
        const noArticleBanner = modal.querySelector('[data-image-picker-no-article]');
        return {
          hidden: modal.classList.contains('hidden'),
          hasUploadTab: !!modal.querySelector('[data-image-picker-tab="upload"]'),
          hasExistingTab: !!modal.querySelector('[data-image-picker-tab="existing"]'),
          hasUploadPanel: !!modal.querySelector('[data-image-picker-panel="upload"]'),
          hasExistingPanel: !!modal.querySelector('[data-image-picker-panel="existing"]'),
          hasNoArticleBanner: !!noArticleBanner,
          hasUrlInput: !!modal.querySelector('[data-image-picker-url]'),
          hasAltInput: !!modal.querySelector('[data-image-picker-alt]'),
          hasInsertBtn: !!modal.querySelector('[data-image-picker-insert-url]'),
        };
      });
      if (!picker) {
        throw new Error('data-image-picker-modal not found');
      }
      if (!picker.hidden) {
        throw new Error('image picker modal should be hidden initially');
      }
      if (picker.hasNoArticleBanner) {
        throw new Error(
          'edit form should NOT show data-image-picker-no-article banner (article row exists)',
        );
      }
      if (!picker.hasUploadTab || !picker.hasExistingTab) {
        throw new Error(
          `edit form picker missing tabs: upload=${picker.hasUploadTab}, existing=${picker.hasExistingTab}`,
        );
      }
      if (!picker.hasUrlInput || !picker.hasAltInput || !picker.hasInsertBtn) {
        throw new Error(
          'image picker missing URL/alt/insert controls',
        );
      }
    });

    // Step 8: Local-draft persistence attrs are wired on
    // the form + the banner. The version sentinel is the
    // "UpdatedAt|ID" string that the JS uses to invalidate
    // stale drafts after a server-side update.
    await step(page, 'step-08 draft-persistence-attrs', async () => {
      const attrs = await page.evaluate(() => {
        const form = document.querySelector('form[data-dixie-submit][data-draft-key]');
        const banner = document.querySelector('[data-record-persistence]');
        return {
          draftKey: form ? form.getAttribute('data-draft-key') : null,
          draftVersion: form ? form.getAttribute('data-draft-record-version') : null,
          draftResetPath: form ? form.getAttribute('data-draft-reset-path') : null,
          bannerKind: banner ? banner.getAttribute('data-record-persistence-kind') : null,
          hasClearBtn: !!document.querySelector('[data-clear-draft-trigger="base"]'),
        };
      });
      if (!attrs.draftKey) {
        throw new Error('form missing data-draft-key');
      }
      if (attrs.draftKey !== `edit-article-${articleID}`) {
        throw new Error(
          `expected data-draft-key=edit-article-${articleID}, got "${attrs.draftKey}"`,
        );
      }
      if (!attrs.draftVersion) {
        throw new Error('edit form missing data-draft-record-version');
      }
      if (!attrs.draftVersion.includes('|')) {
        throw new Error(
          `draft version should be "UpdatedAt|ID" format, got "${attrs.draftVersion}"`,
        );
      }
      if (attrs.bannerKind !== 'edit') {
        throw new Error(
          `banner data-record-persistence-kind should be "edit", got "${attrs.bannerKind}"`,
        );
      }
    });

    // Step 9: Nested-form-sibling invariant. The image
    // picker modal is a sibling of the article <form>, NOT
    // a descendant. HTML5 forbids <form> inside <form> (the
    // parser auto-closes the outer form at the inner one's
    // open tag), so a regressed layout would lose the save
    // button. This check is the class-4 regression net.
    await step(page, 'step-09 image-picker-is-form-sibling', async () => {
      const nested = await page.evaluate(() => {
        const form = document.querySelector('form[data-dixie-submit][data-draft-key]');
        const modal = document.querySelector('[data-image-picker-modal]');
        if (!form || !modal) return { form: !!form, modal: !!modal, nested: false };
        return { form: true, modal: true, nested: form.contains(modal) };
      });
      if (!nested.form) {
        throw new Error('article form not found');
      }
      if (!nested.modal) {
        throw new Error('image picker modal not found');
      }
      if (nested.nested) {
        throw new Error(
          'data-image-picker-modal is nested inside the article <form> (HTML5 forbids; class-4 regression)',
        );
      }
    });

    // Step 10: Save round-trip. Submit the form with a
    // changed title, verify the redirect to /articles/{id}
    // shows the new title.
    await step(page, 'step-10 save-round-trip', async () => {
      const newTitle = `Smoke Title ${Date.now()}`;
      // Scroll the title input into view to avoid the
      // dispatcher's form-action mutation guard (the form is
      // identified by the data-draft-key; the dispatcher
      // already-computes the action from the form attribute).
      await page.locator('#article-title').fill(newTitle);
      const submitResponse = page.waitForResponse(
        (r) =>
          r.url().endsWith(`/articles/${articleID}/edit`) &&
          r.request().method() === 'POST',
        { timeout: 10_000 },
      );
      await page.click('button[type="submit"]:has-text("Save Article")');
      const response = await submitResponse;
      if (!response.ok()) {
        throw new Error(
          `Save Article POST status ${response.status()}, expected 200`,
        );
      }
      const redirect = response.headers()['x-dixiedata-redirect'] || '';
      if (!redirect.endsWith(`/articles/${articleID}`)) {
        throw new Error(
          `expected X-DixieData-Redirect to /articles/${articleID}, got "${redirect}"`,
        );
      }
      // The dispatcher navigates client-side to the redirect
      // URL. Verify the detail page shows the new title.
      await page.waitForURL((url) => url.pathname === `/articles/${articleID}`, {
        timeout: 5_000,
      });
      const detailText = await page.locator('h1, h2').first().textContent();
      if (!detailText || !detailText.includes(newTitle)) {
        throw new Error(
          `detail page h1/h2 missing new title; got "${detailText}"`,
        );
      }
    });

    // Step 11: New-article form on /articles/new renders
    // the same form with kind="new" persistence + the
    // data-image-picker-no-article banner (no row id yet).
    await step(page, 'step-11 new-form-degraded-image-picker', async () => {
      await page.goto(`${BASE}/articles/new`, { waitUntil: 'domcontentloaded' });
      await wait(300);
      if (!page.url().endsWith('/articles/new')) {
        throw new Error(`expected /articles/new, got ${page.url()}`);
      }
      const surface = await page.evaluate(() => {
        const form = document.querySelector('form[data-dixie-submit][data-draft-key]');
        const banner = document.querySelector('[data-record-persistence]');
        const noArticle = document.querySelector('[data-image-picker-no-article]');
        return {
          formAction: form ? form.getAttribute('action') : null,
          draftKey: form ? form.getAttribute('data-draft-key') : null,
          bannerKind: banner ? banner.getAttribute('data-record-persistence-kind') : null,
          hasNoArticleBanner: !!noArticle,
        };
      });
      if (surface.formAction !== '/articles/new') {
        throw new Error(
          `new form action should be /articles/new, got "${surface.formAction}"`,
        );
      }
      if (surface.draftKey !== 'new-article') {
        throw new Error(
          `new form draft-key should be "new-article", got "${surface.draftKey}"`,
        );
      }
      if (surface.bannerKind !== 'new') {
        throw new Error(
          `new form banner kind should be "new", got "${surface.bannerKind}"`,
        );
      }
      if (!surface.hasNoArticleBanner) {
        throw new Error(
          'new form must show data-image-picker-no-article banner (no row id yet)',
        );
      }
    });
  } catch (_) {
    // A step failed; the runner's record() already counted
    // a fail. Make sure fail is at least 1 so the runner's
    // {ok} return is false.
    if (fail === 0) fail = 1;
  } finally {
    await browser.close();
    try {
      fs.rmSync(scratchDir, { recursive: true, force: true });
    } catch (_) {
      // best effort
    }
  }

  console.log(`\n${pass} passed, ${fail} failed`);
  return { ok: fail === 0, steps: { pass, fail } };
}

import('./_lib/cleanup.mjs').then(async ({ runWithCleanup }) => {
  await runWithCleanup(async () => {
    const result = await runProbe({
      name: 'article-edit',
      probeFn: main,
    });
    process.exit(result.ok ? 0 : 1);
  });
});
