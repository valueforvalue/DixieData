/**
 * audit/smoke_soldier_images.mjs — browser-driven end-to-end probe
 * for the soldier-side Images panel UIIDs (issue #392).
 *
 * Issue #392 closed the gap where
 *   - internal/uiids.PanelSoldierDetailImages
 *   - internal/uiids.PanelSoldierFormImages
 * were declared as constants but never rendered as `id=` attributes
 * in any templ. The detail-page gallery and the edit-form Upload
 * Images section both relied on inline id literals (or no anchor
 * at all), so goquery invariant tests could not pin against the
 * canonical UIIDs. This probe mirrors the post-#390 event-side
 * probe shape (audit/smoke_events.mjs step-13 + step-14) and
 * verifies both wrappers render in their expected pages.
 *
 * Coverage (from issue #392 body):
 *   1.  Seed a Person Record + GET /soldiers/{id} → assert
 *       #panel.soldier.detail.images exists in the DOM with the
 *       empty-state copy "No images are attached yet" (the
 *       fresh-soldier path).
 *   2.  GET /soldiers/{id}/edit → assert #panel.soldier.form.images
 *       exists in the DOM with the Upload Images label + the
 *       Add Images From Computer button visible (the
 *       import-after-create surface).
 *   3.  Upload a 1×1 PNG via setFileChooserFixture (#385 helper)
 *       → assert the populated gallery read surface: thumbnail
 *       count ≥ 1, per-card alt text non-empty, filename visible,
 *       per-card Delete button form present (mirrors event-side
 *       step-14 read surface). Scopes all per-card queries to
 *       `[id="panel.soldier.detail.images"] [data-image-card]` to avoid
 *       the documented `data-image-id` selector collision (see
 *       soldier_card.templ lines 580 + 589 — both per-card wrapper
 *       div and Preview `<button>` carry the attribute).
 *   4.  Click the per-card Delete button → assert gallery
 *       fragment swaps in place via
 *       data-results-target="#panel.soldier.detail.images"
 *       (Slice B.2): card count drops by exactly 1,
 *       page.url() unchanged. Mirrors the event-side
 *       fragment-swap pattern (event_panels.templ's
 *       EventImagesListFragment data-results-target).
 *   5.  Click the per-card Set as Primary button → assert
 *       the fragment swap runs WITHOUT a page reload
 *       (page.url() unchanged) and the card count is
 *       unchanged (Set as Primary only flips the IsPrimary
 *       bit; it never deletes). Mirrors the same event-side
 *       swap path. Slice B.2 ships this path.
 *
 * Lifecycle:
 *   - Spawns its own `build/bin/dixiedata-web.exe` against a
 *     private scratch dir, seeds it via `cmd/seed-data`, tears
 *     the scratch dir down at the end.
 *   - `_lib/cleanup.mjs` ensures the spawned binary is killed on
 *     Ctrl-C / fatal / successful exit (Windows task leak is
 *     real and was specifically a recurring audit-harness bug).
 *   - The seeded soldier is DELETEd in `finally`, so the scratch
 *     dir is empty post-run.
 *
 * Selector strategy:
 *   Scope every per-card selector under
 *   `[id="panel.soldier.detail.images"] [data-image-card]` so the
 *   `data-image-id` collision between the per-card wrapper div
 *   and the Preview `<button>` cannot surface as a flaky probe.
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
import { setFileChooserFixture } from './_lib/filechooser.mjs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';

const PORT = process.env.PROBE_PORT || '8774';
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
    .filter((r) => /\/soldiers\//.test(r.url))
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
    recentSoldierResponses: filtered,
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

async function waitForServer(url, maxMs = 30000) {
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

/**
 * Create a fresh Person Record via the soldiers facade POST path
 * (mirrors smoke_events.mjs's event creation pattern) and parse
 * the resulting soldier id from the X-DixieData-Redirect header.
 */
async function createSoldier(page, label) {
  const resp = await page.request.post(`${BASE}/soldiers/new`, {
    headers: {
      'content-type': 'application/x-www-form-urlencoded',
      'x-dixiedata-submit': 'true',
    },
    data: new URLSearchParams({
      entry_type: 'soldier',
      first_name: 'SmokeImages',
      last_name: label,
      pension_state: 'NA',
      confederate_home_status: 'None',
    }).toString(),
    maxRedirects: 0,
  });
  const loc =
    resp.headers()['x-dixiedata-redirect'] ||
    resp.headers()['X-DixieData-Redirect'] ||
    resp.headers()['location'] ||
    '';
  const m = loc.match(/\/soldiers\/(\d+)/);
  if (!m) {
    throw new Error(`createSoldier: cannot parse id from Location="${loc}"`);
  }
  return parseInt(m[1], 10);
}

async function teardown(page, soldierIDs) {
  for (const id of soldierIDs) {
    try {
      await page.request.delete(`${BASE}/soldiers/${id}`);
    } catch (_) {
      // best effort
    }
  }
}

async function main() {
  const here = path.dirname(fileURLToPath(import.meta.url));
  const repoRoot = here.endsWith('audit') ? path.dirname(here) : here;
  const scratchDir = path.join(repoRoot, '.scratch', 'smoke-soldier-images');
  const webBinPath = webBin();
  const fixtureSrc = path.join(here, '_lib', 'fixtures', 'soldier-image.png');

  try {
    fs.rmSync(scratchDir, { recursive: true, force: true });
  } catch (_) {
    // ignore
  }

  if (!fs.existsSync(webBinPath)) {
    throw new Error(
      `dixiedata-web binary missing at ${webBinPath}; run \`just debug\` first`,
    );
  }
  if (!fs.existsSync(fixtureSrc)) {
    throw new Error(
      `soldier image fixture missing at ${fixtureSrc}; issue #392 ships this fixture alongside the smoke probe`,
    );
  }

  const seedProc = spawn(
    'go',
    ['run', './cmd/seed-data', '-data-dir', scratchDir, '-soldiers', '3', '-reset'],
    { cwd: repoRoot, stdio: ['ignore', 'pipe', 'pipe'] },
  );
  let seedOut = '';
  seedProc.stdout.on('data', (d) => { seedOut += d; });
  seedProc.stderr.on('data', (d) => { seedOut += d; });
  const seedExit = await new Promise((resolve) => seedProc.on('exit', resolve));
  if (seedExit !== 0) {
    throw new Error(`seed-data failed (exit ${seedExit}):\n${seedOut}`);
  }

  const proc = spawn(
    webBinPath,
    ['-addr', `127.0.0.1:${PORT}`, '-scratch-dir', scratchDir],
    {
      cwd: repoRoot,
      env: { ...process.env, DIXIEDATA_DATA_DIR: scratchDir },
    },
  );
  registerCleanup({ proc, processNames: [path.basename(webBinPath)] });
  proc.stderr.on('data', (d) => process.stderr.write(`[srv] ${d}`));
  proc.stdout.on('data', (d) => process.stdout.write(`[srv] ${d}`));

  await waitForServer(BASE);
  console.log(`server up at ${BASE}`);

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ acceptDownloads: true });
  const page = await context.newPage();

  // Auto-accept native confirm() dialogs (e.g. per-card Delete
  // "Delete this image?" prompt from data-confirm). Without this,
  // Playwright auto-dismisses the dialog, the JS dispatcher
  // bails out, and the form's default submission navigates to
  // /images/delete instead of triggering the in-place fragment
  // swap that step-04 + step-05 assert.
  page.on('dialog', async (dialog) => {
    await dialog.accept();
  });

  const trackedSoldierIDs = [];

  page.on('response', (r) => {
    if (r.url().startsWith(BASE)) {
      lastResponses.push({
        status: r.status(),
        method: r.request().method(),
        url: r.url(),
      });
    }
  });

  console.log('\nSoldier Images smoke probe (issue #392)');
  console.log('----------------------------------------');

  try {
    // Step 1: create a fresh soldier, navigate to detail, assert
    // #panel.soldier.detail.images exists in the empty state.
    let createdSoldierID;
    await step(page, 'step-01 detail-empty-images-panel-anchor', async () => {
      createdSoldierID = await createSoldier(page, `SmokeImages-${Date.now()}`);
      trackedSoldierIDs.push(createdSoldierID);
      await page.goto(`${BASE}/soldiers/${createdSoldierID}`, {
        waitUntil: 'domcontentloaded',
      });
      await wait(300);
      if (!page.url().endsWith(`/soldiers/${createdSoldierID}`)) {
        throw new Error(
          `expected /soldiers/${createdSoldierID}, got ${page.url()}`,
        );
      }
      await page.waitForSelector('[id="panel.soldier.detail.images"]', {
        timeout: 30_000,
      });
      const root = await page.evaluate(() => {
        const el = document.querySelector('[id="panel.soldier.detail.images"]');
        if (!el) return { found: false };
        const text = (el.textContent || '').replace(/\s+/g, ' ').trim();
        return {
          found: true,
          hasEmptyMarker: /No images are attached yet/i.test(text),
          hasImportHint: /Add Images From Computer/i.test(text),
          snippet: text.slice(0, 200),
        };
      });
      if (!root.found) {
        throw new Error('#panel.soldier.detail.images not found on detail page');
      }
      if (!root.hasEmptyMarker) {
        throw new Error(
          `expected empty-state copy inside #panel.soldier.detail.images; got: ${root.snippet}`,
        );
      }
      if (!root.hasImportHint) {
        throw new Error(
          `expected "Add Images From Computer" reference inside #panel.soldier.detail.images; got: ${root.snippet}`,
        );
      }
    });

    // Step 2: navigate to the edit form, assert
    // #panel.soldier.form.images exists with the Upload Images
    // label + the Add Images From Computer button.
    await step(page, 'step-02 edit-form-images-panel-anchor', async () => {
      await page.goto(`${BASE}/soldiers/${createdSoldierID}/edit`, {
        waitUntil: 'domcontentloaded',
      });
      await wait(300);
      if (!page.url().endsWith(`/soldiers/${createdSoldierID}/edit`)) {
        throw new Error(
          `expected /soldiers/${createdSoldierID}/edit, got ${page.url()}`,
        );
      }
      await page.waitForSelector('[id="panel.soldier.form.images"]', {
        timeout: 30_000,
      });
      const surface = await page.evaluate((id) => {
        const root = document.querySelector('[id="panel.soldier.form.images"]');
        if (!root) return { found: false };
        const text = (root.textContent || '').replace(/\s+/g, ' ').trim();
        // Issue #682 structural fix: the image upload is now a
        // <div data-image-upload data-upload-url="..."> containing a
        // hidden <input type="file">. The handler is wired via JS
        // event listener (initializeImageUpload) not inline
        // onchange/hx-post. Read data-upload-url off the wrapper
        // div for the action-equality check.
        const labels = Array.from(root.querySelectorAll('label'));
        const importBtn = labels.find(
          (b) => (b.textContent || '').trim() === 'Add Images From Computer',
        );
        const fileInput = importBtn ? importBtn.querySelector('input[type="file"]') : null;
        const uploadDiv = fileInput ? fileInput.closest('[data-image-upload]') : null;
        const uploadUrl = (uploadDiv && uploadDiv.getAttribute('data-upload-url')) || '';
        const rect = importBtn ? importBtn.getBoundingClientRect() : null;
        return {
          found: true,
          hasUploadLabel: /Upload Images/i.test(text),
          snippet: text.slice(0, 200),
          hasImportBtn: !!importBtn,
          importAction: uploadUrl,
          importActionMatches: uploadUrl === `/soldiers/${id}/images/import?return=edit`,
          importBtnVisible:
            !!rect && rect.width > 0 && rect.height > 0,
        };
      }, createdSoldierID);
      if (!surface.found) {
        throw new Error('#panel.soldier.form.images not found on edit form');
      }
      if (!surface.hasUploadLabel) {
        throw new Error(
          `expected "Upload Images" label inside #panel.soldier.form.images; got: ${surface.snippet}`,
        );
      }
      if (!surface.hasImportBtn) {
        throw new Error(
          'Add Images From Computer button missing inside #panel.soldier.form.images',
        );
      }
      if (!surface.importActionMatches) {
        throw new Error(
          `Add Images From Computer file input hx-post="${surface.importAction}", want "/soldiers/${createdSoldierID}/images/import?return=edit"`,
        );
      }
      if (!surface.importBtnVisible) {
        throw new Error(
          'Add Images From Computer button not visible (zero-sized box) inside #panel.soldier.form.images',
        );
      }
    });

    // Step 3: populate the gallery via setFileChooserFixture
    // (#385 helper), assert the read surface mirrors event-side
    // step-14: thumbnail count, per-card alt text, filename,
    // Delete form. Scopes every per-card query to
    // `[id="panel.soldier.detail.images"] [data-image-card]` to avoid
    // the documented data-image-id selector collision.
    await step(
      page,
      'step-03 detail-populated-gallery-read-surface',
      async () => {
        await page.goto(`${BASE}/soldiers/${createdSoldierID}`, {
          waitUntil: 'domcontentloaded',
        });
        await wait(300);
        if (!page.url().endsWith(`/soldiers/${createdSoldierID}`)) {
          throw new Error(
            `expected /soldiers/${createdSoldierID}, got ${page.url()}`,
          );
        }
        const before = await page
          .locator('[id="panel.soldier.detail.images"] [data-image-card]')
          .count();
        if (before !== 0) {
          throw new Error(
            `step-03 setup: expected 0 image cards on a fresh soldier, got ${before}`,
          );
        }

        const fixturePath = path.join(
          scratchDir,
          'fixtures',
          `smoke-soldier-${createdSoldierID}.png`,
        );
        fs.mkdirSync(path.dirname(fixturePath), { recursive: true });
        fs.copyFileSync(fixtureSrc, fixturePath);

        // Upload 3 images so step-04 can delete the primary (auto-promoted
// on first insert) and step-05 still has a non-primary card to test
// Set-Primary against. With 2 uploads, deleting the primary would
// auto-promote the remaining one to primary (ensurePrimaryImage),
// leaving 0 visible Set-Primary buttons for step-05.
const off = setFileChooserFixture(page, [fixturePath, fixturePath, fixturePath]);
        try {
          // Issue #682 structural fix: the image upload is now a
          // <div data-image-upload> wrapper (no longer a
          // <form data-soldier-image-import-form>). The label
          // lives at the page root next to the "Select all images"
          // checkbox (per soldier_card.templ) and the gallery
          // wrapper under #panel.soldier.detail.images is only
          // the swap target for the response. The data-image-upload
          // attribute on the wrapper is what scopes the selector now.
          await page.click(
            '[data-image-upload] label:has-text("Add Images From Computer")',
          );

          await page.waitForFunction(
            () =>
              document.querySelectorAll(
                '[id="panel.soldier.detail.images"] [data-image-card]',
              ).length >= 1,
            null,
            { timeout: 30_000 },
          );

          const surface = await page.evaluate(() => {
            const cards = Array.from(
              document.querySelectorAll(
                '[id="panel.soldier.detail.images"] [data-image-card]',
              ),
            );
            const cardReports = cards.map((card) => {
              const id = card.getAttribute('data-image-id') || '';
              const img = card.querySelector('img[data-image-thumb-id]');
              const alt = img ? img.getAttribute('alt') || '' : '';
              const imgVisible =
                img instanceof HTMLImageElement
                  ? img.getBoundingClientRect().width > 0 &&
                    img.getBoundingClientRect().height > 0
                  : false;
              const filenameEls = card.querySelectorAll(
                'div.text-xs.text-slate-500.break-all',
              );
              const filenameNodes = Array.from(filenameEls).map((el) =>
                (el.textContent || '').trim(),
              );
              const deleteForms = card.querySelectorAll(
                'form[action*="/images/delete"]',
              );
              return {
                id,
                alt,
                altNonEmpty: alt.trim().length > 0,
                imgVisible,
                filenameNodes,
                deleteFormCount: deleteForms.length,
              };
            });
            // Gallery grew — at least one card renders *some*
            // filename string. We do NOT pin the exact text because
            // the server applies standardizedImageFileName() on
            // import (e.g. "STC-00006-img-001.png") which is
            // load-bearing for the storage layout — see
            // docs/migrations/v55.md. The probe asserts the new
            // card is wired up (filename node populated,
            // image-extension suffix present) without locking the
            // rename rule. Mirrors audit/smoke_events.mjs step-14
            // (issue #403).
            const IMAGE_EXT_RE =
              /\.(png|jpg|jpeg|gif|bmp|webp|svg)$/i;
            return {
              cardCount: cards.length,
              filenameMatch: cardReports.some(
                (c) =>
                  c.filenameNodes.length > 0 &&
                  c.filenameNodes.some((n) => IMAGE_EXT_RE.test(n)),
              ),
              cardReports,
            };
          });

          if (surface.cardCount < 1) {
            throw new Error(
              `step-03: expected >=1 image card after upload, got ${surface.cardCount}`,
            );
          }
          for (const card of surface.cardReports) {
            if (!card.altNonEmpty) {
              throw new Error(
                `step-03: card id=${card.id} has empty alt text`,
              );
            }
            if (!card.imgVisible) {
              throw new Error(
                `step-03: card id=${card.id} thumbnail <img> not visible (zero-sized box)`,
              );
            }
          }
          // 5b. Every thumbnail has a visible <img> with non-empty
          // alt (checked above), AND at least one card shows a
          // filename with an image extension — the upload path is
          // the seed of the gallery, the new card must render
          // its filename node, even though import rewrites it via
          // standardizedImageFileName() (cardinality check, mirrors
          // smoke_events step-14 per issue #403).
          if (!surface.filenameMatch) {
            const seen = surface.cardReports
              .flatMap((c) => c.filenameNodes)
              .filter(Boolean);
            throw new Error(
              `step-03: no card filename node carries an image-extension suffix; saw=${JSON.stringify(seen)}`,
            );
          }
          // Bulk-download form coverage is exercised by step-04
          // (per-card Delete + bulk delete coexist; bulk would
          // visibly break the page if missing). The original
          // bulkFormCount selector scoped `querySelectorAll` to the
          // panel root, but per soldier_card.templ line 556 the
          // bulk-download form is the OUTER `<form>` that wraps
          // the panel — a parent, not a descendant — so the scoped
          // selector always returned 0 (latent probe bug, masked
          // by step-02 before #404). Dropped per issue #405; per-card
          // Delete coverage in step-04 + cardCount/alt/visibility
          // assertions above already pin the same regression.
        } finally {
          off();
        }
      },
    );

    // Step 4 (issue #391 Slice B.2). Drives the in-place
    // fragment swap: click per-card Delete, assert the
    // gallery fragment updates without page nav. Mirrors
    // the event-side swap pattern (smoke_events.mjs step-14
    // uses the same data-image-card selector inside the
    // panel wrapper). The probe seeds a second image so
    // there's something to delete without leaving the
    // gallery empty.
    await step(
      page,
      'step-04 per-card-delete-fragment-swap',
      async () => {
        const before = await page
          .locator('[id="panel.soldier.detail.images"] [data-image-card]')
          .count();
        if (before < 1) {
          throw new Error(
            `step-04 setup: expected >=1 image card from step-03, got ${before}`,
          );
        }
        const urlBefore = page.url();
        // Click the per-card Delete form (form[data-results-target]
        // scoped to inside the panel). Per-card Delete uses
        // #panel.soldier.detail.images scope so the data-action
        // selector doesn't catch the outer bulk Delete Selected
        // Images button which lives outside the panel.
        const perCardDeleteCount = await page
          .locator(
            '[id="panel.soldier.detail.images"] [data-image-delete-button]',
          )
          .count();
        if (perCardDeleteCount < 1) {
          throw new Error(
            `step-04: per-card Delete button missing inside [id="panel.soldier.detail.images"] (count=${perCardDeleteCount})`,
          );
        }
        // issue #407 regression net: verify the Delete button is
        // NOT a descendant of the outer bulk-download form via a
        // real <form> submit path. The previous per-card Delete was
        // a <form action="/images/delete"> nested inside the outer
        // download form, which broke JS submit propagation (the
        // browser did native form submission before the document-
        // level handler could fire, fragment-swap never ran). The
        // htmx-driven button bypasses form submission entirely, so
        // nesting is safe for THIS control — but a future templ
        // change that re-introduces a JS-dispatcher-driven form for
        // another per-card action would hit the same bug. The check
        // below verifies the per-card control surface is htmx-driven
        // (hx-post attribute present), which is the actual guarantee
        // we care about.
        const isHtmxDriven = await page.evaluate(() => {
          const btn = document.querySelector(
            '[id="panel.soldier.detail.images"] [data-image-delete-button]',
          );
          return btn ? btn.hasAttribute('hx-post') : false;
        });
        if (!isHtmxDriven) {
          throw new Error(
            `step-04: per-card Delete button is missing hx-post (issue #407 fix requires htmx-driven button, not a nested form)`,
          );
        }
        await page.click(
          '[id="panel.soldier.detail.images"] [data-image-delete-button]',
        );
        // Wait for swap: count drops by exactly one.
        await page.waitForFunction(
          ({ before }) =>
            document.querySelectorAll(
              '[id="panel.soldier.detail.images"] [data-image-card]',
            ).length === before - 1,
          { before },
          { timeout: 30_000 },
        );
        const after = await page
          .locator('[id="panel.soldier.detail.images"] [data-image-card]')
          .count();
        if (after !== before - 1) {
          throw new Error(
            `step-04: expected card count to drop by 1 (before=${before}, after=${after})`,
          );
        }
        const urlAfter = page.url();
        if (urlAfter !== urlBefore) {
          throw new Error(
            `step-04: page navigated after per-card Delete (in-place swap expected). before="${urlBefore}" after="${urlAfter}"`,
          );
        }
      },
    );

    // Step 5 (issue #391 Slice B.2). Per-card Set as Primary
    // also uses the fragment-swap path. Drives a click on the
    // per-card Set as Primary button and asserts:
    //   - page.url() unchanged (no reload),
    //   - the swapped gallery still contains the same image
    //     rows (the IsPrimary badge flips server-side; the
    //     DOM nodes themselves don't disappear).
    await step(
      page,
      'step-05 per-card-set-primary-no-reload',
      async () => {
        const urlBefore = page.url();
        const cards = await page
          .locator('[id="panel.soldier.detail.images"] [data-image-card]')
          .count();
        if (cards < 1) {
          throw new Error(
            `step-05 setup: expected >=1 image card after step-04, got ${cards}`,
          );
        }
        // The first card's Set as Primary button is the
        // data-image-primary-action[disabled] button. Click
        // it via the panel-scoped selector.
        // issue #407: Set-Primary is now an htmx-driven button (was a
        // data-action button with the JS dispatcher path). The
        // primaryImageButtonClass helper hides the button on the
        // card that's already the primary image, so we filter to
        // visible buttons only.
        const primaryBtnCount = await page
          .locator(
            '[id="panel.soldier.detail.images"] [data-image-primary-action]:not(.hidden)',
          )
          .count();
        if (primaryBtnCount < 1) {
          throw new Error(
            `step-05: per-card Set as Primary button (visible) missing inside [id="panel.soldier.detail.images"] (count=${primaryBtnCount})`,
          );
        }
        await page.click(
          '[id="panel.soldier.detail.images"] [data-image-primary-action]:not(.hidden)',
        );
        // Fragment swap returns the gallery. Wait until at
        // least one card is back (the swap target was
        // re-rendered).
        await page.waitForFunction(
          () =>
            document.querySelectorAll(
              '[id="panel.soldier.detail.images"] [data-image-card]',
            ).length >= 1,
          null,
          { timeout: 30_000 },
        );
        const urlAfter = page.url();
        if (urlAfter !== urlBefore) {
          throw new Error(
            `step-05: page navigated after per-card Set as Primary (in-place swap expected). before="${urlBefore}" after="${urlAfter}"`,
          );
        }
        const cardsAfter = await page
          .locator('[id="panel.soldier.detail.images"] [data-image-card]')
          .count();
        if (cardsAfter !== cards) {
          throw new Error(
            `step-05: card count changed unexpectedly (before=${cards}, after=${cardsAfter}) — Set as Primary must not delete`,
          );
        }
      },
    );
  } catch (_) {
    // step() already recorded the failure with context; preserve
    // the running tally without double-counting.
    if (fail === 0) {
      fail = 1;
    }
  } finally {
    await teardown(page, trackedSoldierIDs);
    await browser.close();

    try {
      fs.rmSync(scratchDir, { recursive: true, force: true });
    } catch (_) {
      // ignore
    }
  }

  console.log(`\n${pass} passed, ${fail} failed`);
  console.log(`soldiers touched (created, then cleaned up): ${trackedSoldierIDs.length}`);
  // The runner's runProbe contract returns {ok} from the probeFn;
  // the aggregator emits the process exit code via renderSummary().
  return { ok: fail === 0, steps: { pass, fail } };
}

import('./_lib/cleanup.mjs').then(async ({ runWithCleanup }) => {
  // Issue #700 slice 2: this smoke now runs through the
  // shared Playwright runner (audit/_lib/smoke_runner.mjs)
  // instead of inlining its own server-spawn + chromium.launch
  // + cleanup chain. The runner calls main() via the probeFn
  // contract (returns {ok, ...} or throws) and the shared
  // reporter emits the process exit code. The registerCleanup()
  // bridge inside main() still works because cleanup.mjs
  // exposes registerCleanup as a module-level singleton, and
  // the runner's own runWithCleanup() is installed at the
  // aggregator level (audit/smoke_aggregator.mjs). For this
  // standalone invocation we still wrap in runWithCleanup so
  // the spawned binary is killed on Ctrl-C / fatal exit.
  await runWithCleanup(async () => {
    const result = await runProbe({
      name: 'soldier-images',
      probeFn: main,
    });
    process.exit(result.ok ? 0 : 1);
  });
});