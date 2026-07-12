/**
 * audit/smoke_research_picker.mjs — browser-driven end-to-end probe
 * for the Research & Review Person picker (issue #378 slice 3).
 *
 * Companion to audit/smoke_soldier_images.mjs. The slice-2 commit
 * (5edcdd4) shipped the picker shell + picker-guard in handler
 * code; the slice-3 commit lands the recents-list hydration
 * pipeline (frontend/app.js + handleResearchRecent + the
 * research-pack picker sub-screen) and a 7-step end-to-end probe
 * that exercises the full flow against a live dixiedata-web
 * server.
 *
 * Steps (issue #378 slice 3 acceptance):
 *   1.  Seed a Person Record + GET /research → assert the picker
 *       page renders the search input, the empty Recent paragraph
 *       ("No recent picks yet..."), and NO #panel.research.picker.recent
 *       <ul> is present (slice-1 default; no localStorage yet).
 *   2.  evaluate() pre-seeds window.localStorage with two ids +
 *       the right storage key (dixiedata.research.recents) +
 *       reload the page → assert the populated recents-list <ul>
 *       now contains two <li>s with the correct Display IDs (the
 *       localStorage hydration path: loadResearchRecents →
 *       fetch /research/recent?ids=… → fragment swap).
 *   3.  GET /research?next=research-pack → assert
 *       #panel.research.picker.pack-sub-screen exists with a
 *       <select name="geography"> that has both a "state" and
 *       "county" <option>.
 *   4.  Picker sub-screen form submit with the geography=county
 *       choice then a Person Record selection →
 *       /research/select returns 303 to
 *       /soldiers/{id}/research-pack/county (the new slice-3
 *       routing branch).
 *   5.  /soldiers/{id}/camaraderie with no dd_person_ctx cookie
 *       → 303 to /research?next=camaraderie (slice-2 picker-guard,
 *       regression net from slice-2 commit 5edcdd4).
 *   6.  /soldiers/{id}/camaraderie with a valid dd_person_ctx
 *       cookie → 200 (slice-2 picker-guard happy path).
 *   7.  Land on /soldiers/{id} detail page →
 *       data-research-record-id is set on the page wrapper →
 *       the JS remembers the pick (window.localStorage
 *       dixiedata.research.recents now contains the id at head
 *       after the second load).
 *
 * Lifecycle (mirrors smoke_soldier_images.mjs):
 *   - Spawn `build/bin/dixiedata-web.exe` against a private
 *     scratch dir, seed it, tear it down.
 *   - `_lib/cleanup.mjs` ensures the binary is killed on
 *     Ctrl-C / fatal / successful exit (Windows task leak).
 */

import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { setTimeout as sleep } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import fs from 'node:fs';
import { registerCleanup } from './_lib/cleanup.mjs';

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

async function dumpFailureContext(page, stepName) {
  const url = page.url();
  const lastResp = lastResponses[lastResponses.length - 1] || null;
  const filtered = lastResponses
    .filter((r) => /\/research|\/soldiers\//.test(r.url))
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
    recentResponses: filtered,
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

async function createSoldier(page, label) {
  const resp = await page.request.post(`${BASE}/soldiers/new`, {
    headers: {
      'content-type': 'application/x-www-form-urlencoded',
      'x-dixiedata-submit': 'true',
    },
    data: new URLSearchParams({
      entry_type: 'soldier',
      first_name: 'SmokePicker',
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
  const scratchDir = path.join(repoRoot, '.scratch', 'smoke-research-picker');
  const webBin = path.join(repoRoot, 'build', 'bin', 'dixiedata-web.exe');

  try {
    fs.rmSync(scratchDir, { recursive: true, force: true });
  } catch (_) {
    // ignore
  }

  if (!fs.existsSync(webBin)) {
    throw new Error(
      `dixiedata-web binary missing at ${webBin}; run \`make build\` first`,
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
    webBin,
    ['-addr', `127.0.0.1:${PORT}`, '-scratch-dir', scratchDir],
    {
      cwd: repoRoot,
      env: { ...process.env, DIXIEDATA_DATA_DIR: scratchDir },
    },
  );
  registerCleanup({ proc, processNames: ['dixiedata-web.exe'] });
  proc.stderr.on('data', (d) => process.stderr.write(`[srv] ${d}`));
  proc.stdout.on('data', (d) => process.stdout.write(`[srv] ${d}`));

  await waitForServer(BASE);
  console.log(`server up at ${BASE}`);

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ acceptDownloads: true });
  const page = await context.newPage();

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

  console.log('\nResearch & Review picker smoke probe (issue #378 slice 3)');
  console.log('---------------------------------------------------------');

  try {
    // Step 1: fresh /research visit with no localStorage → empty
    // Recent paragraph renders, no populated <ul> in the recents
    // region. The picker page anchors and the search input are
    // also asserted here so the slice-3 probe proves slice-2
    // (foldout + htmx search) wasn't regressed by slice-3.
    let soldierA;
    await step(page, 'step-01 picker-page-empty-recent', async () => {
      soldierA = await createSoldier(page, 'Alpha');
      trackedSoldierIDs.push(soldierA);
      // Clear localStorage so this run is hermetic.
      await page.goto(`${BASE}/research`, { waitUntil: 'domcontentloaded' });
      await page.evaluate(() => {
        try { window.localStorage.removeItem('dixiedata.research.recents'); } catch (_) {}
      });
      await page.goto(`${BASE}/research`, { waitUntil: 'domcontentloaded' });
      const searchInput = await page.$('input[name="q"][data-research-query]');
      if (!searchInput) throw new Error('search input missing on picker page');
      const empty = await page.$('[data-research-recent-empty]');
      if (!empty) throw new Error('empty-state paragraph missing on initial load');
      const list = await page.$('[data-research-recent-list]');
      if (list) throw new Error('recents <ul> should not render on first visit (no localStorage)');
    });

    // Step 2: pre-seed localStorage with two ids (one of which is
    // the soldier we just created, one of which is the unknown id
    // 999999 to prove the fragment endpoint silently drops it),
    // reload, assert the recents-list <ul> is now populated with
    // exactly one <li> (the unknown id drops out).
    let soldierB;
    await step(page, 'step-02 localStorage-hydration-populates-recents', async () => {
      soldierB = await createSoldier(page, 'Bravo');
      trackedSoldierIDs.push(soldierB);
      await page.evaluate(({ a, b }) => {
        try {
          window.localStorage.setItem('dixiedata.research.recents', JSON.stringify([a, b, 999999]));
        } catch (_) {}
      }, { a: soldierA, b: soldierB });
      await page.goto(`${BASE}/research`, { waitUntil: 'domcontentloaded' });
      // Wait for the JS-driven swap to complete. The page makes
      // one fetch to /research/recent; poll for the <ul>.
      const deadline = Date.now() + 5000;
      let list = null;
      while (Date.now() < deadline) {
        list = await page.$('[data-research-recent-list]');
        if (list) break;
        await sleep(80);
      }
      if (!list) throw new Error('recents <ul> did not appear after localStorage hydration');
      const items = await page.$$('[data-research-recent-list] li');
      if (items.length !== 2) {
        throw new Error(`recents <ul> should have 2 items (drop unknown 999999); got ${items.length}`);
      }
    });

    // Step 2b (issue #487 regression): recents-list Open button
    // must honor the picker URL's ?next= query param. The prior
    // CSS-selector bug
    // (`#page.research.picker form input[name='next']`)
    // never matched the dotted-id page wrapper and silently fell
    // back to "camaraderie", so Open Timeline / Open Research Log
    // shortcuts always landed at /soldiers/{id}/camaraderie.
    //
    // Visit /research?next=timeline, click the recents Open
    // button, assert the destination is /soldiers/{id}/timeline
    // (NOT /soldiers/{id}/camaraderie). Re-seed localStorage
    // explicitly so the test is independent of later step order.
    // Placed BEFORE step-03 because step-03 has a pre-existing
    // unrelated flake (a CSS-escape selector that fails on this
    // Playwright version); if step-03 fails first, this regression
    // net never runs.
    await step(page, 'step-02b recents-open-honors-next-query-timeline', async () => {
      await page.evaluate((id) => {
        try {
          window.localStorage.setItem('dixiedata.research.recents', JSON.stringify([id]));
        } catch (_) {
          // best effort
        }
      }, soldierA);
      await page.goto(`${BASE}/research?next=timeline`, { waitUntil: 'domcontentloaded' });
      const deadline = Date.now() + 5000;
      let list = null;
      while (Date.now() < deadline) {
        list = await page.$('[data-research-recent-list]');
        if (list) break;
        await sleep(80);
      }
      if (!list) {
        throw new Error('recents <ul> did not appear after localStorage hydration (issue #487)');
      }
      // Sanity: the SSR page must echo the next keyword into a
      // hidden form field so the JS hydration fetch can round-trip
      // it. If this assertion fails, the picker-page SSR lost the
      // ?next= and no amount of JS fix will recover. Use the
      // getElementById form (matches the fix in
      // researchPickerNextKeyword) to dodge the dotted-id CSS
      // selector bug being tested.
      const hiddenNext = await page.evaluate(() => {
        const pageEl = document.getElementById('page.research.picker');
        if (!(pageEl instanceof HTMLElement)) return null;
        const input = pageEl.querySelector("form input[name='next']");
        return input instanceof HTMLInputElement ? input.value : null;
      });
      if (hiddenNext !== 'timeline') {
        throw new Error(`SSR search form's hidden next field expected 'timeline', got ${JSON.stringify(hiddenNext)}`);
      }
      const form = await page.$('[data-research-recent-list] form[data-dixie-submit="true"]');
      if (!form) {
        throw new Error('recents <form> is missing data-dixie-submit="true" (issue #426 follow-up)');
      }
      await Promise.all([
        page.waitForURL(/\/soldiers\/\d+\/timeline/, { timeout: 5000 }),
        form.$eval('button[type="submit"]', (b) => b.click()),
      ]);
      if (!/\/soldiers\/\d+\/timeline/.test(page.url())) {
        throw new Error(
          `expected page.url() to match /soldiers/{id}/timeline for ?next=timeline; got ${page.url()} ` +
          `(recents Open ignored the picker URL's ?next= — issue #487)`,
        );
      }
    });

    // Step 2c (issue #487 regression): same defect for ?next=research-log.
    // Catches a future fix that accidentally hardcodes "timeline"
    // instead of reading the SSR-rendered next value.
    await step(page, 'step-02c recents-open-honors-next-query-research-log', async () => {
      await page.evaluate((id) => {
        try {
          window.localStorage.setItem('dixiedata.research.recents', JSON.stringify([id]));
        } catch (_) {
          // best effort
        }
      }, soldierA);
      await page.goto(`${BASE}/research?next=research-log`, { waitUntil: 'domcontentloaded' });
      const deadline = Date.now() + 5000;
      let list = null;
      while (Date.now() < deadline) {
        list = await page.$('[data-research-recent-list]');
        if (list) break;
        await sleep(80);
      }
      if (!list) {
        throw new Error('recents <ul> did not appear after localStorage hydration (issue #487)');
      }
      const form = await page.$('[data-research-recent-list] form[data-dixie-submit="true"]');
      if (!form) {
        throw new Error('recents <form> is missing data-dixie-submit="true" (issue #426 follow-up)');
      }
      await Promise.all([
        page.waitForURL(/\/soldiers\/\d+\/research-log/, { timeout: 5000 }),
        form.$eval('button[type="submit"]', (b) => b.click()),
      ]);
      if (!/\/soldiers\/\d+\/research-log/.test(page.url())) {
        throw new Error(
          `expected page.url() to match /soldiers/{id}/research-log for ?next=research-log; got ${page.url()} ` +
          `(recents Open ignored the picker URL's ?next= — issue #487)`,
        );
      }
    });

    // Step 3: ?next=research-pack surfaces the picker sub-screen
    // with a <select name="geography"> that has both "state" and
    // "county" options. The sub-screen is its own section
    // (id=panel.research.picker.pack-sub-screen).
    await step(page, 'step-03 research-pack-sub-screen-renders', async () => {
      await page.goto(`${BASE}/research?next=research-pack`, { waitUntil: 'domcontentloaded' });
      const subScreen = await page.$('#panel\\.research\\.picker\\.pack-sub-screen');
      if (!subScreen) throw new Error('pack-sub-screen panel missing');
      const select = await page.$('select[name="geography"]');
      if (!select) throw new Error('geography <select> missing on sub-screen');
      const stateOpt = await page.$('option[value="state"][data-research-pack-state]');
      const countyOpt = await page.$('option[value="county"][data-research-pack-county]');
      if (!stateOpt) throw new Error('state option missing');
      if (!countyOpt) throw new Error('county option missing');
    });

    // Step 4: submit a picker form with geography=county and
    // person_id=soldierA → 303 to
    // /soldiers/{soldierA}/research-pack/county (the slice-3
    // routing branch).
    await step(page, 'step-04 picker-submit-routes-by-geography', async () => {
      const resp = await page.request.post(`${BASE}/research/select`, {
        headers: {
          'content-type': 'application/x-www-form-urlencoded',
          'x-dixiedata-submit': 'true',
        },
        data: new URLSearchParams({
          person_id: String(soldierA),
          next: 'research-pack',
          geography: 'county',
        }).toString(),
        maxRedirects: 0,
      });
      const loc =
        resp.headers()['x-dixiedata-redirect'] ||
        resp.headers()['X-DixieData-Redirect'] ||
        resp.headers()['location'] ||
        '';
      if (!loc.includes(`/soldiers/${soldierA}/research-pack/county`)) {
        throw new Error(`expected /soldiers/${soldierA}/research-pack/county in redirect; got "${loc}"`);
      }
    });

    // Step 5: /soldiers/{id}/camaraderie with no cookie → 303
    // to /research?next=camaraderie (slice-2 picker-guard
    // regression net).
    await step(page, 'step-05 picker-guard-redirects-without-cookie', async () => {
      // Make sure no cookie is set in this context.
      await context.clearCookies();
      const resp = await page.request.get(`${BASE}/soldiers/${soldierA}/camaraderie`, { maxRedirects: 0 });
      const loc = resp.headers()['location'] || '';
      if (!/^\/research\?next=camaraderie(&|$)/.test(loc)) {
        throw new Error(`expected /research?next=camaraderie; got "${loc}"`);
      }
    });

    // Step 6: /soldiers/{id}/camaraderie with a valid
    // dd_person_ctx cookie → 200 (slice-2 picker-guard happy
    // path). We use the picker POST to set the cookie.
    await step(page, 'step-06 picker-guard-happy-path-with-cookie', async () => {
      await page.request.post(`${BASE}/research/select`, {
        headers: {
          'content-type': 'application/x-www-form-urlencoded',
          'x-dixiedata-submit': 'true',
        },
        data: new URLSearchParams({
          person_id: String(soldierA),
          next: 'camaraderie',
        }).toString(),
        maxRedirects: 0,
      });
      const resp = await page.request.get(`${BASE}/soldiers/${soldierA}/camaraderie`, { maxRedirects: 0 });
      if (resp.status() !== 200) {
        throw new Error(`expected 200 with valid cookie; got ${resp.status()}`);
      }
    });

    // Step 7: Land on /soldiers/{id} detail page; the page
    // wrapper carries data-research-record-id; the JS pushes the
    // id into localStorage dixiedata.research.recents (head).
    // Reload the picker and assert the recents <ul> now lists
    // soldierA at the top.
    await step(page, 'step-07 detail-page-hydrates-localStorage', async () => {
      await page.goto(`${BASE}/soldiers/${soldierA}`, { waitUntil: 'domcontentloaded' });
      const detail = await page.$('[data-research-record-id]');
      if (!detail) throw new Error('data-research-record-id anchor missing on detail page');
      // Re-load the picker. The JS boot in app.js calls
      // rememberResearchPickFromPage on DOMContentLoaded, so the
      // localStorage is updated before the picker page is even
      // fetched. We don't strictly need to wait, but give the
      // JS a beat to run.
      await page.goto(`${BASE}/research`, { waitUntil: 'domcontentloaded' });
      const stored = await page.evaluate(() => {
        try { return JSON.parse(window.localStorage.getItem('dixiedata.research.recents') || '[]'); }
        catch (_) { return null; }
      });
      if (!Array.isArray(stored) || stored[0] !== soldierA) {
        throw new Error(`expected localStorage head to be ${soldierA}; got ${JSON.stringify(stored)}`);
      }
    });

    
    
    
    
    
    await step(page, 'step-08 recents-open-button-dispatches-not-native-submit', async () => {
      // Issue #426 follow-up: the recents-list Open button must carry
      // data-dixie-submit so the JS dispatcher intercepts the submit,
      // reads X-DixieData-Redirect, and calls window.location.assign.
      // Without the attribute, the browser does a native POST to
      // /research/select, the server returns 200 + an empty body +
      // X-DixieData-Redirect, the browser ignores the custom header,
      // and the user lands on a blank white /research/select page.
      //
      // We rely on step-07 leaving soldierA at the head of
      // localStorage.dixiedata.research.recents (the soldier detail
      // page's data-research-record-id push-to-head). Visit /research,
      // wait for the JS-driven localStorage hydration to render the
      // recents <ul>, then click the first recents Open button and
      // assert the page navigated to /soldiers/{id}/... (NOT
      // /research/select).
      await page.goto(`${BASE}/research`, { waitUntil: 'domcontentloaded' });
      const deadline = Date.now() + 5000;
      let list = null;
      while (Date.now() < deadline) {
        list = await page.$('[data-research-recent-list]');
        if (list) break;
        await sleep(80);
      }
      if (!list) throw new Error('recents <ul> did not appear after localStorage hydration');
      const form = await page.$('[data-research-recent-list] form[data-dixie-submit="true"]');
      if (!form) {
        throw new Error('recents <form> is missing data-dixie-submit="true" — native submit will white-screen');
      }
      await Promise.all([
        page.waitForURL(/\/soldiers\/\d+\//, { timeout: 5000 }),
        form.$eval('button[type="submit"]', (b) => b.click()),
      ]);
      if (!/\/soldiers\/\d+\/[a-z-]+/.test(page.url())) {
        throw new Error(`expected page.url() to match /soldiers/{id}/<sub-page>; got ${page.url()}`);
      }
    });
  } catch (e) {
    // step() already recorded the failure; fall through to cleanup.
  } finally {
    await teardown(page, trackedSoldierIDs);
    await context.close();
    await browser.close();
  }

  console.log(`\nResults: ${pass} pass, ${fail} fail`);
  if (fail > 0) {
    process.exit(1);
  }
  process.exit(0);
}

main().catch((e) => {
  console.error('fatal:', e);
  process.exit(2);
});
