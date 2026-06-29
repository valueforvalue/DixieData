// Probe: verify that the shared status panel scrolls into view when
// a non-placeholder response lands in it.
//
// Symptom (F2): #share-status is at y=1372 below a 1280x800 viewport
// fold. User clicks Load Backup (button y=872), htmx swaps the
// response into #share-status, but the user is scrolled near the
// button and never sees the panel — concludes "nothing happened".
//
// Fix: app.js applyResponse calls scrollShareStatusIntoView on the
// target after the innerHTML swap.
//
// This probe drives the REAL htmx click flow end-to-end:
//   1. Boot dixiedata-web against a seeded scratch dir.
//   2. Load /share.
//   3. Use page.route() to intercept POST /import/backup at the
//      network layer and return a canned 200 response. This works
//      for both fetch and XHR (htmx uses XHR by default).
//   4. Auto-accept the htmx window.confirm.
//   5. Instrument #share-status.scrollIntoView.
//   6. Click Load Backup. htmx fires XHR → page.route returns 200 →
//      applyResponse runs → innerHTML swapped → scrollShareStatusIntoView
//      calls scrollIntoView.
//   7. Assert scrollIntoView was called and panel ended up in view.
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { setTimeout as sleep } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { registerCleanup, runWithCleanup } from './_lib/cleanup.mjs';

const PORT = process.env.PROBE_PORT || '8766';
const BASE = `http://127.0.0.1:${PORT}`;

async function waitForServer(url, maxMs = 30000) {
  const deadline = Date.now() + maxMs;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url);
      if (res.status < 500) return;
    } catch (_) {}
    await sleep(200);
  }
  throw new Error(`server at ${url} never came up`);
}

async function main({ registerCleanup }) {
  const here = path.dirname(fileURLToPath(import.meta.url));
  const repoRoot = here.endsWith('audit') ? path.dirname(here) : here;
  const scratchDir = '.scratch/probe-share-status-scroll';

  let proc = null;
  let exitCode = 0;

  try {
    await import('node:fs/promises').then((m) => m.rm(scratchDir, { recursive: true, force: true }));
    {
      const seedProc = spawn('go', ['run', './cmd/seed-data', '-data-dir', scratchDir, '-soldiers', '5', '-reset'],
        { cwd: repoRoot, stdio: ['ignore', 'pipe', 'pipe'] });
      let seedOut = '';
      seedProc.stdout.on('data', (d) => { seedOut += d; });
      seedProc.stderr.on('data', (d) => { seedOut += d; });
      const seedExit = await new Promise((resolve) => seedProc.on('exit', resolve));
      if (seedExit !== 0) throw new Error(`seed-data failed (exit ${seedExit}):\n${seedOut}`);
      console.log(`seeded ${scratchDir}`);
    }

    // Spawn the prebuilt binary directly (not `go run`) so signals
    // reach the real process tree and taskkill-by-name cleanup in
    // audit/_lib/cleanup.mjs is reliable.
    const webBin = path.join(repoRoot, 'build', 'bin', 'dixiedata-web.exe');
    proc = spawn(webBin, ['-addr', `127.0.0.1:${PORT}`, '-scratch-dir', scratchDir],
      { cwd: repoRoot, env: { ...process.env, DIXIEDATA_DATA_DIR: scratchDir } });
    registerCleanup({ proc, processNames: ['dixiedata-web.exe'] });
    proc.stderr.on('data', (d) => process.stderr.write(`[srv] ${d}`));
    proc.stdout.on('data', (d) => process.stdout.write(`[srv] ${d}`));

    await waitForServer(BASE);
    console.log(`server up at ${BASE}`);

    const browser = await chromium.launch();
    const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
    const page = await context.newPage();

    // Network-level intercept: works for fetch AND XHR. Catches the
    // htmx POST to /import/backup and returns a canned 200 with the
    // "Backup loaded: ..." body that matches the production handler.
    let routeHits = 0;
    await page.route('**/import/backup', async (route) => {
      routeHits++;
      await route.fulfill({
        status: 200,
        contentType: 'text/plain; charset=utf-8',
        headers: {
          'X-DixieData-Toast': 'Success: 501 records imported from backup.',
          'X-DixieData-Toast-Type': 'success',
        },
        body: 'Backup loaded: 501 soldiers, 1683 records, 1139 images.',
      });
    });

    // Auto-accept the htmx window.confirm.
    page.on('dialog', async (d) => { await d.accept(); });

    const errors = [];
    page.on('pageerror', (e) => errors.push(e.message));
    page.on('console', (m) => {
      if (m.type() === 'error') errors.push(`console: ${m.text()}`);
      if (m.text().includes('[DEBUG-f2]')) console.log('[page]', m.text());
    });

    await page.goto(`${BASE}/share`, { waitUntil: 'domcontentloaded' });
    await sleep(1500);

    // Instrument scrollIntoView on #share-status BEFORE the click.
    const instrumentResult = await page.evaluate(() => {
      const panel = document.getElementById('share-status');
      if (!panel) return { error: 'no #share-status' };
      const calls = [];
      const original = panel.scrollIntoView.bind(panel);
      panel.scrollIntoView = function (opts) {
        const r = panel.getBoundingClientRect();
        calls.push({ opts: opts || null, y: r.y, h: r.height, ts: Date.now() });
        return original(opts);
      };
      window.__shareStatusScrollCalls = calls;
      return { ok: true, panelInitialY: panel.getBoundingClientRect().y };
    });
    console.log('instrument:', JSON.stringify(instrumentResult));

    const beforeClick = await page.locator('#share-status').boundingBox();
    const viewport = page.viewportSize();
    console.log(`before click: panel y=${Math.round(beforeClick.y)} viewport.height=${viewport.height}`);

    // Click Load Backup. htmx fires XHR → page.route returns 200 →
    // applyResponse runs → innerHTML swap → scrollShareStatusIntoView
    // calls scrollIntoView({ behavior: 'smooth', block: 'nearest' }).
    await page.locator('button.danger-button:has-text("Load Backup")').click();
    await sleep(1500);

    const afterClick = await page.locator('#share-status').boundingBox();
    const scrollCalls = await page.evaluate(() => window.__shareStatusScrollCalls || []);
    const panelText = (await page.locator('#share-status').textContent()) || '';

    console.log(`route hits on /import/backup: ${routeHits}`);
    console.log(`after click:  panel y=${Math.round(afterClick.y)} inViewport=${afterClick.y >= 0 && afterClick.y + afterClick.height <= viewport.height}`);
    console.log(`panel text: ${panelText.slice(0, 120)}`);
    console.log(`scrollIntoView calls: ${JSON.stringify(scrollCalls)}`);

    await browser.close();

    console.log('\n=== VERDICT ===');
    const verdicts = [];
    if (routeHits > 0) {
      verdicts.push(`OK: POST /import/backup was intercepted (${routeHits} hit(s)) — htmx dispatched the request`);
    } else {
      verdicts.push('FAIL: no POST /import/backup — htmx did not dispatch');
      exitCode = 1;
    }
    if (panelText.includes('Backup loaded: 501')) {
      verdicts.push('OK: panel content swapped to "Backup loaded: 501 ..."');
    } else {
      verdicts.push(`FAIL: panel content is "${panelText.slice(0, 80)}" — swap did not happen`);
      exitCode = 1;
    }
    if (scrollCalls.length > 0) {
      const opts = scrollCalls[0].opts;
      const optsStr = opts ? JSON.stringify(opts) : 'no-opts';
      verdicts.push(`OK: scrollIntoView invoked ${scrollCalls.length} time(s) on #share-status (first call opts=${optsStr})`);
    } else {
      verdicts.push('FAIL: scrollIntoView was NOT called on #share-status — fix not active');
      exitCode = 1;
    }
    if (afterClick.y < viewport.height && afterClick.y + afterClick.height > 0) {
      verdicts.push(`OK: panel ended up inside or adjacent to viewport (y=${Math.round(afterClick.y)}, h=${Math.round(afterClick.height)})`);
    } else {
      verdicts.push(`FAIL: panel still out of view (y=${Math.round(afterClick.y)})`);
      exitCode = 1;
    }
    verdicts.forEach((v) => console.log(v));

    if (errors.length > 0) {
      console.log(`\npage errors (${errors.length}):`);
      errors.forEach((e) => console.log(`  - ${e}`));
    }
    return exitCode;
  } catch (e) {
    console.error('FATAL:', e);
    return 2;
  }
}

const exitCode = await runWithCleanup(main);
process.exit(exitCode);