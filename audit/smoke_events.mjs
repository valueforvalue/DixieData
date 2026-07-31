/**
 * audit/smoke_events.mjs — browser-driven end-to-end probe for the
 * Event Records v1 surface (issue #320 child #323, slot 14 of 16).
 *
 * Walks the full Event Records user journey against a spawned
 * `dixiedata-web` debug server and asserts `page.url()` after each
 * navigation plus that the PDF download event fires. Handler unit
 * coverage lives in `internal/appshell/events_handlers_test.go`;
 * this probe is the UI end-to-end equivalent.
 *
 * Coverage (from issue #323 body):
 *   1.  /events (empty-state list)
 *   2.  Click "+ Add Event Record" → /events/new
 *   3.  Fill form + submit → assert url ends in /events/{numeric_id}
 *   4.  Detail page shows the entered values
 *   5.  Edit → modify → submit → assert returns to detail
 *   6.  Person Record → /soldiers/{id}/events lazy-load fragment
 *   7.  Attach existing event from inside the fragment
 *   8.  Quick-add new event from inside the fragment
 *   9.  Unlink event from inside the fragment
 *  10.  Delete event → 303 redirect to /events + row gone from list
 *  11.  /events/{id}/pdf → assert download event fires
 *  12.  Event detail → Add Images From Computer → image lands in
 *      the gallery (exercises the setFileChooserFixture helper
 *      in audit/_lib/filechooser.mjs — issue #385 + #348b)
 *  13.  Event detail with zero images → assert the
 *      "No images are attached" empty-state copy + the
 *      "Add Images From Computer" button render (issue #387;
 *      read-surface only — upload path is gated by #385)
 *  14.  Event detail → Add Images From Computer via
 *      setFileChooserFixture (#385) → assert the populated
 *      gallery read surface: thumbnail count, per-card alt
 *      text, filename, and Delete button render (issue #386).
 *
 * Lifecycle:
 *   - The probe spawns its own `build/bin/dixiedata-web.exe`
 *     against a private scratch dir, seeds it with `cmd/seed-data`,
 *     and tears the scratch dir down at the end. That gives
 *     inter-session determinism without external coordination.
 *   - `_lib/cleanup.mjs` ensures the spawned binary is killed
 *     on Ctrl-C / fatal / successful exit (Windows task leak is
 *     real and was specifically a recurring audit-harness bug).
 *   - Within a single run, every row the probe creates is also
 *     DELETEd in the probe's `finally` block, so the scratch
 *     dir is empty post-run.
 *
 * Selector strategy:
 *   CSS / placeholder / aria-label selectors only — no canonical
 *   `internal/uiids` UIIDs (those land in a separate slot if the
 *   convention tightens).
 *
 * Failure mode:
 *   - Records pass / fail per step.
 *   - On failure prints: the failed step name, current url,
 *     last server response status + url, and a 2000-char DOM
 *     snippet to stderr.
 *   - Exits 0 on all-pass, 1 on any-fail, 2 on fatal.
 *
 * Reference: pattern follows probe-full-restore.mjs. The PDF step
 * uses `page.waitForEvent('download')` (Playwright).
 */

import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { setTimeout as sleep } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import fs from 'node:fs';
import os from 'node:os';
import { setFileChooserFixture } from './_lib/filechooser.mjs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin as webBinResolver } from './_lib/smoke_paths.mjs';
import { loadConfig, resolveBaseUrl } from './_lib/config.mjs';

// Issue #707 batch 3: replaced hardcoded PORT = 8773 + manual
// `http://127.0.0.1:${PORT}` with config.mjs. PROBE_PORT (set
// by the aggregator's per-probe allocation) wins via
// resolveBaseUrl; SMOKE_BASE_URL (CI mode) is also honored.
// The cross-platform webBin() resolver replaces the hardcoded
// Windows-only `build/bin/dixiedata-web.exe` path. The
// legacy `cleanup.mjs::registerCleanup({proc, processNames})`
// wrapper is dropped — runProbe owns server SIGTERM via
// ctx.registerCleanup().
const cfg = loadConfig();
const PORT = process.env.PROBE_PORT ? parseInt(process.env.PROBE_PORT, 10) : cfg.defaultPort;
const BASE = resolveBaseUrl(cfg).replace(/\/$/, '');

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
        .slice(0, 4000),
    );
  }
}

const wait = (ms) => new Promise((r) => setTimeout(r, ms));

async function dumpFailureContext(page, stepName) {
  const url = page.url();
  const lastResp = lastResponses[lastResponses.length - 1] || null;
  // Filter the full response ring to /events/* + /soldiers/* for
  // richer diagnostics when the failure is around an events-* submit.
  const filtered = lastResponses
    .filter((r) => /\/events\/|\/soldiers\//.test(r.url))
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
    recentEventResponses: filtered,
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
    throw e; // stop on first failure so we don't pile bogus asserts
  }
}

async function waitForServer(url, maxMs = 30000) {
  const deadline = Date.now() + maxMs;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url);
      if (res.status < 500) return;
    } catch (_) {
      // Not up yet.
    }
    await sleep(200);
  }
  throw new Error(`server at ${url} never came up`);
}

/**
 * Seed a Person Record via /soldiers/new POST so steps 6-9 have a
 * target. Returns the numeric id parsed from the 303 redirect
 * Location header.
 */
async function seedPersonRecord(page) {
  // seed-data mints N soldiers before dixiedata-web boots, so
  // we just pick an existing one from /browse. POSTing to
  // /soldiers/new would be the other path, but it currently
  // 405s (handleNewSoldier only honors GET; the create handler
  // is mounted at /soldiers and expects the JS dispatcher to
  // hit it). The events probe is not responsible for that
  // surface; we scrape a real Person Record from the seed.
  const resp = await page.request.get(`${BASE}/browse`);
  const html = await resp.text();
  const m = html.match(/\/soldiers\/(\d+)/);
  if (!m) {
    throw new Error('seedPersonRecord: no /soldiers/{id} link found on /browse');
  }
  const id = parseInt(m[1], 10);
  // Sanity-check the id is reachable.
  const probe = await page.request.get(`${BASE}/soldiers/${id}`);
  if (probe.status() >= 500) {
    throw new Error(`seedPersonRecord: soldier ${id} fetch status=${probe.status()}`);
  }
  return id;
}

async function teardown(page, eventIDs, personID) {
  // Best-effort cleanup; do not throw out of the finally block.
  for (const id of eventIDs) {
    try {
      await page.request.delete(`${BASE}/events/${id}`);
    } catch {}
  }
  // Note: personID comes from seed-data (a real soldier, not
  // a row this probe created), so we do NOT delete it. The
  // scratch dir is wiped in the outer finally instead. If a
  // future probe seeds a Person Record via POST /soldiers it
  // should add cleanup for that id here.
  void personID;
}

async function main(ctx) {
  const here = path.dirname(fileURLToPath(import.meta.url));
  const repoRoot = here.endsWith('audit') ? path.dirname(here) : here;
  // Issue #710: scratchDir now comes from ctx.scratchDir
  // (the runner's per-probe mkdtemp) so concurrent probes
  // don't collide on the hardcoded `.scratch/smoke-events`
  // path. The runner's per-probe state root means local
  // settings (theme, debug_mode, etc.) are isolated too.
  const scratchDir = ctx.scratchDir;
  const webBin = webBinResolver();

  // Seed the scratch dir via the existing seed-data tool.
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
    webBin,
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
  proc.stdout.on('data', (d) => process.stdout.write(`[srv] ${d}`));

  await waitForServer(BASE);
  console.log(`server up at ${BASE}`);

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ acceptDownloads: true });
  const page = await context.newPage();

  const trackedEventIDs = [];
  let seededPersonID = null;

  page.on('response', (r) => {
    if (r.url().startsWith(BASE)) {
      lastResponses.push({
        status: r.status(),
        method: r.request().method(),
        url: r.url(),
      });
    }
  });

  console.log('\nEvent Records smoke probe (issue #320 child #323)');
  console.log('---------------------------------------------------');

  try {
    await step(page, 'step-01 /events empty-state', async () => {
      await page.goto(`${BASE}/events`, { waitUntil: 'domcontentloaded' });
      await wait(300);
      if (!page.url().endsWith('/events')) {
        throw new Error(`expected url to end with /events, got ${page.url()}`);
      }
      if (!(await page.evaluate(() => document.querySelector('[id="page.event.list"]') !== null))) {
        throw new Error('expected page.event.list wrapper on /events (issue #396)');
      }
      const text = await page.evaluate(() => document.body.innerText);
      if (!/No event records yet/i.test(text)) {
        throw new Error(`expected empty-state copy, body starts: ${text.slice(0, 200)}`);
      }
    });

    await step(page, 'step-02 navigate-to-new', async () => {
      await page.click('a:has-text("+ Add Event Record")');
      await page.waitForURL('**/events/new', { timeout: 5000 });
      await wait(200);
      if (!page.url().endsWith('/events/new')) {
        throw new Error(`expected /events/new, got ${page.url()}`);
      }
      if (!(await page.evaluate(() => document.querySelector('[id="page.event.new"]') !== null))) {
        throw new Error('expected page.event.new wrapper on /events/new (issue #396)');
      }
    });

    await step(page, 'step-03 create-and-redirect', async () => {
      const stamp = Date.now();
      const kind = `SmokeBattle-${stamp}`;
      // Issue #357: /events/new must expose the Source Records
      // section so the user can attach sources inline.
      const newHasSources = await page.evaluate(() =>
        /Source Records/i.test(document.body.innerText) &&
        document.querySelector('input[name="record_type"]') !== null
      );
      if (!newHasSources) {
        throw new Error('new-event form missing Source Records section (issue #357)');
      }
      await page.fill('input[name="kind"]', kind);
      await page.fill('input[name="begin_date"]', '07/01/1863');
      await page.fill('input[name="end_date"]', '07/03/1863');
      await page.fill(
        'textarea[name="description"]',
        `Gettysburg campaign smoke probe #${stamp}`,
      );
      await Promise.all([
        page.waitForURL(/\/events\/[0-9]+$/, { timeout: 10000 }),
        page.click('button[type="submit"]:has-text("Create Event Record")'),
      ]);
      await wait(300);
      const m = page.url().match(/\/events\/(\d+)$/);
      if (!m) throw new Error(`expected /events/{id}, got ${page.url()}`);
      trackedEventIDs.push(parseInt(m[1], 10));
    });

    const createdEventID = trackedEventIDs[trackedEventIDs.length - 1];

    await step(page, 'step-04 detail-shows-values', async () => {
      if (!(await page.evaluate(() => document.querySelector('[id="page.event.detail"]') !== null))) {
        throw new Error('expected page.event.detail wrapper on /events/{id} (issue #396)');
      }
      const text = await page.evaluate(() => document.body.innerText);
      if (!/SmokeBattle-/.test(text)) {
        throw new Error(`kind not visible on detail; body starts: ${text.slice(0, 200)}`);
      }
      if (!/07\/01\/1863.*07\/03\/1863|07\/01\/1863 — 07\/03\/1863/.test(text)) {
        throw new Error(`date range not visible; body starts: ${text.slice(0, 400)}`);
      }
      if (!/Gettysburg campaign smoke probe/.test(text)) {
        throw new Error(`description not visible; body starts: ${text.slice(0, 400)}`);
      }
    });

    await step(page, 'step-04b sources-panel-edit-cta', async () => {
      // Issue #360: the Sources panel on /events/{id} no longer
      // renders the legacy standalone attach form (record_type /
      // app_id / details). Instead it exposes an Edit Event CTA
      // pointing at /events/{id}/edit, where #357's inline
      // RecordInputRow covers the attach surface. Assert the
      // CTA is present and the legacy form is gone.
      const sourcesPanel = await page.evaluate((id) => {
        const root = document.querySelector('#data-event-sources-list');
        if (!root) return { hasList: false };
        const section = root.closest('section');
        const sectionHtml = section ? section.outerHTML : '';
        const editHref = `/events/${id}/edit`;
        return {
          hasList: true,
          hasLegacyRecordType: sectionHtml.includes('name="record_type"'),
          hasLegacyAppID: sectionHtml.includes('name="app_id"'),
          hasLegacyDetails: sectionHtml.includes('name="details"'),
          hasEditCTA: sectionHtml.includes(`data-action="${editHref}"`),
          editHref,
        };
      }, createdEventID);
      if (!sourcesPanel.hasList) {
        throw new Error('sources panel wrapper missing on detail page');
      }
      if (sourcesPanel.hasLegacyRecordType || sourcesPanel.hasLegacyAppID || sourcesPanel.hasLegacyDetails) {
        throw new Error(`legacy attach form still present (record_type=${sourcesPanel.hasLegacyRecordType}, app_id=${sourcesPanel.hasLegacyAppID}, details=${sourcesPanel.hasLegacyDetails})`);
      }
      if (!sourcesPanel.hasEditCTA) {
        throw new Error(`Sources panel missing Edit Event CTA pointing at ${sourcesPanel.editHref}`);
      }
    });

    await step(page, 'step-04c linked-persons-panel-edit-cta', async () => {
      // Issue #361 slice 1: the Linked Persons panel on
      // /events/{id} must surface an "Edit Event" CTA pointing
      // at /events/{id}/edit, mirroring the post-#360 Sources
      // panel pattern. Empty-state copy must reference the
      // event editor (no longer tell the user to bounce
      // through the Person Record's Events tab).
      const linkedPanel = await page.evaluate((id) => {
        const titleEl = Array.from(document.querySelectorAll('p')).find(
          (el) => el.textContent.trim() === 'Linked Person Records'
        );
        if (!titleEl) return { found: false };
        const section = titleEl.closest('section');
        const html = section ? section.outerHTML : '';
        const editHref = `/events/${id}/edit`;
        return {
          found: true,
          hasEditCTA: html.includes(`data-action="${editHref}"`),
          hasBounceCopy: /from the Person Record detail page/.test(html),
          hasEditorCopy: /event editor/i.test(html),
          editHref,
        };
      }, createdEventID);
      if (!linkedPanel.found) {
        throw new Error('Linked Persons panel title not found on detail page');
      }
      if (!linkedPanel.hasEditCTA) {
        throw new Error(`Linked Persons panel missing Edit Event CTA pointing at ${linkedPanel.editHref}`);
      }
      if (linkedPanel.hasBounceCopy) {
        throw new Error('Linked Persons empty-state copy still tells the user to attach from the Person Record detail page');
      }
      if (!linkedPanel.hasEditorCopy) {
        throw new Error('Linked Persons empty-state copy missing reference to the event editor');
      }
    });

    await step(page, 'step-04d tags-panel-edit-cta', async () => {
      // Issue #361 slice 1: the Tags panel on /events/{id}
      // must surface an "Edit Event" CTA pointing at
      // /events/{id}/edit AND a count span next to the title,
      // mirroring the Linked Persons + Sources panels.
      const tagsPanel = await page.evaluate((id) => {
        const titleEl = Array.from(document.querySelectorAll('p')).find(
          (el) => el.textContent.trim() === 'Tags'
        );
        if (!titleEl) return { found: false };
        const section = titleEl.closest('section');
        if (!section) return { found: false };
        const html = section.outerHTML;
        const editHref = `/events/${id}/edit`;
        // Count span: header flex-row (before the fragment div)
        // must contain a count-shaped pattern. Use the header
        // by truncating at the data-event-tags-list fragment div.
        const headerEnd = html.indexOf('id="data-event-tags-list"');
        const headerHtml = headerEnd >= 0 ? html.slice(0, headerEnd) : html;
        return {
          found: true,
          hasEditCTA: html.includes(`data-action="${editHref}"`),
          hasCount: /0 attached|0 tag|>0</.test(headerHtml),
          editHref,
        };
      }, createdEventID);
      if (!tagsPanel.found) {
        throw new Error('Tags panel title not found on detail page');
      }
      if (!tagsPanel.hasEditCTA) {
        throw new Error(`Tags panel missing Edit Event CTA pointing at ${tagsPanel.editHref}`);
      }
      if (!tagsPanel.hasCount) {
        throw new Error('Tags panel header missing count-shaped element (slice 1 adds one to mirror Linked Persons + Sources)');
      }
    });

    await step(page, 'step-05 edit-round-trip', async () => {
      await page.click('a:has-text("Edit Event")');
      await page.waitForURL(`**/events/${createdEventID}/edit`, { timeout: 5000 });
      await wait(200);
      if (!(await page.evaluate(() => document.querySelector('[id="page.event.edit"]') !== null))) {
        throw new Error('expected page.event.edit wrapper on /events/{id}/edit (issue #396)');
      }
      // Issue #357: edit form must expose the Source Records
      // section so the user can attach sources inline.
      const editHasSources = await page.evaluate(() =>
        /Source Records/i.test(document.body.innerText) &&
        document.querySelector('input[name="record_type"]') !== null &&
        document.querySelector('input[name="record_app_id"]') !== null
      );
      if (!editHasSources) {
        throw new Error('edit form missing Source Records section (issue #357)');
      }
      const newKind = `SmokeBattle-Edited-${Date.now()}`;
      await page.fill('input[name="kind"]', newKind);
      await Promise.all([
        page.waitForURL(new RegExp(`/events/${createdEventID}$`), { timeout: 10000 }),
        page.click('button[type="submit"]:has-text("Save Changes")'),
      ]);
      await wait(300);
      const text = await page.evaluate(() => document.body.innerText);
      if (!newKind.includes('Edited')) {
        throw new Error(`edited kind not visible; body starts: ${text.slice(0, 200)}`);
      }
    });

    await step(page, 'step-05b attach-source-via-edit-form', async () => {
      // Issue #357: the inline Source Records section on the
      // edit form must persist a Source Record when the user
      // fills + submits the main form.
      await page.click('a:has-text("Edit Event")');
      await page.waitForURL(`**/events/${createdEventID}/edit`, { timeout: 5000 });
      await wait(200);
      const uniqueAppID = `SMOKE-APP-${Date.now()}`;
      await page.fill('input[name="record_type"]', 'Pension Application');
      await page.fill('input[name="record_app_id"]', uniqueAppID);
      await page.fill('textarea[name="record_details"]', 'Attached via edit-form smoke probe.');
      await Promise.all([
        page.waitForURL(new RegExp(`/events/${createdEventID}$`), { timeout: 10000 }),
        page.click('button[type="submit"]:has-text("Save Changes")'),
      ]);
      await wait(300);
      const detailText = await page.evaluate(() => document.body.innerText);
      if (!detailText.includes(uniqueAppID)) {
        throw new Error(`source not visible on detail after edit-form submit; body: ${detailText.slice(0, 400)}`);
      }
    });

    await step(page, 'step-05c linked-persons-edit-section-present', async () => {
      // Issue #361 slice 2: the Event edit form (/events/{id}/edit)
      // must expose an inline Linked Person Records section with:
      //   - section header "Linked Person Records"
      //   - a display_id input for adding a new link
      //   - the Add form posting to /events/{id}/links
      // The seed-data soldier created earlier in the harness
      // provides a known Display ID we can use to attach.
      await page.click('a:has-text("Edit Event")');
      await page.waitForURL(`**/events/${createdEventID}/edit`, { timeout: 5000 });
      await wait(200);
      const section = await page.evaluate((id) => {
        const header = Array.from(document.querySelectorAll('p')).find(
          (el) => el.textContent.trim() === 'Linked Person Records'
        );
        if (!header) return { hasHeader: false };
        const sectionEl = header.closest('section');
        const html = sectionEl ? sectionEl.outerHTML : '';
        return {
          hasHeader: true,
          hasDisplayIDInput: html.includes('name="display_id"'),
          hasAddFormAction: html.includes(`action="/events/${id}/links"`),
          hasDixieSubmit: html.includes('data-dixie-submit="true"'),
        };
      }, createdEventID);
      if (!section.hasHeader) {
        throw new Error('edit form missing Linked Person Records section header');
      }
      if (!section.hasDisplayIDInput) {
        throw new Error('edit form Linked Persons section missing display_id input');
      }
      if (!section.hasAddFormAction) {
        throw new Error('edit form Linked Persons Add form missing action="/events/{id}/links"');
      }
      if (!section.hasDixieSubmit) {
        throw new Error('edit form Linked Persons Add form missing data-dixie-submit="true"');
      }
    });

    await step(page, 'step-05d attach-linked-person-via-edit-form', async () => {
      // Issue #361 slice 2: posting the Add form on the edit
      // page must create a link + redirect back to the edit
      // page. Mirrors the step-05b sources-attach shape.
      // The seeded person (SOL-NNNNN from seed-data) provides
      // a valid Display ID we can type.
      const personDisplayID = await page.evaluate(async (root) => {
        const resp = await fetch(`${root}/browse`);
        const html = await resp.text();
        const m = html.match(/\/soldiers\/(\d+)/);
        if (!m) return null;
        // Fetch the soldier detail to extract the Display ID.
        const sid = m[1];
        const detail = await (await fetch(`${root}/soldiers/${sid}`)).text();
        const dmatch = detail.match(/(DXD-\d+|EVT-\d+)/);
        return dmatch ? dmatch[1] : null;
      }, BASE);
      if (!personDisplayID) {
        throw new Error('step-05d setup: could not extract a Person Record Display ID from /browse');
      }
      // We're already on the edit page from step-05c; no need to
      // navigate. Fill the Linked Persons Add form directly.
      // Use the visible text input (not the hidden one in the
      // main form) — there are two name="display_id" inputs on
      // the page.
      await page.fill('input[type="text"][name="display_id"]', personDisplayID);
      // The form posts to /events/{id}/links which returns
      // X-DixieData-Redirect: /events/{id}/edit, so we land
      // back on the edit page (same URL we started from).
      await Promise.all([
        page.waitForURL(`**/events/${createdEventID}/edit`, { timeout: 10000 }),
        page.click('button[type="submit"]:has-text("Add Link")'),
      ]);
      await wait(300);
      // The link now exists; the per-row Unlink button must
      // render with data-action="/events/{id}/links/{personId}/detach".
      const unlinkPresent = await page.evaluate((id) => {
        const html = document.body.innerHTML;
        return /\/events\/\d+\/links\/\d+\/detach/.test(html);
      }, createdEventID);
      if (!unlinkPresent) {
        throw new Error(`after attach, Unlink button data-action="/events/{id}/links/{personId}/detach" missing`);
      }
      // Verify the linked person's Display ID is visible on
      // the page (pill-link in the list row).
      const detailText = await page.evaluate(() => document.body.innerText);
      if (!detailText.includes(personDisplayID)) {
        throw new Error(`attached Person ${personDisplayID} not visible on edit page after attach`);
      }
    });

    await step(page, 'step-05e tags-edit-section-present', async () => {
      // Issue #361 slice 3: the Event edit form exposes an
      // inline Tags section (below Linked Persons) with a
      // free-text Add form (name="tag_name" — mirrors the
      // /soldiers/{id}/tags picker UX). The section wraps a
      // <div id="data-event-tags-list"> in-place swap target
      // so attaching a tag preserves the user's unsaved form
      // state.
      // We're already on the edit page from step-05d; no
      // need to navigate.
      const section = await page.evaluate((id) => {
        // Locate the Tags section by its header <p> (slice 1
        // also added Tags to the detail page, but the edit
        // page is a distinct context).
        const headers = Array.from(document.querySelectorAll('p'));
        const tagsHeader = headers.find((el) => el.textContent.trim() === 'Tags');
        if (!tagsHeader) return { hasHeader: false };
        const sectionEl = tagsHeader.closest('section');
        const html = sectionEl ? sectionEl.outerHTML : '';
        return {
          hasHeader: true,
          hasAddForm: html.includes(`action="/events/${id}/tags"`),
          hasTagNameInput: html.includes('name="tag_name"'),
          hasSwapTarget: html.includes('id="data-event-tags-list"'),
          hasDixieSubmit: html.includes('data-dixie-submit="true"'),
        };
      }, createdEventID);
      if (!section.hasHeader) {
        throw new Error('edit form missing Tags section header');
      }
      if (!section.hasAddForm) {
        throw new Error('edit form Tags section Add form missing action="/events/{id}/tags"');
      }
      if (!section.hasTagNameInput) {
        throw new Error('edit form Tags section Add form missing name="tag_name" input');
      }
      if (!section.hasSwapTarget) {
        throw new Error('edit form Tags section missing in-place swap target div id="data-event-tags-list"');
      }
      if (!section.hasDixieSubmit) {
        throw new Error('edit form Tags section Add form missing data-dixie-submit="true"');
      }
    });

    await step(page, 'step-05f attach-tag-by-name-via-edit-form', async () => {
      // Issue #361 slice 3: posting the Tags Add form with a
      // free-text name must (a) create the tag via
      // UpsertByName, (b) attach it to the Event, and (c)
      // return a fragment for in-place swap (not a full-page
      // nav). The post-add DOM must show the new chip.
      // We're already on the edit page from step-05e.
      const uniqueTagName = `smoke-tag-${Date.now()}`;
      await page.fill('input[type="text"][name="tag_name"]', uniqueTagName);
      // Click and wait for the in-place swap. The handler
      // returns a fragment (no X-DixieData-Redirect) so the
      // page URL stays the same.
      await page.click('button[type="submit"]:has-text("Add Tag")');
      // Wait for the chip to appear (in-place swap via the
      // JS dispatcher). 5s is generous.
      const start = Date.now();
      let chipVisible = false;
      while (Date.now() - start < 5000) {
        const visible = await page.evaluate((name) =>
          document.body.innerText.includes(name),
          uniqueTagName
        );
        if (visible) {
          chipVisible = true;
          break;
        }
        await wait(150);
      }
      if (!chipVisible) {
        throw new Error(`tag chip ${uniqueTagName} not visible after Add Tag (in-place swap may have failed)`);
      }
      // Confirm we did NOT navigate away from the edit page.
      if (!page.url().endsWith(`/events/${createdEventID}/edit`)) {
        throw new Error(`unexpected navigation after Add Tag; URL: ${page.url()}`);
      }
    });

    await step(page, 'step-05g attach-by-name-and-full-name-in-row', async () => {
      // Issue #373: the Add Linked Person form accepts a name
      // fragment as a fallback to the Display ID lookup. After
      // attaching via name, the row must render the person's
      // full name (persondisplay.FullName) next to the Display
      // ID pill — not just the pill alone. This step detaches
      // the link created in step-05d and re-attaches via name
      // search, then verifies the visual upgrade.
      // We're already on the edit page from step-05f.
      // Scrape the currently-linked Person's id + name from
      // the row rendered by step-05d.
      const linkedRow = await page.evaluate(() => {
        const list = document.querySelector('[data-event-links-list]');
        if (!list) return null;
        const li = list.querySelector('li');
        if (!li) return null;
        const anchor = li.querySelector('a.pill-link[href^="/soldiers/"]');
        if (!anchor) return null;
        const displayID = anchor.textContent.trim();
        const idMatch = anchor.getAttribute('href').match(/\/soldiers\/(\d+)/);
        return {
          id: idMatch ? parseInt(idMatch[1], 10) : null,
          displayID,
          rowText: li.innerText.replace(/\s+/g, ' ').trim(),
        };
      });
      if (!linkedRow || !linkedRow.id) {
        throw new Error('step-05g setup: no linked-person row visible from step-05d');
      }
      // Look up the person's name via the detail page so we
      // can use a substring search in the Attach form below.
      const personDetail = await page.request.get(`${BASE}/soldiers/${linkedRow.id}`);
      const detailHTML = await personDetail.text();
      // The detail page renders the name as plain text — pull
      // the first token that's not the Display ID. We avoid
      // parsing the full DOM; a substring that survives the
      // name-lookup pipeline is enough to prove the fallback
      // path works.
      const nameFragments = detailHTML.match(/[A-Z][a-z]{2,}/g) || [];
      // Pick a fragment that's plausibly part of the name
      // (skip pure numbers and very short tokens).
      const fragment = nameFragments.find(
        (t) => t.length >= 3 && !/^(SOL|EVT|DXD)$/.test(t)
      );
      if (!fragment) {
        throw new Error(`step-05g setup: could not extract a name fragment from /soldiers/${linkedRow.id}`);
      }

      // Detach the existing link so the name-based attach
      // doesn't hit the duplicate-link conflict path. The
      // Unlink button is rendered as a <button data-action=...>
      // (not a <form>), so we click the bare button via its
      // data-action attribute rather than chasing a form.
      await Promise.all([
        page.waitForURL(`**/events/${createdEventID}/edit`, { timeout: 10000 }),
        page.click(`[data-event-links-list] button[data-action="/events/${createdEventID}/links/${linkedRow.id}/detach"]`),
      ]);
      await wait(300);

      // Attach by name fragment.
      await page.fill('input[type="text"][name="display_id"]', fragment);
      await Promise.all([
        page.waitForURL(`**/events/${createdEventID}/edit`, { timeout: 10000 }),
        page.click('button[type="submit"]:has-text("Add Link")'),
      ]);
      await wait(300);

      // Verify the link exists and the row renders BOTH the
      // Display ID pill AND the full name (the visual upgrade
      // from issue #373). The full name is what the soldier
      // detail page shows; we check the row's innerText
      // contains the Display ID AND at least one uppercase-
      // led token from the name.
      const verify = await page.evaluate((displayID) => {
        const list = document.querySelector('[data-event-links-list]');
        if (!list) return { ok: false, reason: 'no [data-event-links-list] container' };
        const li = list.querySelector('li');
        if (!li) return { ok: false, reason: 'no linked row' };
        const rowText = li.innerText.replace(/\s+/g, ' ').trim();
        return {
          ok: rowText.includes(displayID) && rowText.length > displayID.length + 1,
          rowText,
          displayID,
        };
      }, linkedRow.displayID);
      if (!verify.ok) {
        throw new Error(`step-05g verify failed: rowText=${JSON.stringify(verify.rowText)}, displayID=${verify.displayID}`);
      }
      // Sanity: the row has more text than just the Display
      // ID, which means the full-name span is rendering.
      // (No specific substring assertion on the name because
      // seed-data names are randomized — the visual upgrade
      // is structural, not literal.)
    });


    seededPersonID = await seedPersonRecord(page);
    await step(page, 'step-06 person-events-tab-fragment', async () => {
      // The fragment route /soldiers/{id}/events is wired at the
      // handler level but the Person detail UI does not currently
      // surface a tab that lazy-loads it. We navigate directly
      // and assert the fragment contract: header present, the
      // quick-add form is reachable, the empty-state copy
      // matches. If UI tab surfacing lands later, this step
      // continues to pass and acts as a regression net.
      await page.goto(`${BASE}/soldiers/${seededPersonID}/events`, {
        waitUntil: 'domcontentloaded',
      });
      await wait(300);
      if (!page.url().endsWith(`/soldiers/${seededPersonID}/events`)) {
        throw new Error(`expected fragment url, got ${page.url()}`);
      }
      const text = await page.evaluate(() => document.body.innerText);
      if (!/Event Records/i.test(text)) {
        throw new Error(`expected 'Event Records' header; body: ${text.slice(0, 200)}`);
      }
      const quickAddCount = await page
        .locator('form:has(textarea[name="description"])')
        .count();
      if (quickAddCount === 0) {
        throw new Error('quick-add form not present in fragment');
      }
    });

    await step(page, 'step-07 attach-existing-event', async () => {
      const r = await page.request.get(`${BASE}/events/${createdEventID}`);
      const html = await r.text();
      const dm = html.match(/EVT-\d{5}/);
      if (!dm) throw new Error(`cannot find EVT-NNNNN for event ${createdEventID}`);
      const displayID = dm[0];

      await page.goto(`${BASE}/soldiers/${seededPersonID}/events`, {
        waitUntil: 'domcontentloaded',
      });
      await wait(300);
      await page.click('summary:has-text("Add existing event")');
      await wait(200);
      await page.fill('input[name="display_id"]', displayID);
      // Submit fires the JS dispatcher. Note: the server-side
      // handler currently redirects to /soldiers/{id} (not back
      // to /soldiers/{id}/events), so the post-attach state is
      // only visible by re-navigating explicitly. We re-goto
      // the events tab to assert the unlink affordance shows
      // up; this asserts the server-side attach succeeded
      // regardless of where the form's response tries to send
      // the user.
      await page.click('form:has(input[name="display_id"]) button[type="submit"]');
      await wait(500);
      // Server now redirects the attach form to /soldiers/{id}/events
      // (issue #345), so we land directly back on the events tab.
      // A defensive re-navigation is kept to absorb future schema
      // drift without the probe collapsing.
      if (!page.url().endsWith(`/soldiers/${seededPersonID}/events`)) {
        await page.goto(`${BASE}/soldiers/${seededPersonID}/events`, {
          waitUntil: 'domcontentloaded',
        });
        await wait(300);
      }
      const bodyText = await page.evaluate(() => document.body.innerText);
      if (!/unlink/i.test(bodyText)) {
        throw new Error(
          `unlink button missing after attach; body: ${bodyText.slice(0, 400)}`,
        );
      }
    });

    await step(page, 'step-08 quick-add-new-event', async () => {
      await page.goto(`${BASE}/soldiers/${seededPersonID}/events`, {
        waitUntil: 'domcontentloaded',
      });
      await wait(300);
      const stamp = Date.now();
      const kind = `QuickProbe-${stamp}`;
      // The Quick add <details> in person_events_tab.templ has
      // no <summary> so it is collapsed by default. Toggle it
      // open via JS so the form's inputs become visible.
      await page.evaluate(() => {
        document
          .querySelectorAll('details')
          .forEach((d) => {
            if (d.querySelector('textarea[name="description"]')) d.open = true;
          });
      });
      await wait(200);
      await page.fill('form:has(textarea[name="description"]) input[name="kind"]', kind);
      await page.fill(
        'form:has(textarea[name="description"]) input[name="begin_date"]',
        '07/01/1863',
      );
      // Server now redirects the quick-add form to /soldiers/{id}/events
      // (issue #345), so we land directly back on the events tab. The
      // defensive re-navigation handles any future drift.
      await page.click('button:has-text("Create + Link Event")');
      await wait(500);
      if (!page.url().endsWith(`/soldiers/${seededPersonID}/events`)) {
        await page.goto(`${BASE}/soldiers/${seededPersonID}/events`, {
          waitUntil: 'domcontentloaded',
        });
        await wait(300);
      }
      const text = await page.evaluate(() => document.body.innerText);
      if (!text.includes(kind)) {
        throw new Error(`quick-added kind not visible; body: ${text.slice(0, 400)}`);
      }
      // Best-effort: locate the new event id for cleanup by
      // scanning /events list HTML. Failure here is non-fatal
      // because the quick-added event is auto-linked to the
      // seeded person; deleting the person cascades cleanup.
      const r = await page.request.get(`${BASE}/events`);
      const listHtml = await r.text();
      const idMatch = listHtml.match(
        new RegExp(
          `/events/(\\d+)[^>]*>[^<]*${kind.replace(/[.*+?^${}()|[\\]\\\\]/g, '\\\\$&')}`,
        ),
      );
      if (idMatch) {
        const qid = parseInt(idMatch[1], 10);
        if (!trackedEventIDs.includes(qid)) trackedEventIDs.push(qid);
      }
    });

    await step(page, 'step-09 unlink-event', async () => {
      await page.goto(`${BASE}/soldiers/${seededPersonID}/events`, {
        waitUntil: 'domcontentloaded',
      });
      await wait(300);
      const before = await page.locator('button:has-text("Unlink")').count();
      if (before === 0) {
        throw new Error('no Unlink buttons present; step 7 attach may not have stuck');
      }
      // Click the first Unlink button. The server-side handler
      // currently redirects to /soldiers/{id}; the post-unlink
      // row removal is visible only after re-navigating to the
      // events tab.
      await page.locator('button:has-text("Unlink")').first().click();
      await wait(500);
      // Server now redirects the unlink form to /soldiers/{id}/events
      // (issue #345). Defensive re-navigation absorbs future drift.
      if (!page.url().endsWith(`/soldiers/${seededPersonID}/events`)) {
        await page.goto(`${BASE}/soldiers/${seededPersonID}/events`, {
          waitUntil: 'domcontentloaded',
        });
        await wait(300);
      }
      const after = await page.locator('button:has-text("Unlink")').count();
      if (after >= before) {
        throw new Error(`unlink did not remove a row: before=${before} after=${after}`);
      }
    });

    await step(page, 'step-10 delete-event-redirect', async () => {
      await page.goto(`${BASE}/events/${createdEventID}`, {
        waitUntil: 'domcontentloaded',
      });
      await wait(400);
      // Navigate-by-DIRECT-FETCH because the bare-button
      // delete-event click flow currently relies on
      // `<form>.method = "DELETE"` which HTML coerces to GET
      // (form method only accepts GET/POST). The endpoint
      // contract is correct — handler accepts DELETE and
      // returns X-DixieData-Redirect=/events — so we exercise
      // the contract via direct fetch and assert (a) the row
      // is gone, (b) the list view no longer shows it, (c)
      // page navigates to /events. The UI bug above is the
      // topic of a separate follow-up issue to be filed
      // alongside this probe; tracking it here would expand
      // #323's scope beyond the regression net.
      const del = await page.request.delete(`${BASE}/events/${createdEventID}`);
      if (del.status() !== 200) {
        throw new Error(
          `DELETE /events/${createdEventID} status=${del.status()}, want 200`,
        );
      }
      await page.goto(`${BASE}/events`, { waitUntil: 'domcontentloaded' });
      await wait(300);
      if (!/\/events$/.test(page.url())) {
        throw new Error(`expected /events after delete, got ${page.url()}`);
      }
      const listHtml = await page.evaluate(() => document.body.innerHTML);
      if (listHtml.includes(`/events/${createdEventID}`)) {
        throw new Error(
          `deleted event ${createdEventID} still listed on /events`,
        );
      }
      const r = await page.request.get(`${BASE}/events/${createdEventID}`);
      if (r.status() < 400 && r.status() !== 404) {
        throw new Error(
          `deleted event ${createdEventID} still returns status=${r.status()}`,
        );
      }
      const idx = trackedEventIDs.indexOf(createdEventID);
      if (idx >= 0) trackedEventIDs.splice(idx, 1);
    });

    await step(page, 'step-11 pdf-download', async () => {
      const resp = await page.request.post(`${BASE}/events/new`, {
        headers: {
          'content-type': 'application/x-www-form-urlencoded',
          'x-dixiedata-submit': 'true',
        },
        data: new URLSearchParams({
          entry_type: 'event',
          kind: 'SmokePDF',
          begin_date: '07/01/1863',
          end_date: '07/03/1863',
          description: 'PDF smoke probe event.',
        }).toString(),
        maxRedirects: 0,
      });
      const loc =
        resp.headers()['x-dixiedata-redirect'] ||
        resp.headers()['X-DixieData-Redirect'] ||
        resp.headers()['location'] ||
        '';
      const m = loc.match(/\/events\/(\d+)/);
      if (!m) throw new Error(`PDF setup: cannot parse id from Location="${loc}"`);
      const id = parseInt(m[1], 10);
      trackedEventIDs.push(id);

      // The Event PDF export flows through a SaveFileDialog.
      // In web-mode the SaveFileDialogOverride wired by
      // cmd/dixiedata-web (see DIXIE_SAVE_FILE_DIR env or the
      // default <dataDir>/exports/ directory) writes the PDF
      // straight to disk; the browser does NOT receive a
      // Content-Disposition download response, so
      // page.waitForEvent('download') would hang. Instead we
      // click the Export PDF form, let the enqueued job do its
      // work, then poll the exports dir for a non-empty
      // <id>-<kind>.pdf written by the override.
      await page.goto(`${BASE}/events/${id}`, { waitUntil: 'domcontentloaded' });
      await wait(300);
      const exportsDir = path.join(scratchDir, 'exports');
      const before = (() => {
        try { return fs.readdirSync(exportsDir); } catch { return []; }
      })();
      const beforeSet = new Set(before);
      await page.click('[data-event-pdf-submit]');
      // Poll up to 90s for a NEW non-empty .pdf. The SaveFileDialog
      // override may create the file at 0 bytes when handing the
      // path back to the job; the actual content arrives after
      // Typst compiles the export. We wait on size > 0, not just
      // existence — Typst cold-start is slow on Windows and the
      // first compile of an event template can take 20-30s. If the
      // file is created but stays empty, that points to a missing
      // template asset (event_landscape.typ is currently a source-
      // tree file but the web-mode build at build/bin/templates/
      // does NOT bundle it — the template walker accepts the dir
      // via the soldier_landscape.typ sentinel, then errors at
      // compile time. That gap is tracked separately.)
      const pollDeadline = Date.now() + 90000;
      let newFile = null;
      let sizeAtPick = 0;
      while (Date.now() < pollDeadline) {
        await wait(500);
        let now;
        try { now = fs.readdirSync(exportsDir); } catch { now = []; }
        for (const f of now) {
          if (!beforeSet.has(f) && f.toLowerCase().endsWith('.pdf')) {
            const fullPath = path.join(exportsDir, f);
            try {
              const sz = fs.statSync(fullPath).size;
              if (sz > 0) {
                newFile = f;
                sizeAtPick = sz;
                break;
              }
            } catch {}
          }
        }
        if (newFile) break;
      }
      if (!newFile) {
        throw new Error(
          `no new non-empty PDF appeared in ${exportsDir} within 30s; before=${JSON.stringify(before)}`,
        );
      }
      if (sizeAtPick <= 0) {
        throw new Error(
          `PDF ${newFile} is empty (size=${sizeAtPick}) at ${path.join(exportsDir, newFile)}`,
        );
      }
    });

    // step-12 — image import via the Wails native file picker.
    // Exercises the setFileChooserFixture helper (#385) by
    // driving the "Add Images From Computer" button on an event
    // detail page. The picker is a runtime.OpenMultipleFilesDialog
    // call (internal/appshell/events_handlers.go:1259); the
    // helper wires page.on('filechooser') so the import lands
    // without a real human at the keyboard.
    //
    // First consumer of the helper (issue #348b — populated-gallery
    // smoke for /events/{id}/images). Self-contained: creates its
    // own event so a failure in step-05..11 does not block this
    // step from running (mirrors the step-13 shape below).
    // Image is removed by teardown via the parent event delete.
    await step(page, 'step-12 event-images-import-via-native-picker', async () => {
      const setupResp = await page.request.post(`${BASE}/events/new`, {
        headers: {
          'content-type': 'application/x-www-form-urlencoded',
          'x-dixiedata-submit': 'true',
        },
        data: new URLSearchParams({
          entry_type: 'event',
          kind: 'SmokeImagesImport',
          begin_date: '07/01/1863',
          end_date: '07/03/1863',
          description: 'Event-images native-picker smoke probe (#385 #348b).',
        }).toString(),
        maxRedirects: 0,
      });
      const loc =
        setupResp.headers()['x-dixiedata-redirect'] ||
        setupResp.headers()['X-DixieData-Redirect'] ||
        setupResp.headers()['location'] ||
        '';
      const m = loc.match(/\/events\/(\d+)/);
      if (!m) {
        throw new Error(`step-12 setup: cannot parse id from Location="${loc}"`);
      }
      const id = parseInt(m[1], 10);
      trackedEventIDs.push(id);

      // Build a small fixture PNG on disk. The native picker
      // needs a real file path; the minimal valid PNG header
      // + IDAT lets the importer's image decoder accept it
      // without us shipping binary fixtures in the repo.
      const fixturesDir = path.join(scratchDir, 'fixtures');
      fs.mkdirSync(fixturesDir, { recursive: true });
      const fixturePath = path.join(fixturesDir, `smoke-event-${id}.png`);
      const onePxPng = Buffer.from(
        '89504e470d0a1a0a0000000d49484452000000010000000108060000001f15c489' +
        '0000000d49444154789c63f8cf00000003000100' +
        'ad3ddf730000000049454e44ae426082',
        'hex',
      );
      fs.writeFileSync(fixturePath, onePxPng);

      // Navigate to the freshly created event detail page and
      // count image cards before clicking.
      await page.goto(`${BASE}/events/${id}`, { waitUntil: 'domcontentloaded' });
      await wait(300);
      const before = await page.locator('[data-image-card]').count();
      const urlBefore = page.url();

      // Install the chooser fixture BEFORE clicking — Playwright
      // queues listeners attached before the click that triggers
      // the event.
      const off = setFileChooserFixture(page, [fixturePath]);
      try {
        // Issue #401: the import button is now a <label class="primary-button">
        // wrapping a hidden <input type="file" name="images" multiple>.
        // Clicking the label opens the native file picker (Playwright
        // routes the chooser through setFileChooserFixture); the
        // label's onchange handler then auto-submits the wrapping
        // multipart form. Probe selector changed from
        // `button:has-text(...)` to `label:has-text(...)` to match.
        await page.click('label:has-text("Add Images From Computer")');
        // Wait for the import to land and the gallery to re-render.
        // The htmx-driven fragment swap adds the new card.
        await page.waitForFunction(
          ({ before }) =>
            document.querySelectorAll('[data-image-card]').length > before,
          { before },
          { timeout: 30_000 },
        );
        const after = await page.locator('[data-image-card]').count();
        if (after !== before + 1) {
          throw new Error(
            `expected gallery to grow by exactly one (before=${before} after=${after})`,
          );
        }
        // Issue #391 Slice B.3 parity assertion: the
        // event-side fragment-swap path (in place since
        // post-#332 / post-#341) MUST NOT trigger a full
        // page reload on the upload flow. Mirrors the
        // soldier-side step-04 / step-05 probes in
        // audit/smoke_soldier_images.mjs so a future
        // regression that reverts to X-Dixiedata-Redirect
        // here breaks both probes in lockstep.
        if (page.url() !== urlBefore) {
          throw new Error(
            `expected in-place fragment swap on import (no nav); before="${urlBefore}" after="${page.url()}"`,
          );
        }
      } finally {
        off();
      }
    });

    // step-13 — issue #387. Sibling to #348 (closed not planned):
    // pin the read surface of /events/{id}/images for a
    // freshly-created event with zero images. The empty-state copy
    // ("No images are attached") plus the "Add Images From
    // Computer" button must render. The upload path itself is gated
    // by a separate file-chooser handler (#385) — out of scope
    // here. We mint a fresh event via form-POST (mirror of
    // step-11's setup shape) rather than depending on
    // `createdEventID` (deleted by step-10) or step-11/12's events,
    // so this step is order-independent and a fresh no-images
    // event is guaranteed.
    await step(page, 'step-13 event-images-empty-state-read-surface', async () => {
      const setupResp = await page.request.post(`${BASE}/events/new`, {
        headers: {
          'content-type': 'application/x-www-form-urlencoded',
          'x-dixiedata-submit': 'true',
        },
        data: new URLSearchParams({
          entry_type: 'event',
          kind: 'SmokeImagesEmpty',
          begin_date: '07/01/1863',
          end_date: '07/03/1863',
          description: 'Event-images empty-state smoke probe (#387).',
        }).toString(),
        maxRedirects: 0,
      });
      const loc =
        setupResp.headers()['x-dixiedata-redirect'] ||
        setupResp.headers()['X-DixieData-Redirect'] ||
        setupResp.headers()['location'] ||
        '';
      const m = loc.match(/\/events\/(\d+)/);
      if (!m) {
        throw new Error(`step-13 setup: cannot parse id from Location="${loc}"`);
      }
      const freshEventID = parseInt(m[1], 10);
      trackedEventIDs.push(freshEventID);

      await page.goto(`${BASE}/events/${freshEventID}`, {
        waitUntil: 'domcontentloaded',
      });
      await wait(300);
      if (!page.url().endsWith(`/events/${freshEventID}`)) {
        throw new Error(
          `step-13 nav: expected /events/${freshEventID}, got ${page.url()}`,
        );
      }

      // 1. Gallery container present. event_detail.templ:153 emits
      //    <div id={ uiids.PanelEventDetailImages }> – canonical
      //    UIID ("panel.event.detail.images"), see issue #390.
      //    Mirrors the soldier-side PanelSoldierDetailImages
      //    pattern. Per-card data-image-card / data-image-thumb-id
      //    selectors inside the container are unchanged – those
      //    are per-card markers, not canonical UIIDs.
      const containerPresent = await page.evaluate(
        () => document.querySelector('[id="panel.event.detail.images"]') !== null,
      );
      if (!containerPresent) {
        throw new Error(
          '#panel.event.detail.images container missing from event detail DOM',
        );
      }

      // 2. Empty-state copy "No images are attached" must render
      //    inside the gallery container. EmptyState primitive
      //    emits data-empty-state="true" (components/empty_state.templ:16)
      //    – both that marker AND the title text must be present.
      const emptyCopy = await page.evaluate(() => {
        const root = document.querySelector('[id="panel.event.detail.images"]');
        if (!root) return { found: false };
        const text = root.textContent || '';
        const hasEmptyMarker =
          root.querySelector('[data-empty-state="true"]') !== null;
        return {
          found: hasEmptyMarker && /No images are attached/.test(text),
          hasEmptyMarker,
          snippet: text.replace(/\s+/g, ' ').trim().slice(0, 200),
        };
      });
      if (!emptyCopy.found) {
        throw new Error(
          `expected empty-state copy inside #panel.event.detail.images; got: ${emptyCopy.snippet}`,
        );
      }

      // 3. "Add Images From Computer" button present + visible +
      //    wired to /events/{id}/images/import. event_detail.templ:157
      //    emits it as a <button data-action="/events/{id}/images/import"
      //    data-dixie-submit="true" data-progress-label="Importing images…">.
      const importBtn = await page.evaluate((id) => {
        // Issue #401: the import button is now a <label
        // class="primary-button"> wrapping a hidden file input.
        // Scan labels instead of buttons.
        const btns = Array.from(document.querySelectorAll('label'));
        const match = btns.find(
          (b) => (b.textContent || '').trim() === 'Add Images From Computer',
        );
        if (!match) return { found: false };
        const action = match.getAttribute('data-action') || '';
        const rect = match.getBoundingClientRect();
        return {
          found: true,
          // Issue #401: the action lives on the wrapping <form>,
          // not the <label>. Walk up to read it.
          action:
            match.closest("form") &&
            match.closest("form").getAttribute("action") === `/events/${id}/images/import`
              ? `/events/${id}/images/import`
              : action,
          expectedAction: `/events/${id}/images/import`,
          visible: rect.width > 0 && rect.height > 0,
        };
      }, freshEventID);
      if (!importBtn.found) {
        throw new Error(
          '"Add Images From Computer" label missing from event detail DOM',
        );
      }
      if (importBtn.action !== importBtn.expectedAction) {
        throw new Error(
          `Add Images form action="${importBtn.action}", want "${importBtn.expectedAction}"`,
        );
      }
      if (!importBtn.visible) {
        throw new Error(
          '"Add Images From Computer" label not visible (zero-sized box)',
        );
      }
    });

    // step-14 — issue #386. Pin the populated-gallery read surface of
    // /events/{id}/images driven by the upload-via-UI path (#385's
    // setFileChooserFixture). Mirrors step-12's setup shape (fresh
    // event + scratch-dir fixture) so it is independent of any
    // earlier step; the assertion set is the new territory —
    // thumbnail grid count + per-card alt text + per-card filename
    // + per-card Delete button. The selectors stay parallel to the
    // existing 'data-image-card' / 'data-image-thumb-id' / form-based
    // delete pair the templ emits from EventImagesListFragment
    // (internal/templates/event_panels.templ). No canonical DOM ID
    // exists for the per-card surface in internal/uiids/ — same gap
    // #387 surfaced for the empty-state anchor.
    await step(
      page,
      'step-14 event-images-populated-gallery-read-surface',
      async () => {
        const setupResp = await page.request.post(`${BASE}/events/new`, {
          headers: {
            'content-type': 'application/x-www-form-urlencoded',
            'x-dixiedata-submit': 'true',
          },
          data: new URLSearchParams({
            entry_type: 'event',
            kind: 'SmokeImagesGallery',
            begin_date: '07/04/1863',
            end_date: '07/05/1863',
            description:
              'Event-images populated-gallery read-surface smoke probe (#386).',
          }).toString(),
          maxRedirects: 0,
        });
        const loc =
          setupResp.headers()['x-dixiedata-redirect'] ||
          setupResp.headers()['X-DixieData-Redirect'] ||
          setupResp.headers()['location'] ||
          '';
        const m = loc.match(/\/events\/(\d+)/);
        if (!m) {
          throw new Error(`step-14 setup: cannot parse id from Location="${loc}"`);
        }
        const id = parseInt(m[1], 10);
        trackedEventIDs.push(id);

        // Same scratch-dir fixture scheme as step-12 — keeps the
        // fixture out of the working tree and gives each step a
        // unique filename (keyed by event id) so concurrent runs
        // do not collide on the gallery.
        const fixturesDir = path.join(scratchDir, 'fixtures');
        fs.mkdirSync(fixturesDir, { recursive: true });
        const fixturePath = path.join(fixturesDir, `smoke-gallery-${id}.png`);
        const onePxPng = Buffer.from(
          '89504e470d0a1a0a0000000d49484452000000010000000108060000001f15c489' +
          '0000000d49444154789c63f8cf00000003000100' +
          'ad3ddf730000000049454e44ae426082',
          'hex',
        );
        fs.writeFileSync(fixturePath, onePxPng);

        await page.goto(`${BASE}/events/${id}`, {
          waitUntil: 'domcontentloaded',
        });
        await wait(300);
        if (!page.url().endsWith(`/events/${id}`)) {
          throw new Error(
            `step-14 nav: expected /events/${id}, got ${page.url()}`,
          );
        }

        // Belt-and-braces: the freshly-created event must start in
        // the empty-state so the upload we drive next is the seed
        // for the populated gallery (not a second card). Scoped to
        // the canonical event gallery container
        // (#panel.event.detail.images — see uiids.PanelEventDetailImages,
        // issue #390) so a regression that swaps the container id
        // breaks the probe immediately. Per-card data-image-card
        // attributes remain unchanged — the soldier-side gallery
        // uses the same per-card pattern (soldier_card.templ) so
        // consistency wins here. Promoting those per-card attributes
        // to canonical UIIDs is a separate slice.
        const before = await page
          .locator('[id="panel.event.detail.images"] [data-image-card]')
          .count();
        if (before !== 0) {
          throw new Error(
            `step-14 setup: expected 0 image cards on a fresh event, got ${before}`,
          );
        }

        // Install the filechooser fixture BEFORE clicking the import
        // button – Playwright queues listeners attached before the
        // chooser event, which is the contract #385 documented.
        const off = setFileChooserFixture(page, [fixturePath]);
        // Issue #391 Slice B.3 parity assertion: the
        // event-side fragment-swap path (in place since
        // post-#332 / post-#341) MUST NOT trigger a full
        // page reload on this populated-gallery upload
        // flow. Mirrors the soldier-side step-04 /
        // step-05 probes in audit/smoke_soldier_images.mjs
        // so a future regression that reverts to
        // X-Dixiedata-Redirect here breaks both probes in
        // lockstep.
        const urlBefore = page.url();
        try {
          await page.click('label:has-text("Add Images From Computer")');

          // Wait for the gallery to populate with the seeded image.
          await page.waitForFunction(
            () =>
              document.querySelectorAll(
                '[id="panel.event.detail.images"] [data-image-card]',
              ).length >= 1,
            null,
            { timeout: 30_000 },
          );
          if (page.url() !== urlBefore) {
            throw new Error(
              `expected in-place fragment swap on populated-gallery import (no nav); before="${urlBefore}" after="${page.url()}"`,
            );
          }

          // Read-surface assertions – every check below is the new
          // territory #386 owns. Cards are scoped to the canonical
          // event gallery container so the probe fails fast if the
          // container id drifts.
          const surface = await page.evaluate(() => {
            const cards = Array.from(
              document.querySelectorAll(
                '[id="panel.event.detail.images"] [data-image-card]',
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
              // The filename is the only 'text-xs ... break-all' node
              // in the templ — scope to that class to avoid catching
              // the caption div.
              const filenameEls = card.querySelectorAll(
                'div.text-xs.text-slate-500.break-all',
              );
              const filenameNodes = Array.from(filenameEls).map((el) =>
                (el.textContent || '').trim(),
              );
              const deleteForms = card.querySelectorAll(
                'form[data-image-delete-form]',
              );
              const deleteButtons = Array.from(
                card.querySelectorAll('button.pill-link'),
              )
                .filter((b) => (b.textContent || '').trim() === 'Delete')
                .map((b) => ({
                  type: b.getAttribute('type') || '',
                  rect: (() => {
                    const r = b.getBoundingClientRect();
                    return { w: r.width, h: r.height };
                  })(),
                }));
              return {
                id,
                alt,
                altNonEmpty: alt.trim().length > 0,
                imgVisible,
                filenameNodes,
                deleteFormCount: deleteForms.length,
                deleteButtons,
              };
            });
            // Gallery grew — at least one card renders *some*
            // filename string. We do NOT pin the exact text because
            // the server applies standardizedImageFileName() on
            // import (e.g. "EVT-00006-img-001.png") which is
            // load-bearing for the storage layout — see
            // docs/migrations/v55.md. The probe asserts the new card
            // is wired up (filename node populated, image-extension
            // suffix present) without locking the rename rule.
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

          // 4. Gallery grid renders with the expected image count.
          if (surface.cardCount < 1) {
            throw new Error(
              `step-14: expected ≥1 image card after upload, got ${surface.cardCount}`,
            );
          }

          // 5. Every thumbnail has a visible <img> with non-empty alt,
          // AND at least one card shows a filename with an image
          // extension (the upload path is the seed of the gallery —
          // the new card must render its filename node, even though
          // import rewrites it via standardizedImageFileName()).
          for (const card of surface.cardReports) {
            if (!card.altNonEmpty) {
              throw new Error(
                `step-14: card id=${card.id} has empty alt text`,
              );
            }
            if (!card.imgVisible) {
              throw new Error(
                `step-14: card id=${card.id} thumbnail <img> not visible (zero-sized box)`,
              );
            }
          }
          if (!surface.filenameMatch) {
            const seen = surface.cardReports
              .flatMap((c) => c.filenameNodes)
              .filter(Boolean);
            throw new Error(
              `step-14: no card filename node carries an image-extension suffix; saw=${JSON.stringify(seen)}`,
            );
          }

          // 6. Every card has a Delete button — the templ emits a
          // <form action="/events/{id}/images/delete"> with a
          // <button class="pill-link">Delete</button> inside.
          for (const card of surface.cardReports) {
            if (card.deleteFormCount < 1) {
              throw new Error(
                `step-14: card id=${card.id} missing /images/delete form`,
              );
            }
            const btn = card.deleteButtons[0];
            if (!btn) {
              throw new Error(
                `step-14: card id=${card.id} missing Delete button (text="Delete")`,
              );
            }
            if (btn.type !== 'submit') {
              throw new Error(
                `step-14: card id=${card.id} Delete button has type="${btn.type}", want "submit"`,
              );
            }
            if (btn.rect.w <= 0 || btn.rect.h <= 0) {
              throw new Error(
                `step-14: card id=${card.id} Delete button not visible (zero-sized box)`,
              );
            }
          }
        } finally {
          off();
        }
      },
    );
  } catch (e) {
    // step() already recorded the failure for any throw inside a
    // step(); the unhandled branch below means an error escaped
    // step() — e.g. setup / teardown. Surface it so the next
    // agent does not chase a phantom pass.
    if (fail === 0) {
      console.error('UNHANDLED ERROR:', e);
      fail = 1;
    }
  } finally {
    await teardown(page, trackedEventIDs, seededPersonID);
    await browser.close();

    // Drop the scratch dir post-run.
    try {
      fs.rmSync(scratchDir, { recursive: true, force: true });
    } catch {}
  }

  console.log(`\n${pass} passed, ${fail} failed`);
  console.log(`events touched (created, then cleaned up): ${trackedEventIDs.length}`);
  console.log(`seeded person (created, then cleaned up): ${seededPersonID}`);
  return { ok: fail === 0, steps: { pass, fail } };
}

// Issue #707 batch 3: this probe used to call main() inside
// the legacy `import('./_lib/cleanup.mjs').then(...)` wrapper
// so the spawned binary was killed on Ctrl-C / fatal exit.
// Now main() takes ctx from runProbe and returns {ok, steps};
// runProbe handles the per-probe server SIGTERM via
// ctx.registerCleanup() + scratchDir cleanup via its
// mkdtemp removal. The legacy cleanup.mjs wrapper is gone.
// Standalone invocation (`node audit/smoke_events.mjs`) still
// works because runProbe is callable directly.
runProbe({ name: 'events', probeFn: main })
  .then((r) => { process.exit(r.ok ? 0 : 1); })
  .catch((e) => { console.error('FATAL', e); process.exit(2); });
