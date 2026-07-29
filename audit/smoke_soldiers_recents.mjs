/**
 * audit/smoke_soldiers_recents.mjs — browser-driven end-to-end probe
 * for the soldiers-page "Recently Accessed" recents list (issue #488).
 *
 * Sibling of audit/smoke_research_picker.mjs (issue #487 established the
 * selector-vs-mark pattern). Bug class: JS selector assumed hx-get
 * lived on the input; the htmxattr.Mux migration in commit bd5f9e6
 * (issue #407) moved hx-* to the parent form, so the selector
 * silently returned null and hydrateRecentSearchResults() never
 * ran. localStorage was being written correctly; the fetch never
 * fired.
 *
 * Steps:
 *   1.  Source scan: internal/templates/soldier_card.templ contains
 *       data-quick-search adjacent to the quick-search input.
 *   2.  Source scan: frontend/app.js::quickSearchInput() uses
 *       selector input[data-quick-search].
 *   3.  Live probe: seed a Person Record + a second Person Record,
 *       navigate to /soldiers/{firstId} so detail-page JS writes
 *       dixiedata.recentRecords, then navigate to /soldiers.
 *       Assert the "Recently Accessed" header is visible inside
 *       #soldier-list and the empty-state is gone.
 *   4.  Live probe: capture the /soldiers/search/recent?ids=…
 *       request fired during step-3 hydration.
 *
 * Lifecycle (mirrors smoke_research_picker.mjs):
 *   - Spawn `build/bin/dixiedata-web.exe` against a private
 *     scratch dir, seed it, tear it down.
 *   - _lib/cleanup.mjs ensures the binary is killed on Ctrl-C /
 *     fatal / successful exit (Windows task leak).
 */

import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { setTimeout as sleep } from 'node:timers/promises';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import fs from 'node:fs';
import { registerCleanup } from './_lib/cleanup.mjs';

const PORT = process.env.PROBE_PORT || '8776';
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
    .filter((r) => /\/soldiers\/search|\/soldiers\//.test(r.url))
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
      // server not up yet
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
      first_name: 'SmokeRecents',
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
      // best-effort cleanup
    }
  }
}

function sourceScan(name, fn) {
  try {
    fn();
    record(name, true);
  } catch (e) {
    record(name, false, { error: e.message });
    throw e;
  }
}

async function main() {
  const here = path.dirname(fileURLToPath(import.meta.url));
  const repoRoot = here.endsWith('audit') ? path.dirname(here) : here;
  const scratchDir = path.join(repoRoot, '.scratch', 'smoke-soldiers-recents');
  const webBin = path.join(repoRoot, 'build', 'bin', 'dixiedata-web.exe');
  const soldierCardTempl = path.join(
    repoRoot,
    'internal',
    'templates',
    'soldier_card.templ',
  );
  const appJs = path.join(repoRoot, 'frontend', 'app.js');

  try {
    fs.rmSync(scratchDir, { recursive: true, force: true });
  } catch (_) {
    // ignore
  }

  if (!fs.existsSync(webBin)) {
    throw new Error(
      `dixiedata-web binary missing at ${webBin}; run \`just debug\` first`,
    );
  }

  // ----- Source-scan assertions (deterministic, no server needed) -----
  console.log('\nSource scans (issue #488)');
  console.log('-------------------------');

  const templSrc = readFileSync(soldierCardTempl, 'utf8');
  const appJsSrc = readFileSync(appJs, 'utf8');

  sourceScan('src-01 soldier_card.templ declares data-quick-search', () => {
    // Match the quick-search input line and require data-quick-search
    // to appear within ~120 chars of name="q".
    const re = /name="q"[\s\S]{0,200}?data-quick-search|data-quick-search[\s\S]{0,200}?name="q"/;
    if (!re.test(templSrc)) {
      throw new Error(
        'soldier_card.templ is missing data-quick-search on the quick-search input (issue #488 fix)',
      );
    }
  });

  sourceScan('src-02 app.js::quickSearchInput uses input[data-quick-search]', () => {
    if (!/querySelector\(\s*["']input\[data-quick-search\]\s*["']\s*\)/.test(appJsSrc)) {
      throw new Error(
        'app.js quickSearchInput() does not use selector input[data-quick-search] (issue #488 fix)',
      );
    }
    // And it must NOT keep the old broken selector.
    if (/input\[name="q"\]\[hx-get="\/soldiers\/search"\]/.test(appJsSrc)) {
      throw new Error(
        'app.js quickSearchInput() still uses the broken selector input[name="q"][hx-get="/soldiers/search"] (issue #488)',
      );
    }
  });

  // ----- Live probe -----
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
  console.log(`\nserver up at ${BASE}`);

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

  console.log('\nSoldiers-page recents hydration smoke probe (issue #488)');
  console.log('----------------------------------------------------------');

  try {
    // ----- step-01: detail-page writes to localStorage -----
    let soldierA;
    await step(page, 'step-01 detail-page-write-stores-id-in-localStorage', async () => {
      soldierA = await createSoldier(page, 'Alpha');
      trackedSoldierIDs.push(soldierA);
      await page.goto(`${BASE}/soldiers/${soldierA}`, {
        waitUntil: 'domcontentloaded',
      });
      const stored = await page.evaluate(() => {
        try {
          return window.localStorage.getItem('dixiedata.recentRecords');
        } catch (_) {
          return null;
        }
      });
      if (!stored) {
        throw new Error(
          'expected dixiedata.recentRecords to be populated after detail-page load',
        );
      }
      const parsed = JSON.parse(stored);
      if (!Array.isArray(parsed) || !parsed.includes(soldierA)) {
        throw new Error(
          `expected localStorage recents to include ${soldierA}; got ${stored}`,
        );
      }
    });

    // ----- step-02: navigate to /soldiers, assert hydration runs -----
    await step(page, 'step-02 soldiers-page-auto-hydrates-recents', async () => {
      // Reset the captured responses so we only watch the upcoming nav.
      lastResponses.length = 0;
      await page.goto(`${BASE}/soldiers`, { waitUntil: 'domcontentloaded' });
      // Wait for the hydration fetch to land.
      const deadline = Date.now() + 5000;
      let recentHeader = null;
      while (Date.now() < deadline) {
        recentHeader = await page.$('#soldier-list :text("Recently Accessed")');
        if (recentHeader) break;
        await sleep(80);
      }
      if (!recentHeader) {
        throw new Error(
          '#soldier-list did not hydrate to "Recently Accessed" within 5s of /soldiers load (issue #488)',
        );
      }
      const recentFetch = lastResponses.find((r) =>
        /\/soldiers\/search\/recent\?ids=/.test(r.url),
      );
      if (!recentFetch) {
        throw new Error(
          'expected page-load hydration to fetch /soldiers/search/recent?ids=… but no such request was observed',
        );
      }
      if (recentFetch.status !== 200) {
        throw new Error(
          `/soldiers/search/recent returned status ${recentFetch.status}; expected 200`,
        );
      }
      // And the empty state must be gone.
      const stillEmpty = await page.$('[data-recent-records-empty]');
      if (stillEmpty) {
        throw new Error(
          'data-recent-records-empty is still present after hydration (the swap did not happen)',
        );
      }
      // The Detail ID from soldierA should appear in the hydrated list.
      const pageHtml = await page.content();
      if (!pageHtml.includes(soldierA)) {
        throw new Error(
          `expected hydrated recents list to include soldier id ${soldierA}; not found in DOM`,
        );
      }
      // URL must not have changed (no accidental redirect).
      if (!/\/soldiers$/.test(page.url())) {
        throw new Error(`unexpected URL after hydration: ${page.url()}`);
      }
    });
  } catch (e) {
    // step() already recorded the failure; just fall through to teardown.
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