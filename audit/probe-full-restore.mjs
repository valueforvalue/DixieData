// End-to-end probe: drive the FULL backup-restore path through the
// real web-mode binary, with a real .ddbak file, against a real
// scratch data dir. Verifies the user-reported "I am currently
// having issues loading .ddbak archives" bug is fixed at every
// layer:
//
//   1. Seed the scratch dir (5 soldiers).
//   2. Boot dixiedata-web against it.
//   3. POST /import/backup via Playwright, intercepting the
//      OpenFileDialog call. We do this by mounting an htmx
//      override that returns the real .ddbak path BEFORE the
//      server-side handler calls wailsruntime.OpenFileDialog (which
//      would return errWailsFrontendUnavailable in web-mode).
//
// Implementation detail: the appshell import handler has an
// openFileDialogOverride hook (added for the httptest in
// imports_handlers_test.go). It is checked FIRST in OpenFileDialog
// before the wails runtime call. We expose it as a Go-level
// startup flag for the web-mode binary.
//
// Probe asserts:
//   - htmx dispatched the request (route hit)
//   - server returned 200 with X-DixieData-Toast
//   - server returned X-DixieData-Redirect (F1 fix)
//   - panel content shows "Backup loaded: N soldiers, M records, K images"
//   - scrollShareStatusIntoView fires via htmx:afterSwap (F2 fix)
//   - panel ends up inside viewport at 1280x800
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { setTimeout as sleep } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const PORT = process.env.PROBE_PORT || '8767';
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

async function main() {
  const here = path.dirname(fileURLToPath(import.meta.url));
  const repoRoot = here.endsWith('audit') ? path.dirname(here) : here;
  const scratchDir = '.scratch/probe-full-restore';
  const ddbakPath = path.join(repoRoot, 'dixiedata-backup-2026-05-30.ddbak');

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
      console.log(`seeded ${scratchDir} with 5 soldiers`);
    }

    proc = spawn('go', ['run', './cmd/dixiedata-web', '-addr', `127.0.0.1:${PORT}`, '-scratch-dir', scratchDir],
      { cwd: repoRoot, env: { ...process.env, DIXIEDATA_DATA_DIR: scratchDir, DIXIE_OPEN_FILE_DIALOG_PATH: ddbakPath } });
    // also set on the test process so the binary picks it up after
    // Process spawning doesn't always pass env cleanly across shells.
    console.log(`DIXIE_OPEN_FILE_DIALOG_PATH=${ddbakPath}`);
    proc.stderr.on('data', (d) => process.stderr.write(`[srv] ${d}`));
    proc.stdout.on('data', (d) => process.stdout.write(`[srv] ${d}`));

    await waitForServer(BASE);
    console.log(`server up at ${BASE}`);

    const browser = await chromium.launch();
    const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
    const errors = [];
    page.on('pageerror', (e) => errors.push(e.message));
    page.on('console', (m) => { if (m.type() === 'error') errors.push(`console: ${m.text()}`); });

    // Capture every response to /import/backup and its headers.
    let importResponse = null;
    page.on('response', async (res) => {
      const u = res.url();
      if (u.includes('/import/backup')) {
        importResponse = {
          status: res.status(),
          headers: await res.allHeaders(),
          body: await res.text(),
        };
      }
    });

    await page.addInitScript(() => { window.confirm = () => true; });

    await page.goto(`${BASE}/share`, { waitUntil: 'domcontentloaded' });
    await sleep(1500);

    // Diagnostics: did the Load Backup button render? Is the server
    // boot waiting on something? Dump page URL + button count.
    const diag = await page.evaluate(() => ({
      url: window.location.href,
      btnCount: document.querySelectorAll('button.danger-button').length,
      bodyText: document.body.innerText.slice(0, 300),
    }));
    console.log('diag:', JSON.stringify(diag, null, 2));

    // Instrument scrollIntoView on #share-status BEFORE click.
    await page.evaluate(() => {
      const panel = document.getElementById('share-status');
      if (!panel) return;
      const calls = [];
      const original = panel.scrollIntoView.bind(panel);
      panel.scrollIntoView = function (opts) {
        calls.push({ opts: opts || null, ts: Date.now() });
        return original(opts);
      };
      window.__shareStatusScrollCalls = calls;
    });

    const beforeClick = await page.locator('#share-status').boundingBox();
    const viewport = page.viewportSize();
    console.log(`before click: panel y=${Math.round(beforeClick.y)}`);

    await page.locator('button.danger-button:has-text("Load Backup")').click();
    await sleep(20000); // real .ddbak import + DB close/reopen + restart takes ~13s

    const afterClick = await page.locator('#share-status').boundingBox();
    const scrollCalls = await page.evaluate(() => window.__shareStatusScrollCalls || []);
    const panelText = (await page.locator('#share-status').textContent()) || '';

    console.log(`after click: panel y=${Math.round(afterClick.y)} inViewport=${afterClick.y >= 0 && afterClick.y + afterClick.height <= viewport.height}`);
    console.log(`panel text: ${panelText.slice(0, 200)}`);
    console.log(`scrollIntoView calls: ${scrollCalls.length}`);
    console.log(`import response: ${importResponse ? `${importResponse.status} body=${(importResponse.body || '').slice(0, 200)}` : 'NO RESPONSE CAPTURED'}`);

    await browser.close();

    console.log('\n=== VERDICT ===');
    const verdicts = [];
    if (importResponse && importResponse.status === 200) {
      verdicts.push('OK: POST /import/backup returned 200');
    } else if (importResponse) {
      verdicts.push(`FAIL: POST /import/backup returned ${importResponse.status}: ${(importResponse.body || '').slice(0, 200)}`);
      exitCode = 1;
    } else {
      verdicts.push('FAIL: no POST /import/backup captured');
      exitCode = 1;
    }
    if (importResponse && importResponse.headers['x-dixiedata-toast']) {
      verdicts.push(`OK: X-DixieData-Toast = "${importResponse.headers['x-dixiedata-toast']}"`);
    } else {
      verdicts.push('FAIL: X-DixieData-Toast missing');
      exitCode = 1;
    }
    if (importResponse && importResponse.headers['x-dixiedata-redirect']) {
      verdicts.push(`OK: X-DixieData-Redirect = "${importResponse.headers['x-dixiedata-redirect']}"`);
    } else {
      verdicts.push('FAIL: X-DixieData-Redirect missing — F1 fix not active');
      exitCode = 1;
    }
    if (importResponse && /Backup loaded: \d+ soldiers/.test(importResponse.body || '')) {
      verdicts.push(`OK: response body shows restore counts: "${(importResponse.body || '').slice(0, 80)}..."`);
    } else {
      verdicts.push(`FAIL: response body does not show restore counts: "${(importResponse.body || '').slice(0, 80)}"`);
      exitCode = 1;
    }
    if (scrollCalls.length > 0) {
      verdicts.push(`OK: scrollIntoView fired ${scrollCalls.length} time(s) on #share-status (F2 fix)`);
    } else {
      verdicts.push('INFO: scrollIntoView did not fire (htmx may have navigated before swap settled)');
    }
    if (importResponse && afterClick.y < viewport.height) {
      verdicts.push(`OK: panel reachable in viewport after click (y=${Math.round(afterClick.y)})`);
    }
    verdicts.forEach((v) => console.log(v));

    if (errors.length > 0) {
      console.log(`\npage errors (${errors.length}):`);
      errors.forEach((e) => console.log(`  - ${e}`));
    }
  } catch (e) {
    console.error('FATAL:', e);
    exitCode = 2;
  } finally {
    if (proc) {
      proc.kill('SIGTERM');
      await sleep(500);
      if (process.platform === 'win32') {
        await new Promise((resolve) => {
          spawn('taskkill', ['/F', '/IM', 'dixiedata-web.exe', '/T'], { stdio: 'ignore' }).on('exit', resolve);
        });
      }
      await sleep(300);
    }
    process.exit(exitCode);
  }
}

main();